package clients

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// =============================================================================
// v8.70 - Multi-Model Intelligence
// =============================================================================

// TaskType represents the type of task for intelligent model routing
type TaskType string

const (
	TaskCodeReview     TaskType = "code_review"
	TaskCodeGeneration TaskType = "code_generation"
	TaskAnalysis       TaskType = "analysis"
	TaskSummarization  TaskType = "summarization"
	TaskReasoning      TaskType = "reasoning"
	TaskConversation   TaskType = "conversation"
	TaskExtraction     TaskType = "extraction"
	TaskClassification TaskType = "classification"
	TaskGeneral        TaskType = "general"
)

// ModelCapability represents what a model is good at
type ModelCapability struct {
	TaskType   TaskType `json:"task_type"`
	Score      float64  `json:"score"`       // 0-100, higher = better
	CostFactor float64  `json:"cost_factor"` // Relative cost (1.0 = baseline)
	Latency    string   `json:"latency"`     // fast, medium, slow
}

// ModelProfile describes a model's capabilities and characteristics
type ModelProfile struct {
	Provider     ProviderType        `json:"provider"`
	Model        string              `json:"model"`
	Capabilities []ModelCapability   `json:"capabilities"`
	CostPer1kIn  float64             `json:"cost_per_1k_input"`   // $ per 1k input tokens
	CostPer1kOut float64             `json:"cost_per_1k_output"`  // $ per 1k output tokens
	MaxTokens    int                 `json:"max_tokens"`
	SupportsJSON bool                `json:"supports_json"`
	Thinking     bool                `json:"thinking"` // Supports extended thinking
	Priority     int                 `json:"priority"` // Higher = preferred
}

// ModelRouter intelligently routes requests to the best model
type ModelRouter struct {
	mu           sync.RWMutex
	providers    map[ProviderType]LLMProvider
	profiles     map[ProviderType]*ModelProfile
	fallbackOrder []ProviderType
	costTracker  *CostTracker
	metrics      *RouterMetrics
}

// CostTracker tracks model usage costs
type CostTracker struct {
	mu          sync.RWMutex
	hourlySpend map[string]float64  // "provider:model" -> spend this hour
	dailySpend  map[string]float64  // "provider:model" -> spend today
	totalSpend  map[string]float64  // "provider:model" -> total spend
	hourStart   time.Time
	dayStart    time.Time
	budgets     map[ProviderType]float64 // Daily budget per provider
}

// RouterMetrics tracks routing decisions and outcomes
type RouterMetrics struct {
	mu            sync.RWMutex
	routingCount  map[ProviderType]int     // How often each provider is selected
	successCount  map[ProviderType]int     // Successful requests per provider
	failureCount  map[ProviderType]int     // Failed requests per provider
	fallbackCount map[ProviderType]int     // How often fallback was needed
	latencies     map[ProviderType][]int64 // Request latencies in ms
}

// RoutingDecision captures why a model was selected
type RoutingDecision struct {
	SelectedProvider ProviderType   `json:"selected_provider"`
	SelectedModel    string         `json:"selected_model"`
	TaskType         TaskType       `json:"task_type"`
	Reason           string         `json:"reason"`
	Score            float64        `json:"score"`
	EstimatedCost    float64        `json:"estimated_cost"`
	Alternatives     []Alternative  `json:"alternatives"`
	FallbackChain    []ProviderType `json:"fallback_chain"`
}

// Alternative represents an alternative model choice
type Alternative struct {
	Provider ProviderType `json:"provider"`
	Model    string       `json:"model"`
	Score    float64      `json:"score"`
	Reason   string       `json:"reason"`
}

// RoutingConfig configures routing behavior
type RoutingConfig struct {
	PreferCost       bool           `json:"prefer_cost"`        // Prefer cheaper models
	PreferSpeed      bool           `json:"prefer_speed"`       // Prefer faster models
	PreferQuality    bool           `json:"prefer_quality"`     // Prefer highest quality
	MaxCostPerRequest float64       `json:"max_cost_per_request"` // Cost limit per request
	RequireThinking  bool           `json:"require_thinking"`   // Require models with thinking
	RequireJSON      bool           `json:"require_json"`       // Require JSON output support
	ExcludeProviders []ProviderType `json:"exclude_providers"`  // Providers to skip
	PreferProvider   ProviderType   `json:"prefer_provider"`    // Preferred provider if suitable
}

// DefaultRoutingConfig returns sensible defaults
func DefaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		PreferQuality:    true,
		MaxCostPerRequest: 0.10, // $0.10 max per request
	}
}

// NewModelRouter creates a new model router
func NewModelRouter() (*ModelRouter, error) {
	router := &ModelRouter{
		providers:    make(map[ProviderType]LLMProvider),
		profiles:     initDefaultProfiles(),
		fallbackOrder: []ProviderType{ProviderClaude, ProviderOpenAI, ProviderGemini},
		costTracker:  newCostTracker(),
		metrics:      newRouterMetrics(),
	}

	// Initialize providers
	if claude, err := NewClaudeClient(); err == nil && claude != nil {
		router.providers[ProviderClaude] = claude
	}
	if openai, err := NewOpenAIClient(); err == nil && openai != nil {
		router.providers[ProviderOpenAI] = openai
	}
	if gemini, err := NewGeminiClient(); err == nil && gemini != nil {
		router.providers[ProviderGemini] = gemini
	}

	if len(router.providers) == 0 {
		return nil, fmt.Errorf("no LLM providers available")
	}

	return router, nil
}

