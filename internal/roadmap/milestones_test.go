package roadmap

import (
	"math"
	"testing"
)

// sampleRoadmap builds a multi-phase roadmap with dependencies for testing.
func sampleRoadmap() *Roadmap {
	rm := &Roadmap{
		Title: "Sample Project",
		Phases: []Phase{
			{
				Name: "Phase 0: Setup (COMPLETE)",
				Sections: []Section{
					{
						Name: "Bootstrap",
						Tasks: []Task{
							{ID: "0.1", Description: "Init repo", Done: true},
							{ID: "0.2", Description: "Add CI", Done: true},
							{ID: "0.3", Description: "Add linter", Done: true},
						},
					},
				},
				Stats: Stats{Total: 3, Completed: 3, Remaining: 0},
			},
			{
				Name: "Phase 1: Core",
				Sections: []Section{
					{
						Name: "Parser",
						Tasks: []Task{
							{ID: "1.1", Description: "Tokenizer", Done: true},
							{ID: "1.2", Description: "AST builder", Done: false, DependsOn: []string{"1.1"}},
							{ID: "1.3", Description: "Error recovery", Done: false, DependsOn: []string{"1.2"}},
						},
					},
					{
						Name: "Evaluator",
						Tasks: []Task{
							{ID: "1.4", Description: "Tree walker", Done: false, DependsOn: []string{"1.2"}},
							{ID: "1.5", Description: "Optimizer", Done: false, DependsOn: []string{"1.4"}},
						},
					},
				},
				Stats: Stats{Total: 5, Completed: 1, Remaining: 4},
			},
			{
				Name: "Phase 2: Extensions",
				Sections: []Section{
					{
						Name: "Plugins",
						Tasks: []Task{
							{ID: "2.1", Description: "Plugin loader", Done: false, DependsOn: []string{"1.4"}},
							{ID: "2.2", Description: "Plugin API", Done: false, DependsOn: []string{"2.1"}},
							{ID: "2.3", Description: "Docs", Done: false},
						},
					},
				},
				Stats: Stats{Total: 3, Completed: 0, Remaining: 3},
			},
		},
		Stats: Stats{Total: 11, Completed: 4, Remaining: 7},
	}
	return rm
}

// --- GroupByPhase ---

func TestGroupByPhase(t *testing.T) {
	t.Parallel()
	rm := sampleRoadmap()
	phases := GroupByPhase(rm)

	if len(phases) != 3 {
		t.Fatalf("got %d phases, want 3", len(phases))
	}

	// Phase 0: 100%
	if phases[0].CompletionPercent != 100 {
		t.Errorf("Phase 0 completion = %.1f%%, want 100%%", phases[0].CompletionPercent)
	}
	if phases[0].Remaining != 0 {
		t.Errorf("Phase 0 remaining = %d, want 0", phases[0].Remaining)
	}

	// Phase 1: 1/5 = 20%
	if phases[1].CompletionPercent != 20 {
		t.Errorf("Phase 1 completion = %.1f%%, want 20%%", phases[1].CompletionPercent)
	}
	if phases[1].Total != 5 {
		t.Errorf("Phase 1 total = %d, want 5", phases[1].Total)
	}

	// Phase 2: 0/3 = 0%
	if phases[2].CompletionPercent != 0 {
		t.Errorf("Phase 2 completion = %.1f%%, want 0%%", phases[2].CompletionPercent)
	}
}

func TestGroupByPhase_EmptyRoadmap(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{}
	phases := GroupByPhase(rm)
	if len(phases) != 0 {
		t.Errorf("expected 0 phases, got %d", len(phases))
	}
}

func TestGroupByPhase_EmptyPhase(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{
		Phases: []Phase{
			{Name: "Empty Phase", Stats: Stats{Total: 0, Completed: 0, Remaining: 0}},
		},
	}
	phases := GroupByPhase(rm)
	if phases[0].CompletionPercent != 0 {
		t.Errorf("empty phase completion = %.1f%%, want 0%%", phases[0].CompletionPercent)
	}
}

// --- EstimateEffort ---

