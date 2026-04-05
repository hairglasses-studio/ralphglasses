package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildSynthesisFromFindings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		tasks        []CycleTask
		findings     []CycleFinding
		wantAccomp   int
		wantRemain   int
		wantPatterns int
	}{
		{
			"no tasks or findings",
			nil,
			nil,
			0, 0, 0,
		},
		{
			"all done",
			[]CycleTask{
				{Title: "task1", Status: "done"},
				{Title: "task2", Status: "done"},
			},
			nil,
			2, 0, 0,
		},
		{
			"mixed statuses",
			[]CycleTask{
				{Title: "done-task", Status: "done"},
				{Title: "failed-task", Status: "failed"},
				{Title: "pending-task", Status: "pending"},
				{Title: "executing-task", Status: "executing"},
			},
			[]CycleFinding{
				{Category: "failure", Description: "something broke"},
			},
			1, 3, 1,
		},
		{
			"all failed with findings",
			[]CycleTask{
				{Title: "t1", Status: "failed"},
			},
			[]CycleFinding{
				{Category: "regression", Description: "regressed"},
				{Category: "stall", Description: "stalled"},
			},
			0, 1, 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cycle := &CycleRun{
				Name:     "test",
				Tasks:    tt.tasks,
				Findings: tt.findings,
			}
			synthesis := buildSynthesisFromFindings(cycle)

			if len(synthesis.Accomplished) != tt.wantAccomp {
				t.Fatalf("accomplished: expected %d, got %d", tt.wantAccomp, len(synthesis.Accomplished))
			}
			if len(synthesis.Remaining) != tt.wantRemain {
				t.Fatalf("remaining: expected %d, got %d", tt.wantRemain, len(synthesis.Remaining))
			}
			if len(synthesis.Patterns) != tt.wantPatterns {
				t.Fatalf("patterns: expected %d, got %d", tt.wantPatterns, len(synthesis.Patterns))
			}
			if synthesis.Summary == "" {
				t.Fatal("expected non-empty summary")
			}
			if tt.wantRemain > 0 && synthesis.NextObjective == "" {
				t.Fatal("expected non-empty NextObjective when remaining > 0")
			}
		})
	}
}

func TestWaitForLoops_EmptyIDs(t *testing.T) {
	t.Parallel()
	m := NewManager()
	err := m.waitForLoops(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty IDs, got %v", err)
	}
}

func TestWaitForLoops_ContextCancelled(t *testing.T) {
	t.Parallel()
	m := NewManager()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// Use a short poll interval.
	old := cyclePollInterval
	cyclePollInterval = 10 * time.Millisecond
	defer func() { cyclePollInterval = old }()

	err := m.waitForLoops(ctx, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected context error")
	}
}

func TestWaitForLoops_UnknownIDsConsidered(t *testing.T) {
	t.Parallel()
	m := NewManager()

	old := cyclePollInterval
	cyclePollInterval = 10 * time.Millisecond
	defer func() { cyclePollInterval = old }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Unknown IDs should be considered done (deleted from pending).
	err := m.waitForLoops(ctx, []string{"unknown-1", "unknown-2"})
	if err != nil {
		t.Fatalf("expected nil for unknown IDs, got %v", err)
	}
}

func TestRunCycle_NoTasksPlanned(t *testing.T) {
	t.Parallel()
	disableCycleSafety(t)

	dir := t.TempDir()
	initGitRepoForCycle(t, dir)

	m := NewManager()

	old := cyclePollInterval
	cyclePollInterval = 10 * time.Millisecond
	defer func() { cyclePollInterval = old }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cycle, err := m.RunCycle(ctx, dir, "empty-cycle", "test objective", nil, 5)
	if err != nil {
		t.Fatalf("RunCycle failed: %v", err)
	}
	if cycle.Phase != CycleComplete {
		t.Fatalf("expected complete phase, got %s", cycle.Phase)
	}
	if cycle.Synthesis.Summary == "" {
		t.Fatal("expected non-empty synthesis summary")
	}
}

func TestCycleSafetyConfig_NilUseDefault(t *testing.T) {
	t.Parallel()
	old := CycleSafety
	CycleSafety = nil
	defer func() { CycleSafety = old }()

	cfg := cycleSafetyConfig()
	if cfg != DefaultCycleSafety {
		t.Fatalf("expected default config, got %+v", cfg)
	}
}