// initDefaultProfiles sets up default model profiles
func initDefaultProfiles() map[ProviderType]*ModelProfile {
	return map[ProviderType]*ModelProfile{
		ProviderClaude: {
			Provider:     ProviderClaude,
			Model:        "claude-sonnet-4-20250514",
			CostPer1kIn:  0.003,
			CostPer1kOut: 0.015,
			MaxTokens:    200000,
			SupportsJSON: true,
			Thinking:     true,
			Priority:     100,
			Capabilities: []ModelCapability{
				{TaskType: TaskCodeReview, Score: 95, CostFactor: 1.0, Latency: "medium"},
				{TaskType: TaskCodeGeneration, Score: 95, CostFactor: 1.0, Latency: "medium"},
				{TaskType: TaskAnalysis, Score: 95, CostFactor: 1.0, Latency: "medium"},
				{TaskType: TaskReasoning, Score: 98, CostFactor: 1.0, Latency: "slow"},
				{TaskType: TaskSummarization, Score: 90, CostFactor: 1.0, Latency: "medium"},
				{TaskType: TaskExtraction, Score: 92, CostFactor: 1.0, Latency: "fast"},
				{TaskType: TaskClassification, Score: 90, CostFactor: 1.0, Latency: "fast"},
				{TaskType: TaskConversation, Score: 95, CostFactor: 1.0, Latency: "medium"},
				{TaskType: TaskGeneral, Score: 95, CostFactor: 1.0, Latency: "medium"},
			},
		},
		ProviderOpenAI: {
			Provider:     ProviderOpenAI,
			Model:        "gpt-4o",
			CostPer1kIn:  0.0025,
			CostPer1kOut: 0.01,
			MaxTokens:    128000,
			SupportsJSON: true,
			Thinking:     false, // o1 models have thinking, gpt-4o doesn't
			Priority:     90,
			Capabilities: []ModelCapability{
				{TaskType: TaskCodeReview, Score: 90, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskCodeGeneration, Score: 92, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskAnalysis, Score: 88, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskReasoning, Score: 85, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskSummarization, Score: 90, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskExtraction, Score: 92, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskClassification, Score: 92, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskConversation, Score: 88, CostFactor: 0.8, Latency: "fast"},
				{TaskType: TaskGeneral, Score: 90, CostFactor: 0.8, Latency: "fast"},
			},
		},
		ProviderGemini: {
			Provider:     ProviderGemini,
			Model:        "gemini-2.0-flash",
			CostPer1kIn:  0.0001,
			CostPer1kOut: 0.0004,
			MaxTokens:    1000000,
			SupportsJSON: true,
			Thinking:     true, // gemini-2.0-flash-thinking-exp
			Priority:     80,
			Capabilities: []ModelCapability{
				{TaskType: TaskCodeReview, Score: 85, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskCodeGeneration, Score: 85, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskAnalysis, Score: 82, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskReasoning, Score: 80, CostFactor: 0.1, Latency: "medium"},
				{TaskType: TaskSummarization, Score: 88, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskExtraction, Score: 88, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskClassification, Score: 88, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskConversation, Score: 82, CostFactor: 0.1, Latency: "fast"},
				{TaskType: TaskGeneral, Score: 85, CostFactor: 0.1, Latency: "fast"},
			},
		},
	}
}

func newCostTracker() *CostTracker {
	now := time.Now()
	return &CostTracker{
		hourlySpend: make(map[string]float64),
		dailySpend:  make(map[string]float64),
		totalSpend:  make(map[string]float64),
		hourStart:   now.Truncate(time.Hour),
		dayStart:    time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()),
		budgets:     getBudgetsFromEnv(),
	}
}

// getBudgetsFromEnv returns daily budgets from environment or defaults
func getBudgetsFromEnv() map[ProviderType]float64 {
	budgets := map[ProviderType]float64{
		ProviderClaude: 10.0,
		ProviderOpenAI: 10.0,
		ProviderGemini: 5.0,
	}
	if v := parseEnvFloat("WEBB_DAILY_BUDGET_CLAUDE", 0); v > 0 {
		budgets[ProviderClaude] = v
	}
	if v := parseEnvFloat("WEBB_DAILY_BUDGET_OPENAI", 0); v > 0 {
		budgets[ProviderOpenAI] = v
	}
	if v := parseEnvFloat("WEBB_DAILY_BUDGET_GEMINI", 0); v > 0 {
		budgets[ProviderGemini] = v
	}
	return budgets
}

// parseEnvFloat parses a float from env var or returns default
func parseEnvFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func newRouterMetrics() *RouterMetrics {
	return &RouterMetrics{
		routingCount:  make(map[ProviderType]int),
		successCount:  make(map[ProviderType]int),
		failureCount:  make(map[ProviderType]int),
		fallbackCount: make(map[ProviderType]int),
		latencies:     make(map[ProviderType][]int64),
	}
}

