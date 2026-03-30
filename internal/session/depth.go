package session

import (
	"strings"
)

// Depth estimation constants.
const (
	DefaultDepth = 10 // default estimated iterations
	MinDepth     = 3  // absolute minimum iterations
	MaxDepth     = 50 // absolute maximum iterations

	// Early stop: minimum delta per iteration to consider progress.
	earlyStopThreshold = 0.01
	earlyStopWindow    = 3

	// Extension: minimum delta per iteration to justify extending.
	extensionDeltaThreshold = 0.1
)

// TaskInfo describes a task for depth estimation.
type TaskInfo struct {
	Title           string   // task title/description
	FileCount       int      // number of files to modify
	LOCEstimate     int      // estimated lines of code to change
	DependencyDepth int      // depth of dependency chain
	Tags            []string // optional complexity tags (e.g. "refactor", "test", "docs")
}

// DepthEstimator estimates iteration depth for loop tasks and decides
// whether to extend or early-stop a running loop.
type DepthEstimator struct {
	reflexion *ReflexionStore // optional: historical performance data
}

// NewDepthEstimator creates a DepthEstimator. The reflexion store may be nil.
func NewDepthEstimator(reflexion *ReflexionStore) *DepthEstimator {
	return &DepthEstimator{reflexion: reflexion}
}

// EstimateDepth returns the recommended number of iterations for a task.
// Uses file count, LOC, dependency depth, and historical data when available.
func (d *DepthEstimator) EstimateDepth(task TaskInfo) int {
	depth := DefaultDepth

	// Adjust by file count: more files -> more iterations.
	switch {
	case task.FileCount <= 1:
		depth -= 3 // single-file change: simple
	case task.FileCount <= 5:
		// no adjustment
	case task.FileCount <= 15:
		depth += 5
	default:
		depth += 10
	}

	// Adjust by LOC estimate.
	switch {
	case task.LOCEstimate > 0 && task.LOCEstimate <= 50:
		depth -= 2
	case task.LOCEstimate > 500:
		depth += 5
	case task.LOCEstimate > 200:
		depth += 3
	}

	// Adjust by dependency depth.
	if task.DependencyDepth > 3 {
		depth += task.DependencyDepth - 3
	}

	// Adjust by tag signals.
	for _, tag := range task.Tags {
		switch strings.ToLower(tag) {
		case "docs", "typo", "comment":
			depth -= 3
		case "refactor", "migration":
			depth += 5
		case "test":
			depth -= 1
		}
	}

	// Consult historical data if available.
	if d.reflexion != nil {
		hist := d.historicalDepth(task.Title)
		if hist > 0 {
			// Blend: 60% historical, 40% estimated.
			depth = (hist*6 + depth*4) / 10
		}
	}

	return clampDepth(depth)
}

// ShouldExtend returns true if the loop should continue past its current max.
// progress is in [0, 1]. budgetRemaining is true if budget allows more iterations.
func (d *DepthEstimator) ShouldExtend(currentIter, maxIter int, progress float64, budgetRemaining bool) bool {
	if currentIter < maxIter {
		return false // not at the limit yet
	}
	if currentIter >= MaxDepth {
		return false // absolute ceiling
	}
	if !budgetRemaining {
		return false
	}
	// Only extend if meaningful progress is being made.
	return progress >= extensionDeltaThreshold
}

// ShouldEarlyStop returns true if recent iterations show diminishing returns.
// recentDeltas contains the progress delta for the most recent iterations.
func (d *DepthEstimator) ShouldEarlyStop(currentIter int, recentDeltas []float64) bool {
	if currentIter < MinDepth {
		return false // always run the minimum
	}
	if len(recentDeltas) < earlyStopWindow {
		return false // not enough data
	}

	// Check last earlyStopWindow deltas — all must be below threshold.
	tail := recentDeltas
	if len(tail) > earlyStopWindow {
		tail = tail[len(tail)-earlyStopWindow:]
	}
	for _, delta := range tail {
		if delta >= earlyStopThreshold {
			return false
		}
	}
	return true
}

// historicalDepth looks up average iteration count for similar tasks in the
// reflexion store. Returns 0 if no relevant history exists.
func (d *DepthEstimator) historicalDepth(taskTitle string) int {
	if d.reflexion == nil || taskTitle == "" {
		return 0
	}
	refs := d.reflexion.RecentForTask(taskTitle, 10)
	if len(refs) == 0 {
		return 0
	}
	total := 0
	for _, r := range refs {
		total += r.IterationNum
	}
	return total / len(refs)
}

// clampDepth ensures the depth stays within [MinDepth, MaxDepth].
func clampDepth(d int) int {
	if d < MinDepth {
		return MinDepth
	}
	if d > MaxDepth {
		return MaxDepth
	}
	return d
}
