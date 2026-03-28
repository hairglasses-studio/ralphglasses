package model

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNotFound", ErrNotFound, "not found"},
		{"ErrInvalidParams", ErrInvalidParams, "invalid parameters"},
		{"ErrBudgetExceeded", ErrBudgetExceeded, "budget exceeded"},
		{"ErrTimeout", ErrTimeout, "operation timed out"},
		{"ErrStalled", ErrStalled, "loop stalled"},
		{"ErrShuttingDown", ErrShuttingDown, "shutting down"},
		{"ErrAlreadyRunning", ErrAlreadyRunning, "already running"},
		{"ErrNotRunning", ErrNotRunning, "not running"},
	}
	for _, tt := range sentinels {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.msg {
				t.Errorf("%s.Error() = %q, want %q", tt.name, tt.err.Error(), tt.msg)
			}
		})
	}
}

func TestSentinelErrors_IsWrapped(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", ErrNotFound)
	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("wrapped ErrNotFound should match via errors.Is")
	}
	if errors.Is(wrapped, ErrTimeout) {
		t.Error("wrapped ErrNotFound should not match ErrTimeout")
	}
}

func TestConfigError(t *testing.T) {
	inner := ErrInvalidParams
	ce := &ConfigError{Key: "max_budget", Err: inner}

	if ce.Error() != "config error [max_budget]: invalid parameters" {
		t.Errorf("ConfigError.Error() = %q", ce.Error())
	}
	if ce.Unwrap() != inner {
		t.Error("ConfigError.Unwrap() should return inner error")
	}
	if !errors.Is(ce, ErrInvalidParams) {
		t.Error("errors.Is(ConfigError, ErrInvalidParams) should be true")
	}
}

func TestConfigError_As(t *testing.T) {
	err := fmt.Errorf("outer: %w", &ConfigError{Key: "scan_path", Err: ErrNotFound})
	var ce *ConfigError
	if !errors.As(err, &ce) {
		t.Fatal("errors.As should find ConfigError")
	}
	if ce.Key != "scan_path" {
		t.Errorf("ConfigError.Key = %q, want %q", ce.Key, "scan_path")
	}
}

func TestSessionError(t *testing.T) {
	inner := ErrTimeout
	se := &SessionError{ID: "sess-123", Err: inner}

	if se.Error() != "session sess-123: operation timed out" {
		t.Errorf("SessionError.Error() = %q", se.Error())
	}
	if se.Unwrap() != inner {
		t.Error("SessionError.Unwrap() should return inner error")
	}
	if !errors.Is(se, ErrTimeout) {
		t.Error("errors.Is(SessionError, ErrTimeout) should be true")
	}
}

func TestSessionError_As(t *testing.T) {
	err := fmt.Errorf("outer: %w", &SessionError{ID: "abc", Err: ErrStalled})
	var se *SessionError
	if !errors.As(err, &se) {
		t.Fatal("errors.As should find SessionError")
	}
	if se.ID != "abc" {
		t.Errorf("SessionError.ID = %q, want %q", se.ID, "abc")
	}
}

func TestLoopError(t *testing.T) {
	inner := ErrBudgetExceeded
	le := &LoopError{RunID: "run-456", Iteration: 3, Err: inner}

	want := "loop run-456 iteration 3: budget exceeded"
	if le.Error() != want {
		t.Errorf("LoopError.Error() = %q, want %q", le.Error(), want)
	}
	if le.Unwrap() != inner {
		t.Error("LoopError.Unwrap() should return inner error")
	}
	if !errors.Is(le, ErrBudgetExceeded) {
		t.Error("errors.Is(LoopError, ErrBudgetExceeded) should be true")
	}
}

func TestLoopError_As(t *testing.T) {
	err := fmt.Errorf("outer: %w", &LoopError{RunID: "run-789", Iteration: 5, Err: ErrNotRunning})
	var le *LoopError
	if !errors.As(err, &le) {
		t.Fatal("errors.As should find LoopError")
	}
	if le.RunID != "run-789" {
		t.Errorf("LoopError.RunID = %q, want %q", le.RunID, "run-789")
	}
	if le.Iteration != 5 {
		t.Errorf("LoopError.Iteration = %d, want 5", le.Iteration)
	}
}

func TestDoubleWrap(t *testing.T) {
	inner := &SessionError{ID: "s1", Err: ErrTimeout}
	outer := &LoopError{RunID: "r1", Iteration: 1, Err: inner}

	if !errors.Is(outer, ErrTimeout) {
		t.Error("double-wrapped ErrTimeout should be reachable via errors.Is")
	}
	var se *SessionError
	if !errors.As(outer, &se) {
		t.Error("errors.As should find SessionError inside LoopError")
	}
}
