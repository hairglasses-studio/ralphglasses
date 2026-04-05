package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_ImmediateSuccess(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestRetry_EventualSuccess(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), RetryConfig{MaxAttempts: 5, InitialDelay: time.Millisecond}, func() error {
		calls++
		if calls < 3 {
			return errBoom
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetry_AllFail(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond}, func() error {
		calls++
		return errBoom
	})
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want errBoom", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestRetry_RetryOnFilter(t *testing.T) {
	errPermanent := errors.New("permanent")
	calls := 0
	err := Retry(context.Background(), RetryConfig{
		MaxAttempts:  5,
		InitialDelay: time.Millisecond,
		RetryOn:      func(e error) bool { return !errors.Is(e, errPermanent) },
	}, func() error {
		calls++
		return errPermanent
	})
	if !errors.Is(err, errPermanent) {
		t.Errorf("err = %v, want errPermanent", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (should stop on non-retryable)", calls)
	}
}

func TestRetry_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(15 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, RetryConfig{
		MaxAttempts:  100,
		InitialDelay: 10 * time.Millisecond,
	}, func() error {
		calls++
		return errBoom
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls >= 100 {
		t.Errorf("should have been cancelled early, calls = %d", calls)
	}
}

func TestRetry_MaxDelayClamp(t *testing.T) {
	start := time.Now()
	calls := 0
	Retry(context.Background(), RetryConfig{
		MaxAttempts:  4,
		InitialDelay: 5 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   10.0, // would be 50ms without clamp
	}, func() error {
		calls++
		return errBoom
	})
	elapsed := time.Since(start)

	// 3 delays: 5ms + 10ms + 10ms = 25ms (clamped). Without clamp: 5ms + 50ms + 500ms = 555ms
	if elapsed > 200*time.Millisecond {
		t.Errorf("elapsed = %v, MaxDelay clamp not working", elapsed)
	}
	if calls != 4 {
		t.Errorf("calls = %d, want 4", calls)
	}
}

func TestRetry_DefaultMaxAttempts(t *testing.T) {
	// MaxAttempts 0 should default to 1
	calls := 0
	Retry(context.Background(), RetryConfig{MaxAttempts: 0}, func() error {
		calls++
		return errBoom
	})
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (default MaxAttempts)", calls)
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig(nil)
	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != time.Second {
		t.Errorf("InitialDelay = %v, want 1s", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Errorf("MaxDelay = %v, want 10s", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Multiplier = %v, want 2.0", cfg.Multiplier)
	}
}

func TestDefaultRetryConfig_WithFilter(t *testing.T) {
	filter := func(error) bool { return false }
	cfg := DefaultRetryConfig(filter)
	if cfg.RetryOn == nil {
		t.Error("RetryOn should not be nil when provided")
	}
}
