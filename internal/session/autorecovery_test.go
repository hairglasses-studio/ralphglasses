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

func TestIsTransientError_CodexPatterns(t *testing.T) {
	// New Codex-specific transient patterns
	codexTransient := []string{
		"model_overloaded: try again later",
		"server_error: internal failure",
	}
	for _, msg := range codexTransient {
		if !isTransientError(msg) {
			t.Errorf("expected transient: %q", msg)
		}
	}
}

func TestIsTransientError_NonTransientOverride(t *testing.T) {
	// Non-transient patterns must override transient even if both match.
	// "quota exhausted" contains no transient pattern, so it's non-transient.
	nonTransient := []string{
		"subscription_usage_exhausted",
		"extra_usage_exhausted",
		"context_length_exceeded: max 128000 tokens",
		"budget exceeded: $5.00 of $5.00",
		"quota exhausted for codex",
	}
	for _, msg := range nonTransient {
		if isTransientError(msg) {
			t.Errorf("expected non-transient (should not retry): %q", msg)
		}
	}

	// Edge case: "overloaded" is transient, but "quota exhausted" takes priority
	mixed := "server overloaded and quota exhausted"
	if isTransientError(mixed) {
		t.Errorf("non-transient should override transient for: %q", mixed)
	}
}

func TestNewAutoRecovery(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)
	hitl := NewHITLTracker(dir)
	config := DefaultAutoRecoveryConfig()

	ar := NewAutoRecovery(nil, dl, hitl, config)
	if ar == nil {
		t.Fatal("expected non-nil AutoRecovery")
	}
}

func TestAutoRecovery_ClearRetryState(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)
	ar := NewAutoRecovery(nil, dl, nil, DefaultAutoRecoveryConfig())

	// Simulate retry state
	ar.retryState["session-1"] = &retryInfo{count: 2}

	ar.ClearRetryState("session-1")
	if _, exists := ar.retryState["session-1"]; exists {
		t.Error("expected retry state to be cleared")
	}

	// Clear nonexistent is a no-op
	ar.ClearRetryState("nonexistent")
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
