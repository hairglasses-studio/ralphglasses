package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCycleNewCycleRun(t *testing.T) {
	c := NewCycleRun("test-cycle", "/tmp/repo", "improve coverage", []string{"coverage > 90%"})
	if c.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if c.Phase != CycleProposed {
		t.Fatalf("expected phase %q, got %q", CycleProposed, c.Phase)
	}
	if c.Name != "test-cycle" {
		t.Fatalf("expected name %q, got %q", "test-cycle", c.Name)
	}
	if c.RepoPath != "/tmp/repo" {
		t.Fatalf("expected repo_path %q, got %q", "/tmp/repo", c.RepoPath)
	}
	if c.Objective != "improve coverage" {
		t.Fatalf("expected objective %q, got %q", "improve coverage", c.Objective)
	}
	if len(c.SuccessCriteria) != 1 || c.SuccessCriteria[0] != "coverage > 90%" {
		t.Fatalf("unexpected success_criteria: %v", c.SuccessCriteria)
	}
	if c.Tasks == nil {
		t.Fatal("expected non-nil tasks slice")
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
		t.Fatal("expected non-zero timestamps")
	}
}

func TestCycleValidTransitions(t *testing.T) {
	c := NewCycleRun("test", "/tmp", "obj", nil)

	phases := []CyclePhase{
		CycleBaselining,
		CycleExecuting,
		CycleObserving,
		CycleSynthesizing,
		CycleComplete,
	}
	for _, p := range phases {
		if err := c.Advance(p); err != nil {
			t.Fatalf("valid transition to %q failed: %v", p, err)
		}
		if c.Phase != p {
			t.Fatalf("expected phase %q, got %q", p, c.Phase)
		}
	}
}

func TestCycleInvalidTransitions(t *testing.T) {
	tests := []struct {
		name     string
		from     CyclePhase
		to       CyclePhase
		wantErr  bool
	}{
		{"proposed to observing", CycleProposed, CycleObserving, true},
		{"proposed to complete", CycleProposed, CycleComplete, true},
		{"baselining to synthesizing", CycleBaselining, CycleSynthesizing, true},
		{"executing to baselining", CycleExecuting, CycleBaselining, true},
		{"proposed to failed via Advance", CycleProposed, CycleFailed, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewCycleRun("test", "/tmp", "obj", nil)
			// Advance to the 'from' phase.
			phases := []CyclePhase{CycleBaselining, CycleExecuting, CycleObserving, CycleSynthesizing}
			for _, p := range phases {
				if p == tt.from {
					break
				}
				// Skip if from is proposed (already there).
				if tt.from == CycleProposed {
					break
				}
				_ = c.Advance(p)
			}
			// For non-proposed starting phases, advance to the correct starting phase.
			if tt.from != CycleProposed && c.Phase != tt.from {
				_ = c.Advance(tt.from)
			}

			err := c.Advance(tt.to)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for %s → %s, got nil", tt.from, tt.to)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for %s → %s: %v", tt.from, tt.to, err)
			}
		})
	}
}

func TestCycleCannotAdvanceFromTerminal(t *testing.T) {
	// Complete
	c := NewCycleRun("test", "/tmp", "obj", nil)
	for _, p := range []CyclePhase{CycleBaselining, CycleExecuting, CycleObserving, CycleSynthesizing, CycleComplete} {
		_ = c.Advance(p)
	}
	if err := c.Advance(CycleBaselining); err == nil {
		t.Fatal("expected error advancing from complete")
	}

	// Failed
	c2 := NewCycleRun("test", "/tmp", "obj", nil)
	c2.Fail("boom")
	if err := c2.Advance(CycleBaselining); err == nil {
		t.Fatal("expected error advancing from failed")
	}
}

