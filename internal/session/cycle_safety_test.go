package session

import (
	"errors"
	"testing"
	"time"
)

func TestValidateCycleAdvance_BlocksWhenTooOld(t *testing.T) {
	cycle := &CycleRun{
		ID:        "test-old",
		Phase:     CycleExecuting,
		CreatedAt: time.Now().Add(-48 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}
	config := DefaultCycleSafety

	err := ValidateCycleAdvance(cycle, config)
	if err == nil {
		t.Fatal("expected error for old cycle, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected CycleSafetyError, got %T", err)
	}
	if safetyErr.Check != "max_cycle_age" {
		t.Errorf("expected check=max_cycle_age, got %s", safetyErr.Check)
	}
}

func TestValidateCycleAdvance_BlocksWhenCooldownNotElapsed(t *testing.T) {
	cycle := &CycleRun{
		ID:        "test-cooldown",
		Phase:     CycleExecuting,
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-5 * time.Second), // only 5s ago
	}
	config := DefaultCycleSafety // PhaseCooldown = 30s

	err := ValidateCycleAdvance(cycle, config)
	if err == nil {
		t.Fatal("expected error for cooldown, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected CycleSafetyError, got %T", err)
	}
	if safetyErr.Check != "phase_cooldown" {
		t.Errorf("expected check=phase_cooldown, got %s", safetyErr.Check)
	}
}

func TestValidateCycleAdvance_RequiresBaselineBeforeExecuting(t *testing.T) {
	cycle := &CycleRun{
		ID:         "test-baseline",
		Phase:      CycleBaselining,
		BaselineID: "", // no baseline set
		CreatedAt:  time.Now().Add(-1 * time.Hour),
		UpdatedAt:  time.Now().Add(-2 * time.Minute),
	}
	config := DefaultCycleSafety

	err := ValidateCycleAdvance(cycle, config)
	if err == nil {
		t.Fatal("expected error for missing baseline, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected CycleSafetyError, got %T", err)
	}
	if safetyErr.Check != "require_baseline" {
		t.Errorf("expected check=require_baseline, got %s", safetyErr.Check)
	}
}

func TestValidateCycleAdvance_BlocksWhenTooManyTasks(t *testing.T) {
	tasks := make([]CycleTask, 15)
	for i := range tasks {
		tasks[i] = CycleTask{Title: "task", Status: "pending"}
	}
	cycle := &CycleRun{
		ID:        "test-tasks",
		Phase:     CycleExecuting,
		Tasks:     tasks,
		CreatedAt: time.Now().Add(-1 * time.Hour),
		UpdatedAt: time.Now().Add(-2 * time.Minute),
	}
	config := DefaultCycleSafety

	err := ValidateCycleAdvance(cycle, config)
	if err == nil {
		t.Fatal("expected error for too many tasks, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected CycleSafetyError, got %T", err)
	}
	if safetyErr.Check != "max_tasks" {
		t.Errorf("expected check=max_tasks, got %s", safetyErr.Check)
	}
}

func TestValidateCycleAdvance_PassesWhenAllChecksPass(t *testing.T) {
	cycle := &CycleRun{
		ID:         "test-ok",
		Phase:      CycleExecuting,
		BaselineID: "baseline-123",
		Tasks:      []CycleTask{{Title: "task1", Status: "pending"}},
		CreatedAt:  time.Now().Add(-1 * time.Hour),
		UpdatedAt:  time.Now().Add(-2 * time.Minute),
	}
	config := DefaultCycleSafety

	if err := ValidateCycleAdvance(cycle, config); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateCycleStart_BlocksAtConcurrentLimit(t *testing.T) {
	existing := []*CycleRun{
		{RepoPath: "/repo", Phase: CycleExecuting},
		{RepoPath: "/repo", Phase: CycleBaselining},
	}
	config := DefaultCycleSafety // MaxConcurrentCycles = 2

	err := ValidateCycleStart("/repo", existing, config)
	if err == nil {
		t.Fatal("expected error at concurrent limit, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected CycleSafetyError, got %T", err)
	}
	if safetyErr.Check != "max_concurrent_cycles" {
		t.Errorf("expected check=max_concurrent_cycles, got %s", safetyErr.Check)
	}
}

func TestValidateCycleStart_PassesUnderLimit(t *testing.T) {
	existing := []*CycleRun{
		{RepoPath: "/repo", Phase: CycleExecuting},
		{RepoPath: "/repo", Phase: CycleComplete}, // terminal, doesn't count
	}
	config := DefaultCycleSafety

	if err := ValidateCycleStart("/repo", existing, config); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestValidateCycleStart_IgnoresOtherRepos(t *testing.T) {
	existing := []*CycleRun{
		{RepoPath: "/other-repo", Phase: CycleExecuting},
		{RepoPath: "/other-repo", Phase: CycleBaselining},
	}
	config := DefaultCycleSafety

	if err := ValidateCycleStart("/repo", existing, config); err != nil {
		t.Fatalf("expected nil error for different repo, got %v", err)
	}
}

func TestValidateCycleAdvance_BatchSkipsCooldown(t *testing.T) {
	// A cycle just advanced (UpdatedAt = now) — normally blocked by 30s cooldown.
	cycle := &CycleRun{
		ID:         "batch-test",
		Phase:      CycleBaselining,
		BaselineID: "b-1",
		CreatedAt:  time.Now().Add(-1 * time.Hour),
		UpdatedAt:  time.Now(), // just advanced
	}
	config := DefaultCycleSafety // PhaseCooldown = 30s

	// Without batch: should fail.
	err := ValidateCycleAdvance(cycle, config)
	if err == nil {
		t.Fatal("expected cooldown error without batch option, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) || safetyErr.Check != "phase_cooldown" {
		t.Fatalf("expected phase_cooldown error, got %v", err)
	}

	// With batch: should pass.
	if err := ValidateCycleAdvance(cycle, config, WithBatchAdvance()); err != nil {
		t.Fatalf("expected batch advance to skip cooldown, got %v", err)
	}
}

func TestValidateCycleAdvance_BatchStillEnforcesAge(t *testing.T) {
	// Batch advance skips cooldown but NOT the max age check.
	cycle := &CycleRun{
		ID:        "batch-age",
		Phase:     CycleExecuting,
		CreatedAt: time.Now().Add(-48 * time.Hour), // well past 24h max age
		UpdatedAt: time.Now(),
	}
	config := DefaultCycleSafety

	err := ValidateCycleAdvance(cycle, config, WithBatchAdvance())
	if err == nil {
		t.Fatal("expected age error even with batch, got nil")
	}
	var safetyErr *CycleSafetyError
	if !errors.As(err, &safetyErr) || safetyErr.Check != "max_cycle_age" {
		t.Fatalf("expected max_cycle_age error, got %v", err)
	}
}
