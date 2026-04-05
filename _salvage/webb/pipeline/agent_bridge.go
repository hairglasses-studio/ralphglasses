// Package clients provides API clients for webb.
// v27.0: Agent Bridge - Connect local Claude Code agents to remote swarm workers
// v109.0: Database-Backed Agent Queue - Production-grade persistence with retry and DLQ
package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// BridgeTaskType defines the type of agent task
type BridgeTaskType string

const (
	BridgeTaskExplore BridgeTaskType = "explore"
	BridgeTaskPlan    BridgeTaskType = "plan"
	BridgeTaskGeneral BridgeTaskType = "general"
)

// BridgeTaskStatus defines the status of a bridge task
type BridgeTaskStatus string

const (
	BridgeTaskQueued     BridgeTaskStatus = "queued"
	BridgeTaskRunning    BridgeTaskStatus = "running"
	BridgeTaskCompleted  BridgeTaskStatus = "completed"
	BridgeTaskFailed     BridgeTaskStatus = "failed"
	BridgeTaskTimeout    BridgeTaskStatus = "timeout"    // v109.0: Task timed out
	BridgeTaskDeadLetter BridgeTaskStatus = "deadletter" // v109.0: Moved to DLQ after max retries
)

// v109.0: Default configuration values
const (
	DefaultMaxRetries       = 3
	DefaultBaseBackoffMs    = 1000  // 1 second base backoff
	DefaultMaxBackoffMs     = 60000 // 60 second max backoff
	DefaultTaskTimeoutMins  = 30    // 30 minute task timeout
	DefaultCleanupDays      = 7     // Keep completed tasks for 7 days
	DefaultMetricsInterval  = 60    // Metrics update interval in seconds
)

// BridgeTask represents a task for local Claude Code agents
type BridgeTask struct {
	ID          string            `json:"id"`
	Type        BridgeTaskType    `json:"type"`
	Query       string            `json:"query"`
	Params      map[string]any    `json:"params,omitempty"`
	Status      BridgeTaskStatus  `json:"status"`
	Priority    int               `json:"priority"`     // v109.0: Task priority (higher = more urgent)
	RetryCount  int               `json:"retry_count"`  // v109.0: Current retry attempt
	MaxRetries  int               `json:"max_retries"`  // v109.0: Max retry attempts
	NextRetryAt *time.Time        `json:"next_retry_at,omitempty"` // v109.0: When to retry
	TimeoutAt   *time.Time        `json:"timeout_at,omitempty"`    // v109.0: When task times out
	Error       string            `json:"error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	CompletedAt *time.Time        `json:"completed_at,omitempty"`
}

// BridgeFinding represents a finding from an agent
type BridgeFinding struct {
	Category    string   `json:"category"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Evidence    []string `json:"evidence"`
	Confidence  int      `json:"confidence"`
	Impact      int      `json:"impact"`
}

// BridgeResult represents the result of an agent task
type BridgeResult struct {
	TaskID      string           `json:"task_id"`
	AgentType   string           `json:"agent_type"`
	Status      BridgeTaskStatus `json:"status"`
	Output      string           `json:"output"`
	Findings    []BridgeFinding  `json:"findings,omitempty"`
	TokensUsed  int64            `json:"tokens_used"`
	CompletedAt time.Time        `json:"completed_at"`
}

// BridgeStats contains bridge queue statistics
type BridgeStats struct {
	QueuedTasks      int       `json:"queued_tasks"`
	RunningTasks     int       `json:"running_tasks"`
	CompletedTasks   int       `json:"completed_tasks"`
	FailedTasks      int       `json:"failed_tasks"`
	TimeoutTasks     int       `json:"timeout_tasks"`      // v109.0
	DeadLetterTasks  int       `json:"deadletter_tasks"`   // v109.0
	TotalResults     int       `json:"total_results"`
	AvgWaitTimeMs    int64     `json:"avg_wait_time_ms"`   // v109.0
	AvgProcessTimeMs int64     `json:"avg_process_time_ms"` // v109.0
	RetryRate        float64   `json:"retry_rate"`         // v109.0: Percentage of tasks that needed retry
	LastUpdated      time.Time `json:"last_updated"`       // v109.0
}

