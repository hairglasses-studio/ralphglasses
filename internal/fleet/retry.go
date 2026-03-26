package fleet

import (
	"sync"
)

// RetryPolicy defines the retry behavior for work items.
type RetryPolicy struct {
	MaxRetries int
}

// DefaultRetryPolicy returns sensible retry defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 2,
	}
}

// RetryTracker tracks retry state for work items.
type RetryTracker struct {
	mu       sync.Mutex
	policy   RetryPolicy
	failures map[string]int // workID -> failure count
}

// NewRetryTracker creates a RetryTracker with the given policy.
func NewRetryTracker(policy RetryPolicy) *RetryTracker {
	return &RetryTracker{
		policy:   policy,
		failures: make(map[string]int),
	}
}

// RecordSuccess marks a work item as successfully completed and cleans up tracking.
func (rt *RetryTracker) RecordSuccess(workID string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.failures, workID)
}

// RecordFailure records a failure for a work item and returns whether it is retryable.
func (rt *RetryTracker) RecordFailure(workID string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.failures[workID]++
	return rt.failures[workID] <= rt.policy.MaxRetries
}

// Failures returns the current failure count for a work item.
func (rt *RetryTracker) Failures(workID string) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.failures[workID]
}

// Policy returns the current retry policy.
func (rt *RetryTracker) Policy() RetryPolicy {
	return rt.policy
}
