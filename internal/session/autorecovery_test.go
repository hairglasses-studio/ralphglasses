package session

import (
	"testing"
)

func TestIsTransientError(t *testing.T) {
	transient := []string{
		"connection reset by peer",
		"request timeout exceeded",
		"rate limit exceeded (429)",
		"503 Service Unavailable",
		"ECONNREFUSED",
		"server is overloaded",
	}
	for _, msg := range transient {
		if !isTransientError(msg) {
			t.Errorf("expected transient: %q", msg)
		}
	}

	nonTransient := []string{
		"invalid API key",
		"file not found",
		"syntax error in prompt",
		"permission denied",
	}
	for _, msg := range nonTransient {
		if isTransientError(msg) {
			t.Errorf("expected non-transient: %q", msg)
		}
	}
}

func TestAutoRecoveryConfig_Defaults(t *testing.T) {
	config := DefaultAutoRecoveryConfig()
	if config.MaxRetries != 3 {
		t.Errorf("max retries: got %d, want 3", config.MaxRetries)
	}
	if config.BackoffFactor != 2.0 {
		t.Errorf("backoff: got %f, want 2.0", config.BackoffFactor)
	}
}
