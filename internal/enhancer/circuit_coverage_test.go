package enhancer

import (
	"testing"
)

// TestCircuitBreaker_StateHalfOpen verifies that State() returns "half-open"
// when the circuit is in the circuitHalfOpen state (set directly).
func TestCircuitBreaker_StateHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.mu.Lock()
	cb.state = circuitHalfOpen
	cb.mu.Unlock()

	got := cb.State()
	if got != "half-open" {
		t.Errorf("State() in circuitHalfOpen = %q, want half-open", got)
	}
}

// TestCircuitBreaker_StateUnknown verifies State() returns "unknown" for an
// unrecognized circuitState value.
func TestCircuitBreaker_StateUnknown(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.mu.Lock()
	cb.state = circuitState(99) // unknown
	cb.mu.Unlock()

	got := cb.State()
	if got != "unknown" {
		t.Errorf("State() unknown = %q, want unknown", got)
	}
}
