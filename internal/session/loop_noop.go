package session

import (
	"fmt"
	"sync"
)

// NoOpDetector tracks consecutive no-op iterations (0 files changed, 0 lines added)
// per loop and signals when the loop should skip to the next planner task.
type NoOpDetector struct {
	MaxConsecutiveNoOps int // threshold before triggering skip (default 2)
	counts              map[string]int
	mu                  sync.Mutex
}

// NewNoOpDetector creates a detector with the given threshold.
// If max <= 0, defaults to 2.
func NewNoOpDetector(max int) *NoOpDetector {
	if max <= 0 {
		max = 2
	}
	return &NoOpDetector{
		MaxConsecutiveNoOps: max,
		counts:              make(map[string]int),
	}
}

// RecordIteration records an iteration's output and returns whether the loop
// should skip to the next planner task. A no-op iteration is one where
// filesChanged == 0 && linesAdded == 0.
func (d *NoOpDetector) RecordIteration(loopID string, filesChanged, linesAdded int) (skip bool, reason string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if filesChanged == 0 && linesAdded == 0 {
		d.counts[loopID]++
		if d.counts[loopID] >= d.MaxConsecutiveNoOps {
			return true, fmt.Sprintf("%d consecutive no-op iterations detected, skipping to next planner task", d.counts[loopID])
		}
		return false, ""
	}

	// Productive iteration — reset counter.
	d.counts[loopID] = 0
	return false, ""
}

// Reset clears the consecutive no-op count for a loop.
func (d *NoOpDetector) Reset(loopID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.counts, loopID)
}

// ConsecutiveCount returns the current consecutive no-op count for a loop.
func (d *NoOpDetector) ConsecutiveCount(loopID string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts[loopID]
}