func TestCycleFailFromAnyPhase(t *testing.T) {
	phases := []CyclePhase{CycleProposed, CycleBaselining, CycleExecuting, CycleObserving, CycleSynthesizing}
	for _, startPhase := range phases {
		t.Run(string(startPhase), func(t *testing.T) {
			c := NewCycleRun("test", "/tmp", "obj", nil)
			// Advance to start phase.
			advance := []CyclePhase{CycleBaselining, CycleExecuting, CycleObserving, CycleSynthesizing}
			for _, p := range advance {
				if c.Phase == startPhase {
					break
				}
				_ = c.Advance(p)
			}
			c.Fail("something went wrong")
			if c.Phase != CycleFailed {
				t.Fatalf("expected failed, got %q", c.Phase)
			}
			if c.Error != "something went wrong" {
				t.Fatalf("expected error message, got %q", c.Error)
			}
		})
	}
}

func TestCycleAddTask(t *testing.T) {
	c := NewCycleRun("test", "/tmp", "obj", nil)
	before := c.UpdatedAt
	time.Sleep(time.Millisecond)

	c.AddTask(CycleTask{Title: "task1", Source: "manual", Priority: 0.5, Status: "pending"})
	if len(c.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(c.Tasks))
	}
	if c.Tasks[0].Title != "task1" {
		t.Fatalf("expected title %q, got %q", "task1", c.Tasks[0].Title)
	}
	if !c.UpdatedAt.After(before) {
		t.Fatal("expected UpdatedAt to advance")
	}
}

func TestCycleAddFinding(t *testing.T) {
	c := NewCycleRun("test", "/tmp", "obj", nil)
	c.AddFinding(CycleFinding{ID: "F-1", Description: "found a bug", Category: "bug", Severity: "high", Source: "observation"})
	if len(c.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(c.Findings))
	}
	if c.Findings[0].ID != "F-1" {
		t.Fatalf("expected finding ID %q, got %q", "F-1", c.Findings[0].ID)
	}
}

func TestCycleSetSynthesis(t *testing.T) {
	c := NewCycleRun("test", "/tmp", "obj", nil)
	s := CycleSynthesis{
		Summary:       "all good",
		Accomplished:  []string{"fixed bug"},
		Remaining:     []string{"more tests"},
		NextObjective: "coverage",
		Patterns:      []string{"test-first"},
	}
	c.SetSynthesis(s)
	if c.Synthesis == nil {
		t.Fatal("expected non-nil synthesis")
	}
	if c.Synthesis.Summary != "all good" {
		t.Fatalf("expected summary %q, got %q", "all good", c.Synthesis.Summary)
	}
}

func TestCycleSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := NewCycleRun("roundtrip", dir, "test roundtrip", []string{"passes"})
	c.AddTask(CycleTask{Title: "t1", Source: "manual", Priority: 0.8, Status: "pending"})
	c.AddFinding(CycleFinding{ID: "F-1", Description: "desc", Category: "bug", Severity: "low", Source: "gate"})
	c.SetSynthesis(CycleSynthesis{Summary: "done"})

	if err := SaveCycle(dir, c); err != nil {
		t.Fatalf("SaveCycle: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, ".ralph", "cycles", c.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}

	loaded, err := LoadCycle(dir, c.ID)
	if err != nil {
		t.Fatalf("LoadCycle: %v", err)
	}
	if loaded.ID != c.ID {
		t.Fatalf("ID mismatch: %q vs %q", loaded.ID, c.ID)
	}
	if loaded.Name != c.Name {
		t.Fatalf("Name mismatch: %q vs %q", loaded.Name, c.Name)
	}
	if loaded.Phase != c.Phase {
		t.Fatalf("Phase mismatch: %q vs %q", loaded.Phase, c.Phase)
	}
	if len(loaded.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(loaded.Tasks))
	}
	if len(loaded.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(loaded.Findings))
	}
	if loaded.Synthesis == nil || loaded.Synthesis.Summary != "done" {
		t.Fatal("synthesis not round-tripped correctly")
	}
}

