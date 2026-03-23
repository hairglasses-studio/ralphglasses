package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// HITLMetricType categorizes the source of a metric.
type HITLMetricType string

const (
	// Category A: Intervention Metrics (direct human actions)
	MetricManualSessionStop   HITLMetricType = "manual_session_stop"
	MetricManualSessionLaunch HITLMetricType = "manual_session_launch"
	MetricManualConfigChange  HITLMetricType = "manual_config_change"
	MetricManualRestart       HITLMetricType = "manual_restart"
	MetricManualBudgetAdjust  HITLMetricType = "manual_budget_adjust"

	// Category B: Error Recovery (system self-healing)
	MetricAutoRecovery       HITLMetricType = "auto_recovery"
	MetricCircuitBreakerReset HITLMetricType = "circuit_breaker_reset"
	MetricProviderFailover   HITLMetricType = "provider_failover"

	// Category C: Decision Quality
	MetricAutoDecisionSuccess HITLMetricType = "auto_decision_success"
	MetricAutoDecisionOverride HITLMetricType = "auto_decision_override"
	MetricSessionCompleted    HITLMetricType = "session_completed"
	MetricSessionErrored      HITLMetricType = "session_errored"
)

// Trigger indicates whether an action was human-initiated or autonomous.
type Trigger string

const (
	TriggerManual    Trigger = "manual"    // human via TUI/MCP
	TriggerAutomatic Trigger = "automatic" // system decision
	TriggerScheduled Trigger = "scheduled" // timer/cron
)

// HITLEvent records a single countable action for HITL scoring.
type HITLEvent struct {
	Timestamp  time.Time      `json:"ts"`
	MetricType HITLMetricType `json:"metric"`
	Trigger    Trigger        `json:"trigger"`
	SessionID  string         `json:"session_id,omitempty"`
	RepoName   string         `json:"repo_name,omitempty"`
	Details    string         `json:"details,omitempty"`
}

// HITLSnapshot is a periodic summary of HITL metrics.
type HITLSnapshot struct {
	Timestamp             time.Time `json:"ts"`
	PeriodHours           float64   `json:"period_hours"`
	TotalActions          int       `json:"total_actions"`
	ManualInterventions   int       `json:"manual_interventions"`
	AutoActions           int       `json:"auto_actions"`
	HITLScore             float64   `json:"hitl_score"`       // manual / total * 100
	AutoRecoveryRate      float64   `json:"auto_recovery_rate"` // auto-recovered / total errors
	SessionCompletionRate float64   `json:"session_completion_rate"`
	CostEfficiency        float64   `json:"cost_efficiency"`  // completed tasks / total USD
	Trend                 string    `json:"trend"`            // "improving", "stable", "degrading"
}

// HITLTracker records and computes HITL metrics.
type HITLTracker struct {
	mu     sync.Mutex
	events []HITLEvent
	stateDir string
}

// NewHITLTracker creates a tracker that persists to the given directory.
func NewHITLTracker(stateDir string) *HITLTracker {
	t := &HITLTracker{
		stateDir: stateDir,
	}
	t.load()
	return t
}

// Record adds a HITL event.
func (t *HITLTracker) Record(event HITLEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	t.mu.Lock()
	t.events = append(t.events, event)
	t.mu.Unlock()

	// Append to file
	t.appendToFile(event)
}

// RecordManual is a convenience method for recording a manual human action.
func (t *HITLTracker) RecordManual(metric HITLMetricType, sessionID, repoName, details string) {
	t.Record(HITLEvent{
		MetricType: metric,
		Trigger:    TriggerManual,
		SessionID:  sessionID,
		RepoName:   repoName,
		Details:    details,
	})
}

// RecordAuto is a convenience method for recording an autonomous action.
func (t *HITLTracker) RecordAuto(metric HITLMetricType, sessionID, repoName, details string) {
	t.Record(HITLEvent{
		MetricType: metric,
		Trigger:    TriggerAutomatic,
		SessionID:  sessionID,
		RepoName:   repoName,
		Details:    details,
	})
}

// CurrentScore computes the current HITL score over the given time window.
func (t *HITLTracker) CurrentScore(window time.Duration) HITLSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-window)
	var (
		total, manual, auto int
		completed, errored  int
		autoRecovered       int
		totalErrors         int
		totalSpend          float64
	)

	for _, e := range t.events {
		if e.Timestamp.Before(cutoff) {
			continue
		}
		total++
		switch e.Trigger {
		case TriggerManual:
			manual++
		case TriggerAutomatic:
			auto++
		}

		switch e.MetricType {
		case MetricSessionCompleted:
			completed++
		case MetricSessionErrored:
			errored++
			totalErrors++
		case MetricAutoRecovery:
			autoRecovered++
		case MetricManualRestart:
			totalErrors++
		}
	}

	snap := HITLSnapshot{
		Timestamp:           time.Now(),
		PeriodHours:         window.Hours(),
		TotalActions:        total,
		ManualInterventions: manual,
		AutoActions:         auto,
	}

	if total > 0 {
		snap.HITLScore = float64(manual) / float64(total) * 100
	}
	if totalErrors > 0 {
		snap.AutoRecoveryRate = float64(autoRecovered) / float64(totalErrors) * 100
	}
	totalSessions := completed + errored
	if totalSessions > 0 {
		snap.SessionCompletionRate = float64(completed) / float64(totalSessions) * 100
	}
	if totalSpend > 0 {
		snap.CostEfficiency = float64(completed) / totalSpend
	}

	// Compute trend by comparing current vs previous window
	snap.Trend = t.computeTrend(cutoff, window)

	return snap
}

// History returns recent HITL events within the given window.
func (t *HITLTracker) History(window time.Duration, limit int) []HITLEvent {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-window)
	var result []HITLEvent
	for i := len(t.events) - 1; i >= 0 && len(result) < limit; i-- {
		if t.events[i].Timestamp.After(cutoff) {
			result = append(result, t.events[i])
		}
	}

	// Reverse to chronological
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

func (t *HITLTracker) computeTrend(cutoff time.Time, window time.Duration) string {
	prevCutoff := cutoff.Add(-window)

	var currManual, currTotal, prevManual, prevTotal int
	for _, e := range t.events {
		if e.Timestamp.After(cutoff) {
			currTotal++
			if e.Trigger == TriggerManual {
				currManual++
			}
		} else if e.Timestamp.After(prevCutoff) {
			prevTotal++
			if e.Trigger == TriggerManual {
				prevManual++
			}
		}
	}

	if currTotal == 0 || prevTotal == 0 {
		return "insufficient_data"
	}

	currRate := float64(currManual) / float64(currTotal)
	prevRate := float64(prevManual) / float64(prevTotal)
	diff := currRate - prevRate

	switch {
	case diff < -0.05:
		return "improving"
	case diff > 0.05:
		return "degrading"
	default:
		return "stable"
	}
}

func (t *HITLTracker) appendToFile(event HITLEvent) {
	if t.stateDir == "" {
		return
	}
	_ = os.MkdirAll(t.stateDir, 0755)

	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	data = append(data, '\n')

	path := filepath.Join(t.stateDir, "hitl_events.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(data)
}

func (t *HITLTracker) load() {
	if t.stateDir == "" {
		return
	}
	path := filepath.Join(t.stateDir, "hitl_events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var events []HITLEvent
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var e HITLEvent
		if json.Unmarshal(line, &e) == nil {
			events = append(events, e)
		}
	}

	t.mu.Lock()
	t.events = events
	t.mu.Unlock()
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