// v109.0: BridgeQueueMetrics for alerting
type BridgeQueueMetrics struct {
	QueueDepth       int       `json:"queue_depth"`
	OldestTaskAge    int64     `json:"oldest_task_age_seconds"`
	ProcessingRate   float64   `json:"processing_rate_per_min"`
	ErrorRate        float64   `json:"error_rate"`
	DeadLetterCount  int       `json:"dead_letter_count"`
	HealthScore      int       `json:"health_score"` // 0-100
	Alerts           []string  `json:"alerts,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
}

// AgentBridge connects local Claude Code agents to remote swarm workers
// v109.0: Now backed by SQLite for persistence and crash recovery
type AgentBridge struct {
	db            *sql.DB
	dbPath        string
	config        BridgeConfig
	metrics       *BridgeQueueMetrics
	metricsCache  *BridgeStats
	metricsMu     sync.RWMutex
	stopCh        chan struct{}
	mu            sync.RWMutex
	// Legacy file paths for backwards compatibility during migration
	legacyQueuePath   string
	legacyResultsPath string
}

// v109.0: BridgeConfig holds configuration for the agent bridge
type BridgeConfig struct {
	MaxRetries           int           `json:"max_retries"`
	BaseBackoffMs        int           `json:"base_backoff_ms"`
	MaxBackoffMs         int           `json:"max_backoff_ms"`
	TaskTimeoutMins      int           `json:"task_timeout_mins"`
	CleanupDays          int           `json:"cleanup_days"`
	MetricsIntervalSecs  int           `json:"metrics_interval_secs"`
	AlertQueueDepth      int           `json:"alert_queue_depth"`      // Alert when queue exceeds this
	AlertOldestTaskMins  int           `json:"alert_oldest_task_mins"` // Alert when oldest task exceeds this age
	AlertDeadLetterCount int           `json:"alert_dead_letter_count"` // Alert when DLQ exceeds this
}

// DefaultBridgeConfig returns sensible defaults
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		MaxRetries:           DefaultMaxRetries,
		BaseBackoffMs:        DefaultBaseBackoffMs,
		MaxBackoffMs:         DefaultMaxBackoffMs,
		TaskTimeoutMins:      DefaultTaskTimeoutMins,
		CleanupDays:          DefaultCleanupDays,
		MetricsIntervalSecs:  DefaultMetricsInterval,
		AlertQueueDepth:      100,
		AlertOldestTaskMins:  60,
		AlertDeadLetterCount: 10,
	}
}

// Global agent bridge singleton
var (
	globalAgentBridge   *AgentBridge
	agentBridgeOnce     sync.Once
)

// GetAgentBridge returns the global agent bridge instance
func GetAgentBridge() *AgentBridge {
	agentBridgeOnce.Do(func() {
		bridge, err := NewAgentBridge()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to initialize agent bridge: %v\n", err)
			return
		}
		globalAgentBridge = bridge
	})
	return globalAgentBridge
}

// NewAgentBridge creates a new agent bridge with SQLite backing
func NewAgentBridge() (*AgentBridge, error) {
	return NewAgentBridgeWithConfig(DefaultBridgeConfig())
}

// NewAgentBridgeWithConfig creates a new agent bridge with custom config
func NewAgentBridgeWithConfig(config BridgeConfig) (*AgentBridge, error) {
	homeDir, _ := os.UserHomeDir()
	dbPath := filepath.Join(homeDir, ".webb", "agent-bridge.db")
	legacyBasePath := filepath.Join(homeDir, "webb-vault", "bridge")

	b := &AgentBridge{
		dbPath:            dbPath,
		config:            config,
		stopCh:            make(chan struct{}),
		legacyQueuePath:   filepath.Join(legacyBasePath, "queue"),
		legacyResultsPath: filepath.Join(legacyBasePath, "results"),
	}

	if err := b.initDB(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Migrate any legacy file-based tasks
	b.migrateLegacyTasks()

	// Start background workers
	go b.metricsWorker()
	go b.timeoutWorker()
	go b.cleanupWorker()

	return b, nil
}

// initDB initializes the SQLite database
func (b *AgentBridge) initDB() error {
	if err := os.MkdirAll(filepath.Dir(b.dbPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	db, err := sql.Open("sqlite", b.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for crash recovery
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return fmt.Errorf("failed to enable WAL: %w", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS bridge_tasks (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		query TEXT NOT NULL,
		params TEXT,
		status TEXT NOT NULL DEFAULT 'queued',
		priority INTEGER DEFAULT 0,
		retry_count INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 3,
		next_retry_at DATETIME,
		timeout_at DATETIME,
		error TEXT,
		created_at DATETIME NOT NULL,
		started_at DATETIME,
		completed_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS bridge_results (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		agent_type TEXT,
		status TEXT NOT NULL,
		output TEXT,
		findings TEXT,
		tokens_used INTEGER DEFAULT 0,
		completed_at DATETIME NOT NULL,
		processed INTEGER DEFAULT 0,
		FOREIGN KEY (task_id) REFERENCES bridge_tasks(id)
	);

	CREATE TABLE IF NOT EXISTS bridge_dead_letter (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL,
		original_task TEXT NOT NULL,
		failure_reason TEXT NOT NULL,
		retry_count INTEGER,
		moved_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS bridge_metrics_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		queue_depth INTEGER,
		running_tasks INTEGER,
		completed_tasks INTEGER,
		failed_tasks INTEGER,
		dead_letter_count INTEGER,
		avg_wait_time_ms INTEGER,
		avg_process_time_ms INTEGER,
		health_score INTEGER,
		recorded_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_tasks_status ON bridge_tasks(status);
	CREATE INDEX IF NOT EXISTS idx_tasks_priority ON bridge_tasks(priority DESC, created_at ASC);
	CREATE INDEX IF NOT EXISTS idx_tasks_timeout ON bridge_tasks(timeout_at);
	CREATE INDEX IF NOT EXISTS idx_tasks_retry ON bridge_tasks(next_retry_at);
	CREATE INDEX IF NOT EXISTS idx_results_task ON bridge_results(task_id);
	CREATE INDEX IF NOT EXISTS idx_dlq_moved ON bridge_dead_letter(moved_at);
	CREATE INDEX IF NOT EXISTS idx_metrics_recorded ON bridge_metrics_history(recorded_at);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return fmt.Errorf("failed to create schema: %w", err)
	}

	b.db = db
	return nil
}

// Close shuts down the agent bridge
func (b *AgentBridge) Close() error {
	close(b.stopCh)
	if b.db != nil {
		return b.db.Close()
	}
	return nil
}

// migrateLegacyTasks migrates any file-based tasks to SQLite
func (b *AgentBridge) migrateLegacyTasks() {
	// Check if legacy directories exist
	if _, err := os.Stat(b.legacyQueuePath); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(b.legacyQueuePath)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(b.legacyQueuePath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var task BridgeTask
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}

		// Insert into SQLite if not already exists
		b.insertTaskIfNotExists(&task)

		// Remove legacy file after successful migration
		_ = os.Remove(path)
	}
}

// insertTaskIfNotExists inserts a task if it doesn't already exist
func (b *AgentBridge) insertTaskIfNotExists(task *BridgeTask) {
	paramsJSON, _ := json.Marshal(task.Params)
	timeoutAt := time.Now().Add(time.Duration(b.config.TaskTimeoutMins) * time.Minute)

	_, _ = b.db.Exec(`
		INSERT OR IGNORE INTO bridge_tasks
		(id, type, query, params, status, priority, retry_count, max_retries, timeout_at, error, created_at, started_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.Type, task.Query, string(paramsJSON), task.Status, task.Priority,
		task.RetryCount, task.MaxRetries, timeoutAt, task.Error,
		task.CreatedAt, task.StartedAt, task.CompletedAt)
}

