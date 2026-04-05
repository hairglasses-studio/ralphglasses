package session

import (
	"context"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// SpendRateMonitor tracks rolling hourly spend and trips when it exceeds a threshold.
// It uses 60 one-minute buckets in a circular buffer to approximate a 1-hour rolling window.
type SpendRateMonitor struct {
	mu          sync.Mutex
	buckets     [60]float64 // 60 one-minute buckets
	bucketIdx   int         // current bucket index
	lastAdvance time.Time   // last time buckets were advanced
	threshold   float64     // USD per hour, 0 = disabled
	tripped     bool        // whether the breaker is currently tripped
	totalSpend  float64     // cumulative spend for reporting
}

// NewSpendRateMonitor creates a new SpendRateMonitor with the given hourly threshold.
// A threshold of 0 disables the breaker (Tripped always returns false).
func NewSpendRateMonitor(thresholdUSD float64) *SpendRateMonitor {
	return &SpendRateMonitor{
		threshold:   thresholdUSD,
		lastAdvance: time.Now(),
	}
}

// Record adds a spend amount to the current bucket and checks the threshold.
// It advances stale buckets before recording. Thread-safe.
func (s *SpendRateMonitor) Record(amount float64) {
	if amount <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.advance()
	s.buckets[s.bucketIdx] += amount
	s.totalSpend += amount

	if s.threshold > 0 && !s.tripped {
		if s.hourlyRate() >= s.threshold {
			s.tripped = true
		}
	}
}

// HourlyRate returns the sum of all buckets, representing the spend over the last hour.
// Thread-safe.
func (s *SpendRateMonitor) HourlyRate() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advance()
	return s.hourlyRate()
}

// hourlyRate sums all buckets. Caller must hold s.mu.
func (s *SpendRateMonitor) hourlyRate() float64 {
	var total float64
	for _, b := range s.buckets {
		total += b
	}
	return total
}

// Tripped returns true if the hourly spend limit has been exceeded.
// Thread-safe.
func (s *SpendRateMonitor) Tripped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tripped
}

// Reset manually clears the trip state and zeroes all buckets.
// The monitor resumes tracking from a clean state. Thread-safe.
func (s *SpendRateMonitor) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tripped = false
	s.buckets = [60]float64{}
	s.lastAdvance = time.Now()
	s.bucketIdx = 0
}

// advance zeroes buckets that correspond to elapsed time since lastAdvance
// and moves bucketIdx forward. Caller must hold s.mu.
func (s *SpendRateMonitor) advance() {
	now := time.Now()
	elapsed := now.Sub(s.lastAdvance)
	minutes := int(elapsed.Minutes())
	if minutes <= 0 {
		return
	}
	// Cap at 60: if more than an hour has passed, all buckets should be zeroed.
	if minutes > 60 {
		minutes = 60
	}
	for i := 0; i < minutes; i++ {
		s.bucketIdx = (s.bucketIdx + 1) % 60
		s.buckets[s.bucketIdx] = 0
	}
	// Move lastAdvance forward by the number of whole minutes consumed.
	s.lastAdvance = s.lastAdvance.Add(time.Duration(minutes) * time.Minute)
}

// SubscribeToBus wires the monitor to an event bus, recording cost deltas from
// CostUpdate events. The subscription runs until ctx is cancelled or the bus
// channel is closed. Safe to call from any goroutine.
func (s *SpendRateMonitor) SubscribeToBus(ctx context.Context, bus *events.Bus) {
	if bus == nil {
		return
	}
	ch := bus.SubscribeFiltered("spend-monitor", events.CostUpdate)
	go func() {
		defer bus.Unsubscribe("spend-monitor")
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				if cost, ok := e.Data["cost_usd"].(float64); ok {
					s.Record(cost)
				}
			}
		}
	}()
}
