package roadmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyze(t *testing.T) {
	t.Parallel()
	// Create a temp repo with some dirs matching roadmap keywords
	dir := t.TempDir()

	// Write roadmap
	rmPath := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(rmPath, []byte(testRoadmap), 0644); err != nil {
		t.Fatal(err)
	}

	// Create some evidence dirs
	if err := os.MkdirAll(filepath.Join(dir, "internal", "parser"), 0755); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Should have gaps (incomplete tasks with no evidence)
	if len(analysis.Gaps) == 0 && len(analysis.Stale) == 0 {
		t.Error("expected some gaps or stale items")
	}

	// Should have ready tasks (tasks with no unmet deps)
	if len(analysis.Ready) == 0 {
		t.Error("expected some ready tasks")
	}

	// Summary should be populated
	if analysis.Summary.TotalTasks != rm.Stats.Total {
		t.Errorf("summary total = %d, want %d", analysis.Summary.TotalTasks, rm.Stats.Total)
	}
}

func TestAnalyze_EmptyRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rmPath := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(rmPath, []byte(testRoadmap), 0644); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// All incomplete tasks should be gaps
	if analysis.Summary.GapCount == 0 {
		t.Error("expected gaps in empty repo")
	}
}

func TestAnalyze_WithEvidence(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Roadmap referencing backtick paths
	content := `# Project

## Phase 1

### Core
- [ ] 1.1 — Implement ` + "`internal/parser`" + ` module
- [x] 1.2 — Setup done
`
	rmPath := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(rmPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Create evidence directory
	if err := os.MkdirAll(filepath.Join(dir, "internal", "parser"), 0755); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Task 1.1 references internal/parser which exists — should be stale
	if len(analysis.Stale) == 0 {
		t.Error("expected stale items when evidence exists")
	}
}

func TestAnalyze_DependenciesBlocked(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	content := `# Project

## Phase 1

### Tasks
- [ ] 1.1 — First task
- [ ] 1.2 — Second task [BLOCKED BY 1.1]
- [x] 1.3 — Completed dep
- [ ] 1.4 — Depends on completed [BLOCKED BY 1.3]
`
	rmPath := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(rmPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Task 1.4 depends on 1.3 (completed) — should be in Ready with deps
	foundReady14 := false
	for _, r := range analysis.Ready {
		if r.TaskID == "1.4" {
			foundReady14 = true
		}
	}
	if !foundReady14 {
		t.Error("expected task 1.4 in Ready list (dep 1.3 is completed)")
	}
}

func TestAnalyze_AllTasksDone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	content := `# Done Project

## Phase 1

### Core
- [x] 1.1 — All done
- [x] 1.2 — Also done
`
	rmPath := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(rmPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	if len(analysis.Gaps) != 0 {
		t.Errorf("expected 0 gaps for all-done roadmap, got %d", len(analysis.Gaps))
	}
	if len(analysis.Stale) != 0 {
		t.Errorf("expected 0 stale for all-done roadmap, got %d", len(analysis.Stale))
	}
}

func TestFindOrphaned_WithInternalDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create internal dirs
	for _, d := range []string{"parser", "tui", "mystery"} {
		if err := os.MkdirAll(filepath.Join(dir, "internal", d), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Roadmap only mentions parser
	rm := &Roadmap{
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{Name: "Core", Tasks: []Task{{Description: "parser module"}}},
				},
			},
		},
	}

	orphaned := findOrphaned(rm, dir)
	// "mystery" and "tui" are not in roadmap text
	if len(orphaned) < 1 {
		t.Errorf("expected at least 1 orphaned dir, got %d", len(orphaned))
	}

	// Verify "parser" is NOT orphaned (it's in task description)
	for _, o := range orphaned {
		if o.Path == filepath.Join("internal", "parser") {
			t.Error("parser should not be orphaned — it's referenced in roadmap")
		}
	}
}

func TestFindOrphaned_NoInternalDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// No internal/ dir at all
	rm := &Roadmap{}
	orphaned := findOrphaned(rm, dir)
	if len(orphaned) != 0 {
		t.Errorf("expected 0 orphaned when no internal/ dir, got %d", len(orphaned))
	}
}

func TestExtractKeywords(t *testing.T) {
	t.Parallel()
	tests := []struct {
		desc string
		want int // minimum number of keywords
	}{
		{"implement `internal/parser` module", 1},
		{"add internal/tui/views handler", 1},
		{"basic description with no paths", 0},
	}
	for _, tt := range tests {
		kw := extractKeywords(tt.desc)
		if len(kw) < tt.want {
			t.Errorf("extractKeywords(%q) got %d keywords, want >= %d", tt.desc, len(kw), tt.want)
		}
	}
}