func TestEstimateEffort(t *testing.T) {
	t.Parallel()
	rm := sampleRoadmap()
	estimates := EstimateEffort(rm, Velocity{TasksPerPeriod: 2})

	// Phase 0 is complete, should be skipped.
	for _, e := range estimates {
		if e.PhaseName == "Phase 0: Setup (COMPLETE)" {
			t.Error("completed phase should not appear in estimates")
		}
	}

	if len(estimates) != 2 {
		t.Fatalf("got %d estimates, want 2", len(estimates))
	}

	// Phase 1: 4 remaining / 2 per period = 2.0
	if estimates[0].PeriodsRemaining != 2.0 {
		t.Errorf("Phase 1 periods = %.1f, want 2.0", estimates[0].PeriodsRemaining)
	}
	if estimates[0].RemainingTasks != 4 {
		t.Errorf("Phase 1 remaining = %d, want 4", estimates[0].RemainingTasks)
	}

	// Phase 2: 3 remaining / 2 per period = 1.5
	if estimates[1].PeriodsRemaining != 1.5 {
		t.Errorf("Phase 2 periods = %.1f, want 1.5", estimates[1].PeriodsRemaining)
	}
}

func TestEstimateEffort_ZeroVelocity(t *testing.T) {
	t.Parallel()
	rm := sampleRoadmap()
	estimates := EstimateEffort(rm, Velocity{TasksPerPeriod: 0})

	for _, e := range estimates {
		if !math.IsInf(e.PeriodsRemaining, 1) {
			t.Errorf("%s: expected +Inf with zero velocity, got %.1f", e.PhaseName, e.PeriodsRemaining)
		}
	}
}

func TestEstimateEffort_AllComplete(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{
		Phases: []Phase{
			{Name: "Done", Stats: Stats{Total: 5, Completed: 5, Remaining: 0}},
		},
	}
	estimates := EstimateEffort(rm, Velocity{TasksPerPeriod: 1})
	if len(estimates) != 0 {
		t.Errorf("expected 0 estimates for fully complete roadmap, got %d", len(estimates))
	}
}

func TestEstimateEffort_FractionalVelocity(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{
		Phases: []Phase{
			{Name: "Slow Phase", Stats: Stats{Total: 10, Completed: 0, Remaining: 10}},
		},
	}
	estimates := EstimateEffort(rm, Velocity{TasksPerPeriod: 0.5})
	if len(estimates) != 1 {
		t.Fatalf("expected 1 estimate, got %d", len(estimates))
	}
	// 10 / 0.5 = 20
	if estimates[0].PeriodsRemaining != 20 {
		t.Errorf("periods = %.1f, want 20.0", estimates[0].PeriodsRemaining)
	}
}

// --- CriticalPath ---

func TestCriticalPath(t *testing.T) {
	t.Parallel()
	rm := sampleRoadmap()
	crit := CriticalPath(rm)

	if len(crit) == 0 {
		t.Fatal("expected critical path items")
	}

	// 1.2 blocks: 1.3, 1.4, 1.5, 2.1, 2.2 (5 tasks transitively)
	first := crit[0]
	if first.TaskID != "1.2" {
		t.Errorf("top critical item = %q, want 1.2", first.TaskID)
	}
	if first.BlockedCount != 5 {
		t.Errorf("1.2 blocked count = %d, want 5", first.BlockedCount)
	}

	// Verify blocked IDs contain the expected tasks.
	wantBlocked := map[string]bool{"1.3": true, "1.4": true, "1.5": true, "2.1": true, "2.2": true}
	for _, id := range first.BlockedTaskIDs {
		if !wantBlocked[id] {
			t.Errorf("unexpected blocked ID %q for task 1.2", id)
		}
		delete(wantBlocked, id)
	}
	if len(wantBlocked) > 0 {
		t.Errorf("missing blocked IDs for task 1.2: %v", wantBlocked)
	}
}

func TestCriticalPath_NoDependencies(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{
						Name: "Work",
						Tasks: []Task{
							{ID: "1.1", Description: "Independent A", Done: false},
							{ID: "1.2", Description: "Independent B", Done: false},
						},
					},
				},
			},
		},
	}
	crit := CriticalPath(rm)
	if len(crit) != 0 {
		t.Errorf("expected 0 critical items when no deps, got %d", len(crit))
	}
}

func TestCriticalPath_AllDone(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{
						Name: "Work",
						Tasks: []Task{
							{ID: "1.1", Description: "Done A", Done: true},
							{ID: "1.2", Description: "Done B", Done: true, DependsOn: []string{"1.1"}},
						},
					},
				},
			},
		},
	}
	crit := CriticalPath(rm)
	if len(crit) != 0 {
		t.Errorf("expected 0 critical items when all done, got %d", len(crit))
	}
}