func TestCycleSafetyConfig_Custom(t *testing.T) {
	t.Parallel()
	custom := CycleSafetyConfig{MaxConcurrentCycles: 99}
	old := CycleSafety
	CycleSafety = &custom
	defer func() { CycleSafety = old }()

	cfg := cycleSafetyConfig()
	if cfg.MaxConcurrentCycles != 99 {
		t.Fatalf("expected 99, got %d", cfg.MaxConcurrentCycles)
	}
}

func TestCollectCycleFindings_WrongPhase(t *testing.T) {
	t.Parallel()
	m := NewManager()
	cycle := &CycleRun{Phase: CycleExecuting}
	err := m.CollectCycleFindings(cycle, nil)
	if err == nil {
		t.Fatal("expected error for wrong phase")
	}
}

func TestCollectCycleFindings_AllStatuses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepoForCycle(t, dir)
	m := NewManager()

	cycle := &CycleRun{
		Phase:    CycleObserving,
		RepoPath: dir,
		LoopIDs:  []string{"loop-1", "loop-2", "loop-3", "loop-4", "loop-5"},
	}

	observations := []LoopObservation{
		{LoopID: "loop-1", Status: "failed", TaskTitle: "t1", Error: "err1"},
		{LoopID: "loop-2", Status: "regressed", TaskTitle: "t2", Error: "err2"},
		{LoopID: "loop-3", Status: "noop", TaskTitle: "t3", Error: "noop"},
		{LoopID: "loop-4", Status: "stalled", TaskTitle: "t4", Error: "slow"},
		{LoopID: "loop-5", Status: "passed", TaskTitle: "t5"},       // should be skipped
		{LoopID: "unknown", Status: "failed", TaskTitle: "unknown"},  // should be skipped (not in loopIDs)
	}

	if err := m.CollectCycleFindings(cycle, observations); err != nil {
		t.Fatalf("CollectCycleFindings failed: %v", err)
	}

	if len(cycle.Findings) != 4 {
		t.Fatalf("expected 4 findings, got %d", len(cycle.Findings))
	}
}

func TestSetCycleSynthesis_WrongPhase(t *testing.T) {
	t.Parallel()
	m := NewManager()
	cycle := &CycleRun{Phase: CycleExecuting}
	err := m.SetCycleSynthesis(cycle, CycleSynthesis{Summary: "test"})
	if err == nil {
		t.Fatal("expected error for wrong phase")
	}
}

func TestLaunchCycleTask_WrongPhase(t *testing.T) {
	t.Parallel()
	m := NewManager()
	cycle := &CycleRun{Phase: CycleBaselining}
	_, err := m.LaunchCycleTask(context.Background(), cycle, 0, LaunchOptions{})
	if err == nil {
		t.Fatal("expected error for wrong phase")
	}
}

func TestLaunchCycleTask_OutOfRange(t *testing.T) {
	t.Parallel()
	m := NewManager()
	cycle := &CycleRun{
		Phase: CycleExecuting,
		Tasks: []CycleTask{{Title: "t1", Status: "pending"}},
	}
	_, err := m.LaunchCycleTask(context.Background(), cycle, 5, LaunchOptions{})
	if err == nil {
		t.Fatal("expected error for out of range index")
	}
}

func TestLaunchCycleTask_NotPending(t *testing.T) {
	t.Parallel()
	m := NewManager()
	cycle := &CycleRun{
		Phase: CycleExecuting,
		Tasks: []CycleTask{{Title: "t1", Status: "done"}},
	}
	_, err := m.LaunchCycleTask(context.Background(), cycle, 0, LaunchOptions{})
	if err == nil {
		t.Fatal("expected error for non-pending task")
	}
}

func TestPlanCycleTasks_WrongPhase(t *testing.T) {
	t.Parallel()
	m := NewManager()
	cycle := &CycleRun{Phase: CycleExecuting}
	err := m.PlanCycleTasks(cycle, nil, 5)
	if err == nil {
		t.Fatal("expected error for wrong phase")
	}
}

// initGitRepoForCycle initializes a bare git repo at the given path for testing.
func initGitRepoForCycle(t *testing.T, dir string) {
	t.Helper()
	// Create .git dir to pass the git repo check.
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create .ralph/cycles for cycle persistence.
	if err := os.MkdirAll(filepath.Join(dir, ".ralph", "cycles"), 0755); err != nil {
		t.Fatal(err)
	}
}
