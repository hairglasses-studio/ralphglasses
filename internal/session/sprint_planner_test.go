package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSprintPlannerParsePrioritiesAndSizes(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	content := `# Roadmap

## Phase 1

- [x] **Done item** — already complete ` + "`P0` `S`" + `
- [ ] **Task A** — high priority small ` + "`P0` `S`" + `
- [ ] **Task B** — medium priority large ` + "`P1` `L`" + `
- [ ] **Task C** — low priority medium ` + "`P2` `M`" + `
- [ ] **Task D** — high priority medium ` + "`P0` `M`" + `
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := NewSprintPlanner(roadmap)
	cycle := sp.PlanNextSprint(dir)
	if cycle == nil {
		t.Fatal("expected a cycle, got nil")
	}

	// Verify priorities were parsed correctly.
	for _, task := range cycle.Tasks {
		switch task.Title {
		case "Task A":
			if task.Priority != 1.0 {
				t.Errorf("Task A: expected priority 1.0, got %f", task.Priority)
			}
			if task.Size != "S" {
				t.Errorf("Task A: expected size S, got %q", task.Size)
			}
		case "Task B":
			if task.Priority != 0.8 {
				t.Errorf("Task B: expected priority 0.8, got %f", task.Priority)
			}
			if task.Size != "L" {
				t.Errorf("Task B: expected size L, got %q", task.Size)
			}
		case "Task C":
			if task.Priority != 0.5 {
				t.Errorf("Task C: expected priority 0.5, got %f", task.Priority)
			}
			if task.Size != "M" {
				t.Errorf("Task C: expected size M, got %q", task.Size)
			}
		case "Task D":
			if task.Priority != 1.0 {
				t.Errorf("Task D: expected priority 1.0, got %f", task.Priority)
			}
			if task.Size != "M" {
				t.Errorf("Task D: expected size M, got %q", task.Size)
			}
		}
	}
}

func TestSprintPlannerMaxItemsAndSizePoints(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	// 6 small items (1pt each) but max 5 items per sprint.
	content := `# Roadmap
- [ ] **T1** — task 1 ` + "`P0` `S`" + `
- [ ] **T2** — task 2 ` + "`P0` `S`" + `
- [ ] **T3** — task 3 ` + "`P0` `S`" + `
- [ ] **T4** — task 4 ` + "`P0` `S`" + `
- [ ] **T5** — task 5 ` + "`P0` `S`" + `
- [ ] **T6** — task 6 ` + "`P0` `S`" + `
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := NewSprintPlanner(roadmap)
	sp.MaxItemsPerSprint = 3
	sp.MaxSizePoints = 10 // not limiting
	cycle := sp.PlanNextSprint(dir)
	if cycle == nil {
		t.Fatal("expected a cycle, got nil")
	}
	if len(cycle.Tasks) != 3 {
		t.Fatalf("expected 3 tasks (MaxItemsPerSprint), got %d", len(cycle.Tasks))
	}

	// Now test size points limit.
	sp.MaxItemsPerSprint = 10 // not limiting
	sp.MaxSizePoints = 3      // only 3 small items fit
	cycle = sp.PlanNextSprint(dir)
	if cycle == nil {
		t.Fatal("expected a cycle, got nil")
	}
	if len(cycle.Tasks) != 3 {
		t.Fatalf("expected 3 tasks (MaxSizePoints=3 with S=1pt each), got %d", len(cycle.Tasks))
	}
}

func TestSprintPlannerP0BeforeP1(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	content := `# Roadmap
- [ ] **Low** — low priority ` + "`P2` `S`" + `
- [ ] **Med** — medium priority ` + "`P1` `S`" + `
- [ ] **High** — high priority ` + "`P0` `S`" + `
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := NewSprintPlanner(roadmap)
	cycle := sp.PlanNextSprint(dir)
	if cycle == nil {
		t.Fatal("expected a cycle, got nil")
	}
	if len(cycle.Tasks) < 2 {
		t.Fatalf("expected at least 2 tasks, got %d", len(cycle.Tasks))
	}
	// P0 should come first.
	if cycle.Tasks[0].Title != "High" {
		t.Errorf("expected first task to be 'High' (P0), got %q", cycle.Tasks[0].Title)
	}
	if cycle.Tasks[1].Title != "Med" {
		t.Errorf("expected second task to be 'Med' (P1), got %q", cycle.Tasks[1].Title)
	}
}

func TestSprintPlannerEmptyRoadmapReturnsNil(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	// All items checked.
	content := `# Roadmap
- [x] **Done 1** — complete ` + "`P0` `S`" + `
- [x] **Done 2** — also complete ` + "`P1` `M`" + `
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := NewSprintPlanner(roadmap)
	cycle := sp.PlanNextSprint(dir)
	if cycle != nil {
		t.Fatalf("expected nil for all-checked roadmap, got cycle with %d tasks", len(cycle.Tasks))
	}
}