func TestCriticalPath_DiamondDependency(t *testing.T) {
	t.Parallel()
	// Diamond: A -> B, A -> C, B -> D, C -> D
	rm := &Roadmap{
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{
						Name: "Diamond",
						Tasks: []Task{
							{ID: "A", Description: "Root", Done: false},
							{ID: "B", Description: "Left", Done: false, DependsOn: []string{"A"}},
							{ID: "C", Description: "Right", Done: false, DependsOn: []string{"A"}},
							{ID: "D", Description: "Join", Done: false, DependsOn: []string{"B", "C"}},
						},
					},
				},
			},
		},
	}
	crit := CriticalPath(rm)

	// A should block B, C, D (3 tasks).
	if len(crit) == 0 {
		t.Fatal("expected critical path items")
	}
	if crit[0].TaskID != "A" {
		t.Errorf("top critical item = %q, want A", crit[0].TaskID)
	}
	if crit[0].BlockedCount != 3 {
		t.Errorf("A blocked count = %d, want 3", crit[0].BlockedCount)
	}
}

func TestCriticalPath_LinearChain(t *testing.T) {
	t.Parallel()
	// A -> B -> C -> D: A blocks 3, B blocks 2, C blocks 1
	rm := &Roadmap{
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{
						Name: "Chain",
						Tasks: []Task{
							{ID: "A", Description: "First", Done: false},
							{ID: "B", Description: "Second", Done: false, DependsOn: []string{"A"}},
							{ID: "C", Description: "Third", Done: false, DependsOn: []string{"B"}},
							{ID: "D", Description: "Fourth", Done: false, DependsOn: []string{"C"}},
						},
					},
				},
			},
		},
	}
	crit := CriticalPath(rm)

	if len(crit) != 3 {
		t.Fatalf("got %d critical items, want 3", len(crit))
	}

	// Sorted descending by blocked count.
	expected := []struct {
		id    string
		count int
	}{
		{"A", 3},
		{"B", 2},
		{"C", 1},
	}
	for i, want := range expected {
		if crit[i].TaskID != want.id {
			t.Errorf("crit[%d].TaskID = %q, want %q", i, crit[i].TaskID, want.id)
		}
		if crit[i].BlockedCount != want.count {
			t.Errorf("crit[%d].BlockedCount = %d, want %d", i, crit[i].BlockedCount, want.count)
		}
	}
}

func TestCriticalPath_SkipsTasksWithoutID(t *testing.T) {
	t.Parallel()
	rm := &Roadmap{
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{
						Name: "Mixed",
						Tasks: []Task{
							{ID: "", Description: "No ID task", Done: false},
							{ID: "1.1", Description: "Has ID", Done: false},
							{ID: "1.2", Description: "Depends on 1.1", Done: false, DependsOn: []string{"1.1"}},
						},
					},
				},
			},
		},
	}
	crit := CriticalPath(rm)
	// Only 1.1 should appear (blocking 1.2). The no-ID task is excluded.
	if len(crit) != 1 {
		t.Fatalf("got %d critical items, want 1", len(crit))
	}
	if crit[0].TaskID != "1.1" {
		t.Errorf("critical item = %q, want 1.1", crit[0].TaskID)
	}
}

// --- Integration: parsed roadmap ---

func TestMilestones_FromParsedRoadmap(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// GroupByPhase
	phases := GroupByPhase(rm)
	if len(phases) != 3 {
		t.Fatalf("expected 3 phases, got %d", len(phases))
	}
	// Phase 0 should be 100% complete (3/3).
	if phases[0].CompletionPercent != 100 {
		t.Errorf("Phase 0 = %.1f%%, want 100%%", phases[0].CompletionPercent)
	}

	// EstimateEffort
	estimates := EstimateEffort(rm, Velocity{TasksPerPeriod: 3})
	// Phase 0 is complete, phases 1 and 2 have remaining work.
	if len(estimates) != 2 {
		t.Fatalf("expected 2 estimates, got %d", len(estimates))
	}

	// CriticalPath
	crit := CriticalPath(rm)
	// Task 1.1.1 blocks 1.1.2 and 1.2.2, and task 1.2.1 blocks 1.2.2.
	if len(crit) == 0 {
		t.Error("expected critical path items from parsed roadmap")
	}
	// 1.1.1 should be the top blocker (blocks 1.1.2 + 1.2.2 = 2).
	if crit[0].TaskID != "1.1.1" {
		t.Errorf("top critical item = %q, want 1.1.1", crit[0].TaskID)
	}
}