// QueueTask queues a task for local agent execution
func (b *AgentBridge) QueueTask(task *BridgeTask) (string, error) {
	return b.QueueTaskWithPriority(task, 0)
}

// QueueTaskWithPriority queues a task with a specific priority
func (b *AgentBridge) QueueTaskWithPriority(task *BridgeTask, priority int) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if task.ID == "" {
		task.ID = uuid.New().String()[:8]
	}
	task.Status = BridgeTaskQueued
	task.Priority = priority
	task.CreatedAt = time.Now()
	task.MaxRetries = b.config.MaxRetries
	task.RetryCount = 0

	timeoutAt := time.Now().Add(time.Duration(b.config.TaskTimeoutMins) * time.Minute)
	task.TimeoutAt = &timeoutAt

	paramsJSON, _ := json.Marshal(task.Params)

	_, err := b.db.Exec(`
		INSERT INTO bridge_tasks
		(id, type, query, params, status, priority, retry_count, max_retries, timeout_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID, task.Type, task.Query, string(paramsJSON), task.Status, task.Priority,
		task.RetryCount, task.MaxRetries, task.TimeoutAt, task.CreatedAt)

	if err != nil {
		return "", fmt.Errorf("failed to queue task: %w", err)
	}

	return task.ID, nil
}

// RunExploreAgent queues an explore task for local Claude Code agent
func (b *AgentBridge) RunExploreAgent(query string, thoroughness string) (string, error) {
	task := &BridgeTask{
		Type:   BridgeTaskExplore,
		Query:  query,
		Params: map[string]any{"thoroughness": thoroughness},
	}
	return b.QueueTask(task)
}

// RunPlanAgent queues a planning task
func (b *AgentBridge) RunPlanAgent(goal string, context string) (string, error) {
	task := &BridgeTask{
		Type:   BridgeTaskPlan,
		Query:  goal,
		Params: map[string]any{"context": context},
	}
	return b.QueueTask(task)
}

// RunTaskAgent queues a general task
func (b *AgentBridge) RunTaskAgent(prompt string) (string, error) {
	task := &BridgeTask{
		Type:  BridgeTaskGeneral,
		Query: prompt,
	}
	return b.QueueTask(task)
}

// GetPendingTasks returns all pending tasks in the queue (ordered by priority)
func (b *AgentBridge) GetPendingTasks() ([]*BridgeTask, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rows, err := b.db.Query(`
		SELECT id, type, query, params, status, priority, retry_count, max_retries,
		       next_retry_at, timeout_at, error, created_at, started_at, completed_at
		FROM bridge_tasks
		WHERE status = 'queued' AND (next_retry_at IS NULL OR next_retry_at <= datetime('now'))
		ORDER BY priority DESC, created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	return b.scanTasks(rows)
}

