package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagerCreateCycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, err := m.CreateCycle(repoPath, "test-cycle", "improve coverage", []string{"80% coverage"})
	if err != nil {
		t.Fatal(err)
	}
	if cycle.ID == "" {
		t.Error("expected non-empty cycle ID")
	}
	if cycle.Phase != CycleProposed {
		t.Errorf("expected proposed, got %s", cycle.Phase)
	}

	// Verify persisted to disk.
	loaded, err := LoadCycle(repoPath, cycle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "test-cycle" {
		t.Errorf("expected test-cycle, got %s", loaded.Name)
	}
}

func TestManagerGetCycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	got, err := m.GetCycle(repoPath, cycle.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != cycle.ID {
		t.Errorf("expected %s, got %s", cycle.ID, got.ID)
	}
}

func TestManagerGetCycle_NotFound(t *testing.T) {
	m := NewManager()
	_, err := m.GetCycle(t.TempDir(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent cycle")
	}
}

func TestManagerGetActiveCycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	// No cycles → nil.
	active, err := m.GetActiveCycle(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Error("expected nil active cycle")
	}

	// Create one → active.
	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	active, err = m.GetActiveCycle(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.ID != cycle.ID {
		t.Error("expected active cycle to match created cycle")
	}
}

func TestManagerAdvanceCycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)

	// Advance proposed → baselining.
	if err := m.AdvanceCycle(cycle); err != nil {
		t.Fatal(err)
	}
	if cycle.Phase != CycleBaselining {
		t.Errorf("expected baselining, got %s", cycle.Phase)
	}

	// Verify persisted.
	loaded, _ := LoadCycle(repoPath, cycle.ID)
	if loaded.Phase != CycleBaselining {
		t.Errorf("persisted phase: expected baselining, got %s", loaded.Phase)
	}
}

func TestManagerAdvanceCycle_FullPath(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "full", "obj", nil)
	phases := []CyclePhase{CycleBaselining, CycleExecuting, CycleObserving, CycleSynthesizing, CycleComplete}
	for _, expected := range phases {
		if err := m.AdvanceCycle(cycle); err != nil {
			t.Fatalf("advance to %s: %v", expected, err)
		}
		if cycle.Phase != expected {
			t.Errorf("expected %s, got %s", expected, cycle.Phase)
		}
	}

	// Cannot advance past complete.
	if err := m.AdvanceCycle(cycle); err == nil {
		t.Error("expected error advancing past complete")
	}
}

func TestManagerFailCycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	if err := m.FailCycle(cycle, "something broke"); err != nil {
		t.Fatal(err)
	}
	if cycle.Phase != CycleFailed {
		t.Errorf("expected failed, got %s", cycle.Phase)
	}
	if cycle.Error != "something broke" {
		t.Errorf("expected error message, got %q", cycle.Error)
	}

	// Verify persisted.
	loaded, _ := LoadCycle(repoPath, cycle.ID)
	if loaded.Phase != CycleFailed {
		t.Errorf("persisted: expected failed, got %s", loaded.Phase)
	}
}

func TestManagerPlanCycleTasks(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	// Create a ROADMAP.md with unchecked items.
	roadmap := "## Features\n- [ ] Add widget support\n- [x] Done item\n- [ ] Fix flaky test\n"
	os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte(roadmap), 0o644)

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	// Advance to baselining.
	m.AdvanceCycle(cycle)

	observations := []LoopObservation{
		{Status: "failed", TaskTitle: "build broke", Error: "compile error", LoopID: "loop-1"},
		{Status: "noop", TaskTitle: "nothing happened", LoopID: "loop-2", IterationNumber: 1},
	}

	if err := m.PlanCycleTasks(cycle, observations, 10); err != nil {
		t.Fatal(err)
	}

	if len(cycle.Tasks) == 0 {
		t.Error("expected at least one task")
	}

	// Should have both observation-derived and roadmap-derived tasks.
	hasObs, hasRoadmap := false, false
	for _, task := range cycle.Tasks {
		if task.Source == "finding" {
			hasObs = true
		}
		if task.Source == "roadmap" {
			hasRoadmap = true
		}
	}
	if !hasObs {
		t.Error("expected observation-derived tasks")
	}
	if !hasRoadmap {
		t.Error("expected roadmap-derived tasks")
	}
}

