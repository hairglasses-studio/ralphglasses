package session

import (
	"errors"
	"testing"
	"time"
)

// TestCycleSafetyEndToEnd verifies that safety gates fire at the right times
// during cycle phase transitions.
func TestCycleSafetyEndToEnd(t *testing.T) {
	config := DefaultCycleSafety

	t.Run("phase_cooldown_blocks_rapid_advance", func(t *testing.T) {
		// Create a cycle that was just updated (within cooldown window).
		now := time.Now()
		cycle := &CycleRun{
			ID:         "e2e-cooldown",
			Phase:      CycleExecuting,
			BaselineID: "bl-1",
			Tasks:      []CycleTask{{Title: "t1", Status: "pending"}},
			CreatedAt:  now.Add(-1 * time.Hour),
			UpdatedAt:  now.Add(-1 * time.Second), // 1s ago, well within 30s cooldown
		}

		err := ValidateCycleAdvance(cycle, config)
		if err == nil {
			t.Fatal("expected cooldown to block advance")
		}
		var safetyErr *CycleSafetyError
		if !errors.As(err, &safetyErr) {
			t.Fatalf("expected CycleSafetyError, got %T: %v", err, err)
		}
		if safetyErr.Check != "phase_cooldown" {
			t.Errorf("expected check=phase_cooldown, got %s", safetyErr.Check)
		}

		// After cooldown elapses, advance should succeed.
		cycle.UpdatedAt = now.Add(-2 * time.Minute)
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("expected advance to succeed after cooldown, got: %v", err)
		}
	})

	t.Run("baseline_requirement_blocks_executing_without_baseline", func(t *testing.T) {
		now := time.Now()
		cycle := &CycleRun{
			ID:         "e2e-baseline",
			Phase:      CycleBaselining,
			BaselineID: "", // no baseline
			CreatedAt:  now.Add(-1 * time.Hour),
			UpdatedAt:  now.Add(-2 * time.Minute),
		}

		err := ValidateCycleAdvance(cycle, config)
		if err == nil {
			t.Fatal("expected missing baseline to block advance")
		}
		var safetyErr *CycleSafetyError
		if !errors.As(err, &safetyErr) {
			t.Fatalf("expected CycleSafetyError, got %T: %v", err, err)
		}
		if safetyErr.Check != "require_baseline" {
			t.Errorf("expected check=require_baseline, got %s", safetyErr.Check)
		}

		// With baseline set, advance should succeed.
		cycle.BaselineID = "bl-123"
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("expected advance to succeed with baseline, got: %v", err)
		}
	})

	t.Run("max_age_blocks_advance_on_stale_cycle", func(t *testing.T) {
		now := time.Now()
		cycle := &CycleRun{
			ID:         "e2e-stale",
			Phase:      CycleObserving,
			BaselineID: "bl-1",
			Tasks:      []CycleTask{{Title: "t1", Status: "done"}},
			CreatedAt:  now.Add(-48 * time.Hour), // 48h ago, exceeds 24h max
			UpdatedAt:  now.Add(-2 * time.Minute),
		}

		err := ValidateCycleAdvance(cycle, config)
		if err == nil {
			t.Fatal("expected max age to block advance")
		}
		var safetyErr *CycleSafetyError
		if !errors.As(err, &safetyErr) {
			t.Fatalf("expected CycleSafetyError, got %T: %v", err, err)
		}
		if safetyErr.Check != "max_cycle_age" {
			t.Errorf("expected check=max_cycle_age, got %s", safetyErr.Check)
		}
	})

	t.Run("concurrent_limit_blocks_new_cycle_when_at_max", func(t *testing.T) {
		existing := []*CycleRun{
			{RepoPath: "/test-repo", Phase: CycleExecuting},
			{RepoPath: "/test-repo", Phase: CycleBaselining},
		}
		// Default limit is 2 concurrent cycles.
		err := ValidateCycleStart("/test-repo", existing, config)
		if err == nil {
			t.Fatal("expected concurrent limit to block new cycle")
		}
		var safetyErr *CycleSafetyError
		if !errors.As(err, &safetyErr) {
			t.Fatalf("expected CycleSafetyError, got %T: %v", err, err)
		}
		if safetyErr.Check != "max_concurrent_cycles" {
			t.Errorf("expected check=max_concurrent_cycles, got %s", safetyErr.Check)
		}

		// When one completes, a new cycle should be allowed.
		existing[1].Phase = CycleComplete
		if err := ValidateCycleStart("/test-repo", existing, config); err != nil {
			t.Fatalf("expected new cycle allowed after one completes, got: %v", err)
		}
	})

	t.Run("full_lifecycle_with_safety", func(t *testing.T) {
		// Verify a cycle can pass through all gates when properly configured.
		now := time.Now()
		cycle := &CycleRun{
			ID:         "e2e-full",
			Phase:      CycleProposed,
			BaselineID: "",
			Tasks:      []CycleTask{{Title: "t1", Status: "pending"}},
			CreatedAt:  now.Add(-1 * time.Hour),
			UpdatedAt:  now.Add(-2 * time.Minute),
		}

		// proposed -> baselining: no baseline check needed here
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("proposed->baselining should pass: %v", err)
		}
		cycle.Phase = CycleBaselining
		cycle.UpdatedAt = now.Add(-2 * time.Minute)

		// baselining -> executing: requires baseline
		cycle.BaselineID = "bl-real"
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("baselining->executing should pass with baseline: %v", err)
		}
		cycle.Phase = CycleExecuting
		cycle.UpdatedAt = now.Add(-2 * time.Minute)

		// executing -> observing
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("executing->observing should pass: %v", err)
		}
		cycle.Phase = CycleObserving
		cycle.UpdatedAt = now.Add(-2 * time.Minute)

		// observing -> synthesizing
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("observing->synthesizing should pass: %v", err)
		}
		cycle.Phase = CycleSynthesizing
		cycle.UpdatedAt = now.Add(-2 * time.Minute)

		// synthesizing -> complete
		if err := ValidateCycleAdvance(cycle, config); err != nil {
			t.Fatalf("synthesizing->complete should pass: %v", err)
		}
	})
}

