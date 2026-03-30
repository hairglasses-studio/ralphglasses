package session

import "time"

// activeIterStatuses are the iteration statuses that indicate work is in progress.
// An iteration in any other status (completed, failed, etc.) is not considered stallable.
var activeIterStatuses = map[string]bool{
	"running":   true,
	"planning":  true,
	"executing": true,
	"verifying": true,
}

// LoopStallDetector monitors loop iterations for stalls (stuck sessions).
// It is designed to be called synchronously at step boundaries — no goroutines.
type LoopStallDetector struct {
	timeout    time.Duration
	checkEvery time.Duration
	onStall    func(run *LoopRun, iter *LoopIteration)
	now        func() time.Time // injectable clock for tests
}

// NewLoopStallDetector creates a detector with the given timeout and stall callback.
// A zero timeout disables detection. Default checkEvery = timeout/3 (min 30s).
func NewLoopStallDetector(timeout time.Duration, onStall func(*LoopRun, *LoopIteration)) *LoopStallDetector {
	checkEvery := timeout / 3
	if checkEvery < 30*time.Second && timeout > 0 {
		checkEvery = 30 * time.Second
	}
	return &LoopStallDetector{
		timeout:    timeout,
		checkEvery: checkEvery,
		onStall:    onStall,
		now:        time.Now,
	}
}

// CheckIteration returns true if the iteration is stalled. An iteration is
// stalled when it has an active status and StartedAt plus timeout has elapsed.
// A zero timeout always returns false (detection disabled).
func (d *LoopStallDetector) CheckIteration(run *LoopRun, iter *LoopIteration) bool {
	if d.timeout <= 0 {
		return false
	}
	if !activeIterStatuses[iter.Status] {
		return false
	}
	if iter.StartedAt.IsZero() {
		return false
	}
	return d.now().Sub(iter.StartedAt) > d.timeout
}

// CheckRun checks all iterations in a run and returns indices of stalled ones.
// For each stalled iteration, the onStall callback is invoked (if set).
func (d *LoopStallDetector) CheckRun(run *LoopRun) []int {
	if d.timeout <= 0 {
		return nil
	}
	var stalled []int
	for i := range run.Iterations {
		if d.CheckIteration(run, &run.Iterations[i]) {
			stalled = append(stalled, i)
			if d.onStall != nil {
				d.onStall(run, &run.Iterations[i])
			}
		}
	}
	return stalled
}
