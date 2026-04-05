package metrics

import (
	"math"
	"sync"
	"testing"
	"time"
)

// fakeClock returns a controllable clock and an advance function.
func fakeClock(start time.Time) (func() time.Time, func(d time.Duration)) {
	mu := sync.Mutex{}
	now := start
	return func() time.Time {
			mu.Lock()
			defer mu.Unlock()
			return now
		}, func(d time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			now = now.Add(d)
		}
}

func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// ---------------------------------------------------------------------------
// Core RateCalculator tests
// ---------------------------------------------------------------------------

func TestRateCalculator_EmptyRate(t *testing.T) {
	r := NewRateCalculator()
	if got := r.Rate(); got != 0 {
		t.Errorf("empty Rate() = %f, want 0", got)
	}
}

func TestRateCalculator_SingleRecord(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(withClock(clock))

	r.Record(120) // 120 units in a 60s window => 2/s
	got := r.Rate()
	want := 2.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("Rate() = %f, want %f", got, want)
	}
}

func TestRateCalculator_MultipleRecords(t *testing.T) {
	clock, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(withClock(clock))

	// Record 10 each second for 10 seconds.
	for range 10 {
		r.Record(10)
		advance(time.Second)
	}

	// Total = 100 over 60s window => 100/60 ~ 1.667/s
	got := r.Rate()
	want := 100.0 / 60.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("Rate() = %f, want %f", got, want)
	}
}

func TestRateCalculator_WindowRollover(t *testing.T) {
	clock, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(WithWindow(5*time.Second), withClock(clock))

	// Record 10 at t=0
	r.Record(10)

	// Advance 3s, record 20 at t=3
	advance(3 * time.Second)
	r.Record(20)

	// At t=3, window covers [t=-2, t=3]. Both records are in window.
	// Sum = 30, rate = 30/5 = 6.0
	got := r.Rate()
	want := 6.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("Rate() at t=3 = %f, want %f", got, want)
	}

	// Advance to t=6 => window covers (t=1, t=6]. t=0 bucket is evicted.
	advance(3 * time.Second)
	got = r.Rate()
	want = 20.0 / 5.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("Rate() at t=6 = %f, want %f", got, want)
	}

	// Advance to t=9 => window covers (t=4, t=9]. t=3 bucket is evicted.
	advance(3 * time.Second)
	got = r.Rate()
	if got != 0 {
		t.Errorf("Rate() at t=9 = %f, want 0 (all evicted)", got)
	}
}

func TestRateCalculator_CustomWindow(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(WithWindow(10*time.Second), withClock(clock))

	r.Record(50) // 50 in 10s window => 5/s
	got := r.Rate()
	want := 5.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("Rate() = %f, want %f", got, want)
	}
}

func TestRateCalculator_SubSecondWindowClamps(t *testing.T) {
	// A sub-second window should be ignored, keeping the default 60s.
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(WithWindow(500*time.Millisecond), withClock(clock))

	r.Record(60)
	got := r.Rate()
	// Default 60s window: 60/60 = 1.0
	want := 1.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("Rate() = %f, want %f (expected default window)", got, want)
	}
}

func TestRateCalculator_Sum(t *testing.T) {
	clock, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(WithWindow(5*time.Second), withClock(clock))

	r.Record(3)
	advance(time.Second)
	r.Record(7)

	got := r.Sum()
	if !approxEqual(got, 10.0, 0.001) {
		t.Errorf("Sum() = %f, want 10", got)
	}

	// Evict first bucket.
	advance(5 * time.Second)
	got = r.Sum()
	if !approxEqual(got, 0.0, 0.001) {
		t.Errorf("Sum() after eviction = %f, want 0", got)
	}
}

func TestRateCalculator_MultipleSameBucket(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	r := NewRateCalculator(WithWindow(10*time.Second), withClock(clock))

	// Multiple records in the same second should aggregate.
	r.Record(5)
	r.Record(3)
	r.Record(2)

	got := r.Sum()
	if !approxEqual(got, 10.0, 0.001) {
		t.Errorf("Sum() = %f, want 10", got)
	}
}

func TestRateCalculator_ConcurrentSafety(t *testing.T) {
	r := NewRateCalculator(WithWindow(5 * time.Second))

	var wg sync.WaitGroup
	writers := 20
	recordsPerWriter := 100

	wg.Add(writers)
	for range writers {
		go func() {
			defer wg.Done()
			for range recordsPerWriter {
				r.Record(1)
			}
		}()
	}

	// Read concurrently while writers are active.
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				_ = r.Rate()
				_ = r.Sum()
			}
		}
	}()

	wg.Wait()
	close(done)

	got := r.Sum()
	want := float64(writers * recordsPerWriter)
	if !approxEqual(got, want, 0.001) {
		t.Errorf("concurrent Sum() = %f, want %f", got, want)
	}
}