func TestManagerPlanCycleTasks_WrongPhase(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	// Still in proposed — should fail.
	err := m.PlanCycleTasks(cycle, nil, 10)
	if err == nil {
		t.Error("expected error for wrong phase")
	}
}

func TestManagerCollectCycleFindings(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	// Fast-forward to observing.
	cycle.Advance(CycleBaselining)
	cycle.Advance(CycleExecuting)
	cycle.LoopIDs = []string{"loop-A", "loop-B"}
	cycle.Advance(CycleObserving)
	SaveCycle(repoPath, cycle)

	observations := []LoopObservation{
		{LoopID: "loop-A", Status: "failed", TaskTitle: "build", Error: "compile error"},
		{LoopID: "loop-B", Status: "noop", TaskTitle: "lint"},
		{LoopID: "loop-C", Status: "failed", TaskTitle: "unrelated"}, // not in cycle
		{LoopID: "loop-A", Status: "completed", TaskTitle: "test"},   // pass — skipped
	}

	if err := m.CollectCycleFindings(cycle, observations); err != nil {
		t.Fatal(err)
	}

	if len(cycle.Findings) != 2 {
		t.Errorf("expected 2 findings, got %d", len(cycle.Findings))
	}
}

func TestManagerCollectCycleFindings_WrongPhase(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	err := m.CollectCycleFindings(cycle, nil)
	if err == nil {
		t.Error("expected error for wrong phase")
	}
}

func TestManagerSetCycleSynthesis(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	// Fast-forward to synthesizing.
	cycle.Advance(CycleBaselining)
	cycle.Advance(CycleExecuting)
	cycle.Advance(CycleObserving)
	cycle.Advance(CycleSynthesizing)
	SaveCycle(repoPath, cycle)

	synthesis := CycleSynthesis{
		Summary:       "Improved coverage from 80% to 86%",
		Accomplished:  []string{"Fixed 3 flaky tests"},
		Remaining:     []string{"QW-7 still open"},
		NextObjective: "Target 90% coverage",
		Patterns:      []string{"Test flakiness correlates with I/O mocking"},
	}

	if err := m.SetCycleSynthesis(cycle, synthesis); err != nil {
		t.Fatal(err)
	}

	if cycle.Synthesis == nil {
		t.Fatal("expected synthesis to be set")
	}
	if cycle.Synthesis.Summary != "Improved coverage from 80% to 86%" {
		t.Error("synthesis summary mismatch")
	}

	// Verify persisted.
	loaded, _ := LoadCycle(repoPath, cycle.ID)
	if loaded.Synthesis == nil || loaded.Synthesis.NextObjective != "Target 90% coverage" {
		t.Error("persisted synthesis mismatch")
	}
}

func TestManagerSetCycleSynthesis_WrongPhase(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	err := m.SetCycleSynthesis(cycle, CycleSynthesis{})
	if err == nil {
		t.Error("expected error for wrong phase")
	}
}

func TestManagerLaunchCycleTask_WrongPhase(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	_, err := m.LaunchCycleTask(context.Background(), cycle, 0, LaunchOptions{})
	if err == nil {
		t.Error("expected error for wrong phase")
	}
}

func TestManagerLaunchCycleTask_OutOfRange(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	cycle, _ := m.CreateCycle(repoPath, "c1", "obj", nil)
	cycle.Advance(CycleBaselining)
	cycle.Advance(CycleExecuting)
	SaveCycle(repoPath, cycle)

	_, err := m.LaunchCycleTask(context.Background(), cycle, 0, LaunchOptions{})
	if err == nil {
		t.Error("expected error for empty task list")
	}
}

func TestTimeNowOverride(t *testing.T) {
	fixed := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	original := timeNow
	defer func() { timeNow = original }()
	timeNow = func() time.Time { return fixed }

	got := timeNow()
	if !got.Equal(fixed) {
		t.Errorf("expected %v, got %v", fixed, got)
	}
}