// Route selects the best model for a task
func (r *ModelRouter) Route(taskType TaskType, config *RoutingConfig) (*RoutingDecision, error) {
	if config == nil {
		config = DefaultRoutingConfig()
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	decision := &RoutingDecision{
		TaskType:      taskType,
		Alternatives:  make([]Alternative, 0),
		FallbackChain: make([]ProviderType, 0),
	}

	// Score each available provider
	type scoredProvider struct {
		provider ProviderType
		score    float64
		reason   string
	}
	var scored []scoredProvider

	for pt, profile := range r.profiles {
		// Skip if provider not available
		if _, ok := r.providers[pt]; !ok {
			continue
		}

		// Skip excluded providers
		excluded := false
		for _, ex := range config.ExcludeProviders {
			if ex == pt {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}

		// Check requirements
		if config.RequireThinking && !profile.Thinking {
			continue
		}
		if config.RequireJSON && !profile.SupportsJSON {
			continue
		}

		// Calculate score for this task
		score := r.calculateScore(pt, profile, taskType, config)
		reason := r.explainScore(pt, profile, taskType, score, config)

		scored = append(scored, scoredProvider{
			provider: pt,
			score:    score,
			reason:   reason,
		})
	}

	if len(scored) == 0 {
		return nil, fmt.Errorf("no suitable providers for task %s with given constraints", taskType)
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Select best
	best := scored[0]
	decision.SelectedProvider = best.provider
	decision.SelectedModel = r.profiles[best.provider].Model
	decision.Score = best.score
	decision.Reason = best.reason
	decision.EstimatedCost = r.estimateCost(best.provider, 1000, 500) // Estimate for avg request

	// Add alternatives
	for i := 1; i < len(scored) && i < 3; i++ {
		decision.Alternatives = append(decision.Alternatives, Alternative{
			Provider: scored[i].provider,
			Model:    r.profiles[scored[i].provider].Model,
			Score:    scored[i].score,
			Reason:   scored[i].reason,
		})
	}

	// Build fallback chain
	for _, sp := range scored {
		if sp.provider != decision.SelectedProvider {
			decision.FallbackChain = append(decision.FallbackChain, sp.provider)
		}
	}

	// Record routing
	r.metrics.recordRouting(decision.SelectedProvider)

	return decision, nil
}

func (r *ModelRouter) calculateScore(pt ProviderType, profile *ModelProfile, taskType TaskType, config *RoutingConfig) float64 {
	// Base score from capability
	baseScore := 50.0
	for _, cap := range profile.Capabilities {
		if cap.TaskType == taskType {
			baseScore = cap.Score
			break
		}
	}

	score := baseScore

	// Adjust for preferences
	if config.PreferCost {
		// Lower cost = higher score bonus
		for _, cap := range profile.Capabilities {
			if cap.TaskType == taskType {
				score += (1.0 - cap.CostFactor) * 20 // Up to +20 for cheap models
				break
			}
		}
	}

	if config.PreferSpeed {
		for _, cap := range profile.Capabilities {
			if cap.TaskType == taskType {
				switch cap.Latency {
				case "fast":
					score += 15
				case "medium":
					score += 5
				case "slow":
					score -= 5
				}
				break
			}
		}
	}

	if config.PreferQuality {
		score += float64(profile.Priority) / 10 // Up to +10 for high priority
	}

	if config.PreferProvider == pt {
		score += 10 // Bonus for preferred provider
	}

	// Check budget constraints
	if r.costTracker.isOverBudget(pt) {
		score -= 50 // Heavy penalty for over-budget
	}

	return score
}

func (r *ModelRouter) explainScore(pt ProviderType, profile *ModelProfile, taskType TaskType, score float64, config *RoutingConfig) string {
	var reasons []string

	for _, cap := range profile.Capabilities {
		if cap.TaskType == taskType {
			reasons = append(reasons, fmt.Sprintf("%.0f%% capability for %s", cap.Score, taskType))
			if cap.Latency == "fast" {
				reasons = append(reasons, "fast response time")
			}
			if cap.CostFactor < 0.5 {
				reasons = append(reasons, "low cost")
			}
			break
		}
	}

	if profile.Priority >= 90 {
		reasons = append(reasons, "high quality model")
	}

	if config.PreferProvider == pt {
		reasons = append(reasons, "preferred provider")
	}

	return strings.Join(reasons, ", ")
}

func (r *ModelRouter) estimateCost(pt ProviderType, inputTokens, outputTokens int) float64 {
	profile := r.profiles[pt]
	if profile == nil {
		return 0
	}
	return (float64(inputTokens)/1000)*profile.CostPer1kIn + (float64(outputTokens)/1000)*profile.CostPer1kOut
}

// AnalyzeWithRouting routes to best model and performs analysis
func (r *ModelRouter) AnalyzeWithRouting(ctx context.Context, req AnalysisRequest, taskType TaskType, config *RoutingConfig) (*AnalysisResult, *RoutingDecision, error) {
	decision, err := r.Route(taskType, config)
	if err != nil {
		return nil, nil, err
	}

	return r.AnalyzeWithFallback(ctx, req, decision)
}

// AnalyzeWithFallback performs analysis with automatic fallback on failure
func (r *ModelRouter) AnalyzeWithFallback(ctx context.Context, req AnalysisRequest, decision *RoutingDecision) (*AnalysisResult, *RoutingDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Try primary provider
	start := time.Now()
	provider := r.providers[decision.SelectedProvider]
	if provider != nil {
		result, err := provider.AnalyzeWithRetry(ctx, req, 2)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			r.metrics.recordSuccess(decision.SelectedProvider, latency)
			r.costTracker.recordUsage(decision.SelectedProvider, r.profiles[decision.SelectedProvider].Model, decision.EstimatedCost)
			return result, decision, nil
		}

		r.metrics.recordFailure(decision.SelectedProvider)
	}

	// Try fallback chain
	for _, fallbackPt := range decision.FallbackChain {
		fallbackProvider := r.providers[fallbackPt]
		if fallbackProvider == nil {
			continue
		}

		r.metrics.recordFallback(fallbackPt)

		start := time.Now()
		result, err := fallbackProvider.AnalyzeWithRetry(ctx, req, 2)
		latency := time.Since(start).Milliseconds()

		if err == nil {
			r.metrics.recordSuccess(fallbackPt, latency)
			fallbackCost := r.estimateCost(fallbackPt, 1000, 500)
			r.costTracker.recordUsage(fallbackPt, r.profiles[fallbackPt].Model, fallbackCost)

			// Update decision to reflect fallback
			decision.SelectedProvider = fallbackPt
			decision.SelectedModel = r.profiles[fallbackPt].Model
			decision.Reason = "fallback after primary failure"
			decision.EstimatedCost = fallbackCost

			return result, decision, nil
		}

		r.metrics.recordFailure(fallbackPt)
	}

	return nil, decision, fmt.Errorf("all providers failed for task")
}

// =============================================================================
// Cost Tracking
// =============================================================================

func (ct *CostTracker) recordUsage(pt ProviderType, model string, cost float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	key := fmt.Sprintf("%s:%s", pt, model)
	now := time.Now()

	// Reset hourly/daily if needed
	if now.After(ct.hourStart.Add(time.Hour)) {
		ct.hourlySpend = make(map[string]float64)
		ct.hourStart = now.Truncate(time.Hour)
	}
	if now.After(ct.dayStart.Add(24 * time.Hour)) {
		ct.dailySpend = make(map[string]float64)
		ct.dayStart = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	}

	ct.hourlySpend[key] += cost
	ct.dailySpend[key] += cost
	ct.totalSpend[key] += cost
}

func (ct *CostTracker) isOverBudget(pt ProviderType) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	budget, ok := ct.budgets[pt]
	if !ok {
		return false
	}

	var dailyTotal float64
	prefix := string(pt) + ":"
	for key, spend := range ct.dailySpend {
		if strings.HasPrefix(key, prefix) {
			dailyTotal += spend
		}
	}

	return dailyTotal >= budget
}

// GetCostSummary returns cost tracking summary
func (ct *CostTracker) GetCostSummary() map[string]interface{} {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	return map[string]interface{}{
		"hourly_spend": ct.hourlySpend,
		"daily_spend":  ct.dailySpend,
		"total_spend":  ct.totalSpend,
		"budgets":      ct.budgets,
	}
}

// SetBudget sets the daily budget for a provider
func (ct *CostTracker) SetBudget(pt ProviderType, budget float64) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.budgets[pt] = budget
}

