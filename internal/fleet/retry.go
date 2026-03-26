package fleet

import (
	"math"
	"math/rand"
	"time"
)

// RetryPolicy defines when and how to retry failed work items.
type RetryPolicy struct {
	MaxRetries     int           // maximum retry attempts (default 3)
	BaseDelay      time.Duration // initial backoff delay (default 1s)
	MaxDelay       time.Duration // maximum backoff delay (default 30s)
	Multiplier     float64       // backoff multiplier (default 2.0)
	JitterFraction float64       // fraction of delay to add as jitter (default 0.1)
}

// DefaultRetryPolicy returns sensible defaults.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:     3,
		BaseDelay:      time.Second,
		MaxDelay:       30 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.1,
	}
}

// ShouldRetry returns whether to retry and the delay before the next attempt.
func (p RetryPolicy) ShouldRetry(attempt int) (bool, time.Duration) {
	if attempt >= p.MaxRetries {
		return false, 0
	}
	delay := float64(p.BaseDelay) * math.Pow(p.Multiplier, float64(attempt))
	if delay > float64(p.MaxDelay) {
		delay = float64(p.MaxDelay)
	}
	// Add jitter
	jitter := delay * p.JitterFraction * (rand.Float64()*2 - 1)
	delay += jitter
	if delay < 0 {
		delay = 0
	}
	return true, time.Duration(delay)
}

// RetryTracker tracks retry state for work items.
type RetryTracker struct {
	attempts map[string]int // work ID -> attempt count
	policy   RetryPolicy
}

// NewRetryTracker creates a RetryTracker with the given policy.
func NewRetryTracker(policy RetryPolicy) *RetryTracker {
	return &RetryTracker{
		attempts: make(map[string]int),
		policy:   policy,
	}
}

// RecordFailure increments the attempt count and returns whether to retry.
func (rt *RetryTracker) RecordFailure(workID string) (retry bool, delay time.Duration) {
	rt.attempts[workID]++
	return rt.policy.ShouldRetry(rt.attempts[workID] - 1)
}

// RecordSuccess clears the retry state for a work item.
func (rt *RetryTracker) RecordSuccess(workID string) {
	delete(rt.attempts, workID)
}

// Attempts returns the current attempt count for a work item.
func (rt *RetryTracker) Attempts(workID string) int {
	return rt.attempts[workID]
}
