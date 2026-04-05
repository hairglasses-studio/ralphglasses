package gateway

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_InitiallyClosed(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{})
	if err := cb.Allow(); err != nil {
		t.Fatalf("new circuit should be closed, got %v", err)
	}
	cb.RecordSuccess()
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3})
	for range 3 {
		cb.Allow() //nolint:errcheck
		cb.RecordFailure()
	}
	if err := cb.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen after %d failures, got %v", 3, err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          10 * time.Millisecond,
	})
	cb.Allow() //nolint:errcheck
	cb.RecordFailure()
	if s := cb.Status().State; s != StateOpen {
		t.Fatalf("expected open, got %v", s)
	}

	time.Sleep(20 * time.Millisecond)

	if err := cb.Allow(); err != nil {
		t.Fatalf("expected nil after timeout (half-open probe), got %v", err)
	}
	if s := cb.Status().State; s != StateHalfOpen {
		t.Fatalf("expected half-open, got %v", s)
	}
}

func TestCircuitBreaker_ClosesAfterHalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          5 * time.Millisecond,
	})
	cb.Allow() //nolint:errcheck
	cb.RecordFailure()
	time.Sleep(10 * time.Millisecond)
	cb.Allow() //nolint:errcheck
	cb.RecordSuccess()

	if s := cb.Status().State; s != StateClosed {
		t.Fatalf("expected closed after half-open success, got %v", s)
	}
}

func TestCircuitBreaker_ReopensOnHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		FailureThreshold: 1,
		Timeout:          5 * time.Millisecond,
	})
	cb.Allow() //nolint:errcheck
	cb.RecordFailure()
	time.Sleep(10 * time.Millisecond)
	cb.Allow() //nolint:errcheck
	cb.RecordFailure()

	if s := cb.Status().State; s != StateOpen {
		t.Fatalf("expected re-open after half-open failure, got %v", s)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 1})
	cb.Allow() //nolint:errcheck
	cb.RecordFailure()
	cb.Reset()
	if s := cb.Status().State; s != StateClosed {
		t.Fatalf("expected closed after reset, got %v", s)
	}
	if err := cb.Allow(); err != nil {
		t.Fatalf("allow should succeed after reset, got %v", err)
	}
}

func TestCircuitBreaker_StatusFields(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{FailureThreshold: 3})
	cb.Allow() //nolint:errcheck
	cb.RecordSuccess()
	cb.Allow() //nolint:errcheck
	cb.RecordFailure()

	s := cb.Status()
	if s.Failures != 1 {
		t.Errorf("expected 1 failure, got %d", s.Failures)
	}
	if s.State != StateClosed {
		t.Errorf("expected closed, got %v", s.State)
	}
}

func TestCircuitBreakerStateString(t *testing.T) {
	if StateClosed.String() != "closed" {
		t.Error("closed string wrong")
	}
	if StateOpen.String() != "open" {
		t.Error("open string wrong")
	}
	if StateHalfOpen.String() != "half-open" {
		t.Error("half-open string wrong")
	}
}

func TestProviderCircuitBreakers_DefaultProviders(t *testing.T) {
	pcb := NewProviderCircuitBreakers(CircuitBreakerConfig{})
	for _, p := range []string{"claude", "gemini", "openai"} {
		cb := pcb.Get(p)
		if cb == nil {
			t.Errorf("expected breaker for %s", p)
		}
		if err := cb.Allow(); err != nil {
			t.Errorf("provider %s should start closed: %v", p, err)
		}
		cb.RecordSuccess()
	}
}

func TestProviderCircuitBreakers_LazyCreaion(t *testing.T) {
	pcb := NewProviderCircuitBreakers(CircuitBreakerConfig{})
	cb := pcb.Get("new-provider")
	if cb == nil {
		t.Fatal("expected lazy-created breaker")
	}
	if err := cb.Allow(); err != nil {
		t.Fatalf("new provider should start closed: %v", err)
	}
	cb.RecordSuccess()
}

func TestProviderCircuitBreakers_Register(t *testing.T) {
	pcb := NewProviderCircuitBreakers(CircuitBreakerConfig{})
	pcb.Register("custom", CircuitBreakerConfig{FailureThreshold: 2})
	cb := pcb.Get("custom")
	if cb == nil {
		t.Fatal("expected registered breaker")
	}
}

func TestProviderCircuitBreakers_Status(t *testing.T) {
	pcb := NewProviderCircuitBreakers(CircuitBreakerConfig{})
	status := pcb.Status()
	for _, p := range []string{"claude", "gemini", "openai"} {
		s, ok := status[p]
		if !ok {
			t.Errorf("expected status for %s", p)
		}
		if s.State != StateClosed {
			t.Errorf("expected closed for %s, got %v", p, s.State)
		}
	}
}

func TestProviderCircuitBreakers_IndependentState(t *testing.T) {
	pcb := NewProviderCircuitBreakers(CircuitBreakerConfig{FailureThreshold: 1})
	// Trip claude.
	claude := pcb.Get("claude")
	claude.Allow() //nolint:errcheck
	claude.RecordFailure()
	if err := claude.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("claude should be open: %v", err)
	}
	// gemini should still be closed.
	if err := pcb.Get("gemini").Allow(); err != nil {
		t.Fatalf("gemini should be unaffected: %v", err)
	}
	pcb.Get("gemini").RecordSuccess()
}
