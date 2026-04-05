package chains

import (
	"sort"
	"sync"
	"time"
)

// MetricsCollector tracks chain execution metrics
type MetricsCollector struct {
	mu            sync.RWMutex
	chainMetrics  map[string]*ChainMetrics
	stepMetrics   map[string]map[string]*StepMetrics // chain -> step -> metrics
	recentExecutions []ExecutionSummary
	maxRecent     int
}

// ChainMetrics holds metrics for a specific chain
type ChainMetrics struct {
	ChainName        string        `json:"chain_name"`
	TotalExecutions  int64         `json:"total_executions"`
	SuccessCount     int64         `json:"success_count"`
	FailureCount     int64         `json:"failure_count"`
	CancelledCount   int64         `json:"cancelled_count"`
	TotalDuration    time.Duration `json:"-"`
	AvgDurationMs    float64       `json:"avg_duration_ms"`
	MinDurationMs    float64       `json:"min_duration_ms"`
	MaxDurationMs    float64       `json:"max_duration_ms"`
	LastExecutedAt   *time.Time    `json:"last_executed_at,omitempty"`
	LastSuccessAt    *time.Time    `json:"last_success_at,omitempty"`
	LastFailureAt    *time.Time    `json:"last_failure_at,omitempty"`
	ConsecutiveFails int           `json:"consecutive_fails"`
}

// StepMetrics holds metrics for a specific step
type StepMetrics struct {
	StepID          string        `json:"step_id"`
	TotalExecutions int64         `json:"total_executions"`
	SuccessCount    int64         `json:"success_count"`
	FailureCount    int64         `json:"failure_count"`
	RetryCount      int64         `json:"retry_count"`
	TotalDuration   time.Duration `json:"-"`
	AvgDurationMs   float64       `json:"avg_duration_ms"`
}

// ExecutionSummary is a brief summary of an execution
type ExecutionSummary struct {
	ExecutionID  string        `json:"execution_id"`
	ChainName    string        `json:"chain_name"`
	Status       string        `json:"status"`
	StartedAt    time.Time     `json:"started_at"`
	Duration     time.Duration `json:"duration"`
	TriggeredBy  string        `json:"triggered_by"`
	StepsRun     int           `json:"steps_run"`
	Error        string        `json:"error,omitempty"`
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		chainMetrics: make(map[string]*ChainMetrics),
		stepMetrics:  make(map[string]map[string]*StepMetrics),
		maxRecent:    100,
	}
}

// RecordExecution records metrics for a completed chain execution
func (m *MetricsCollector) RecordExecution(exec *ChainExecution) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create chain metrics
	cm, exists := m.chainMetrics[exec.ChainName]
	if !exists {
		cm = &ChainMetrics{
			ChainName:     exec.ChainName,
			MinDurationMs: -1, // Initialize to -1 to detect first execution
		}
		m.chainMetrics[exec.ChainName] = cm
	}

	// Calculate duration
	var duration time.Duration
	if exec.CompletedAt != nil {
		duration = exec.CompletedAt.Sub(exec.StartedAt)
	}

	// Update chain metrics
	cm.TotalExecutions++
	cm.TotalDuration += duration
	now := time.Now()
	cm.LastExecutedAt = &now

	durationMs := float64(duration.Milliseconds())

	switch exec.Status {
	case StatusCompleted:
		cm.SuccessCount++
		cm.LastSuccessAt = &now
		cm.ConsecutiveFails = 0
	case StatusFailed:
		cm.FailureCount++
		cm.LastFailureAt = &now
		cm.ConsecutiveFails++
	case StatusCancelled:
		cm.CancelledCount++
	}

	// Update duration stats
	if cm.MinDurationMs < 0 || durationMs < cm.MinDurationMs {
		cm.MinDurationMs = durationMs
	}
	if durationMs > cm.MaxDurationMs {
		cm.MaxDurationMs = durationMs
	}
	cm.AvgDurationMs = float64(cm.TotalDuration.Milliseconds()) / float64(cm.TotalExecutions)

	// Record step metrics
	for stepID, stepResult := range exec.StepResults {
		m.recordStepMetrics(exec.ChainName, stepID, &stepResult)
	}

	// Add to recent executions
	summary := ExecutionSummary{
		ExecutionID: exec.ID,
		ChainName:   exec.ChainName,
		Status:      string(exec.Status),
		StartedAt:   exec.StartedAt,
		Duration:    duration,
		TriggeredBy: exec.TriggeredBy,
		StepsRun:    len(exec.StepResults),
		Error:       exec.Error,
	}

	m.recentExecutions = append([]ExecutionSummary{summary}, m.recentExecutions...)
	if len(m.recentExecutions) > m.maxRecent {
		m.recentExecutions = m.recentExecutions[:m.maxRecent]
	}
}

