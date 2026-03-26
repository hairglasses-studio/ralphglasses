package fleet

import (
	"math"
	"sort"
	"sync"
	"time"
)

// FleetAnalytics provides rolling-window metrics for fleet operations.
type FleetAnalytics struct {
	mu              sync.RWMutex
	completions     []completionSample
	failures        []failureSample
	maxSamples      int
	retentionWindow time.Duration
}

type completionSample struct {
	Timestamp  time.Time
	WorkerID   string
	Provider   string
	DurationMs int64
	CostUSD    float64
}

type failureSample struct {
	Timestamp time.Time
	WorkerID  string
	Error     string
}

// AnalyticsSnapshot represents a point-in-time view of fleet metrics.
type AnalyticsSnapshot struct {
	Window            time.Duration
	TotalCompletions  int
	TotalFailures     int
	FailureRate       float64
	LatencyP50Ms      float64
	LatencyP95Ms      float64
	LatencyP99Ms      float64
	TotalCostUSD      float64
	CostPerProvider   map[string]float64
	WorkerUtilization map[string]int // worker ID -> completion count
}

// NewFleetAnalytics creates a new analytics engine with the given sample cap and retention window.
func NewFleetAnalytics(maxSamples int, retention time.Duration) *FleetAnalytics {
	return &FleetAnalytics{
		completions:     make([]completionSample, 0, maxSamples),
		failures:        make([]failureSample, 0, maxSamples),
		maxSamples:      maxSamples,
		retentionWindow: retention,
	}
}

// RecordCompletion records a successful task completion.
func (fa *FleetAnalytics) RecordCompletion(workerID, provider string, duration time.Duration, cost float64) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	fa.completions = append(fa.completions, completionSample{
		Timestamp:  time.Now(),
		WorkerID:   workerID,
		Provider:   provider,
		DurationMs: duration.Milliseconds(),
		CostUSD:    cost,
	})

	// Trim if over capacity.
	if len(fa.completions) > fa.maxSamples {
		excess := len(fa.completions) - fa.maxSamples
		fa.completions = fa.completions[excess:]
	}
}

// RecordFailure records a task failure.
func (fa *FleetAnalytics) RecordFailure(workerID, errMsg string) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	fa.failures = append(fa.failures, failureSample{
		Timestamp: time.Now(),
		WorkerID:  workerID,
		Error:     errMsg,
	})

	// Trim if over capacity.
	if len(fa.failures) > fa.maxSamples {
		excess := len(fa.failures) - fa.maxSamples
		fa.failures = fa.failures[excess:]
	}
}

// recordCompletionAt is an internal helper for testing with explicit timestamps.
func (fa *FleetAnalytics) recordCompletionAt(ts time.Time, workerID, provider string, duration time.Duration, cost float64) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	fa.completions = append(fa.completions, completionSample{
		Timestamp:  ts,
		WorkerID:   workerID,
		Provider:   provider,
		DurationMs: duration.Milliseconds(),
		CostUSD:    cost,
	})

	if len(fa.completions) > fa.maxSamples {
		excess := len(fa.completions) - fa.maxSamples
		fa.completions = fa.completions[excess:]
	}
}

// recordFailureAt is an internal helper for testing with explicit timestamps.
func (fa *FleetAnalytics) recordFailureAt(ts time.Time, workerID, errMsg string) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	fa.failures = append(fa.failures, failureSample{
		Timestamp: ts,
		WorkerID:  workerID,
		Error:     errMsg,
	})

	if len(fa.failures) > fa.maxSamples {
		excess := len(fa.failures) - fa.maxSamples
		fa.failures = fa.failures[excess:]
	}
}

// Snapshot returns a point-in-time view of fleet metrics within the given window.
func (fa *FleetAnalytics) Snapshot(window time.Duration) AnalyticsSnapshot {
	fa.mu.RLock()
	defer fa.mu.RUnlock()

	cutoff := time.Now().Add(-window)

	snap := AnalyticsSnapshot{
		Window:            window,
		CostPerProvider:   make(map[string]float64),
		WorkerUtilization: make(map[string]int),
	}

	var latencies []float64

	for _, c := range fa.completions {
		if c.Timestamp.Before(cutoff) {
			continue
		}
		snap.TotalCompletions++
		snap.TotalCostUSD += c.CostUSD
		snap.CostPerProvider[c.Provider] += c.CostUSD
		snap.WorkerUtilization[c.WorkerID]++
		latencies = append(latencies, float64(c.DurationMs))
	}

	for _, f := range fa.failures {
		if f.Timestamp.Before(cutoff) {
			continue
		}
		snap.TotalFailures++
	}

	total := snap.TotalCompletions + snap.TotalFailures
	if total > 0 {
		snap.FailureRate = float64(snap.TotalFailures) / float64(total)
	}

	if len(latencies) > 0 {
		sort.Float64s(latencies)
		snap.LatencyP50Ms = percentile(latencies, 50)
		snap.LatencyP95Ms = percentile(latencies, 95)
		snap.LatencyP99Ms = percentile(latencies, 99)
	}

	return snap
}

// CostForecast extrapolates cost over the given horizon using the trend from the last hour.
func (fa *FleetAnalytics) CostForecast(horizon time.Duration) float64 {
	fa.mu.RLock()
	defer fa.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-time.Hour)

	var totalCost float64
	var earliest, latest time.Time
	var count int

	for _, c := range fa.completions {
		if c.Timestamp.Before(windowStart) {
			continue
		}
		totalCost += c.CostUSD
		count++
		if earliest.IsZero() || c.Timestamp.Before(earliest) {
			earliest = c.Timestamp
		}
		if latest.IsZero() || c.Timestamp.After(latest) {
			latest = c.Timestamp
		}
	}

	if count < 2 {
		return 0
	}

	span := latest.Sub(earliest)
	if span <= 0 {
		return 0
	}

	// Cost rate per unit time, extrapolated over the horizon.
	rate := totalCost / span.Seconds()
	return math.Round(rate*horizon.Seconds()*100) / 100
}

// percentile returns the p-th percentile from a sorted slice of values.
func percentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