// GetTask returns a specific task by ID
func (b *AgentBridge) GetTask(taskID string) (*BridgeTask, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	row := b.db.QueryRow(`
		SELECT id, type, query, params, status, priority, retry_count, max_retries,
		       next_retry_at, timeout_at, error, created_at, started_at, completed_at
		FROM bridge_tasks WHERE id = ?`, taskID)

	return b.scanTask(row)
}

// scanTask scans a single task row
func (b *AgentBridge) scanTask(row *sql.Row) (*BridgeTask, error) {
	var task BridgeTask
	var paramsJSON sql.NullString
	var nextRetryAt, timeoutAt, startedAt, completedAt sql.NullTime
	var errorStr sql.NullString

	err := row.Scan(&task.ID, &task.Type, &task.Query, &paramsJSON, &task.Status,
		&task.Priority, &task.RetryCount, &task.MaxRetries, &nextRetryAt, &timeoutAt,
		&errorStr, &task.CreatedAt, &startedAt, &completedAt)
	if err != nil {
		return nil, err
	}

	if paramsJSON.Valid {
		_ = json.Unmarshal([]byte(paramsJSON.String), &task.Params)
	}
	if nextRetryAt.Valid {
		task.NextRetryAt = &nextRetryAt.Time
	}
	if timeoutAt.Valid {
		task.TimeoutAt = &timeoutAt.Time
	}
	if startedAt.Valid {
		task.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		task.CompletedAt = &completedAt.Time
	}
	if errorStr.Valid {
		task.Error = errorStr.String
	}

	return &task, nil
}

// scanTasks scans multiple task rows
func (b *AgentBridge) scanTasks(rows *sql.Rows) ([]*BridgeTask, error) {
	var tasks []*BridgeTask

	for rows.Next() {
		var task BridgeTask
		var paramsJSON sql.NullString
		var nextRetryAt, timeoutAt, startedAt, completedAt sql.NullTime
		var errorStr sql.NullString

		err := rows.Scan(&task.ID, &task.Type, &task.Query, &paramsJSON, &task.Status,
			&task.Priority, &task.RetryCount, &task.MaxRetries, &nextRetryAt, &timeoutAt,
			&errorStr, &task.CreatedAt, &startedAt, &completedAt)
		if err != nil {
			continue
		}

		if paramsJSON.Valid {
			_ = json.Unmarshal([]byte(paramsJSON.String), &task.Params)
		}
		if nextRetryAt.Valid {
			task.NextRetryAt = &nextRetryAt.Time
		}
		if timeoutAt.Valid {
			task.TimeoutAt = &timeoutAt.Time
		}
		if startedAt.Valid {
			task.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			task.CompletedAt = &completedAt.Time
		}
		if errorStr.Valid {
			task.Error = errorStr.String
		}

		tasks = append(tasks, &task)
	}

	return tasks, nil
}

// UpdateTaskStatus updates the status of a task
func (b *AgentBridge) UpdateTaskStatus(taskID string, status BridgeTaskStatus) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()

	switch status {
	case BridgeTaskRunning:
		_, err := b.db.Exec(`UPDATE bridge_tasks SET status = ?, started_at = ? WHERE id = ?`,
			status, now, taskID)
		return err
	case BridgeTaskCompleted, BridgeTaskFailed:
		_, err := b.db.Exec(`UPDATE bridge_tasks SET status = ?, completed_at = ? WHERE id = ?`,
			status, now, taskID)
		return err
	default:
		_, err := b.db.Exec(`UPDATE bridge_tasks SET status = ? WHERE id = ?`, status, taskID)
		return err
	}
}

