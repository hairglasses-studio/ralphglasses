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
