package promptdj

import (
	"sync"
	"time"
)

// MetricsCollector aggregates Prompt DJ routing decision statistics.
type MetricsCollector struct {
	mu       sync.Mutex
	counters MetricsCounters
	window   []metricEvent
	maxWindow int
}

// MetricsCounters holds aggregate counters.
type MetricsCounters struct {
	TotalDecisions   int64              `json:"total_decisions"`
	TotalDispatches  int64              `json:"total_dispatches"`
	TotalFeedback    int64              `json:"total_feedback"`
	Successes        int64              `json:"successes"`
	Failures         int64              `json:"failures"`
	TotalCostUSD     float64            `json:"total_cost_usd"`
	EnhancedCount    int64              `json:"enhanced_count"`
	ByProvider       map[string]int64   `json:"by_provider"`
	ByTaskType       map[string]int64   `json:"by_task_type"`
	ByConfidence     map[string]int64   `json:"by_confidence"` // high/medium/low
	AvgConfidence    float64            `json:"avg_confidence"`
	AvgScore         float64            `json:"avg_score"`
	AvgLatencyMs     float64            `json:"avg_latency_ms"`
}

type metricEvent struct {
	Timestamp  time.Time
	Provider   string
	TaskType   string
	Score      int
	Confidence float64
	LatencyMs  int64
	Enhanced   bool
	Success    *bool // nil = no feedback yet
	CostUSD    float64
}

// NewMetricsCollector creates a collector with a sliding window.
func NewMetricsCollector(maxWindow int) *MetricsCollector {
	if maxWindow <= 0 {
		maxWindow = 1000
	}
	return &MetricsCollector{
		maxWindow: maxWindow,
		counters: MetricsCounters{
			ByProvider:   make(map[string]int64),
			ByTaskType:   make(map[string]int64),
			ByConfidence: make(map[string]int64),
		},
	}
}

// RecordDecision records a routing decision event.
func (m *MetricsCollector) RecordDecision(d *RoutingDecision) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counters.TotalDecisions++
	m.counters.ByProvider[string(d.Provider)]++
	m.counters.ByTaskType[string(d.TaskType)]++
	m.counters.ByConfidence[d.ConfidenceLevel]++

	if d.WasEnhanced {
		m.counters.EnhancedCount++
	}

	// Rolling averages
	n := float64(m.counters.TotalDecisions)
	m.counters.AvgConfidence += (d.Confidence - m.counters.AvgConfidence) / n
	m.counters.AvgScore += (float64(d.OriginalScore) - m.counters.AvgScore) / n
	m.counters.AvgLatencyMs += (float64(d.LatencyMs) - m.counters.AvgLatencyMs) / n

	m.window = append(m.window, metricEvent{
		Timestamp:  d.Timestamp,
		Provider:   string(d.Provider),
		TaskType:   string(d.TaskType),
		Score:      d.OriginalScore,
		Confidence: d.Confidence,
		LatencyMs:  d.LatencyMs,
		Enhanced:   d.WasEnhanced,
	})
	if len(m.window) > m.maxWindow {
		m.window = m.window[len(m.window)-m.maxWindow:]
	}
}

// RecordOutcome records feedback for a decision.
func (m *MetricsCollector) RecordOutcome(success bool, costUSD float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counters.TotalFeedback++
	m.counters.TotalCostUSD += costUSD
	if success {
		m.counters.Successes++
	} else {
		m.counters.Failures++
	}
}

// Snapshot returns a copy of current metrics.
func (m *MetricsCollector) Snapshot() MetricsCounters {
	m.mu.Lock()
	defer m.mu.Unlock()

	snap := m.counters
	snap.ByProvider = copyMap(m.counters.ByProvider)
	snap.ByTaskType = copyMap(m.counters.ByTaskType)
	snap.ByConfidence = copyMap(m.counters.ByConfidence)
	return snap
}

// SuccessRate returns the ratio of successes to total feedback.
func (m *MetricsCollector) SuccessRate() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counters.TotalFeedback == 0 {
		return 0
	}
	return float64(m.counters.Successes) / float64(m.counters.TotalFeedback)
}

func copyMap(src map[string]int64) map[string]int64 {
	dst := make(map[string]int64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
