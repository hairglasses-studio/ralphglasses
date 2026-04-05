package resilience

import (
	"errors"
	"sync"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

func TestNewCircuitBreaker_Defaults(t *testing.T) {
	cb := NewCircuitBreaker("test", CircuitBreakerConfig{})
	if cb.config.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", cb.config.FailureThreshold)
	}
	if cb.config.SuccessThreshold != 2 {
		t.Errorf("SuccessThreshold = %d, want 2", cb.config.SuccessThreshold)
	}
	if cb.config.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cb.config.Timeout)
	}
	if cb.config.HalfOpenMaxCalls != 1 {
		t.Errorf("HalfOpenMaxCalls = %d, want 1", cb.config.HalfOpenMaxCalls)
	}
}

func TestCircuitBreaker_ClosedSuccess(t *testing.T) {
	cb := NewCircuitBreaker("test", DefaultConfig())
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.State() != StateClosed {
		t.Errorf("state = %v, want Closed", cb.State())
	}
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{FailureThreshold: 3, Timeout: time.Hour}
	cb := NewCircuitBreaker("test", cfg)

	for i := 0; i < 3; i++ {
		cb.Execute(func() error { return errBoom })
	}

	if cb.State() != StateOpen {
		t.Errorf("state = %v, want Open after %d failures", cb.State(), 3)
	}
}

func TestCircuitBreaker_OpenRejectsAll(t *testing.T) {
	cfg := CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour}
	cb := NewCircuitBreaker("test", cfg)

	cb.Execute(func() error { return errBoom }) // opens

	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("err = %v, want ErrCircuitOpen", err)
	}
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cfg := CircuitBreakerConfig{FailureThreshold: 1, Timeout: 1 * time.Millisecond}
	cb := NewCircuitBreaker("test", cfg)

	cb.Execute(func() error { return errBoom })
	if cb.State() != StateOpen {
		t.Fatalf("state = %v, want Open", cb.State())
	}

	time.Sleep(5 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Errorf("state = %v, want HalfOpen after timeout", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenSuccess(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          1 * time.Millisecond,
		HalfOpenMaxCalls: 10, // allow multiple calls in half-open
	}
	cb := NewCircuitBreaker("test", cfg)

	cb.Execute(func() error { return errBoom })
	time.Sleep(5 * time.Millisecond) // → half-open

	// Two successes in half-open should close
	cb.Execute(func() error { return nil })
	cb.Execute(func() error { return nil })

	if cb.State() != StateClosed {
		t.Errorf("state = %v, want Closed after %d successes", cb.State(), 2)
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Millisecond,
	}
	cb := NewCircuitBreaker("test", cfg)

	cb.Execute(func() error { return errBoom }) // open
	time.Sleep(5 * time.Millisecond)            // → half-open

	cb.Execute(func() error { return errBoom }) // single failure → back to open

	if cb.State() != StateOpen {
		t.Errorf("state = %v, want Open after half-open failure", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenMaxCalls(t *testing.T) {
	cfg := CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          1 * time.Millisecond,
		HalfOpenMaxCalls: 1,
		SuccessThreshold: 5, // high so we stay in half-open
	}
	cb := NewCircuitBreaker("test", cfg)

	cb.Execute(func() error { return errBoom })
	time.Sleep(5 * time.Millisecond)

	// First call in half-open should be allowed
	done := make(chan error, 1)
	go func() {
		done <- cb.Execute(func() error {
			time.Sleep(50 * time.Millisecond) // hold the slot
			return nil
		})
	}()
	time.Sleep(5 * time.Millisecond) // let goroutine start

	// Second call should be rejected (max 1 concurrent in half-open)
	err := cb.Execute(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("second half-open call: err = %v, want ErrCircuitOpen", err)
	}

	<-done // cleanup
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cfg := CircuitBreakerConfig{FailureThreshold: 100, Timeout: time.Hour}
	cb := NewCircuitBreaker("test", cfg)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				cb.Execute(func() error { return nil })
			} else {
				cb.Execute(func() error { return errBoom })
			}
		}(i)
	}
	wg.Wait()

	// Should not have panicked; state should still be valid
	s := cb.State()
	if s != StateClosed && s != StateOpen {
		t.Errorf("unexpected state: %v", s)
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		s    CircuitState
		want string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("CircuitState(%d).String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}

// --- Registry tests ---

func TestRegistry_GetOrCreate(t *testing.T) {
	r := NewRegistry()
	cb1 := r.Get("api")
	cb2 := r.Get("api")
	if cb1 != cb2 {
		t.Error("Get should return same instance for same name")
	}

	cb3 := r.Get("other")
	if cb1 == cb3 {
		t.Error("Get should return different instance for different name")
	}
}

func TestRegistry_Configure(t *testing.T) {
	r := NewRegistry()
	r.Configure("custom", CircuitBreakerConfig{FailureThreshold: 10, Timeout: 5 * time.Minute})

	cb := r.Get("custom")
	if cb.config.FailureThreshold != 10 {
		t.Errorf("FailureThreshold = %d, want 10", cb.config.FailureThreshold)
	}
	if cb.config.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", cb.config.Timeout)
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := NewRegistry()
	called := false
	err := r.Execute("test", func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
}

func TestRegistry_Execute_Failure(t *testing.T) {
	r := NewRegistry()
	r.Configure("fragile", CircuitBreakerConfig{FailureThreshold: 1, Timeout: time.Hour})

	r.Execute("fragile", func() error { return errBoom })

	err := r.Execute("fragile", func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("err = %v, want ErrCircuitOpen", err)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", cfg.FailureThreshold)
	}
	if cfg.SuccessThreshold != 2 {
		t.Errorf("SuccessThreshold = %d, want 2", cfg.SuccessThreshold)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.HalfOpenMaxCalls != 1 {
		t.Errorf("HalfOpenMaxCalls = %d, want 1", cfg.HalfOpenMaxCalls)
	}
}