func TestSprintPlannerLargeItemsSizeBudget(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	// 3 large items (3pts each), budget is 6.
	content := `# Roadmap
- [ ] **Big A** — large task ` + "`P0` `L`" + `
- [ ] **Big B** — large task ` + "`P0` `L`" + `
- [ ] **Big C** — large task ` + "`P0` `L`" + `
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := NewSprintPlanner(roadmap)
	// Default MaxSizePoints is 6, each L=3pts, so max 2 items.
	cycle := sp.PlanNextSprint(dir)
	if cycle == nil {
		t.Fatal("expected a cycle, got nil")
	}
	if len(cycle.Tasks) != 2 {
		t.Fatalf("expected 2 tasks (6pts budget, L=3pts each), got %d", len(cycle.Tasks))
	}
}

func TestSprintPlannerMissingRoadmapReturnsNil(t *testing.T) {
	sp := NewSprintPlanner("/nonexistent/ROADMAP.md")
	cycle := sp.PlanNextSprint("/tmp")
	if cycle != nil {
		t.Fatal("expected nil for missing roadmap")
	}
}

func TestSprintPlannerMixedSizesRespectsBudget(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	// L(3) + M(2) + S(1) = 6 fits; adding another S would exceed if items allow.
	content := `# Roadmap
- [ ] **Big** — large ` + "`P0` `L`" + `
- [ ] **Med** — medium ` + "`P0` `M`" + `
- [ ] **Sm1** — small ` + "`P0` `S`" + `
- [ ] **Sm2** — small 2 ` + "`P0` `S`" + `
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	sp := NewSprintPlanner(roadmap)
	sp.MaxSizePoints = 6
	cycle := sp.PlanNextSprint(dir)
	if cycle == nil {
		t.Fatal("expected a cycle, got nil")
	}
	// L(3)+M(2)+S(1)=6, exactly fits. Sm2 would make 7, doesn't fit.
	if len(cycle.Tasks) != 3 {
		t.Fatalf("expected 3 tasks (L+M+S=6pts), got %d", len(cycle.Tasks))
	}
}

func TestRoadmapToTasksParsesAnnotations(t *testing.T) {
	dir := t.TempDir()
	roadmap := filepath.Join(dir, "ROADMAP.md")

	content := `# Roadmap
## Section A
- [ ] **Item X** — do something ` + "`P0` `S`" + `
- [ ] **Item Y** — do something else ` + "`P1` `L`" + `
- [ ] **Item Z** — no annotations
`
	if err := os.WriteFile(roadmap, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := RoadmapToTasks(roadmap, 0)
	if err != nil {
		t.Fatalf("RoadmapToTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}

	// Item X: P0 S
	if tasks[0].Priority != 1.0 {
		t.Errorf("Item X: expected priority 1.0, got %f", tasks[0].Priority)
	}
	if tasks[0].Size != "S" {
		t.Errorf("Item X: expected size S, got %q", tasks[0].Size)
	}

	// Item Y: P1 L
	if tasks[1].Priority != 0.8 {
		t.Errorf("Item Y: expected priority 0.8, got %f", tasks[1].Priority)
	}
	if tasks[1].Size != "L" {
		t.Errorf("Item Y: expected size L, got %q", tasks[1].Size)
	}

	// Item Z: no annotations, defaults
	if tasks[2].Priority != 0.5 {
		t.Errorf("Item Z: expected priority 0.5 (default), got %f", tasks[2].Priority)
	}
	if tasks[2].Size != "" {
		t.Errorf("Item Z: expected empty size (default), got %q", tasks[2].Size)
	}
}
