package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCircuitBreakerDefaults(t *testing.T) {
	cb := NewCircuitBreaker(0, 0, 0)
	if cb.maxFailures != defaultMaxFailures {
		t.Errorf("maxFailures = %d, want %d", cb.maxFailures, defaultMaxFailures)
	}
	if cb.resetTimeout != defaultResetTimeout {
		t.Errorf("resetTimeout = %v, want %v", cb.resetTimeout, defaultResetTimeout)
	}
	if cb.failureWindow != defaultFailureWindow {
		t.Errorf("failureWindow = %v, want %v", cb.failureWindow, defaultFailureWindow)
	}
	if cb.State() != CircuitClosed {
		t.Errorf("initial state = %q, want %q", cb.State(), CircuitClosed)
	}
}

func TestCircuitBreakerAllowSpawn(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*CircuitBreaker)
		want      bool
		wantState CircuitState
	}{
		{
			name:      "closed allows spawn",
			setup:     func(_ *CircuitBreaker) {},
			want:      true,
			wantState: CircuitClosed,
		},
		{
			name: "open refuses spawn",
			setup: func(cb *CircuitBreaker) {
				for range 3 {
					cb.RecordFailure()
				}
			},
			want:      false,
			wantState: CircuitOpen,
		},
		{
			name: "half-open allows probe",
			setup: func(cb *CircuitBreaker) {
				cb.mu.Lock()
				cb.state = CircuitHalfOpen
				cb.mu.Unlock()
			},
			want:      true,
			wantState: CircuitHalfOpen,
		},
		{
			name: "open transitions to half-open after timeout",
			setup: func(cb *CircuitBreaker) {
				cb.mu.Lock()
				cb.state = CircuitOpen
				cb.openUntil = time.Now().Add(-1 * time.Second) // expired
				cb.mu.Unlock()
			},
			want:      true,
			wantState: CircuitHalfOpen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(3, 5*time.Minute, 60*time.Second)
			tt.setup(cb)
			got := cb.AllowSpawn()
			if got != tt.want {
				t.Errorf("AllowSpawn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCircuitBreakerRecordSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute, 60*time.Second)
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("state after success = %q, want %q", cb.State(), CircuitClosed)
	}
	cb.mu.Lock()
	if cb.failures != 0 {
		t.Errorf("failures after success = %d, want 0", cb.failures)
	}
	cb.mu.Unlock()
}

func TestCircuitBreakerRecordFailure(t *testing.T) {
	tests := []struct {
		name      string
		failures  int
		wantState CircuitState
	}{
		{"below threshold", 2, CircuitClosed},
		{"at threshold", 3, CircuitOpen},
		{"above threshold", 5, CircuitOpen},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewCircuitBreaker(3, 5*time.Minute, 60*time.Second)
			for i := 0; i < tt.failures; i++ {
				cb.RecordFailure()
			}
			if got := cb.State(); got != tt.wantState {
				t.Errorf("state after %d failures = %q, want %q", tt.failures, got, tt.wantState)
			}
		})
	}
}

func TestCircuitBreakerFailureWindowReset(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute, 10*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()

	// Wait for the failure window to expire.
	time.Sleep(20 * time.Millisecond)

	// This failure should reset the counter first, so we're at 1 not 3.
	cb.RecordFailure()

	if cb.State() != CircuitClosed {
		t.Errorf("state = %q, want %q (window should have reset)", cb.State(), CircuitClosed)
	}
}

func TestCircuitBreakerHalfOpenSuccess(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute, 60*time.Second)
	cb.mu.Lock()
	cb.state = CircuitHalfOpen
	cb.mu.Unlock()

	if !cb.AllowSpawn() {
		t.Fatal("half-open should allow probe spawn")
	}
	cb.RecordSuccess()
	if cb.State() != CircuitClosed {
		t.Errorf("state after half-open success = %q, want %q", cb.State(), CircuitClosed)
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(1, 100*time.Millisecond, 60*time.Second)
	cb.mu.Lock()
	cb.state = CircuitHalfOpen
	cb.failures = 0
	cb.mu.Unlock()

	cb.RecordFailure()
	if cb.State() != CircuitOpen {
		t.Errorf("state after half-open failure = %q, want %q", cb.State(), CircuitOpen)
	}
}

func TestCircuitBreakerWriteStateFile(t *testing.T) {
	cb := NewCircuitBreaker(3, 5*time.Minute, 60*time.Second)
	cb.RecordFailure()

	if err := cb.WriteStateFile(); err != nil {
		t.Fatalf("WriteStateFile() error: %v", err)
	}

	target := filepath.Join(coordDir, circuitStateFile)
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile(%s) error: %v", target, err)
	}

	var state circuitStateJSON
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if state.State != CircuitClosed {
		t.Errorf("persisted state = %q, want %q", state.State, CircuitClosed)
	}
	if state.Failures != 1 {
		t.Errorf("persisted failures = %d, want 1", state.Failures)
	}
	if state.LastFailure == nil {
		t.Error("persisted last_failure is nil, want non-nil")
	}

	// Cleanup
	os.Remove(target)
}

func TestCircuitBreakerConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(100, 5*time.Minute, 60*time.Second)
	done := make(chan struct{})
	for range 10 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 50 {
				cb.AllowSpawn()
				cb.RecordFailure()
				cb.State()
				cb.RecordSuccess()
			}
		}()
	}
	for range 10 {
		<-done
	}
	// Just verify no panics or deadlocks.
}