// MarkTaskFailed marks a task as failed with error and handles retry logic
func (b *AgentBridge) MarkTaskFailed(taskID string, errMsg string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get current task
	row := b.db.QueryRow(`SELECT retry_count, max_retries FROM bridge_tasks WHERE id = ?`, taskID)
	var retryCount, maxRetries int
	if err := row.Scan(&retryCount, &maxRetries); err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	retryCount++

	if retryCount >= maxRetries {
		// Move to dead letter queue
		return b.moveToDeadLetter(taskID, errMsg, retryCount)
	}

	// Calculate exponential backoff
	backoffMs := b.config.BaseBackoffMs * int(math.Pow(2, float64(retryCount-1)))
	if backoffMs > b.config.MaxBackoffMs {
		backoffMs = b.config.MaxBackoffMs
	}
	nextRetry := time.Now().Add(time.Duration(backoffMs) * time.Millisecond)

	_, err := b.db.Exec(`
		UPDATE bridge_tasks
		SET status = 'queued', retry_count = ?, next_retry_at = ?, error = ?
		WHERE id = ?`, retryCount, nextRetry, errMsg, taskID)

	return err
}

// moveToDeadLetter moves a task to the dead letter queue
func (b *AgentBridge) moveToDeadLetter(taskID string, reason string, retryCount int) error {
	// Get the original task
	row := b.db.QueryRow(`SELECT * FROM bridge_tasks WHERE id = ?`, taskID)
	task, err := b.scanTask(row)
	if err != nil {
		return err
	}

	taskJSON, _ := json.Marshal(task)

	// Insert into dead letter queue
	dlqID := uuid.New().String()[:8]
	_, err = b.db.Exec(`
		INSERT INTO bridge_dead_letter (id, task_id, original_task, failure_reason, retry_count, moved_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		dlqID, taskID, string(taskJSON), reason, retryCount, time.Now())
	if err != nil {
		return fmt.Errorf("failed to insert into DLQ: %w", err)
	}

	// Update task status
	_, err = b.db.Exec(`UPDATE bridge_tasks SET status = 'deadletter' WHERE id = ?`, taskID)
	return err
}

// GetDeadLetterTasks returns all tasks in the dead letter queue
func (b *AgentBridge) GetDeadLetterTasks() ([]map[string]interface{}, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rows, err := b.db.Query(`
		SELECT id, task_id, original_task, failure_reason, retry_count, moved_at
		FROM bridge_dead_letter
		ORDER BY moved_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, taskID, originalTask, failureReason string
		var retryCount int
		var movedAt time.Time

		if err := rows.Scan(&id, &taskID, &originalTask, &failureReason, &retryCount, &movedAt); err != nil {
			continue
		}

		results = append(results, map[string]interface{}{
			"id":             id,
			"task_id":        taskID,
			"original_task":  originalTask,
			"failure_reason": failureReason,
			"retry_count":    retryCount,
			"moved_at":       movedAt,
		})
	}

	return results, nil
}

// RequeueDeadLetterTask moves a task from DLQ back to the queue
func (b *AgentBridge) RequeueDeadLetterTask(dlqID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Get the original task from DLQ
	row := b.db.QueryRow(`SELECT task_id, original_task FROM bridge_dead_letter WHERE id = ?`, dlqID)
	var taskID, originalTaskJSON string
	if err := row.Scan(&taskID, &originalTaskJSON); err != nil {
		return fmt.Errorf("DLQ entry not found: %w", err)
	}

	// Reset task status and retry count
	timeoutAt := time.Now().Add(time.Duration(b.config.TaskTimeoutMins) * time.Minute)
	_, err := b.db.Exec(`
		UPDATE bridge_tasks
		SET status = 'queued', retry_count = 0, next_retry_at = NULL, timeout_at = ?, error = NULL
		WHERE id = ?`, timeoutAt, taskID)
	if err != nil {
		return err
	}

	// Remove from DLQ
	_, err = b.db.Exec(`DELETE FROM bridge_dead_letter WHERE id = ?`, dlqID)
	return err
}