func (m *MetricsCollector) recordStepMetrics(chainName, stepID string, result *StepResult) {
	if _, exists := m.stepMetrics[chainName]; !exists {
		m.stepMetrics[chainName] = make(map[string]*StepMetrics)
	}

	sm, exists := m.stepMetrics[chainName][stepID]
	if !exists {
		sm = &StepMetrics{StepID: stepID}
		m.stepMetrics[chainName][stepID] = sm
	}

	sm.TotalExecutions++
	if result.Status == StatusCompleted {
		sm.SuccessCount++
	} else if result.Status == StatusFailed {
		sm.FailureCount++
	}

	if result.Attempts > 1 {
		sm.RetryCount += int64(result.Attempts - 1)
	}

	if result.CompletedAt != nil {
		duration := result.CompletedAt.Sub(result.StartedAt)
		sm.TotalDuration += duration
		sm.AvgDurationMs = float64(sm.TotalDuration.Milliseconds()) / float64(sm.TotalExecutions)
	}
}

// GetChainMetrics returns metrics for a specific chain
func (m *MetricsCollector) GetChainMetrics(chainName string) *ChainMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if cm, exists := m.chainMetrics[chainName]; exists {
		// Return a copy
		copy := *cm
		return &copy
	}
	return nil
}

// GetAllChainMetrics returns metrics for all chains
func (m *MetricsCollector) GetAllChainMetrics() []*ChainMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ChainMetrics, 0, len(m.chainMetrics))
	for _, cm := range m.chainMetrics {
		copy := *cm
		result = append(result, &copy)
	}

	// Sort by total executions descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalExecutions > result[j].TotalExecutions
	})

	return result
}

// GetStepMetrics returns metrics for steps in a chain
func (m *MetricsCollector) GetStepMetrics(chainName string) []*StepMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if steps, exists := m.stepMetrics[chainName]; exists {
		result := make([]*StepMetrics, 0, len(steps))
		for _, sm := range steps {
			copy := *sm
			result = append(result, &copy)
		}
		return result
	}
	return nil
}

// GetRecentExecutions returns recent execution summaries
func (m *MetricsCollector) GetRecentExecutions(limit int) []ExecutionSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if limit <= 0 || limit > len(m.recentExecutions) {
		limit = len(m.recentExecutions)
	}

	result := make([]ExecutionSummary, limit)
	copy(result, m.recentExecutions[:limit])
	return result
}

// GetSummary returns an overall metrics summary
func (m *MetricsCollector) GetSummary() MetricsSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := MetricsSummary{
		TotalChains: len(m.chainMetrics),
	}

	for _, cm := range m.chainMetrics {
		summary.TotalExecutions += cm.TotalExecutions
		summary.TotalSuccesses += cm.SuccessCount
		summary.TotalFailures += cm.FailureCount

		if cm.ConsecutiveFails >= 3 {
			summary.ChainsWithIssues++
		}
	}

	if summary.TotalExecutions > 0 {
		summary.SuccessRate = float64(summary.TotalSuccesses) / float64(summary.TotalExecutions) * 100
	}

	return summary
}

// MetricsSummary provides an overall metrics summary
type MetricsSummary struct {
	TotalChains      int     `json:"total_chains"`
	TotalExecutions  int64   `json:"total_executions"`
	TotalSuccesses   int64   `json:"total_successes"`
	TotalFailures    int64   `json:"total_failures"`
	SuccessRate      float64 `json:"success_rate"`
	ChainsWithIssues int     `json:"chains_with_issues"`
}

// Reset clears all metrics
func (m *MetricsCollector) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.chainMetrics = make(map[string]*ChainMetrics)
	m.stepMetrics = make(map[string]map[string]*StepMetrics)
	m.recentExecutions = nil
}
