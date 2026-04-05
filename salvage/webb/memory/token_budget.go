// Package clients provides API clients for webb.
package clients

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// TokenBudgetConfig holds configuration for token budget management
type TokenBudgetConfig struct {
	DefaultSessionBudget   int64   // Default tokens per session (0 = unlimited)
	DefaultWorkflowBudget  int64   // Default tokens per workflow
	WarningThresholdPct    float64 // Warn when usage exceeds this % of budget (0.8 = 80%)
	CriticalThresholdPct   float64 // Critical alert threshold (0.95 = 95%)
	EnablePrediction       bool    // Enable token cost prediction
	PreferLowTokenOnBudget bool    // Prefer low-token alternatives when near budget
	Enabled                bool    // Enable token tracking
}

// DefaultTokenBudgetConfig returns sensible defaults
func DefaultTokenBudgetConfig() TokenBudgetConfig {
	return TokenBudgetConfig{
		DefaultSessionBudget:   100000, // 100K tokens per session
		DefaultWorkflowBudget:  25000,  // 25K tokens per workflow
		WarningThresholdPct:    0.80,
		CriticalThresholdPct:   0.95,
		EnablePrediction:       true,
		PreferLowTokenOnBudget: true,
		Enabled:                true,
	}
}

// ToolTokenUsage represents token usage for a tool call
type ToolTokenUsage struct {
	ToolName      string    `json:"tool_name"`
	SessionID     string    `json:"session_id"`
	WorkflowID    string    `json:"workflow_id,omitempty"`
	InputTokens   int64     `json:"input_tokens"`
	OutputTokens  int64     `json:"output_tokens"`
	TotalTokens   int64     `json:"total_tokens"`
	Timestamp     time.Time `json:"timestamp"`
	WasPredicted  bool      `json:"was_predicted"`
	PredictedCost int64     `json:"predicted_cost,omitempty"`
}

// SessionBudget represents a session's token budget
type SessionBudget struct {
	SessionID       string    `json:"session_id"`
	TotalBudget     int64     `json:"total_budget"`
	UsedTokens      int64     `json:"used_tokens"`
	RemainingTokens int64     `json:"remaining_tokens"`
	ToolCalls       int       `json:"tool_calls"`
	StartedAt       time.Time `json:"started_at"`
	LastActivity    time.Time `json:"last_activity"`
	Status          string    `json:"status"` // "normal", "warning", "critical", "exhausted"
}

// ToolTokenEstimate represents estimated token cost for a tool
type ToolTokenEstimate struct {
	ToolName        string  `json:"tool_name"`
	AvgInputTokens  int64   `json:"avg_input_tokens"`
	AvgOutputTokens int64   `json:"avg_output_tokens"`
	AvgTotalTokens  int64   `json:"avg_total_tokens"`
	P50TotalTokens  int64   `json:"p50_total_tokens"`
	P95TotalTokens  int64   `json:"p95_total_tokens"`
	MaxTotalTokens  int64   `json:"max_total_tokens"`
	SampleCount     int     `json:"sample_count"`
	Confidence      float64 `json:"confidence"` // 0-1 based on sample count
}

// TokenAlert represents a token budget alert
type TokenAlert struct {
	SessionID  string    `json:"session_id"`
	Level      string    `json:"level"` // "warning", "critical", "exhausted"
	Message    string    `json:"message"`
	UsedTokens int64     `json:"used_tokens"`
	Budget     int64     `json:"budget"`
	Percentage float64   `json:"percentage"`
	Timestamp  time.Time `json:"timestamp"`
}

// LowTokenAlternative represents a lower-cost tool alternative
type LowTokenAlternative struct {
	OriginalTool    string `json:"original_tool"`
	AlternativeTool string `json:"alternative_tool"`
	TokenSavings    int64  `json:"token_savings"`
	Reason          string `json:"reason"`
}