// SaveResult saves the result of a completed task
func (b *AgentBridge) SaveResult(result *BridgeResult) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	result.CompletedAt = time.Now()

	findingsJSON, _ := json.Marshal(result.Findings)
	resultID := uuid.New().String()[:8]

	_, err := b.db.Exec(`
		INSERT INTO bridge_results (id, task_id, agent_type, status, output, findings, tokens_used, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		resultID, result.TaskID, result.AgentType, result.Status, result.Output,
		string(findingsJSON), result.TokensUsed, result.CompletedAt)
	if err != nil {
		return fmt.Errorf("failed to save result: %w", err)
	}

	// Update task status
	_, err = b.db.Exec(`UPDATE bridge_tasks SET status = ?, completed_at = ? WHERE id = ?`,
		result.Status, result.CompletedAt, result.TaskID)
	return err
}

// GetResult retrieves the result of a completed task
func (b *AgentBridge) GetResult(taskID string) (*BridgeResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	row := b.db.QueryRow(`
		SELECT task_id, agent_type, status, output, findings, tokens_used, completed_at
		FROM bridge_results WHERE task_id = ?`, taskID)

	var result BridgeResult
	var findingsJSON sql.NullString
	var agentType sql.NullString

	err := row.Scan(&result.TaskID, &agentType, &result.Status, &result.Output,
		&findingsJSON, &result.TokensUsed, &result.CompletedAt)
	if err != nil {
		return nil, err
	}

	if agentType.Valid {
		result.AgentType = agentType.String
	}
	if findingsJSON.Valid {
		_ = json.Unmarshal([]byte(findingsJSON.String), &result.Findings)
	}

	return &result, nil
}

// GetPendingResults returns all completed results that haven't been processed
func (b *AgentBridge) GetPendingResults() ([]*BridgeResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rows, err := b.db.Query(`
		SELECT task_id, agent_type, status, output, findings, tokens_used, completed_at
		FROM bridge_results WHERE status = 'completed' AND processed = 0
		ORDER BY completed_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*BridgeResult
	for rows.Next() {
		var result BridgeResult
		var findingsJSON, agentType sql.NullString

		if err := rows.Scan(&result.TaskID, &agentType, &result.Status, &result.Output,
			&findingsJSON, &result.TokensUsed, &result.CompletedAt); err != nil {
			continue
		}

		if agentType.Valid {
			result.AgentType = agentType.String
		}
		if findingsJSON.Valid {
			_ = json.Unmarshal([]byte(findingsJSON.String), &result.Findings)
		}

		results = append(results, &result)
	}

	return results, nil
}

// MarkResultProcessed marks a result as processed by the ingest service
func (b *AgentBridge) MarkResultProcessed(taskID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	_, err := b.db.Exec(`UPDATE bridge_results SET processed = 1 WHERE task_id = ?`, taskID)
	return err
}

// GetStats returns statistics about the bridge queue
func (b *AgentBridge) GetStats() *BridgeStats {
	b.metricsMu.RLock()
	if b.metricsCache != nil && time.Since(b.metricsCache.LastUpdated) < 30*time.Second {
		defer b.metricsMu.RUnlock()
		return b.metricsCache
	}
	b.metricsMu.RUnlock()

	return b.computeStats()
}

// computeStats calculates current queue statistics
func (b *AgentBridge) computeStats() *BridgeStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := &BridgeStats{LastUpdated: time.Now()}

	// Count tasks by status
	rows, err := b.db.Query(`
		SELECT status, COUNT(*) FROM bridge_tasks GROUP BY status`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int
			if rows.Scan(&status, &count) == nil {
				switch BridgeTaskStatus(status) {
				case BridgeTaskQueued:
					stats.QueuedTasks = count
				case BridgeTaskRunning:
					stats.RunningTasks = count
				case BridgeTaskCompleted:
					stats.CompletedTasks = count
				case BridgeTaskFailed:
					stats.FailedTasks = count
				case BridgeTaskTimeout:
					stats.TimeoutTasks = count
				case BridgeTaskDeadLetter:
					stats.DeadLetterTasks = count
				}
			}
		}
	}

	// Count results
	row := b.db.QueryRow(`SELECT COUNT(*) FROM bridge_results`)
	_ = row.Scan(&stats.TotalResults)

	// Calculate average wait time (time from created to started)
	row = b.db.QueryRow(`
		SELECT AVG((julianday(started_at) - julianday(created_at)) * 86400000)
		FROM bridge_tasks WHERE started_at IS NOT NULL`)
	_ = row.Scan(&stats.AvgWaitTimeMs)

	// Calculate average process time (time from started to completed)
	row = b.db.QueryRow(`
		SELECT AVG((julianday(completed_at) - julianday(started_at)) * 86400000)
		FROM bridge_tasks WHERE completed_at IS NOT NULL AND started_at IS NOT NULL`)
	_ = row.Scan(&stats.AvgProcessTimeMs)

	// Calculate retry rate
	var totalTasks, retriedTasks int
	row = b.db.QueryRow(`SELECT COUNT(*) FROM bridge_tasks`)
	_ = row.Scan(&totalTasks)
	row = b.db.QueryRow(`SELECT COUNT(*) FROM bridge_tasks WHERE retry_count > 0`)
	_ = row.Scan(&retriedTasks)
	if totalTasks > 0 {
		stats.RetryRate = float64(retriedTasks) / float64(totalTasks) * 100
	}

	// Cache the stats
	b.metricsMu.Lock()
	b.metricsCache = stats
	b.metricsMu.Unlock()

	return stats
}