// ---------------------------------------------------------------------------
// TokenRateCalculator
// ---------------------------------------------------------------------------

func TestTokenRateCalculator(t *testing.T) {
	clock, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	tr := NewTokenRateCalculator(WithWindow(10*time.Second), withClock(clock))

	// Simulate 500 tokens over 5 seconds.
	for range 5 {
		tr.RecordTokens(100)
		advance(time.Second)
	}

	got := tr.TokensPerSecond()
	want := 500.0 / 10.0 // 50 tok/s
	if !approxEqual(got, want, 0.001) {
		t.Errorf("TokensPerSecond() = %f, want %f", got, want)
	}
}

// ---------------------------------------------------------------------------
// CostRateCalculator
// ---------------------------------------------------------------------------

func TestCostRateCalculator(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cr := NewCostRateCalculator(WithWindow(60*time.Second), withClock(clock))

	cr.RecordCost(0.12)
	cr.RecordCost(0.08)

	// Sum = 0.20 in 60s, rate = 0.20/60 per second, per minute = 0.20
	got := cr.CostPerMinute()
	want := 0.20
	if !approxEqual(got, want, 0.001) {
		t.Errorf("CostPerMinute() = %f, want %f", got, want)
	}
}

// ---------------------------------------------------------------------------
// EventRateCalculator
// ---------------------------------------------------------------------------

func TestEventRateCalculator(t *testing.T) {
	clock, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	er := NewEventRateCalculator(WithWindow(10*time.Second), withClock(clock))

	for range 30 {
		er.RecordEvent()
	}
	advance(5 * time.Second)
	for range 20 {
		er.RecordEvent()
	}

	// 50 events in 10s window => 5 events/s
	got := er.EventsPerSecond()
	want := 50.0 / 10.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("EventsPerSecond() = %f, want %f", got, want)
	}
}

// ---------------------------------------------------------------------------
// ErrorRateCalculator
// ---------------------------------------------------------------------------

func TestErrorRateCalculator_NoEvents(t *testing.T) {
	er := NewErrorRateCalculator()
	if got := er.ErrorPercent(); got != 0 {
		t.Errorf("ErrorPercent() with no events = %f, want 0", got)
	}
}

func TestErrorRateCalculator_AllSuccess(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	er := NewErrorRateCalculator(withClock(clock))

	for range 100 {
		er.RecordSuccess()
	}

	got := er.ErrorPercent()
	if got != 0 {
		t.Errorf("ErrorPercent() with all success = %f, want 0", got)
	}
}

func TestErrorRateCalculator_MixedErrors(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	er := NewErrorRateCalculator(withClock(clock))

	// 80 successes + 20 errors = 100 total, 20% error rate.
	for range 80 {
		er.RecordSuccess()
	}
	for range 20 {
		er.RecordError()
	}

	got := er.ErrorPercent()
	want := 20.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("ErrorPercent() = %f, want %f", got, want)
	}
}

func TestErrorRateCalculator_WindowRollover(t *testing.T) {
	clock, advance := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	er := NewErrorRateCalculator(WithWindow(5*time.Second), withClock(clock))

	// Record errors at t=0.
	for range 10 {
		er.RecordError()
	}

	// Record successes at t=3.
	advance(3 * time.Second)
	for range 10 {
		er.RecordSuccess()
	}

	// At t=3: 10 errors + 10 successes = 20 total, 10 errors => 50%
	got := er.ErrorPercent()
	want := 50.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("ErrorPercent() at t=3 = %f, want %f", got, want)
	}

	// At t=6: errors evicted, only 10 successes remain => 0%
	advance(3 * time.Second)
	got = er.ErrorPercent()
	want = 0.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("ErrorPercent() at t=6 = %f, want %f", got, want)
	}
}

func TestErrorRateCalculator_AllErrors(t *testing.T) {
	clock, _ := fakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	er := NewErrorRateCalculator(withClock(clock))

	for range 50 {
		er.RecordError()
	}

	got := er.ErrorPercent()
	want := 100.0
	if !approxEqual(got, want, 0.001) {
		t.Errorf("ErrorPercent() = %f, want %f", got, want)
	}
}
