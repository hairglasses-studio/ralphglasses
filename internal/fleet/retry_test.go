package fleet

import (
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	p := DefaultRetryPolicy()
	if p.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", p.MaxRetries)
	}
	if p.BaseDelay != time.Second {
		t.Errorf("BaseDelay = %v, want 1s", p.BaseDelay)
	}
	if p.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", p.MaxDelay)
	}
	if p.Multiplier != 2.0 {
		t.Errorf("Multiplier = %f, want 2.0", p.Multiplier)
	}
	if p.JitterFraction != 0.1 {
		t.Errorf("JitterFraction = %f, want 0.1", p.JitterFraction)
	}
}

func TestShouldRetry_BelowMax(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     3,
		BaseDelay:      time.Second,
		MaxDelay:       30 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0, // no jitter for deterministic test
	}

	for attempt := range 3 {
		retry, _ := p.ShouldRetry(attempt)
		if !retry {
			t.Errorf("ShouldRetry(%d) = false, want true", attempt)
		}
	}
}

func TestShouldRetry_AtMax(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     3,
		BaseDelay:      time.Second,
		MaxDelay:       30 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0,
	}

	retry, delay := p.ShouldRetry(3)
	if retry {
		t.Error("ShouldRetry(3) = true, want false when MaxRetries=3")
	}
	if delay != 0 {
		t.Errorf("delay = %v, want 0 when max reached", delay)
	}

	retry, _ = p.ShouldRetry(5)
	if retry {
		t.Error("ShouldRetry(5) = true, want false when MaxRetries=3")
	}
}

func TestShouldRetry_ExponentialBackoff(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     5,
		BaseDelay:      time.Second,
		MaxDelay:       60 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0, // no jitter for deterministic test
	}

	expected := []time.Duration{
		1 * time.Second,  // 1 * 2^0
		2 * time.Second,  // 1 * 2^1
		4 * time.Second,  // 1 * 2^2
		8 * time.Second,  // 1 * 2^3
		16 * time.Second, // 1 * 2^4
	}

	for i, want := range expected {
		retry, got := p.ShouldRetry(i)
		if !retry {
			t.Fatalf("ShouldRetry(%d) = false, want true", i)
		}
		if got != want {
			t.Errorf("attempt %d: delay = %v, want %v", i, got, want)
		}
	}
}

func TestShouldRetry_DelayCappedAtMaxDelay(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     10,
		BaseDelay:      time.Second,
		MaxDelay:       5 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0,
	}

	// attempt 3: 1 * 2^3 = 8s, capped to 5s
	retry, delay := p.ShouldRetry(3)
	if !retry {
		t.Fatal("ShouldRetry(3) = false, want true")
	}
	if delay != 5*time.Second {
		t.Errorf("delay = %v, want %v (capped)", delay, 5*time.Second)
	}

	// attempt 7: 1 * 2^7 = 128s, capped to 5s
	retry, delay = p.ShouldRetry(7)
	if !retry {
		t.Fatal("ShouldRetry(7) = false, want true")
	}
	if delay != 5*time.Second {
		t.Errorf("delay = %v, want %v (capped)", delay, 5*time.Second)
	}
}

func TestRetryTracker_TracksAttempts(t *testing.T) {
	tracker := NewRetryTracker(RetryPolicy{
		MaxRetries:     3,
		BaseDelay:      time.Second,
		MaxDelay:       30 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0,
	})

	if got := tracker.Attempts("work-1"); got != 0 {
		t.Errorf("initial attempts = %d, want 0", got)
	}

	retry, _ := tracker.RecordFailure("work-1")
	if !retry {
		t.Error("first failure: retry = false, want true")
	}
	if got := tracker.Attempts("work-1"); got != 1 {
		t.Errorf("after 1 failure: attempts = %d, want 1", got)
	}

	retry, _ = tracker.RecordFailure("work-1")
	if !retry {
		t.Error("second failure: retry = false, want true")
	}
	if got := tracker.Attempts("work-1"); got != 2 {
		t.Errorf("after 2 failures: attempts = %d, want 2", got)
	}

	retry, _ = tracker.RecordFailure("work-1")
	if !retry {
		t.Error("third failure: retry = false, want true")
	}
	if got := tracker.Attempts("work-1"); got != 3 {
		t.Errorf("after 3 failures: attempts = %d, want 3", got)
	}

	// Fourth failure should not retry (attempts[work-1]=4, ShouldRetry(3) = false)
	retry, _ = tracker.RecordFailure("work-1")
	if retry {
		t.Error("fourth failure: retry = true, want false (max reached)")
	}
}

func TestRetryTracker_RecordSuccessClearsState(t *testing.T) {
	tracker := NewRetryTracker(DefaultRetryPolicy())

	tracker.RecordFailure("work-1")
	tracker.RecordFailure("work-1")
	if got := tracker.Attempts("work-1"); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}

	tracker.RecordSuccess("work-1")
	if got := tracker.Attempts("work-1"); got != 0 {
		t.Errorf("after success: attempts = %d, want 0", got)
	}
}

func TestShouldRetry_JitterBounded(t *testing.T) {
	p := RetryPolicy{
		MaxRetries:     3,
		BaseDelay:      10 * time.Second,
		MaxDelay:       60 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.1,
	}

	// Run many times to check jitter stays within bounds
	for range 100 {
		retry, delay := p.ShouldRetry(0)
		if !retry {
			t.Fatal("ShouldRetry(0) = false")
		}
		// Base delay is 10s, jitter is +/- 10% = [9s, 11s]
		if delay < 9*time.Second || delay > 11*time.Second {
			t.Errorf("delay = %v, want between 9s and 11s", delay)
		}
	}
}
