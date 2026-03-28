package session

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