// GetQueueMetrics returns detailed queue metrics for monitoring
func (b *AgentBridge) GetQueueMetrics() *BridgeQueueMetrics {
	b.mu.RLock()
	defer b.mu.RUnlock()

	metrics := &BridgeQueueMetrics{Timestamp: time.Now()}

	// Queue depth
	row := b.db.QueryRow(`SELECT COUNT(*) FROM bridge_tasks WHERE status = 'queued'`)
	_ = row.Scan(&metrics.QueueDepth)

	// Oldest task age
	var oldestCreated sql.NullTime
	row = b.db.QueryRow(`SELECT MIN(created_at) FROM bridge_tasks WHERE status = 'queued'`)
	if row.Scan(&oldestCreated) == nil && oldestCreated.Valid {
		metrics.OldestTaskAge = int64(time.Since(oldestCreated.Time).Seconds())
	}

	// Processing rate (tasks completed in last 5 minutes)
	var completedRecently int
	row = b.db.QueryRow(`
		SELECT COUNT(*) FROM bridge_tasks
		WHERE status = 'completed' AND completed_at > datetime('now', '-5 minutes')`)
	_ = row.Scan(&completedRecently)
	metrics.ProcessingRate = float64(completedRecently) / 5.0 // per minute

	// Error rate (failures in last hour)
	var totalRecent, failedRecent int
	row = b.db.QueryRow(`
		SELECT COUNT(*) FROM bridge_tasks
		WHERE completed_at > datetime('now', '-1 hour')`)
	_ = row.Scan(&totalRecent)
	row = b.db.QueryRow(`
		SELECT COUNT(*) FROM bridge_tasks
		WHERE status IN ('failed', 'timeout', 'deadletter') AND completed_at > datetime('now', '-1 hour')`)
	_ = row.Scan(&failedRecent)
	if totalRecent > 0 {
		metrics.ErrorRate = float64(failedRecent) / float64(totalRecent) * 100
	}

	// Dead letter count
	row = b.db.QueryRow(`SELECT COUNT(*) FROM bridge_dead_letter`)
	_ = row.Scan(&metrics.DeadLetterCount)

	// Calculate health score (0-100)
	metrics.HealthScore = b.calculateHealthScore(metrics)

	// Generate alerts
	metrics.Alerts = b.generateAlerts(metrics)

	return metrics
}

// calculateHealthScore computes a health score based on metrics
func (b *AgentBridge) calculateHealthScore(m *BridgeQueueMetrics) int {
	score := 100

	// Deduct for queue depth
	if m.QueueDepth > b.config.AlertQueueDepth {
		score -= 20
	} else if m.QueueDepth > b.config.AlertQueueDepth/2 {
		score -= 10
	}

	// Deduct for old tasks
	oldestMins := m.OldestTaskAge / 60
	if oldestMins > int64(b.config.AlertOldestTaskMins) {
		score -= 25
	} else if oldestMins > int64(b.config.AlertOldestTaskMins/2) {
		score -= 10
	}

	// Deduct for error rate
	if m.ErrorRate > 20 {
		score -= 30
	} else if m.ErrorRate > 10 {
		score -= 15
	} else if m.ErrorRate > 5 {
		score -= 5
	}

	// Deduct for dead letter queue
	if m.DeadLetterCount > b.config.AlertDeadLetterCount {
		score -= 20
	} else if m.DeadLetterCount > 0 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	return score
}

// generateAlerts creates alert messages based on metrics
func (b *AgentBridge) generateAlerts(m *BridgeQueueMetrics) []string {
	var alerts []string

	if m.QueueDepth > b.config.AlertQueueDepth {
		alerts = append(alerts, fmt.Sprintf("HIGH_QUEUE_DEPTH: %d tasks queued (threshold: %d)",
			m.QueueDepth, b.config.AlertQueueDepth))
	}

	if m.OldestTaskAge/60 > int64(b.config.AlertOldestTaskMins) {
		alerts = append(alerts, fmt.Sprintf("STALE_TASK: oldest task is %d minutes old (threshold: %d)",
			m.OldestTaskAge/60, b.config.AlertOldestTaskMins))
	}

	if m.ErrorRate > 20 {
		alerts = append(alerts, fmt.Sprintf("HIGH_ERROR_RATE: %.1f%% of tasks failing", m.ErrorRate))
	}

	if m.DeadLetterCount > b.config.AlertDeadLetterCount {
		alerts = append(alerts, fmt.Sprintf("DLQ_OVERFLOW: %d tasks in dead letter queue (threshold: %d)",
			m.DeadLetterCount, b.config.AlertDeadLetterCount))
	}

	return alerts
}

// metricsWorker periodically records metrics
func (b *AgentBridge) metricsWorker() {
	ticker := time.NewTicker(time.Duration(b.config.MetricsIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.recordMetrics()
		}
	}
}