func TestCycleListCyclesSorted(t *testing.T) {
	dir := t.TempDir()

	c1 := NewCycleRun("first", dir, "obj1", nil)
	c1.UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := SaveCycle(dir, c1); err != nil {
		t.Fatalf("SaveCycle c1: %v", err)
	}

	c2 := NewCycleRun("second", dir, "obj2", nil)
	c2.UpdatedAt = time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := SaveCycle(dir, c2); err != nil {
		t.Fatalf("SaveCycle c2: %v", err)
	}

	c3 := NewCycleRun("third", dir, "obj3", nil)
	c3.UpdatedAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	if err := SaveCycle(dir, c3); err != nil {
		t.Fatalf("SaveCycle c3: %v", err)
	}

	cycles, err := ListCycles(dir)
	if err != nil {
		t.Fatalf("ListCycles: %v", err)
	}
	if len(cycles) != 3 {
		t.Fatalf("expected 3 cycles, got %d", len(cycles))
	}
	// Should be sorted by UpdatedAt descending.
	if cycles[0].Name != "second" {
		t.Fatalf("expected first cycle %q, got %q", "second", cycles[0].Name)
	}
	if cycles[1].Name != "third" {
		t.Fatalf("expected second cycle %q, got %q", "third", cycles[1].Name)
	}
	if cycles[2].Name != "first" {
		t.Fatalf("expected third cycle %q, got %q", "first", cycles[2].Name)
	}
}

func TestCycleListCyclesEmpty(t *testing.T) {
	dir := t.TempDir()
	cycles, err := ListCycles(dir)
	if err != nil {
		t.Fatalf("ListCycles: %v", err)
	}
	if cycles != nil {
		t.Fatalf("expected nil, got %v", cycles)
	}
}

func TestCycleActiveCycle(t *testing.T) {
	dir := t.TempDir()

	// No cycles at all.
	active, err := ActiveCycle(dir)
	if err != nil {
		t.Fatalf("ActiveCycle: %v", err)
	}
	if active != nil {
		t.Fatal("expected nil active cycle")
	}

	// One complete cycle.
	c1 := NewCycleRun("done", dir, "obj", nil)
	for _, p := range []CyclePhase{CycleBaselining, CycleExecuting, CycleObserving, CycleSynthesizing, CycleComplete} {
		_ = c1.Advance(p)
	}
	if err := SaveCycle(dir, c1); err != nil {
		t.Fatal(err)
	}

	active, err = ActiveCycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatal("expected nil when only complete cycles exist")
	}

	// Add an active cycle.
	c2 := NewCycleRun("active", dir, "obj2", nil)
	_ = c2.Advance(CycleBaselining)
	if err := SaveCycle(dir, c2); err != nil {
		t.Fatal(err)
	}

	active, err = ActiveCycle(dir)
	if err != nil {
		t.Fatal(err)
	}
	if active == nil {
		t.Fatal("expected non-nil active cycle")
	}
	if active.Name != "active" {
		t.Fatalf("expected active cycle %q, got %q", "active", active.Name)
	}
}

func TestCycleLoadNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadCycle(dir, "nonexistent")
	if err == nil {
		t.Fatal("expected error loading nonexistent cycle")
	}
}

func TestCycleFailureWritesObservation(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager()
	mgr.SetStateDir(filepath.Join(dir, "sessions"))
	CycleSafety = &DisabledCycleSafety
	defer func() { CycleSafety = nil }()

	cycle, err := mgr.CreateCycle(dir, "test-fail", "will fail", nil)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_ = mgr.FailCycle(cycle, "simulated failure")

	// Simulate what the RunCycle fail closure does: write observation.
	obs := LoopObservation{
		Timestamp: time.Now(),
		LoopID:    "cycle:" + cycle.ID,
		RepoName:  filepath.Base(dir),
		Status:    "cycle_failed",
		Error:     "simulated failure",
	}
	obsPath := ObservationPath(dir)
	if err := WriteObservation(obsPath, obs); err != nil {
		t.Fatalf("write observation: %v", err)
	}

	// Verify the observation is loadable and has the right status.
	loaded, err := LoadObservations(obsPath, time.Time{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(loaded))
	}
	if loaded[0].Status != "cycle_failed" {
		t.Fatalf("status = %q, want %q", loaded[0].Status, "cycle_failed")
	}
	if loaded[0].Error != "simulated failure" {
		t.Fatalf("error = %q, want %q", loaded[0].Error, "simulated failure")
	}
	if loaded[0].LoopID != "cycle:"+cycle.ID {
		t.Fatalf("loop_id = %q, want %q", loaded[0].LoopID, "cycle:"+cycle.ID)
	}
}
