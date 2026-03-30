package mcpserver

import (
	"fmt"
	"sync"
	"time"
)

// NamespaceRate configures the token bucket parameters for a single namespace.
type NamespaceRate struct {
	// Rate is the number of tokens added per second.
	Rate float64
	// Burst is the maximum number of tokens the bucket can hold.
	Burst int
}

// RateLimiterStats holds usage statistics for a single namespace.
type RateLimiterStats struct {
	// Allowed is the total number of requests that were permitted.
	Allowed int64
	// Throttled is the total number of requests that were denied.
	Throttled int64
}

// bucket is an internal token bucket for a single namespace.
type bucket struct {
	mu       sync.Mutex
	tokens   float64
	burst    int
	rate     float64 // tokens per second
	lastTick time.Time

	allowed   int64
	throttled int64
}

// newBucket creates a token bucket that starts full.
func newBucket(rate float64, burst int, now time.Time) *bucket {
	return &bucket{
		tokens:   float64(burst),
		burst:    burst,
		rate:     rate,
		lastTick: now,
	}
}

// allow attempts to consume one token. Returns true if the request is allowed.
func (b *bucket) allow(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Refill tokens based on elapsed time.
	elapsed := now.Sub(b.lastTick).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > float64(b.burst) {
			b.tokens = float64(b.burst)
		}
		b.lastTick = now
	}

	if b.tokens >= 1.0 {
		b.tokens--
		b.allowed++
		return true
	}
	b.throttled++
	return false
}

// stats returns a snapshot of the bucket's counters.
func (b *bucket) stats() RateLimiterStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return RateLimiterStats{
		Allowed:   b.allowed,
		Throttled: b.throttled,
	}
}

// RateLimiter provides per-namespace token bucket rate limiting.
// It is safe for concurrent use.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket

	// defaultRate is used for namespaces without an explicit configuration.
	defaultRate NamespaceRate

	// nowFn allows injecting a clock for testing.
	nowFn func() time.Time
}

// RateLimiterOption configures a RateLimiter.
type RateLimiterOption func(*RateLimiter)

// WithDefaultRate sets the default rate applied to namespaces that have no
// explicit configuration.
func WithDefaultRate(rate float64, burst int) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.defaultRate = NamespaceRate{Rate: rate, Burst: burst}
	}
}

// WithNamespaceRate sets a per-namespace rate override.
func WithNamespaceRate(namespace string, rate float64, burst int) RateLimiterOption {
	return func(rl *RateLimiter) {
		b := newBucket(rate, burst, rl.nowFn())
		rl.buckets[namespace] = b
	}
}

// WithClock overrides the time source (primarily for testing).
func WithClock(fn func() time.Time) RateLimiterOption {
	return func(rl *RateLimiter) {
		rl.nowFn = fn
	}
}

// NewRateLimiter creates a RateLimiter. The default rate is 10 req/s with a
// burst of 20 unless overridden via options.
func NewRateLimiter(opts ...RateLimiterOption) *RateLimiter {
	rl := &RateLimiter{
		buckets:     make(map[string]*bucket),
		defaultRate: NamespaceRate{Rate: 10, Burst: 20},
		nowFn:       time.Now,
	}

	// Apply clock option first so namespace options use the right time.
	for _, opt := range opts {
		opt(rl)
	}

	return rl
}

// Allow checks whether a request to the given namespace should be permitted.
// It returns true if a token was available, false if the request is throttled.
func (rl *RateLimiter) Allow(namespace string) bool {
	b := rl.getBucket(namespace)
	return b.allow(rl.nowFn())
}

// Stats returns the usage statistics for a namespace. If the namespace has no
// bucket yet, zero stats are returned.
func (rl *RateLimiter) Stats(namespace string) RateLimiterStats {
	rl.mu.RLock()
	b, ok := rl.buckets[namespace]
	rl.mu.RUnlock()
	if !ok {
		return RateLimiterStats{}
	}
	return b.stats()
}

// AllStats returns stats for every namespace that has been accessed.
func (rl *RateLimiter) AllStats() map[string]RateLimiterStats {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	m := make(map[string]RateLimiterStats, len(rl.buckets))
	for ns, b := range rl.buckets {
		m[ns] = b.stats()
	}
	return m
}

// SetNamespaceRate dynamically updates the rate for a namespace. If the
// namespace already has a bucket, its rate and burst are updated in place.
// Otherwise a new bucket is created.
func (rl *RateLimiter) SetNamespaceRate(namespace string, rate float64, burst int) error {
	if rate <= 0 || burst <= 0 {
		return fmt.Errorf("rate and burst must be positive, got rate=%f burst=%d", rate, burst)
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if b, ok := rl.buckets[namespace]; ok {
		b.mu.Lock()
		b.rate = rate
		b.burst = burst
		if b.tokens > float64(burst) {
			b.tokens = float64(burst)
		}
		b.mu.Unlock()
	} else {
		rl.buckets[namespace] = newBucket(rate, burst, rl.nowFn())
	}
	return nil
}

// getBucket returns the bucket for a namespace, creating one with the default
// rate if it does not yet exist.
func (rl *RateLimiter) getBucket(namespace string) *bucket {
	rl.mu.RLock()
	b, ok := rl.buckets[namespace]
	rl.mu.RUnlock()
	if ok {
		return b
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	// Double-check after acquiring write lock.
	if b, ok = rl.buckets[namespace]; ok {
		return b
	}
	b = newBucket(rl.defaultRate.Rate, rl.defaultRate.Burst, rl.nowFn())
	rl.buckets[namespace] = b
	return b
}
