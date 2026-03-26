package session

import (
	"strings"
	"testing"
)

func TestNormalizeLoopProfile_EmptyProfile(t *testing.T) {
	t.Parallel()

	// An empty profile should be normalized to defaults without error,
	// since normalizeLoopProfile fills in missing fields.
	profile, err := normalizeLoopProfile(LoopProfile{})
	if err != nil {
		t.Fatalf("unexpected error for empty profile: %v", err)
	}

	// Defaults should be populated.
	if profile.PlannerProvider == "" {
		t.Error("expected PlannerProvider to be filled from defaults")
	}
	if profile.WorkerProvider == "" {
		t.Error("expected WorkerProvider to be filled from defaults")
	}
	if profile.MaxConcurrentWorkers <= 0 {
		t.Error("expected MaxConcurrentWorkers > 0 from defaults")
	}
}

func TestNormalizeLoopProfile_InvalidProvider(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		PlannerProvider: "nonexistent_provider",
	}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Fatal("expected error for invalid provider")
	}
	if !strings.Contains(err.Error(), "nonexistent_provider") {
		t.Errorf("error should mention the invalid provider, got: %v", err)
	}
}

func TestNormalizeLoopProfile_NegativeRetryLimit(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		RetryLimit: -1,
	}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Fatal("expected error for negative retry limit")
	}
	if !strings.Contains(err.Error(), "retry limit") {
		t.Errorf("error should mention retry limit, got: %v", err)
	}
}

func TestNormalizeLoopProfile_TooManyWorkers(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		MaxConcurrentWorkers: 20,
	}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Fatal("expected error for >8 concurrent workers")
	}
	if !strings.Contains(err.Error(), "capped at 8") {
		t.Errorf("error should mention cap, got: %v", err)
	}
}

func TestNormalizeLoopProfile_UnsupportedWorktreePolicy(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		WorktreePolicy: "docker",
	}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Fatal("expected error for unsupported worktree policy")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Errorf("error should mention the policy, got: %v", err)
	}
}

func TestNormalizeLoopProfile_ValidProfile(t *testing.T) {
	t.Parallel()

	def := DefaultLoopProfile()
	profile := LoopProfile{
		PlannerProvider:      def.PlannerProvider,
		WorkerProvider:       def.WorkerProvider,
		VerifierProvider:     def.VerifierProvider,
		MaxConcurrentWorkers: 4,
		RetryLimit:           3,
		WorktreePolicy:       "git",
		VerifyCommands:       []string{"go test ./..."},
	}
	normalized, err := normalizeLoopProfile(profile)
	if err != nil {
		t.Fatalf("unexpected error for valid profile: %v", err)
	}
	if normalized.MaxConcurrentWorkers != 4 {
		t.Errorf("expected MaxConcurrentWorkers=4, got %d", normalized.MaxConcurrentWorkers)
	}
	if normalized.RetryLimit != 3 {
		t.Errorf("expected RetryLimit=3, got %d", normalized.RetryLimit)
	}
}

func TestNormalizeLoopProfile_RetryLimitZero(t *testing.T) {
	t.Parallel()

	// RetryLimit=0 should be fine (no retries).
	profile := LoopProfile{
		RetryLimit: 0,
	}
	_, err := normalizeLoopProfile(profile)
	if err != nil {
		t.Fatalf("unexpected error for RetryLimit=0: %v", err)
	}
}

func TestNormalizeLoopProfile_CompactionDefaults(t *testing.T) {
	t.Parallel()

	// CompactionEnabled with no threshold should get default threshold.
	profile := LoopProfile{
		CompactionEnabled: true,
	}
	normalized, err := normalizeLoopProfile(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized.CompactionThreshold <= 0 {
		t.Errorf("expected positive compaction threshold, got %d", normalized.CompactionThreshold)
	}
}

func TestNormalizeLoopProfile_CompactionExplicitThreshold(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		CompactionEnabled:   true,
		CompactionThreshold: 5,
	}
	normalized, err := normalizeLoopProfile(profile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if normalized.CompactionThreshold != 5 {
		t.Errorf("expected CompactionThreshold=5, got %d", normalized.CompactionThreshold)
	}
}

func TestNormalizeLoopProfile_WorkerProviderInvalid(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		WorkerProvider: "unknown_worker",
	}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Fatal("expected error for invalid worker provider")
	}
	if !strings.Contains(err.Error(), "unknown_worker") {
		t.Errorf("error should mention the provider name, got: %v", err)
	}
}

func TestNormalizeLoopProfile_VerifierProviderInvalid(t *testing.T) {
	t.Parallel()

	profile := LoopProfile{
		VerifierProvider: "bad_verifier",
	}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Fatal("expected error for invalid verifier provider")
	}
	if !strings.Contains(err.Error(), "bad_verifier") {
		t.Errorf("error should mention the provider name, got: %v", err)
	}
}
