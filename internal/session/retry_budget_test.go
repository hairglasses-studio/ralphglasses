package session

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRetryBudget_DefaultConfig(t *testing.T) {
	b := NewDefaultRetryBudget("sess-1")
	stats := b.Stats()

	if stats.MaxRetries != DefaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", stats.MaxRetries, DefaultMaxRetries)
	}
	if stats.Remaining != DefaultMaxRetries {
		t.Errorf("Remaining = %d, want %d", stats.Remaining, DefaultMaxRetries)
	}
	if stats.TotalAttempts != 0 {
		t.Errorf("TotalAttempts = %d, want 0", stats.TotalAttempts)
	}
}

func TestRetryBudget_ExhaustsAfterMaxRetries(t *testing.T) {
	cfg := RetryBudgetConfig{
		MaxRetries: 3,
		BaseDelay:  time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		Factor:     2.0,
	}
	b := NewRetryBudget("sess-2", cfg)

	// 3 failures should be allowed (using retries 1, 2, 3).
	for i := range 3 {
		if !b.CanRetry() {
			t.Fatalf("expected CanRetry=true on attempt %d", i)
		}
		err := b.RecordFailure()
		if err != nil {
			t.Fatalf("unexpected error on attempt %d: %v", i, err)
		}
	}

	// 4th failure exceeds the budget.
	if b.CanRetry() {
		t.Error("expected CanRetry=false after 3 retries")
	}
	err := b.RecordFailure()
	if !errors.Is(err, ErrRetryBudgetExhausted) {
		t.Errorf("expected ErrRetryBudgetExhausted, got: %v", err)
	}

	stats := b.Stats()
	if stats.Remaining != 0 {
		t.Errorf("Remaining = %d, want 0", stats.Remaining)
	}
	if stats.Failures != 4 {
		t.Errorf("Failures = %d, want 4", stats.Failures)
	}
}

func TestRetryBudget_SuccessRate(t *testing.T) {
	b := NewDefaultRetryBudget("sess-3")

	b.RecordSuccess()
	b.RecordSuccess()
	b.RecordFailure()
	b.RecordSuccess()

	stats := b.Stats()
	if stats.TotalAttempts != 4 {
		t.Errorf("TotalAttempts = %d, want 4", stats.TotalAttempts)
	}
	if stats.Successes != 3 {
		t.Errorf("Successes = %d, want 3", stats.Successes)
	}
	if stats.Failures != 1 {
		t.Errorf("Failures = %d, want 1", stats.Failures)
	}
	expectedRate := 0.75
	if stats.SuccessRate != expectedRate {
		t.Errorf("SuccessRate = %f, want %f", stats.SuccessRate, expectedRate)
	}
}

func TestRetryBudget_NextDelay(t *testing.T) {
	cfg := RetryBudgetConfig{
		MaxRetries: 10,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		Factor:     2.0,
	}
	b := NewRetryBudget("sess-4", cfg)

	// Delay should be non-negative and within bounds.
	for range 5 {
		d := b.NextDelay()
		if d < 0 {
			t.Errorf("negative delay: %v", d)
		}
		if d > cfg.MaxDelay {
			t.Errorf("delay %v exceeds MaxDelay %v", d, cfg.MaxDelay)
		}
	}
}

func TestRetryBudget_Reset(t *testing.T) {
	b := NewDefaultRetryBudget("sess-5")
	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess()

	b.Reset()
	stats := b.Stats()

	if stats.TotalAttempts != 0 {
		t.Errorf("after Reset: TotalAttempts = %d, want 0", stats.TotalAttempts)
	}
	if stats.Remaining != DefaultMaxRetries {
		t.Errorf("after Reset: Remaining = %d, want %d", stats.Remaining, DefaultMaxRetries)
	}
}

func TestRetryBudget_ThreadSafety(t *testing.T) {
	b := NewRetryBudget("sess-6", RetryBudgetConfig{
		MaxRetries: 1000,
		BaseDelay:  time.Millisecond,
		MaxDelay:   time.Millisecond,
		Factor:     1.0,
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			b.RecordSuccess()
		}()
		go func() {
			defer wg.Done()
			b.RecordFailure()
		}()
		go func() {
			defer wg.Done()
			b.Stats()
		}()
	}
	wg.Wait()

	stats := b.Stats()
	if stats.TotalAttempts != 200 {
		t.Errorf("TotalAttempts = %d, want 200", stats.TotalAttempts)
	}
}

func TestRetryBudget_ZeroRetries(t *testing.T) {
	cfg := RetryBudgetConfig{
		MaxRetries: 0,
		BaseDelay:  time.Millisecond,
		MaxDelay:   time.Millisecond,
		Factor:     2.0,
	}
	b := NewRetryBudget("sess-7", cfg)

	if b.CanRetry() {
		t.Error("expected CanRetry=false with MaxRetries=0")
	}

	err := b.RecordFailure()
	if !errors.Is(err, ErrRetryBudgetExhausted) {
		t.Errorf("expected ErrRetryBudgetExhausted, got: %v", err)
	}
}
