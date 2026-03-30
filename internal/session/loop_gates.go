package session

import "fmt"

// LoopBaseline captures the expected performance characteristics of a loop,
// derived from historical observations. Used as a regression gate — if a new
// iteration deviates significantly from the baseline, the loop can pause for
// human review.
type LoopBaseline struct {
	AvgPlannerLatencyMs int64   `json:"avg_planner_latency_ms"`
	AvgWorkerLatencyMs  int64   `json:"avg_worker_latency_ms"`
	AvgTotalLatencyMs   int64   `json:"avg_total_latency_ms"`
	AvgTotalCostUSD     float64 `json:"avg_total_cost_usd"`
	AvgFilesChanged     int     `json:"avg_files_changed"`
	AvgLinesAdded       int     `json:"avg_lines_added"`
	SampleCount         int     `json:"sample_count"`
}

// IsZero reports whether the baseline was zero-initialized (no real data).
// A valid baseline always has SampleCount > 0.
func (b *LoopBaseline) IsZero() bool {
	return b == nil || b.SampleCount == 0
}

// BaselineStatus describes the state of a baseline check.
type BaselineStatus string

const (
	BaselineReady      BaselineStatus = "ready"       // baseline exists with real data
	BaselineNotYet     BaselineStatus = "no_baseline"  // no baseline established yet
	BaselineZeroInit   BaselineStatus = "zero_init"    // baseline exists but has zero sample count
)

// BaselineCheck holds the result of checking whether a baseline is usable.
type BaselineCheck struct {
	Status   BaselineStatus `json:"status"`
	Message  string         `json:"message"`
	Baseline *LoopBaseline  `json:"baseline,omitempty"`
}

// CheckBaseline validates that a baseline is usable for gate comparisons.
// Returns a clear status when baseline is nil, zero-initialized, or ready.
func CheckBaseline(baseline *LoopBaseline) BaselineCheck {
	if baseline == nil {
		return BaselineCheck{
			Status:  BaselineNotYet,
			Message: "no baseline established — run at least one observation before gate checks produce meaningful deltas",
		}
	}
	if baseline.SampleCount == 0 {
		return BaselineCheck{
			Status:  BaselineZeroInit,
			Message: "baseline has zero sample count — likely zero-initialized, not populated from real observations",
		}
	}
	return BaselineCheck{
		Status:   BaselineReady,
		Message:  fmt.Sprintf("baseline ready with %d samples", baseline.SampleCount),
		Baseline: baseline,
	}
}

// BaselineDelta holds the difference between an observation and a baseline.
type BaselineDelta struct {
	CostDelta    float64 `json:"cost_delta_usd"`
	LatencyDelta int64   `json:"latency_delta_ms"`
	FilesDelta   int     `json:"files_delta"`
	LinesDelta   int     `json:"lines_delta"`
	Valid        bool    `json:"valid"` // false when baseline was nil/zero
}

// ComputeDelta computes the delta between an observation and a baseline.
// Returns a delta with Valid=false if the baseline is nil or zero-initialized,
// preventing meaningless zero-deltas (QW-6).
func ComputeDelta(obs LoopObservation, baseline *LoopBaseline) BaselineDelta {
	if baseline.IsZero() {
		return BaselineDelta{Valid: false}
	}
	return BaselineDelta{
		CostDelta:    obs.TotalCostUSD - baseline.AvgTotalCostUSD,
		LatencyDelta: obs.TotalLatencyMs - baseline.AvgTotalLatencyMs,
		FilesDelta:   obs.FilesChanged - baseline.AvgFilesChanged,
		LinesDelta:   obs.LinesAdded - baseline.AvgLinesAdded,
		Valid:        true,
	}
}

// EnsureBaseline returns an existing baseline if usable, or initializes one
// from the provided observations. isNew is true when a new baseline was just
// created from the first real observation (cycle 1) — callers MUST skip gate
// evaluation in this case since there is no prior reference point to compare
// against; evaluating on cycle 1 produces trivially-passing relative gates
// (ratio ≈ 1.0) because the baseline equals the current observations.
//
// The save function is called only when a new baseline is created; errors are
// propagated (not swallowed).
func EnsureBaseline(existing *LoopBaseline, observations []LoopObservation, save func(*LoopBaseline) error) (baseline *LoopBaseline, isNew bool, err error) {
	if !existing.IsZero() {
		return existing, false, nil
	}
	baseline = InitBaselineFromFirstObservation(observations)
	if baseline == nil {
		return nil, false, nil // no observations available
	}
	if save != nil {
		if err := save(baseline); err != nil {
			return nil, false, fmt.Errorf("save initial baseline: %w", err)
		}
	}
	return baseline, true, nil
}

// InitBaselineFromFirstObservation creates a baseline from the first loop
// observation when no explicit baseline exists. Returns nil if no observations.
func InitBaselineFromFirstObservation(observations []LoopObservation) *LoopBaseline {
	if len(observations) == 0 {
		return nil
	}
	first := observations[0]
	return &LoopBaseline{
		AvgPlannerLatencyMs: first.PlannerLatencyMs,
		AvgWorkerLatencyMs:  first.WorkerLatencyMs,
		AvgTotalLatencyMs:   first.TotalLatencyMs,
		AvgTotalCostUSD:     first.TotalCostUSD,
		AvgFilesChanged:     first.FilesChanged,
		AvgLinesAdded:       first.LinesAdded,
		SampleCount:         1,
	}
}

// BaselineFromObservations computes a baseline by averaging all provided
// observations. Returns nil if the slice is empty.
func BaselineFromObservations(observations []LoopObservation) *LoopBaseline {
	if len(observations) == 0 {
		return nil
	}
	var b LoopBaseline
	for _, o := range observations {
		b.AvgPlannerLatencyMs += o.PlannerLatencyMs
		b.AvgWorkerLatencyMs += o.WorkerLatencyMs
		b.AvgTotalLatencyMs += o.TotalLatencyMs
		b.AvgTotalCostUSD += o.TotalCostUSD
		b.AvgFilesChanged += o.FilesChanged
		b.AvgLinesAdded += o.LinesAdded
	}
	n := int64(len(observations))
	b.AvgPlannerLatencyMs /= n
	b.AvgWorkerLatencyMs /= n
	b.AvgTotalLatencyMs /= n
	b.AvgTotalCostUSD /= float64(n)
	b.AvgFilesChanged /= int(n)
	b.AvgLinesAdded /= int(n)
	b.SampleCount = len(observations)
	return &b
}
