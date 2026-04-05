package metrics

import (
	"sync"
	"time"
)

// DefaultWindow is the default sliding window duration for rate calculations.
const DefaultWindow = 60 * time.Second

// RateCalculator computes a per-second rate over a configurable sliding
// window using 1-second buckets. It is safe for concurrent use.
//
// Typical uses: tokens/s, cost/min, events/s, error-rate %.
type RateCalculator struct {
	mu      sync.Mutex
	window  time.Duration
	buckets []bucket
	now     func() time.Time // injectable clock for testing
}

type bucket struct {
	ts    int64   // unix second
	value float64 // accumulated value for that second
}

// RateOption configures a RateCalculator.
type RateOption func(*RateCalculator)

// WithWindow sets the sliding window duration.
// The window is rounded down to whole seconds (minimum 1 s).
func WithWindow(d time.Duration) RateOption {
	return func(r *RateCalculator) {
		if s := d.Truncate(time.Second); s >= time.Second {
			r.window = s
		}
	}
}

// withClock is an unexported option used by tests to inject a fake clock.
func withClock(fn func() time.Time) RateOption {
	return func(r *RateCalculator) { r.now = fn }
}

// NewRateCalculator creates a RateCalculator with the given options.
// Without options the default 60 s window is used.
func NewRateCalculator(opts ...RateOption) *RateCalculator {
	r := &RateCalculator{
		window: DefaultWindow,
		now:    time.Now,
	}
	for _, o := range opts {
		o(r)
	}
	// Pre-allocate buckets slice with capacity for the full window.
	cap := max(int(r.window.Seconds()), 1)
	r.buckets = make([]bucket, 0, cap)
	return r
}

// Record adds value to the current 1-second bucket.
func (r *RateCalculator) Record(value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now().Unix()
	r.evict(now)

	// Append to the latest bucket or create a new one.
	if n := len(r.buckets); n > 0 && r.buckets[n-1].ts == now {
		r.buckets[n-1].value += value
	} else {
		r.buckets = append(r.buckets, bucket{ts: now, value: value})
	}
}

// Rate returns the per-second rate over the sliding window.
// If the window contains no data, 0 is returned.
func (r *RateCalculator) Rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now().Unix()
	r.evict(now)

	var total float64
	for _, b := range r.buckets {
		total += b.value
	}

	windowSec := r.window.Seconds()
	if windowSec == 0 {
		return 0
	}
	return total / windowSec
}

// Sum returns the total accumulated value currently inside the window.
func (r *RateCalculator) Sum() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now().Unix()
	r.evict(now)

	var total float64
	for _, b := range r.buckets {
		total += b.value
	}
	return total
}

// evict removes buckets older than the window. Caller must hold r.mu.
func (r *RateCalculator) evict(nowUnix int64) {
	cutoff := nowUnix - int64(r.window.Seconds())
	i := 0
	for i < len(r.buckets) && r.buckets[i].ts <= cutoff {
		i++
	}
	if i > 0 {
		// Shift remaining buckets to the front without re-allocating.
		n := copy(r.buckets, r.buckets[i:])
		r.buckets = r.buckets[:n]
	}
}

// ---------------------------------------------------------------------------
// Convenience calculators for common fleet metrics
// ---------------------------------------------------------------------------

// TokenRateCalculator tracks tokens per second.
type TokenRateCalculator struct{ *RateCalculator }

// NewTokenRateCalculator returns a RateCalculator labelled for token throughput.
func NewTokenRateCalculator(opts ...RateOption) *TokenRateCalculator {
	return &TokenRateCalculator{NewRateCalculator(opts...)}
}

// RecordTokens records n tokens at the current instant.
func (t *TokenRateCalculator) RecordTokens(n int) { t.Record(float64(n)) }

// TokensPerSecond returns the token throughput rate.
func (t *TokenRateCalculator) TokensPerSecond() float64 { return t.Rate() }

// CostRateCalculator tracks cost per minute.
type CostRateCalculator struct{ *RateCalculator }

// NewCostRateCalculator returns a RateCalculator labelled for cost tracking.
func NewCostRateCalculator(opts ...RateOption) *CostRateCalculator {
	return &CostRateCalculator{NewRateCalculator(opts...)}
}

// RecordCost records a cost amount in USD.
func (c *CostRateCalculator) RecordCost(usd float64) { c.Record(usd) }

// CostPerMinute returns the cost rate in USD/minute.
func (c *CostRateCalculator) CostPerMinute() float64 { return c.Rate() * 60 }

// EventRateCalculator tracks events per second.
type EventRateCalculator struct{ *RateCalculator }

// NewEventRateCalculator returns a RateCalculator labelled for event throughput.
func NewEventRateCalculator(opts ...RateOption) *EventRateCalculator {
	return &EventRateCalculator{NewRateCalculator(opts...)}
}

// RecordEvent records one event occurrence.
func (e *EventRateCalculator) RecordEvent() { e.Record(1) }

// EventsPerSecond returns the event throughput rate.
func (e *EventRateCalculator) EventsPerSecond() float64 { return e.Rate() }

// ErrorRateCalculator tracks error rate as a percentage of total events.
type ErrorRateCalculator struct {
	total  *RateCalculator
	errors *RateCalculator
}

// NewErrorRateCalculator returns a calculator that tracks error percentage.
// Both the total and error counters share the same window configuration.
func NewErrorRateCalculator(opts ...RateOption) *ErrorRateCalculator {
	return &ErrorRateCalculator{
		total:  NewRateCalculator(opts...),
		errors: NewRateCalculator(opts...),
	}
}

// RecordSuccess records a successful (non-error) event.
func (e *ErrorRateCalculator) RecordSuccess() { e.total.Record(1) }

// RecordError records an error event (also counted toward the total).
func (e *ErrorRateCalculator) RecordError() {
	e.total.Record(1)
	e.errors.Record(1)
}

// ErrorPercent returns the error rate as a percentage (0-100).
// Returns 0 if no events have been recorded in the window.
func (e *ErrorRateCalculator) ErrorPercent() float64 {
	total := e.total.Sum()
	if total == 0 {
		return 0
	}
	return (e.errors.Sum() / total) * 100
}
