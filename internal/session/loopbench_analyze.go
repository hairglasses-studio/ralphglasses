package session

import (
	"sort"
	"time"
)

// LoopVelocity returns useful iterations per hour within the given window.
// An iteration is "useful" if verification passed and files were changed.
func LoopVelocity(observations []LoopObservation, windowHours float64) float64 {
	if windowHours <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-time.Duration(windowHours * float64(time.Hour)))
	useful := 0
	for _, obs := range observations {
		if obs.Timestamp.After(cutoff) && obs.VerifyPassed && obs.FilesChanged > 0 {
			useful++
		}
	}
	return float64(useful) / windowHours
}

// ObservationSummary provides rolling statistics over a time window.
type ObservationSummary struct {
	WindowHours     float64            `json:"window_hours"`
	TotalIterations int                `json:"total_iterations"`
	CompletionRate  float64            `json:"completion_rate"`
	AvgCostPerIter  float64            `json:"avg_cost_per_iter"`
	CostTrend       string             `json:"cost_trend"`      // "decreasing", "stable", "increasing"
	EfficiencyScore float64            `json:"efficiency_score"` // completions per dollar
	CostByProvider  map[string]float64 `json:"cost_by_provider"`
	Velocity        float64            `json:"velocity"` // useful iterations per hour
}

// AggregateObservations computes rolling statistics from loop observations.
func AggregateObservations(observations []LoopObservation, windowHours float64) *ObservationSummary {
	if windowHours <= 0 || len(observations) == 0 {
		return &ObservationSummary{WindowHours: windowHours, CostByProvider: map[string]float64{}}
	}

	cutoff := time.Now().Add(-time.Duration(windowHours * float64(time.Hour)))
	prevCutoff := cutoff.Add(-time.Duration(windowHours * float64(time.Hour)))

	var current, previous []LoopObservation
	for _, obs := range observations {
		if obs.Timestamp.After(cutoff) {
			current = append(current, obs)
		} else if obs.Timestamp.After(prevCutoff) {
			previous = append(previous, obs)
		}
	}

	summary := &ObservationSummary{
		WindowHours:     windowHours,
		TotalIterations: len(current),
		CostByProvider:  make(map[string]float64),
	}

	if len(current) == 0 {
		return summary
	}

	var totalCost float64
	completed := 0
	for _, obs := range current {
		totalCost += obs.TotalCostUSD
		if obs.Status == "idle" {
			completed++
		}
		if obs.PlannerProvider != "" {
			summary.CostByProvider[obs.PlannerProvider] += obs.PlannerCostUSD
		}
		if obs.WorkerProvider != "" {
			summary.CostByProvider[obs.WorkerProvider] += obs.WorkerCostUSD
		}
	}

	summary.CompletionRate = float64(completed) / float64(len(current))
	summary.AvgCostPerIter = totalCost / float64(len(current))

	if totalCost > 0 {
		summary.EfficiencyScore = float64(completed) / totalCost
	}

	summary.Velocity = LoopVelocity(current, windowHours)

	// Compute cost trend by comparing current window to previous window
	if len(previous) > 0 {
		var prevCost float64
		for _, obs := range previous {
			prevCost += obs.TotalCostUSD
		}
		prevAvg := prevCost / float64(len(previous))
		curAvg := summary.AvgCostPerIter

		ratio := curAvg / prevAvg
		switch {
		case ratio < 0.85:
			summary.CostTrend = "decreasing"
		case ratio > 1.15:
			summary.CostTrend = "increasing"
		default:
			summary.CostTrend = "stable"
		}
	} else {
		summary.CostTrend = "stable"
	}

	return summary
}

// IterationSummary aggregates statistics across multiple observations.
type IterationSummary struct {
	TotalIterations   int            `json:"total_iterations"`
	CompletedCount    int            `json:"completed_count"`
	FailedCount       int            `json:"failed_count"`
	TotalStalls       int            `json:"total_stalls"`
	AvgDurationSec    float64        `json:"avg_duration_sec"`
	TotalFilesChanged int            `json:"total_files_changed"`
	TotalInsertions   int            `json:"total_insertions"`
	TotalDeletions    int            `json:"total_deletions"`
	AcceptanceCounts  map[string]int `json:"acceptance_counts"` // "auto_merge" -> N, etc.
	ModelUsage        map[string]int `json:"model_usage"`       // model ID -> count

	// Latency percentiles (seconds).
	LatencyP50 float64 `json:"latency_p50"`
	LatencyP95 float64 `json:"latency_p95"`
	LatencyP99 float64 `json:"latency_p99"`

	// Cost percentiles (USD).
	CostP50 float64 `json:"cost_p50"`
	CostP95 float64 `json:"cost_p95"`
	CostP99 float64 `json:"cost_p99"`
}

// SummarizeObservations computes aggregate statistics from a slice of observations.
func SummarizeObservations(obs []LoopObservation) IterationSummary {
	s := IterationSummary{
		AcceptanceCounts: make(map[string]int),
		ModelUsage:       make(map[string]int),
	}
	if len(obs) == 0 {
		return s
	}

	s.TotalIterations = len(obs)
	var totalDurationMs int64
	latencies := make([]float64, 0, len(obs))
	costs := make([]float64, 0, len(obs))
	for _, o := range obs {
		// Status accounting
		switch o.Status {
		case "idle":
			s.CompletedCount++
		case "failed":
			s.FailedCount++
		}

		// Stall accounting
		s.TotalStalls += o.StallCount

		// Duration
		totalDurationMs += o.TotalLatencyMs
		latencies = append(latencies, float64(o.TotalLatencyMs)/1000.0)
		costs = append(costs, o.TotalCostUSD)

		// Diff stats from DiffStat if present, otherwise from flat fields
		if o.GitDiffStat != nil {
			s.TotalFilesChanged += o.GitDiffStat.FilesChanged
			s.TotalInsertions += o.GitDiffStat.Insertions
			s.TotalDeletions += o.GitDiffStat.Deletions
		} else {
			s.TotalFilesChanged += o.FilesChanged
			s.TotalInsertions += o.LinesAdded
			s.TotalDeletions += o.LinesRemoved
		}

		// Acceptance path
		if o.AcceptancePath != "" {
			s.AcceptanceCounts[o.AcceptancePath]++
		}

		// Model usage
		if o.PlannerModelUsed != "" {
			s.ModelUsage[o.PlannerModelUsed]++
		}
		if o.WorkerModelUsed != "" {
			s.ModelUsage[o.WorkerModelUsed]++
		}
	}

	s.AvgDurationSec = float64(totalDurationMs) / float64(len(obs)) / 1000.0

	// Compute latency percentiles (seconds).
	sort.Float64s(latencies)
	s.LatencyP50 = percentile(latencies, 50)
	s.LatencyP95 = percentile(latencies, 95)
	s.LatencyP99 = percentile(latencies, 99)

	// Compute cost percentiles (USD).
	sort.Float64s(costs)
	s.CostP50 = percentile(costs, 50)
	s.CostP95 = percentile(costs, 95)
	s.CostP99 = percentile(costs, 99)

	return s
}
