package process

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
		wrapped  error
	}{
		{"ErrAlreadyRunning", ErrAlreadyRunning, fmt.Errorf("%w: my-repo", ErrAlreadyRunning)},
		{"ErrNoLoopScript", ErrNoLoopScript, fmt.Errorf("%w: my-repo", ErrNoLoopScript)},
		{"ErrNotRunning", ErrNotRunning, fmt.Errorf("%w: my-repo", ErrNotRunning)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !errors.Is(tt.wrapped, tt.sentinel) {
				t.Errorf("errors.Is(%v, %v) = false, want true", tt.wrapped, tt.sentinel)
			}
		})
	}
}

func TestSentinelErrorMessages(t *testing.T) {
	wrapped := fmt.Errorf("%w: my-repo", ErrAlreadyRunning)
	want := "loop already running: my-repo"
	if got := wrapped.Error(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