// =============================================================================
// Metrics
// =============================================================================

func (m *RouterMetrics) recordRouting(pt ProviderType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routingCount[pt]++
}

func (m *RouterMetrics) recordSuccess(pt ProviderType, latencyMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.successCount[pt]++
	m.latencies[pt] = append(m.latencies[pt], latencyMs)
	// Keep last 100 latencies
	if len(m.latencies[pt]) > 100 {
		m.latencies[pt] = m.latencies[pt][len(m.latencies[pt])-100:]
	}
}

func (m *RouterMetrics) recordFailure(pt ProviderType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failureCount[pt]++
}

func (m *RouterMetrics) recordFallback(pt ProviderType) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallbackCount[pt]++
}

// GetMetrics returns router metrics summary
func (m *RouterMetrics) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgLatencies := make(map[ProviderType]float64)
	for pt, latencies := range m.latencies {
		if len(latencies) > 0 {
			var sum int64
			for _, l := range latencies {
				sum += l
			}
			avgLatencies[pt] = float64(sum) / float64(len(latencies))
		}
	}

	successRates := make(map[ProviderType]float64)
	for pt := range m.successCount {
		total := m.successCount[pt] + m.failureCount[pt]
		if total > 0 {
			successRates[pt] = float64(m.successCount[pt]) / float64(total) * 100
		}
	}

	return map[string]interface{}{
		"routing_count":  m.routingCount,
		"success_count":  m.successCount,
		"failure_count":  m.failureCount,
		"fallback_count": m.fallbackCount,
		"avg_latency_ms": avgLatencies,
		"success_rates":  successRates,
	}
}

// GetAvailableProviders returns list of available providers
func (r *ModelRouter) GetAvailableProviders() []ProviderType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]ProviderType, 0, len(r.providers))
	for pt := range r.providers {
		providers = append(providers, pt)
	}
	return providers
}

// GetModelProfiles returns all model profiles
func (r *ModelRouter) GetModelProfiles() map[ProviderType]*ModelProfile {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.profiles
}

// GetCostTracker returns the cost tracker
func (r *ModelRouter) GetCostTracker() *CostTracker {
	return r.costTracker
}

// GetRouterMetrics returns the router metrics
func (r *ModelRouter) GetRouterMetrics() *RouterMetrics {
	return r.metrics
}

// FormatRoutingReport formats a routing report as markdown
func FormatRoutingReport(decision *RoutingDecision, metrics map[string]interface{}, costs map[string]interface{}) string {
	var sb strings.Builder

	sb.WriteString("# Model Routing Report\n\n")

	sb.WriteString("## Selected Model\n\n")
	sb.WriteString(fmt.Sprintf("- **Provider:** %s\n", decision.SelectedProvider))
	sb.WriteString(fmt.Sprintf("- **Model:** %s\n", decision.SelectedModel))
	sb.WriteString(fmt.Sprintf("- **Task:** %s\n", decision.TaskType))
	sb.WriteString(fmt.Sprintf("- **Score:** %.1f\n", decision.Score))
	sb.WriteString(fmt.Sprintf("- **Reason:** %s\n", decision.Reason))
	sb.WriteString(fmt.Sprintf("- **Estimated Cost:** $%.4f\n\n", decision.EstimatedCost))

	if len(decision.Alternatives) > 0 {
		sb.WriteString("## Alternatives Considered\n\n")
		sb.WriteString("| Provider | Model | Score | Reason |\n")
		sb.WriteString("|----------|-------|-------|--------|\n")
		for _, alt := range decision.Alternatives {
			sb.WriteString(fmt.Sprintf("| %s | %s | %.1f | %s |\n",
				alt.Provider, alt.Model, alt.Score, alt.Reason))
		}
		sb.WriteString("\n")
	}

	if len(decision.FallbackChain) > 0 {
		sb.WriteString("## Fallback Chain\n\n")
		for i, pt := range decision.FallbackChain {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, pt))
		}
		sb.WriteString("\n")
	}

	if metrics != nil {
		sb.WriteString("## Router Metrics\n\n")
		if successRates, ok := metrics["success_rates"].(map[ProviderType]float64); ok {
			sb.WriteString("| Provider | Success Rate | Avg Latency |\n")
			sb.WriteString("|----------|--------------|-------------|\n")
			avgLatencies, _ := metrics["avg_latency_ms"].(map[ProviderType]float64)
			for pt, rate := range successRates {
				latency := "N/A"
				if avgLatencies != nil {
					if l, ok := avgLatencies[pt]; ok {
						latency = fmt.Sprintf("%.0fms", l)
					}
				}
				sb.WriteString(fmt.Sprintf("| %s | %.1f%% | %s |\n", pt, rate, latency))
			}
		}
	}

	return sb.String()
}