// TestCycleSafetyDefaultConfig verifies that DefaultCycleSafety values are
// reasonable — not zero (which would disable checks) and not absurdly high.
func TestCycleSafetyDefaultConfig(t *testing.T) {
	cfg := DefaultCycleSafety

	if cfg.MaxConcurrentCycles <= 0 {
		t.Errorf("MaxConcurrentCycles = %d, should be positive", cfg.MaxConcurrentCycles)
	}
	if cfg.MaxConcurrentCycles > 10 {
		t.Errorf("MaxConcurrentCycles = %d, unreasonably high", cfg.MaxConcurrentCycles)
	}

	if cfg.MaxTasksPerCycle <= 0 {
		t.Errorf("MaxTasksPerCycle = %d, should be positive", cfg.MaxTasksPerCycle)
	}
	if cfg.MaxTasksPerCycle > 100 {
		t.Errorf("MaxTasksPerCycle = %d, unreasonably high", cfg.MaxTasksPerCycle)
	}

	if cfg.PhaseCooldown <= 0 {
		t.Errorf("PhaseCooldown = %v, should be positive", cfg.PhaseCooldown)
	}
	if cfg.PhaseCooldown > 10*time.Minute {
		t.Errorf("PhaseCooldown = %v, unreasonably high", cfg.PhaseCooldown)
	}

	if !cfg.RequireBaseline {
		t.Error("RequireBaseline should be true by default")
	}

	if cfg.MaxCycleAge <= 0 {
		t.Errorf("MaxCycleAge = %v, should be positive", cfg.MaxCycleAge)
	}
	if cfg.MaxCycleAge > 7*24*time.Hour {
		t.Errorf("MaxCycleAge = %v, unreasonably high (> 1 week)", cfg.MaxCycleAge)
	}
}

// TestCycleSafetyDisabledConfig verifies that DisabledCycleSafety permits
// everything (for tests and migrations).
func TestCycleSafetyDisabledConfig(t *testing.T) {
	cfg := DisabledCycleSafety

	// With disabled config, all checks should pass.
	now := time.Now()
	cycle := &CycleRun{
		ID:         "disabled-test",
		Phase:      CycleBaselining,
		BaselineID: "", // no baseline, should still pass
		Tasks:      make([]CycleTask, 50),
		CreatedAt:  now.Add(-100 * 24 * time.Hour), // very old
		UpdatedAt:  now,                              // just updated
	}

	if err := ValidateCycleAdvance(cycle, cfg); err != nil {
		t.Fatalf("disabled safety should pass all checks, got: %v", err)
	}

	existing := make([]*CycleRun, 20)
	for i := range existing {
		existing[i] = &CycleRun{RepoPath: "/repo", Phase: CycleExecuting}
	}
	if err := ValidateCycleStart("/repo", existing, cfg); err != nil {
		t.Fatalf("disabled safety should allow unlimited concurrent cycles, got: %v", err)
	}
}