// recordMetrics saves current metrics to history
func (b *AgentBridge) recordMetrics() {
	metrics := b.GetQueueMetrics()
	stats := b.GetStats()

	b.mu.Lock()
	defer b.mu.Unlock()

	_, _ = b.db.Exec(`
		INSERT INTO bridge_metrics_history
		(queue_depth, running_tasks, completed_tasks, failed_tasks, dead_letter_count,
		 avg_wait_time_ms, avg_process_time_ms, health_score, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		metrics.QueueDepth, stats.RunningTasks, stats.CompletedTasks, stats.FailedTasks,
		metrics.DeadLetterCount, stats.AvgWaitTimeMs, stats.AvgProcessTimeMs,
		metrics.HealthScore, time.Now())
}

// timeoutWorker checks for and handles timed out tasks
func (b *AgentBridge) timeoutWorker() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.processTimeouts()
		}
	}
}

// processTimeouts marks timed out tasks
func (b *AgentBridge) processTimeouts() {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find running tasks that have exceeded their timeout
	rows, err := b.db.Query(`
		SELECT id FROM bridge_tasks
		WHERE status = 'running' AND timeout_at < datetime('now')`)
	if err != nil {
		return
	}
	defer rows.Close()

	var timedOutIDs []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			timedOutIDs = append(timedOutIDs, id)
		}
	}

	for _, id := range timedOutIDs {
		_, _ = b.db.Exec(`UPDATE bridge_tasks SET status = 'timeout', completed_at = ? WHERE id = ?`,
			time.Now(), id)
	}
}

// cleanupWorker periodically cleans up old data
func (b *AgentBridge) cleanupWorker() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.cleanupOldData()
		}
	}
}

// cleanupOldData removes old completed tasks and metrics
func (b *AgentBridge) cleanupOldData() {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -b.config.CleanupDays)

	// Clean old completed tasks
	_, _ = b.db.Exec(`DELETE FROM bridge_tasks WHERE status IN ('completed', 'timeout') AND completed_at < ?`, cutoff)

	// Clean old results
	_, _ = b.db.Exec(`DELETE FROM bridge_results WHERE completed_at < ?`, cutoff)

	// Clean old metrics (keep 30 days)
	metricsCutoff := time.Now().AddDate(0, 0, -30)
	_, _ = b.db.Exec(`DELETE FROM bridge_metrics_history WHERE recorded_at < ?`, metricsCutoff)
}

// CleanupOldTasks removes tasks older than the specified duration (legacy compatibility)
func (b *AgentBridge) CleanupOldTasks(olderThan time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)

	_, err := b.db.Exec(`DELETE FROM bridge_tasks WHERE completed_at < ?`, cutoff)
	if err != nil {
		return err
	}

	_, err = b.db.Exec(`DELETE FROM bridge_results WHERE completed_at < ?`, cutoff)
	return err
}

// GetMetricsHistory returns historical metrics for the specified duration
func (b *AgentBridge) GetMetricsHistory(ctx context.Context, hours int) ([]map[string]interface{}, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	rows, err := b.db.QueryContext(ctx, `
		SELECT queue_depth, running_tasks, completed_tasks, failed_tasks, dead_letter_count,
		       avg_wait_time_ms, avg_process_time_ms, health_score, recorded_at
		FROM bridge_metrics_history
		WHERE recorded_at > datetime('now', ? || ' hours')
		ORDER BY recorded_at ASC`, fmt.Sprintf("-%d", hours))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var history []map[string]interface{}
	for rows.Next() {
		var queueDepth, runningTasks, completedTasks, failedTasks, dlqCount int
		var avgWaitMs, avgProcessMs int64
		var healthScore int
		var recordedAt time.Time

		if err := rows.Scan(&queueDepth, &runningTasks, &completedTasks, &failedTasks, &dlqCount,
			&avgWaitMs, &avgProcessMs, &healthScore, &recordedAt); err != nil {
			continue
		}

		history = append(history, map[string]interface{}{
			"queue_depth":         queueDepth,
			"running_tasks":       runningTasks,
			"completed_tasks":     completedTasks,
			"failed_tasks":        failedTasks,
			"dead_letter_count":   dlqCount,
			"avg_wait_time_ms":    avgWaitMs,
			"avg_process_time_ms": avgProcessMs,
			"health_score":        healthScore,
			"recorded_at":         recordedAt,
		})
	}

	return history, nil
}

// GetConfig returns the current bridge configuration
func (b *AgentBridge) GetConfig() BridgeConfig {
	return b.config
}

// UpdateConfig updates the bridge configuration
func (b *AgentBridge) UpdateConfig(config BridgeConfig) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.config = config
}