// =============================================================================
// v8.75 - Specialized Model Agents
// =============================================================================

// SpecializedAgent represents a task-specific model configuration
type SpecializedAgent struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	DefaultModel   ProviderType           `json:"default_model"`
	TaskType       TaskType               `json:"task_type"`
	SystemPrompt   string                 `json:"system_prompt"`
	UseThinking    bool                   `json:"use_thinking"`
	ThinkingBudget int                    `json:"thinking_budget"`
	MaxTokens      int                    `json:"max_tokens"`
	Temperature    float64                `json:"temperature"`
	Config         map[string]interface{} `json:"config"`
}

// CodeReviewerAgent is a Claude-powered code review agent with extended thinking
type CodeReviewerAgent struct {
	router     *ModelRouter
	agent      *SpecializedAgent
}

// NewCodeReviewerAgent creates a new code review agent
func NewCodeReviewerAgent() (*CodeReviewerAgent, error) {
	router, err := NewModelRouter()
	if err != nil {
		return nil, err
	}

	// Use dynamic thinking budget based on tier (v131.3)
	thinkingBudget := GetThinkingBudget("code_review")

	return &CodeReviewerAgent{
		router: router,
		agent: &SpecializedAgent{
			Name:        "code-reviewer",
			Description: "Deep code review with extended thinking for complex analysis",
			DefaultModel: ProviderClaude,
			TaskType:    TaskCodeReview,
			SystemPrompt: `You are an expert code reviewer. Analyze the code for:
1. Security vulnerabilities (injection, XSS, authentication issues)
2. Performance issues (N+1 queries, memory leaks, inefficient algorithms)
3. Code quality (readability, maintainability, SOLID principles)
4. Best practices (error handling, logging, documentation)
5. Potential bugs and edge cases

Provide actionable feedback with specific line references and suggested fixes.`,
			UseThinking:    true,
			ThinkingBudget: thinkingBudget,
			MaxTokens:      8000,
			Temperature:    0.2,
		},
	}, nil
}

// Review performs a deep code review with thinking
func (a *CodeReviewerAgent) Review(ctx context.Context, code string, language string, context string) (*CodeReviewResult, error) {
	prompt := fmt.Sprintf("Review this %s code:\n\n```%s\n%s\n```", language, language, code)
	if context != "" {
		prompt += fmt.Sprintf("\n\nContext: %s", context)
	}

	req := AnalysisRequest{
		SystemPrompt:   a.agent.SystemPrompt,
		Prompt:         prompt,
		MaxTokens:      a.agent.MaxTokens,
		Temperature:    a.agent.Temperature,
		ThinkingBudget: a.agent.ThinkingBudget,
	}

	result, decision, err := a.router.AnalyzeWithRouting(ctx, req, a.agent.TaskType, &RoutingConfig{
		PreferQuality:   true,
		RequireThinking: a.agent.UseThinking,
		PreferProvider:  a.agent.DefaultModel,
	})
	if err != nil {
		return nil, err
	}

	return &CodeReviewResult{
		Review:      result.Content,
		Thinking:    result.Thinking,
		Provider:    decision.SelectedProvider,
		Model:       decision.SelectedModel,
		InputTokens: result.InputTokens,
		OutputTokens: result.OutputTokens,
		ThinkingTokens: result.ThinkingTokens,
	}, nil
}

// CodeReviewResult contains the code review output
type CodeReviewResult struct {
	Review         string       `json:"review"`
	Thinking       string       `json:"thinking,omitempty"`
	Provider       ProviderType `json:"provider"`
	Model          string       `json:"model"`
	InputTokens    int          `json:"input_tokens"`
	OutputTokens   int          `json:"output_tokens"`
	ThinkingTokens int          `json:"thinking_tokens,omitempty"`
	Issues         []CodeIssue  `json:"issues,omitempty"`
}