// TokenBudgetClient manages token budgets and tracking
type TokenBudgetClient struct {
	db       *sql.DB
	config   TokenBudgetConfig
	mu       sync.RWMutex
	usageCh  chan ToolTokenUsage
	stopCh   chan struct{}
	alertCh  chan TokenAlert
	sessions map[string]*SessionBudget

	// Tool alternatives for low-token routing
	alternatives map[string][]LowTokenAlternative
}

// Global singleton for token budget client
var (
	globalTokenBudgetClient *TokenBudgetClient
	tokenBudgetOnce         sync.Once
)

// GetTokenBudgetClient returns the global token budget client
func GetTokenBudgetClient() *TokenBudgetClient {
	tokenBudgetOnce.Do(func() {
		client, err := NewTokenBudgetClient()
		if err != nil {
			// Log error but don't fail - token tracking is optional
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize token budget client: %v\n", err)
			return
		}
		globalTokenBudgetClient = client
	})
	return globalTokenBudgetClient
}

// NewTokenBudgetClient creates a new token budget client
func NewTokenBudgetClient() (*TokenBudgetClient, error) {
	configDir := os.Getenv("WEBB_CONFIG_DIR")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config", "webb")
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	dbPath := filepath.Join(configDir, "token_budget.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	client := &TokenBudgetClient{
		db:           db,
		config:       DefaultTokenBudgetConfig(),
		usageCh:      make(chan ToolTokenUsage, 1000),
		stopCh:       make(chan struct{}),
		alertCh:      make(chan TokenAlert, 100),
		sessions:     make(map[string]*SessionBudget),
		alternatives: make(map[string][]LowTokenAlternative),
	}

	if err := client.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	if err := client.loadConfig(); err != nil {
		// Use defaults if config load fails
		client.config = DefaultTokenBudgetConfig()
	}

	// Initialize tool alternatives
	client.initAlternatives()

	// Start async usage processor
	go client.processUsage()

	return client, nil
}

