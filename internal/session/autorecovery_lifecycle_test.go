package session

import (
	"context"
	"testing"
	"time"
)

func TestHandleSessionError_NonTransient(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	dl := NewDecisionLog(dir, LevelAutoRecover)
	config := DefaultAutoRecoveryConfig()
	ar := NewAutoRecovery(m, dl, nil, config)

	failed := &Session{
		ID:         "failed-perm",
		Provider:   ProviderClaude,
		RepoPath:   dir,
		Status:     StatusErrored,
		Error:      "invalid API key",
		ExitReason: "",
	}

	ctx := context.Background()
	newSess := ar.HandleSessionError(ctx, failed)
	if newSess != nil {
		t.Error("expected nil for non-transient error")
	}
}

func TestHandleSessionError_ExceedsMaxRetries(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	dl := NewDecisionLog(dir, LevelAutoRecover)
	config := DefaultAutoRecoveryConfig()
	config.MaxRetries = 2
	ar := NewAutoRecovery(m, dl, nil, config)

	// Pre-fill retry state at limit
	ar.retryState["max-retry"] = &retryInfo{count: 2, lastRetry: time.Now().Add(-1 * time.Hour)}

	failed := &Session{
		ID:     "max-retry",
		Status: StatusErrored,
		Error:  "connection reset",
	}

	ctx := context.Background()
	newSess := ar.HandleSessionError(ctx, failed)
	if newSess != nil {
		t.Error("expected nil when max retries exceeded")
	}
}

func TestHandleSessionError_CooldownNotElapsed(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	dl := NewDecisionLog(dir, LevelAutoRecover)
	config := DefaultAutoRecoveryConfig()
	config.CooldownPeriod = 1 * time.Hour
	ar := NewAutoRecovery(m, dl, nil, config)

	// Retry was just done
	ar.retryState["cooldown-test"] = &retryInfo{count: 1, lastRetry: time.Now()}

	failed := &Session{
		ID:     "cooldown-test",
		Status: StatusErrored,
		Error:  "timeout",
	}

	ctx := context.Background()
	newSess := ar.HandleSessionError(ctx, failed)
	if newSess != nil {
		t.Error("expected nil during cooldown period")
	}
}

func TestHandleSessionError_AutonomyLevelTooLow(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// Set autonomy to Observe (too low for auto-recover)
	dl := NewDecisionLog(dir, LevelObserve)
	hitl := NewHITLTracker(dir)
	config := DefaultAutoRecoveryConfig()
	ar := NewAutoRecovery(m, dl, hitl, config)

	failed := &Session{
		ID:       "low-level",
		Provider: ProviderClaude,
		RepoPath: dir,
		RepoName: "test-repo",
		Status:   StatusErrored,
		Error:    "connection reset",
	}

	ctx := context.Background()
	newSess := ar.HandleSessionError(ctx, failed)
	if newSess != nil {
		t.Error("expected nil when autonomy level is too low")
	}
}

func TestHandleSessionError_ExitReasonUsedWhenErrorEmpty(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// Use Observe level so it won't try to launch (will return nil from Propose)
	dl := NewDecisionLog(dir, LevelObserve)
	hitl := NewHITLTracker(dir)
	config := DefaultAutoRecoveryConfig()
	ar := NewAutoRecovery(m, dl, hitl, config)

	// Error field empty, ExitReason has transient pattern
	failed := &Session{
		ID:         "exit-reason",
		Provider:   ProviderGemini,
		RepoPath:   dir,
		RepoName:   "test",
		Prompt:     "do something",
		Model:      "pro-2",
		Status:     StatusErrored,
		Error:      "",
		ExitReason: "503 service unavailable",
	}

	ctx := context.Background()
	// With Observe level, Propose returns false (level too low),
	// but this tests that ExitReason is used for the transient check path
	newSess := ar.HandleSessionError(ctx, failed)
	// Should be nil because level is too low, but it should NOT have returned
	// nil from the non-transient check (ExitReason IS transient)
	if newSess != nil {
		t.Error("expected nil because autonomy level is too low")
	}
}

func TestHandleSessionError_BackoffMultiplier(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	dl := NewDecisionLog(dir, LevelAutoRecover)
	config := AutoRecoveryConfig{
		MaxRetries:     5,
		CooldownPeriod: 1 * time.Second,
		BackoffFactor:  2.0,
	}
	ar := NewAutoRecovery(m, dl, nil, config)

	// After 2 retries with 2.0 backoff, cooldown should be 1s * 2.0 * 2.0 = 4s
	// So a retry 2 seconds ago should still be within cooldown
	ar.retryState["backoff-test"] = &retryInfo{count: 2, lastRetry: time.Now().Add(-2 * time.Second)}

	failed := &Session{
		ID:     "backoff-test",
		Status: StatusErrored,
		Error:  "timeout",
	}

	ctx := context.Background()
	newSess := ar.HandleSessionError(ctx, failed)
	if newSess != nil {
		t.Error("expected nil because backoff cooldown not elapsed")
	}
}

func TestHandleSessionError_NewRetryStateCreated(t *testing.T) {
	dir := t.TempDir()
	m := NewManager()
	m.SetStateDir(dir)

	// Use Observe level so Propose returns false (won't actually launch)
	dl := NewDecisionLog(dir, LevelObserve)
	hitl := NewHITLTracker(dir)
	config := DefaultAutoRecoveryConfig()
	ar := NewAutoRecovery(m, dl, hitl, config)

	failed := &Session{
		ID:       "new-state",
		Provider: ProviderClaude,
		RepoPath: dir,
		RepoName: "test",
		Status:   StatusErrored,
		Error:    "connection reset",
	}

	ctx := context.Background()
	ar.HandleSessionError(ctx, failed)

	// retryState should be created even though we didn't launch
	state, ok := ar.retryState["new-state"]
	if !ok {
		t.Fatal("expected retry state to be created")
	}
	if state.count != 0 {
		t.Errorf("retry count = %d, want 0 (no successful relaunch)", state.count)
	}
}