// CodeIssue represents a specific issue found in code
type CodeIssue struct {
	Line        int    `json:"line"`
	Severity    string `json:"severity"` // critical, high, medium, low, info
	Category    string `json:"category"` // security, performance, quality, bug
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// ConsensusAgent uses multiple models and combines their outputs
type ConsensusAgent struct {
	router       *ModelRouter
	agent        *SpecializedAgent
	models       []ProviderType
	minConsensus float64
}

// NewConsensusAgent creates a new multi-model consensus agent
func NewConsensusAgent(models []ProviderType) (*ConsensusAgent, error) {
	router, err := NewModelRouter()
	if err != nil {
		return nil, err
	}

	if len(models) == 0 {
		models = []ProviderType{ProviderClaude, ProviderOpenAI, ProviderGemini}
	}

	return &ConsensusAgent{
		router: router,
		agent: &SpecializedAgent{
			Name:        "consensus",
			Description: "Multi-model consensus for complex analysis requiring validation",
			TaskType:    TaskAnalysis,
			MaxTokens:   4000,
			Temperature: 0.3,
		},
		models:       models,
		minConsensus: 0.66, // 2/3 must agree
	}, nil
}

// Analyze performs consensus-based analysis across multiple models
func (a *ConsensusAgent) Analyze(ctx context.Context, prompt string, systemPrompt string) (*AgentConsensusOutput, error) {
	type modelResult struct {
		provider ProviderType
		result   *AnalysisResult
		err      error
	}

	resultCh := make(chan modelResult, len(a.models))

	// Query all models in parallel
	for _, pt := range a.models {
		go func(provider ProviderType) {
			req := AnalysisRequest{
				SystemPrompt: systemPrompt,
				Prompt:       prompt,
				MaxTokens:    a.agent.MaxTokens,
				Temperature:  a.agent.Temperature,
			}

			config := &RoutingConfig{
				PreferProvider:   provider,
				ExcludeProviders: []ProviderType{},
			}
			// Only use the specified provider
			for _, other := range a.models {
				if other != provider {
					config.ExcludeProviders = append(config.ExcludeProviders, other)
				}
			}

			result, _, err := a.router.AnalyzeWithRouting(ctx, req, a.agent.TaskType, config)
			resultCh <- modelResult{provider: provider, result: result, err: err}
		}(pt)
	}

	// Collect results
	var responses []AgentModelResponse
	for i := 0; i < len(a.models); i++ {
		mr := <-resultCh
		if mr.err == nil && mr.result != nil {
			responses = append(responses, AgentModelResponse{
				Provider: mr.provider,
				Content:  mr.result.Content,
				Tokens:   mr.result.InputTokens + mr.result.OutputTokens,
			})
		}
	}

	if len(responses) == 0 {
		return nil, fmt.Errorf("all models failed")
	}

	// Calculate consensus
	consensus := a.calculateConsensus(responses)

	return &AgentConsensusOutput{
		Responses:       responses,
		ConsensusLevel:  consensus.Level,
		Summary:         consensus.Summary,
		Disagreements:   consensus.Disagreements,
		Recommendation:  consensus.Recommendation,
	}, nil
}

// AgentModelResponse represents a single model's response
type AgentModelResponse struct {
	Provider ProviderType `json:"provider"`
	Content  string       `json:"content"`
	Tokens   int          `json:"tokens"`
}

// AgentConsensusOutput contains the multi-model consensus output
type AgentConsensusOutput struct {
	Responses      []AgentModelResponse `json:"responses"`
	ConsensusLevel float64              `json:"consensus_level"` // 0-1
	Summary        string               `json:"summary"`
	Disagreements  []string             `json:"disagreements,omitempty"`
	Recommendation string               `json:"recommendation"`
}

type consensusAnalysis struct {
	Level          float64
	Summary        string
	Disagreements  []string
	Recommendation string
}

func (a *ConsensusAgent) calculateConsensus(responses []AgentModelResponse) *consensusAnalysis {
	if len(responses) == 1 {
		return &consensusAnalysis{
			Level:          1.0,
			Summary:        responses[0].Content,
			Recommendation: "Single model response - no consensus validation possible",
		}
	}

	// Simple heuristic: check for key agreement by comparing response lengths and keywords
	// In production, you'd use semantic similarity or structured output comparison
	avgLen := 0
	for _, r := range responses {
		avgLen += len(r.Content)
	}
	avgLen /= len(responses)

	// Calculate variance in response lengths as a rough similarity proxy
	variance := 0.0
	for _, r := range responses {
		diff := float64(len(r.Content) - avgLen)
		variance += diff * diff
	}
	variance /= float64(len(responses))

	// Normalize to 0-1 (lower variance = higher consensus)
	maxVariance := float64(avgLen * avgLen)
	consensusLevel := 1.0 - (variance / maxVariance)
	if consensusLevel < 0 {
		consensusLevel = 0
	}

	var disagreements []string
	if consensusLevel < a.minConsensus {
		disagreements = append(disagreements, "Significant divergence in response lengths detected")
	}

	// Build summary from shortest response (most concise)
	shortestIdx := 0
	for i, r := range responses {
		if len(r.Content) < len(responses[shortestIdx].Content) {
			shortestIdx = i
		}
	}

	rec := "High consensus - all models agree"
	if consensusLevel < a.minConsensus {
		rec = "Low consensus - recommend manual review"
	} else if consensusLevel < 0.8 {
		rec = "Moderate consensus - consider additional validation"
	}

	return &consensusAnalysis{
		Level:          consensusLevel,
		Summary:        responses[shortestIdx].Content,
		Disagreements:  disagreements,
		Recommendation: rec,
	}
}

// ExtractionAgent is optimized for fast, high-volume data extraction
type ExtractionAgent struct {
	router *ModelRouter
	agent  *SpecializedAgent
}

// NewExtractionAgent creates a new extraction agent (Gemini-optimized)
func NewExtractionAgent() (*ExtractionAgent, error) {
	router, err := NewModelRouter()
	if err != nil {
		return nil, err
	}

	return &ExtractionAgent{
		router: router,
		agent: &SpecializedAgent{
			Name:        "extractor",
			Description: "Fast data extraction optimized for high volume and low cost",
			DefaultModel: ProviderGemini,
			TaskType:    TaskExtraction,
			SystemPrompt: "Extract the requested information in a structured format. Be precise and concise.",
			MaxTokens:   2000,
			Temperature: 0.1,
		},
	}, nil
}

// Extract extracts structured data from content
func (a *ExtractionAgent) Extract(ctx context.Context, content string, schema string) (*ExtractionResult, error) {
	prompt := fmt.Sprintf("Extract data from the following content according to this schema:\n\nSchema:\n%s\n\nContent:\n%s", schema, content)

	req := AnalysisRequest{
		SystemPrompt: a.agent.SystemPrompt,
		Prompt:       prompt,
		MaxTokens:    a.agent.MaxTokens,
		Temperature:  a.agent.Temperature,
		JSONMode:     true,
	}

	result, decision, err := a.router.AnalyzeWithRouting(ctx, req, a.agent.TaskType, &RoutingConfig{
		PreferCost:     true,
		PreferSpeed:    true,
		PreferProvider: a.agent.DefaultModel,
		RequireJSON:    true,
	})
	if err != nil {
		return nil, err
	}

	return &ExtractionResult{
		Data:         result.Content,
		Provider:     decision.SelectedProvider,
		Model:        decision.SelectedModel,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Cost:         decision.EstimatedCost,
	}, nil
}

// ExtractionResult contains the extraction output
type ExtractionResult struct {
	Data         string       `json:"data"`
	Provider     ProviderType `json:"provider"`
	Model        string       `json:"model"`
	InputTokens  int          `json:"input_tokens"`
	OutputTokens int          `json:"output_tokens"`
	Cost         float64      `json:"cost"`
}

// ModelCompareTest represents an A/B test between models
type ModelCompareTest struct {
	ID          string                         `json:"id"`
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	ModelA      ProviderType                   `json:"model_a"`
	ModelB      ProviderType                   `json:"model_b"`
	Metrics     map[string]*ModelCompareMetrics `json:"metrics"`
	StartTime   time.Time                      `json:"start_time"`
	EndTime     *time.Time                     `json:"end_time,omitempty"`
	Status      string                         `json:"status"` // running, completed, stopped
}

// ModelCompareMetrics tracks metrics for each model in an A/B test
type ModelCompareMetrics struct {
	Requests      int       `json:"requests"`
	Successes     int       `json:"successes"`
	Failures      int       `json:"failures"`
	TotalLatency  int64     `json:"total_latency_ms"`
	TotalTokens   int       `json:"total_tokens"`
	TotalCost     float64   `json:"total_cost"`
	QualityScores []float64 `json:"quality_scores,omitempty"`
}

// ModelCompareRunner manages A/B tests between models
type ModelCompareRunner struct {
	mu     sync.RWMutex
	router *ModelRouter
	tests  map[string]*ModelCompareTest
}

// NewModelCompareRunner creates a new A/B test runner
func NewModelCompareRunner() (*ModelCompareRunner, error) {
	router, err := NewModelRouter()
	if err != nil {
		return nil, err
	}

	return &ModelCompareRunner{
		router: router,
		tests:  make(map[string]*ModelCompareTest),
	}, nil
}

// CreateTest creates a new A/B test
func (r *ModelCompareRunner) CreateTest(name string, description string, modelA, modelB ProviderType) *ModelCompareTest {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := fmt.Sprintf("ab-%d", time.Now().UnixNano())
	test := &ModelCompareTest{
		ID:          id,
		Name:        name,
		Description: description,
		ModelA:      modelA,
		ModelB:      modelB,
		Metrics: map[string]*ModelCompareMetrics{
			string(modelA): {QualityScores: []float64{}},
			string(modelB): {QualityScores: []float64{}},
		},
		StartTime: time.Now(),
		Status:    "running",
	}

	r.tests[id] = test
	return test
}

// modelCompareRun holds result of a single model run
type modelCompareRun struct {
	provider ProviderType
	result   *AnalysisResult
	latency  int64
	err      error
}

// RunTest executes a request in the context of an A/B test
func (r *ModelCompareRunner) RunTest(ctx context.Context, testID string, req AnalysisRequest, taskType TaskType) (*ModelCompareRunResult, error) {
	r.mu.RLock()
	test, ok := r.tests[testID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("test not found: %s", testID)
	}
	if test.Status != "running" {
		return nil, fmt.Errorf("test is not running: %s", test.Status)
	}

	resultCh := make(chan modelCompareRun, 2)

	for _, provider := range []ProviderType{test.ModelA, test.ModelB} {
		go func(pt ProviderType) {
			start := time.Now()
			config := &RoutingConfig{PreferProvider: pt}
			result, _, err := r.router.AnalyzeWithRouting(ctx, req, taskType, config)
			latency := time.Since(start).Milliseconds()
			resultCh <- modelCompareRun{provider: pt, result: result, latency: latency, err: err}
		}(provider)
	}

	// Collect results
	results := make(map[ProviderType]*modelCompareRun)
	for i := 0; i < 2; i++ {
		mr := <-resultCh
		results[mr.provider] = &mr
	}

	// Update metrics
	r.mu.Lock()
	for provider, mr := range results {
		m := test.Metrics[string(provider)]
		m.Requests++
		if mr.err == nil {
			m.Successes++
			m.TotalLatency += mr.latency
			if mr.result != nil {
				m.TotalTokens += mr.result.InputTokens + mr.result.OutputTokens
			}
		} else {
			m.Failures++
		}
	}
	r.mu.Unlock()

	return &ModelCompareRunResult{
		TestID: testID,
		ModelA: results[test.ModelA],
		ModelB: results[test.ModelB],
	}, nil
}

// ModelCompareRunResult contains results from a single A/B test run
type ModelCompareRunResult struct {
	TestID string           `json:"test_id"`
	ModelA *modelCompareRun `json:"-"`
	ModelB *modelCompareRun `json:"-"`
}

// GetTestSummary returns a summary of test results
func (r *ModelCompareRunner) GetTestSummary(testID string) (*ModelCompareSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	test, ok := r.tests[testID]
	if !ok {
		return nil, fmt.Errorf("test not found: %s", testID)
	}

	summary := &ModelCompareSummary{
		Test:    test,
		Results: make(map[string]ModelCompareModelSummary),
	}

	for provider, metrics := range test.Metrics {
		avgLatency := float64(0)
		successRate := float64(0)
		if metrics.Requests > 0 {
			if metrics.Successes > 0 {
				avgLatency = float64(metrics.TotalLatency) / float64(metrics.Successes)
			}
			successRate = float64(metrics.Successes) / float64(metrics.Requests) * 100
		}

		summary.Results[provider] = ModelCompareModelSummary{
			Provider:    ProviderType(provider),
			Requests:    metrics.Requests,
			SuccessRate: successRate,
			AvgLatency:  avgLatency,
			TotalTokens: metrics.TotalTokens,
			TotalCost:   metrics.TotalCost,
		}
	}

	// Determine winner
	if len(summary.Results) == 2 {
		var modelA, modelB ModelCompareModelSummary
		for _, m := range summary.Results {
			if m.Provider == test.ModelA {
				modelA = m
			} else {
				modelB = m
			}
		}

		// Simple scoring: success rate + speed + cost efficiency
		scoreA := modelA.SuccessRate - (modelA.AvgLatency / 100) - (modelA.TotalCost * 10)
		scoreB := modelB.SuccessRate - (modelB.AvgLatency / 100) - (modelB.TotalCost * 10)

		if scoreA > scoreB {
			summary.Winner = test.ModelA
			summary.WinReason = "Higher composite score (success + speed + cost)"
		} else if scoreB > scoreA {
			summary.Winner = test.ModelB
			summary.WinReason = "Higher composite score (success + speed + cost)"
		} else {
			summary.WinReason = "Tie - no clear winner"
		}
	}

	return summary, nil
}

// ModelCompareSummary contains the summary of an A/B test
type ModelCompareSummary struct {
	Test      *ModelCompareTest                 `json:"test"`
	Results   map[string]ModelCompareModelSummary `json:"results"`
	Winner    ProviderType                      `json:"winner,omitempty"`
	WinReason string                            `json:"win_reason,omitempty"`
}

// ModelCompareModelSummary contains summary metrics for a model
type ModelCompareModelSummary struct {
	Provider    ProviderType `json:"provider"`
	Requests    int          `json:"requests"`
	SuccessRate float64      `json:"success_rate"`
	AvgLatency  float64      `json:"avg_latency_ms"`
	TotalTokens int          `json:"total_tokens"`
	TotalCost   float64      `json:"total_cost"`
}

// ListTests returns all tests
func (r *ModelCompareRunner) ListTests() []*ModelCompareTest {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tests := make([]*ModelCompareTest, 0, len(r.tests))
	for _, t := range r.tests {
		tests = append(tests, t)
	}
	return tests
}

// StopTest stops a running test
func (r *ModelCompareRunner) StopTest(testID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	test, ok := r.tests[testID]
	if !ok {
		return fmt.Errorf("test not found: %s", testID)
	}

	now := time.Now()
	test.EndTime = &now
	test.Status = "completed"
	return nil
}

// FormatAgentSummary formats agent info as markdown
func FormatAgentSummary(agents []SpecializedAgent) string {
	var sb strings.Builder
	sb.WriteString("# Specialized Model Agents\n\n")

	for _, agent := range agents {
		sb.WriteString(fmt.Sprintf("## %s\n\n", agent.Name))
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", agent.Description))
		sb.WriteString(fmt.Sprintf("- **Default Model:** %s\n", agent.DefaultModel))
		sb.WriteString(fmt.Sprintf("- **Task Type:** %s\n", agent.TaskType))
		sb.WriteString(fmt.Sprintf("- **Extended Thinking:** %v\n", agent.UseThinking))
		if agent.ThinkingBudget > 0 {
			sb.WriteString(fmt.Sprintf("- **Thinking Budget:** %d tokens\n", agent.ThinkingBudget))
		}
		sb.WriteString(fmt.Sprintf("- **Max Tokens:** %d\n", agent.MaxTokens))
		sb.WriteString(fmt.Sprintf("- **Temperature:** %.1f\n\n", agent.Temperature))
	}

	return sb.String()
}

// GetAvailableAgents returns list of available specialized agents
func GetAvailableAgents() []SpecializedAgent {
	return []SpecializedAgent{
		{
			Name:           "code-reviewer",
			Description:    "Deep code review with extended thinking for complex analysis",
			DefaultModel:   ProviderClaude,
			TaskType:       TaskCodeReview,
			UseThinking:    true,
			ThinkingBudget: 10000,
			MaxTokens:      8000,
			Temperature:    0.2,
		},
		{
			Name:           "consensus",
			Description:    "Multi-model consensus for complex analysis requiring validation",
			DefaultModel:   ProviderClaude,
			TaskType:       TaskAnalysis,
			UseThinking:    false,
			MaxTokens:      4000,
			Temperature:    0.3,
		},
		{
			Name:           "extractor",
			Description:    "Fast data extraction optimized for high volume and low cost",
			DefaultModel:   ProviderGemini,
			TaskType:       TaskExtraction,
			UseThinking:    false,
			MaxTokens:      2000,
			Temperature:    0.1,
		},
	}
}