// initSchema creates the database tables
func (c *TokenBudgetClient) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS token_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tool_name TEXT NOT NULL,
		session_id TEXT NOT NULL,
		workflow_id TEXT,
		input_tokens INTEGER NOT NULL,
		output_tokens INTEGER NOT NULL,
		total_tokens INTEGER NOT NULL,
		was_predicted INTEGER DEFAULT 0,
		predicted_cost INTEGER,
		timestamp DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_token_usage_session ON token_usage(session_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_token_usage_tool ON token_usage(tool_name, timestamp DESC);

	CREATE TABLE IF NOT EXISTS session_budgets (
		session_id TEXT PRIMARY KEY,
		total_budget INTEGER NOT NULL,
		used_tokens INTEGER DEFAULT 0,
		tool_calls INTEGER DEFAULT 0,
		started_at DATETIME NOT NULL,
		last_activity DATETIME NOT NULL,
		status TEXT DEFAULT 'normal'
	);

	CREATE TABLE IF NOT EXISTS tool_estimates (
		tool_name TEXT PRIMARY KEY,
		avg_input_tokens INTEGER,
		avg_output_tokens INTEGER,
		avg_total_tokens INTEGER,
		p50_total_tokens INTEGER,
		p95_total_tokens INTEGER,
		max_total_tokens INTEGER,
		sample_count INTEGER DEFAULT 0,
		last_updated DATETIME
	);

	CREATE TABLE IF NOT EXISTS token_alerts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		level TEXT NOT NULL,
		message TEXT NOT NULL,
		used_tokens INTEGER,
		budget INTEGER,
		percentage REAL,
		timestamp DATETIME NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_token_alerts_session ON token_alerts(session_id, timestamp DESC);

	CREATE TABLE IF NOT EXISTS token_config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`

	_, err := c.db.Exec(schema)
	return err
}

// loadConfig loads configuration from the database
func (c *TokenBudgetClient) loadConfig() error {
	rows, err := c.db.Query("SELECT key, value FROM token_config")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "enabled":
			c.config.Enabled = value == "1"
		case "default_session_budget":
			fmt.Sscanf(value, "%d", &c.config.DefaultSessionBudget)
		case "warning_threshold":
			fmt.Sscanf(value, "%f", &c.config.WarningThresholdPct)
		case "critical_threshold":
			fmt.Sscanf(value, "%f", &c.config.CriticalThresholdPct)
		case "enable_prediction":
			c.config.EnablePrediction = value == "1"
		case "prefer_low_token":
			c.config.PreferLowTokenOnBudget = value == "1"
		}
	}
	return nil
}

// initAlternatives sets up tool alternatives for low-token routing
func (c *TokenBudgetClient) initAlternatives() {
	// Map high-cost tools to lower-cost alternatives
	c.alternatives = map[string][]LowTokenAlternative{
		"webb_cluster_health_full": {
			{AlternativeTool: "webb_quick_check", TokenSavings: 2500, Reason: "Quick health check with minimal output"},
			{AlternativeTool: "webb_k8s_pods", TokenSavings: 1500, Reason: "Pod status only"},
		},
		"webb_ticket_summary": {
			{AlternativeTool: "webb_pylon_list", TokenSavings: 1000, Reason: "Pylon tickets only"},
			{AlternativeTool: "webb_incidentio_list", TokenSavings: 800, Reason: "Incidents only"},
		},
		"webb_investigate_summary": {
			{AlternativeTool: "webb_quick_check", TokenSavings: 3000, Reason: "Quick health snapshot"},
		},
		"webb_database_health_full": {
			{AlternativeTool: "webb_postgres_tables", TokenSavings: 1500, Reason: "Postgres tables only"},
			{AlternativeTool: "webb_clickhouse_tables", TokenSavings: 1200, Reason: "ClickHouse tables only"},
		},
		"webb_oncall_dashboard": {
			{AlternativeTool: "webb_grafana_alerts", TokenSavings: 2000, Reason: "Alerts only"},
			{AlternativeTool: "webb_incidentio_list", TokenSavings: 1500, Reason: "Active incidents only"},
		},
	}

	// Set original tool for each alternative
	for original, alts := range c.alternatives {
		for i := range alts {
			alts[i].OriginalTool = original
		}
	}
}

// Close shuts down the token budget client
func (c *TokenBudgetClient) Close() error {
	close(c.stopCh)
	return c.db.Close()
}

// processUsage handles async token usage recording
func (c *TokenBudgetClient) processUsage() {
	for {
		select {
		case <-c.stopCh:
			return
		case usage := <-c.usageCh:
			c.recordUsageSync(usage)
		}
	}
}

// RecordUsage records token usage for a tool call (async)
func (c *TokenBudgetClient) RecordUsage(usage ToolTokenUsage) {
	if !c.config.Enabled {
		return
	}

	select {
	case c.usageCh <- usage:
	default:
		// Channel full, drop usage (non-blocking)
	}
}

// recordUsageSync handles the actual recording
func (c *TokenBudgetClient) recordUsageSync(usage ToolTokenUsage) {
	// Insert usage record
	_, err := c.db.Exec(`
		INSERT INTO token_usage (tool_name, session_id, workflow_id, input_tokens, output_tokens, total_tokens, was_predicted, predicted_cost, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, usage.ToolName, usage.SessionID, usage.WorkflowID, usage.InputTokens, usage.OutputTokens, usage.TotalTokens, usage.WasPredicted, usage.PredictedCost, usage.Timestamp)
	if err != nil {
		return
	}

	// Update session budget
	c.updateSessionBudget(usage.SessionID, usage.TotalTokens)

	// Update tool estimates
	c.updateToolEstimate(usage.ToolName, usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
}

// updateSessionBudget updates the session's token usage
func (c *TokenBudgetClient) updateSessionBudget(sessionID string, tokens int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, exists := c.sessions[sessionID]
	if !exists {
		// Create new session budget
		session = &SessionBudget{
			SessionID:       sessionID,
			TotalBudget:     c.config.DefaultSessionBudget,
			UsedTokens:      0,
			RemainingTokens: c.config.DefaultSessionBudget,
			ToolCalls:       0,
			StartedAt:       time.Now(),
			LastActivity:    time.Now(),
			Status:          "normal",
		}
		c.sessions[sessionID] = session
	}

	session.UsedTokens += tokens
	session.RemainingTokens = session.TotalBudget - session.UsedTokens
	session.ToolCalls++
	session.LastActivity = time.Now()

	// Check thresholds
	if session.TotalBudget > 0 {
		pct := float64(session.UsedTokens) / float64(session.TotalBudget)
		if pct >= 1.0 {
			session.Status = "exhausted"
			c.sendAlert(sessionID, "exhausted", pct)
		} else if pct >= c.config.CriticalThresholdPct {
			session.Status = "critical"
			c.sendAlert(sessionID, "critical", pct)
		} else if pct >= c.config.WarningThresholdPct {
			session.Status = "warning"
			c.sendAlert(sessionID, "warning", pct)
		}
	}

	// Persist to database
	c.db.Exec(`
		INSERT INTO session_budgets (session_id, total_budget, used_tokens, tool_calls, started_at, last_activity, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			used_tokens = ?,
			tool_calls = ?,
			last_activity = ?,
			status = ?
	`, session.SessionID, session.TotalBudget, session.UsedTokens, session.ToolCalls, session.StartedAt, session.LastActivity, session.Status,
		session.UsedTokens, session.ToolCalls, session.LastActivity, session.Status)
}

// updateToolEstimate updates the rolling estimate for a tool
func (c *TokenBudgetClient) updateToolEstimate(toolName string, input, output, total int64) {
	// Use exponential moving average
	c.db.Exec(`
		INSERT INTO tool_estimates (tool_name, avg_input_tokens, avg_output_tokens, avg_total_tokens, p50_total_tokens, p95_total_tokens, max_total_tokens, sample_count, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?)
		ON CONFLICT(tool_name) DO UPDATE SET
			avg_input_tokens = (avg_input_tokens * sample_count + ?) / (sample_count + 1),
			avg_output_tokens = (avg_output_tokens * sample_count + ?) / (sample_count + 1),
			avg_total_tokens = (avg_total_tokens * sample_count + ?) / (sample_count + 1),
			max_total_tokens = MAX(max_total_tokens, ?),
			sample_count = sample_count + 1,
			last_updated = ?
	`, toolName, input, output, total, total, total, total, time.Now(),
		input, output, total, total, time.Now())
}

// sendAlert sends a token budget alert
func (c *TokenBudgetClient) sendAlert(sessionID, level string, pct float64) {
	session := c.sessions[sessionID]
	if session == nil {
		return
	}

	alert := TokenAlert{
		SessionID:  sessionID,
		Level:      level,
		UsedTokens: session.UsedTokens,
		Budget:     session.TotalBudget,
		Percentage: pct * 100,
		Timestamp:  time.Now(),
	}

	switch level {
	case "warning":
		alert.Message = fmt.Sprintf("Token usage at %.0f%% of budget (%d/%d tokens)", pct*100, session.UsedTokens, session.TotalBudget)
	case "critical":
		alert.Message = fmt.Sprintf("Token usage critical at %.0f%% (%d/%d tokens) - consider using lighter tools", pct*100, session.UsedTokens, session.TotalBudget)
	case "exhausted":
		alert.Message = fmt.Sprintf("Token budget exhausted (%d/%d tokens) - switching to minimal-token mode", session.UsedTokens, session.TotalBudget)
	}

	// Record alert
	c.db.Exec(`
		INSERT INTO token_alerts (session_id, level, message, used_tokens, budget, percentage, timestamp)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, alert.SessionID, alert.Level, alert.Message, alert.UsedTokens, alert.Budget, alert.Percentage, alert.Timestamp)

	// Send to alert channel (non-blocking)
	select {
	case c.alertCh <- alert:
	default:
	}
}

// GetSessionBudget returns the current session budget status
func (c *TokenBudgetClient) GetSessionBudget(sessionID string) (*SessionBudget, error) {
	c.mu.RLock()
	if session, exists := c.sessions[sessionID]; exists {
		c.mu.RUnlock()
		return session, nil
	}
	c.mu.RUnlock()

	// Try to load from database
	var session SessionBudget
	err := c.db.QueryRow(`
		SELECT session_id, total_budget, used_tokens, tool_calls, started_at, last_activity, status
		FROM session_budgets WHERE session_id = ?
	`, sessionID).Scan(&session.SessionID, &session.TotalBudget, &session.UsedTokens, &session.ToolCalls, &session.StartedAt, &session.LastActivity, &session.Status)

	if err == sql.ErrNoRows {
		// Create new session
		session = SessionBudget{
			SessionID:       sessionID,
			TotalBudget:     c.config.DefaultSessionBudget,
			UsedTokens:      0,
			RemainingTokens: c.config.DefaultSessionBudget,
			ToolCalls:       0,
			StartedAt:       time.Now(),
			LastActivity:    time.Now(),
			Status:          "normal",
		}
		return &session, nil
	}

	if err != nil {
		return nil, err
	}

	session.RemainingTokens = session.TotalBudget - session.UsedTokens
	return &session, nil
}

// SetSessionBudget sets a custom budget for a session
func (c *TokenBudgetClient) SetSessionBudget(sessionID string, budget int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	session, exists := c.sessions[sessionID]
	if !exists {
		session = &SessionBudget{
			SessionID:       sessionID,
			TotalBudget:     budget,
			UsedTokens:      0,
			RemainingTokens: budget,
			ToolCalls:       0,
			StartedAt:       time.Now(),
			LastActivity:    time.Now(),
			Status:          "normal",
		}
		c.sessions[sessionID] = session
	} else {
		session.TotalBudget = budget
		session.RemainingTokens = budget - session.UsedTokens
	}

	_, err := c.db.Exec(`
		INSERT INTO session_budgets (session_id, total_budget, used_tokens, tool_calls, started_at, last_activity, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET total_budget = ?
	`, session.SessionID, session.TotalBudget, session.UsedTokens, session.ToolCalls, session.StartedAt, session.LastActivity, session.Status, budget)

	return err
}

// PredictTokenCost predicts the token cost of a tool call
func (c *TokenBudgetClient) PredictTokenCost(toolName string) (*ToolTokenEstimate, error) {
	if !c.config.EnablePrediction {
		return nil, fmt.Errorf("prediction disabled")
	}

	var estimate ToolTokenEstimate
	err := c.db.QueryRow(`
		SELECT tool_name, avg_input_tokens, avg_output_tokens, avg_total_tokens,
		       COALESCE(p50_total_tokens, avg_total_tokens) as p50_total_tokens,
		       COALESCE(p95_total_tokens, avg_total_tokens * 2) as p95_total_tokens,
		       max_total_tokens, sample_count
		FROM tool_estimates WHERE tool_name = ?
	`, toolName).Scan(&estimate.ToolName, &estimate.AvgInputTokens, &estimate.AvgOutputTokens,
		&estimate.AvgTotalTokens, &estimate.P50TotalTokens, &estimate.P95TotalTokens,
		&estimate.MaxTotalTokens, &estimate.SampleCount)

	if err == sql.ErrNoRows {
		// Return default estimate for unknown tools
		return &ToolTokenEstimate{
			ToolName:        toolName,
			AvgInputTokens:  500,
			AvgOutputTokens: 1000,
			AvgTotalTokens:  1500,
			P50TotalTokens:  1500,
			P95TotalTokens:  3000,
			MaxTotalTokens:  5000,
			SampleCount:     0,
			Confidence:      0.0,
		}, nil
	}

	if err != nil {
		return nil, err
	}

	// Calculate confidence based on sample count
	if estimate.SampleCount >= 100 {
		estimate.Confidence = 1.0
	} else if estimate.SampleCount >= 50 {
		estimate.Confidence = 0.9
	} else if estimate.SampleCount >= 20 {
		estimate.Confidence = 0.7
	} else if estimate.SampleCount >= 10 {
		estimate.Confidence = 0.5
	} else if estimate.SampleCount >= 5 {
		estimate.Confidence = 0.3
	} else {
		estimate.Confidence = 0.1
	}

	return &estimate, nil
}

// GetLowTokenAlternatives returns lower-cost alternatives for a tool
func (c *TokenBudgetClient) GetLowTokenAlternatives(toolName string) []LowTokenAlternative {
	if alts, exists := c.alternatives[toolName]; exists {
		return alts
	}
	return nil
}

// ShouldUseLowTokenAlternative checks if we should suggest a lower-token alternative
func (c *TokenBudgetClient) ShouldUseLowTokenAlternative(sessionID, toolName string) (*LowTokenAlternative, bool) {
	if !c.config.PreferLowTokenOnBudget {
		return nil, false
	}

	session, err := c.GetSessionBudget(sessionID)
	if err != nil || session.TotalBudget == 0 {
		return nil, false
	}

	// Check if we're near budget limit
	pct := float64(session.UsedTokens) / float64(session.TotalBudget)
	if pct < c.config.WarningThresholdPct {
		return nil, false
	}

	// Get alternatives
	alts := c.GetLowTokenAlternatives(toolName)
	if len(alts) == 0 {
		return nil, false
	}

	// Return the best alternative (highest token savings)
	return &alts[0], true
}

// GetTopTokenConsumers returns the tools with highest token usage
func (c *TokenBudgetClient) GetTopTokenConsumers(limit int) ([]ToolTokenEstimate, error) {
	rows, err := c.db.Query(`
		SELECT tool_name, avg_input_tokens, avg_output_tokens, avg_total_tokens,
		       COALESCE(p50_total_tokens, avg_total_tokens) as p50,
		       COALESCE(p95_total_tokens, avg_total_tokens * 2) as p95,
		       max_total_tokens, sample_count
		FROM tool_estimates
		ORDER BY avg_total_tokens DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var estimates []ToolTokenEstimate
	for rows.Next() {
		var e ToolTokenEstimate
		if err := rows.Scan(&e.ToolName, &e.AvgInputTokens, &e.AvgOutputTokens,
			&e.AvgTotalTokens, &e.P50TotalTokens, &e.P95TotalTokens,
			&e.MaxTotalTokens, &e.SampleCount); err != nil {
			continue
		}
		estimates = append(estimates, e)
	}
	return estimates, nil
}

// GetRecentAlerts returns recent token budget alerts
func (c *TokenBudgetClient) GetRecentAlerts(sessionID string, limit int) ([]TokenAlert, error) {
	query := `
		SELECT session_id, level, message, used_tokens, budget, percentage, timestamp
		FROM token_alerts
	`
	args := []interface{}{}

	if sessionID != "" {
		query += " WHERE session_id = ?"
		args = append(args, sessionID)
	}

	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []TokenAlert
	for rows.Next() {
		var a TokenAlert
		if err := rows.Scan(&a.SessionID, &a.Level, &a.Message, &a.UsedTokens, &a.Budget, &a.Percentage, &a.Timestamp); err != nil {
			continue
		}
		alerts = append(alerts, a)
	}
	return alerts, nil
}

// GetUsageSummary returns a summary of token usage
func (c *TokenBudgetClient) GetUsageSummary(ctx context.Context, timeRange string) (*TokenUsageSummary, error) {
	// Calculate time filter
	var since time.Time
	switch timeRange {
	case "1h":
		since = time.Now().Add(-1 * time.Hour)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-24 * time.Hour)
	}

	summary := &TokenUsageSummary{
		TimeRange: timeRange,
	}

	// Total tokens and calls
	err := c.db.QueryRow(`
		SELECT COALESCE(SUM(total_tokens), 0), COUNT(*), COUNT(DISTINCT session_id)
		FROM token_usage WHERE timestamp > ?
	`, since).Scan(&summary.TotalTokens, &summary.TotalCalls, &summary.UniqueSessions)
	if err != nil {
		return nil, err
	}

	// Tokens by tool (top 10)
	rows, err := c.db.Query(`
		SELECT tool_name, SUM(total_tokens) as total, COUNT(*) as calls
		FROM token_usage WHERE timestamp > ?
		GROUP BY tool_name
		ORDER BY total DESC
		LIMIT 10
	`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item TokensByTool
		if err := rows.Scan(&item.ToolName, &item.TotalTokens, &item.CallCount); err != nil {
			continue
		}
		summary.TopTools = append(summary.TopTools, item)
	}

	// Calculate average
	if summary.TotalCalls > 0 {
		summary.AvgTokensPerCall = summary.TotalTokens / int64(summary.TotalCalls)
	}

	return summary, nil
}

// TokenUsageSummary represents aggregated token usage
type TokenUsageSummary struct {
	TimeRange        string        `json:"time_range"`
	TotalTokens      int64         `json:"total_tokens"`
	TotalCalls       int           `json:"total_calls"`
	UniqueSessions   int           `json:"unique_sessions"`
	AvgTokensPerCall int64         `json:"avg_tokens_per_call"`
	TopTools         []TokensByTool `json:"top_tools"`
}

// TokensByTool represents token usage by tool
type TokensByTool struct {
	ToolName    string `json:"tool_name"`
	TotalTokens int64  `json:"total_tokens"`
	CallCount   int    `json:"call_count"`
}

// UpdateConfig updates the token budget configuration
func (c *TokenBudgetClient) UpdateConfig(config TokenBudgetConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = config

	// Persist to database
	enabled := 0
	if config.Enabled {
		enabled = 1
	}
	prediction := 0
	if config.EnablePrediction {
		prediction = 1
	}
	lowToken := 0
	if config.PreferLowTokenOnBudget {
		lowToken = 1
	}

	updates := map[string]string{
		"enabled":                fmt.Sprintf("%d", enabled),
		"default_session_budget": fmt.Sprintf("%d", config.DefaultSessionBudget),
		"warning_threshold":      fmt.Sprintf("%.2f", config.WarningThresholdPct),
		"critical_threshold":     fmt.Sprintf("%.2f", config.CriticalThresholdPct),
		"enable_prediction":      fmt.Sprintf("%d", prediction),
		"prefer_low_token":       fmt.Sprintf("%d", lowToken),
	}

	for key, value := range updates {
		c.db.Exec("INSERT INTO token_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?", key, value, value)
	}

	return nil
}

// GetConfig returns the current configuration
func (c *TokenBudgetClient) GetConfig() TokenBudgetConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

// ResetSession clears token usage for a session
func (c *TokenBudgetClient) ResetSession(sessionID string) error {
	c.mu.Lock()
	delete(c.sessions, sessionID)
	c.mu.Unlock()

	_, err := c.db.Exec("DELETE FROM session_budgets WHERE session_id = ?", sessionID)
	if err != nil {
		return err
	}

	_, err = c.db.Exec("DELETE FROM token_usage WHERE session_id = ?", sessionID)
	return err
}

// GetAllSessionBudgets returns all active session budgets
func (c *TokenBudgetClient) GetAllSessionBudgets() ([]SessionBudget, error) {
	rows, err := c.db.Query(`
		SELECT session_id, total_budget, used_tokens, tool_calls, started_at, last_activity, status
		FROM session_budgets
		WHERE last_activity > datetime('now', '-1 hour')
		ORDER BY last_activity DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []SessionBudget
	for rows.Next() {
		var s SessionBudget
		if err := rows.Scan(&s.SessionID, &s.TotalBudget, &s.UsedTokens, &s.ToolCalls, &s.StartedAt, &s.LastActivity, &s.Status); err != nil {
			continue
		}
		s.RemainingTokens = s.TotalBudget - s.UsedTokens
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// GetAlternatives returns all tool alternative mappings
func (c *TokenBudgetClient) GetAlternatives() map[string][]LowTokenAlternative {
	return c.alternatives
}

// GetAlternativesFor returns alternatives for a specific tool
func (c *TokenBudgetClient) GetAlternativesFor(toolName string) []LowTokenAlternative {
	if alts, exists := c.alternatives[toolName]; exists {
		return alts
	}
	return nil
}

// SetAlertThreshold sets the alert threshold for a session
func (c *TokenBudgetClient) SetAlertThreshold(sessionID string, threshold float64) error {
	// For now, alert threshold is global via config
	// Individual session thresholds could be added if needed
	return nil
}

// TokenConsumer represents a tool's token consumption stats for display
type TokenConsumer struct {
	ToolName     string `json:"tool_name"`
	TotalTokens  int64  `json:"total_tokens"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CallCount    int    `json:"call_count"`
}

// TokenBudgetConfigView is a simplified config view for MCP tools
type TokenBudgetConfigView struct {
	Enabled               bool    `json:"enabled"`
	DefaultBudget         int64   `json:"default_budget"`
	AlertThreshold        float64 `json:"alert_threshold"`
	OptimizationThreshold float64 `json:"optimization_threshold"`
}

// GetConfigView returns a simplified config for display
func (c *TokenBudgetClient) GetConfigView() TokenBudgetConfigView {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return TokenBudgetConfigView{
		Enabled:               c.config.Enabled,
		DefaultBudget:         c.config.DefaultSessionBudget,
		AlertThreshold:        c.config.WarningThresholdPct,
		OptimizationThreshold: c.config.WarningThresholdPct - 0.1, // 10% before warning
	}
}

// SessionBudgetView is a simplified budget view for MCP tools
type SessionBudgetView struct {
	Budget         int64   `json:"budget"`
	Used           int64   `json:"used"`
	AlertThreshold float64 `json:"alert_threshold"`
}

// GetSessionBudgetView returns a simplified budget view
func (c *TokenBudgetClient) GetSessionBudgetView(sessionID string) (*SessionBudgetView, error) {
	budget, err := c.GetSessionBudget(sessionID)
	if err != nil {
		return nil, err
	}
	return &SessionBudgetView{
		Budget:         budget.TotalBudget,
		Used:           budget.UsedTokens,
		AlertThreshold: c.config.WarningThresholdPct,
	}, nil
}

// GetTopTokenConsumersDetailed returns detailed token consumption by tool
func (c *TokenBudgetClient) GetTopTokenConsumersDetailed(limit int) ([]TokenConsumer, error) {
	rows, err := c.db.Query(`
		SELECT tool_name,
		       SUM(total_tokens) as total_tokens,
		       SUM(input_tokens) as input_tokens,
		       SUM(output_tokens) as output_tokens,
		       COUNT(*) as call_count
		FROM token_usage
		WHERE timestamp > datetime('now', '-24 hours')
		GROUP BY tool_name
		ORDER BY total_tokens DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var consumers []TokenConsumer
	for rows.Next() {
		var c TokenConsumer
		if err := rows.Scan(&c.ToolName, &c.TotalTokens, &c.InputTokens, &c.OutputTokens, &c.CallCount); err != nil {
			continue
		}
		consumers = append(consumers, c)
	}
	return consumers, nil
}
