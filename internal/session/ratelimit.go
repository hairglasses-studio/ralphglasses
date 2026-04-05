package session

import (
	"fmt"
	"maps"
	"sync"
	"time"
)

// ProviderRateLimits holds default max requests per minute per provider.
var ProviderRateLimits = map[Provider]int{
	ProviderClaude: 50,
	ProviderGemini: 60,
	ProviderCodex:  20,
}

// RateLimiter enforces per-provider request rate limits using a sliding
// 1-minute window. Safe for concurrent use.
type RateLimiter struct {
	mu     sync.Mutex
	limits map[Provider]int
	calls  map[Provider][]time.Time
}

// NewRateLimiter creates a RateLimiter populated with default provider limits.
func NewRateLimiter() *RateLimiter {
	limits := make(map[Provider]int, len(ProviderRateLimits))
	maps.Copy(limits, ProviderRateLimits)
	return &RateLimiter{
		limits: limits,
		calls:  make(map[Provider][]time.Time),
	}
}

// SetLimit overrides the rate limit for a provider (requests per minute).
// Set to 0 to disable rate limiting for that provider.
func (r *RateLimiter) SetLimit(p Provider, requestsPerMinute int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limits[p] = requestsPerMinute
}

// Allow returns nil if a call to the provider is allowed now.
// Returns an error with the required wait duration if the limit is exceeded.
func (r *RateLimiter) Allow(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	limit, ok := r.limits[p]
	if !ok || limit <= 0 {
		return nil // no limit configured
	}

	now := time.Now()
	window := now.Add(-time.Minute)

	// Evict calls outside the 1-minute window.
	calls := r.calls[p]
	start := 0
	for start < len(calls) && calls[start].Before(window) {
		start++
	}
	calls = calls[start:]
	r.calls[p] = calls

	if len(calls) >= limit {
		wait := calls[0].Add(time.Minute).Sub(now)
		return fmt.Errorf("rate limit: provider %s allows %d req/min; retry in %.1fs",
			p, limit, wait.Seconds())
	}

	r.calls[p] = append(r.calls[p], now)
	return nil
}

// Remaining returns calls remaining in the current window for a provider.
// Returns -1 if the provider has no configured limit.
func (r *RateLimiter) Remaining(p Provider) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	limit, ok := r.limits[p]
	if !ok || limit <= 0 {
		return -1
	}

	window := time.Now().Add(-time.Minute)
	active := 0
	for _, t := range r.calls[p] {
		if t.After(window) {
			active++
		}
	}
	if remaining := limit - active; remaining > 0 {
		return remaining
	}
	return 0
}
