package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCycleObservationsToTasksMixed(t *testing.T) {
	obs := []LoopObservation{
		{Status: "failed", TaskTitle: "fix bug A", Error: "compile error", TotalCostUSD: 0.50},
		{Status: "noop", TaskTitle: "refactor module B", LoopID: "loop-1", IterationNumber: 3},
		{Status: "regressed", TaskTitle: "test suite C", Error: "test timeout", TotalCostUSD: 1.0},
		{Status: "stalled", TaskTitle: "deploy D", LoopID: "loop-2"},
		{Status: "done", TaskTitle: "completed task"}, // should be ignored
	}

	tasks := ObservationsToTasks(obs)
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	// Should be sorted by priority descending.
	for i := 1; i < len(tasks); i++ {
		if tasks[i].Priority > tasks[i-1].Priority {
			t.Fatalf("tasks not sorted by priority: %v > %v at index %d", tasks[i].Priority, tasks[i-1].Priority, i)
		}
	}

	// All should have source "finding".
	for _, task := range tasks {
		if task.Source != "finding" {
			t.Fatalf("expected source 'finding', got %q", task.Source)
		}
		if task.Status != "pending" {
			t.Fatalf("expected status 'pending', got %q", task.Status)
		}
	}
}

func TestCycleObservationsToTasksDeduplication(t *testing.T) {
	obs := []LoopObservation{
		{Status: "failed", TaskTitle: "fix bug in module X", Error: "err1", TotalCostUSD: 0.5},
		{Status: "failed", TaskTitle: "fix bug in module X", Error: "err1", TotalCostUSD: 0.5}, // near-duplicate
		{Status: "failed", TaskTitle: "completely different task", Error: "err2", TotalCostUSD: 0.3},
	}

	tasks := ObservationsToTasks(obs)
	// The two "fix bug in module X" observations produce identical titles,
	// so one should be deduplicated.
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks after dedup, got %d", len(tasks))
	}
}

func TestCycleObservationsToTasksEmpty(t *testing.T) {
	tasks := ObservationsToTasks(nil)
	if tasks != nil {
		t.Fatalf("expected nil for empty observations, got %v", tasks)
	}

	tasks = ObservationsToTasks([]LoopObservation{})
	if tasks != nil {
		t.Fatalf("expected nil for empty slice, got %v", tasks)
	}
}

func TestCycleRoadmapToTasks(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	content := `# Roadmap

## Phase 1

- [x] **Done item** — already complete
- [ ] **QW-2** — Enable cascade routing by default
- [ ] **QW-4** — Fix prompt_analyze score inflation

## Phase 2

- [ ] **QW-6** — Fix loop_gates zero-baseline bug
- [ ] **QW-7** — Fix snapshot path saving
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := RoadmapToTasks(roadmap, 0)
	if err != nil {
		t.Fatalf("RoadmapToTasks: %v", err)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	// All should be roadmap source.
	for _, task := range tasks {
		if task.Source != "roadmap" {
			t.Fatalf("expected source 'roadmap', got %q", task.Source)
		}
		if task.Status != "pending" {
			t.Fatalf("expected status 'pending', got %q", task.Status)
		}
	}

	// First task title should contain QW-2.
	if tasks[0].Title != "QW-2" {
		t.Fatalf("expected first task title %q, got %q", "QW-2", tasks[0].Title)
	}
}

func TestCycleRoadmapToTasksMaxLimit(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	content := `# Roadmap
- [ ] **Item 1** — first
- [ ] **Item 2** — second
- [ ] **Item 3** — third
- [ ] **Item 4** — fourth
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := RoadmapToTasks(roadmap, 2)
	if err != nil {
		t.Fatalf("RoadmapToTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks with maxTasks=2, got %d", len(tasks))
	}
}

func TestCycleRoadmapToTasksFileNotFound(t *testing.T) {
	_, err := RoadmapToTasks("/nonexistent/ROADMAP.md", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestCycleJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b     string
		wantMin  float64
		wantMax  float64
	}{
		{"fix bug in module", "fix bug in module", 1.0, 1.0},
		{"completely different", "nothing alike here", 0.0, 0.01},
		{"fix bug in X", "fix bug in Y", 0.5, 0.9},
		{"", "", 1.0, 1.0},
	}
	for _, tt := range tests {
		sim := cycleJaccardSimilarity(tt.a, tt.b)
		if sim < tt.wantMin || sim > tt.wantMax {
			t.Errorf("cycleJaccardSimilarity(%q, %q) = %f, want [%f, %f]", tt.a, tt.b, sim, tt.wantMin, tt.wantMax)
		}
	}
}
