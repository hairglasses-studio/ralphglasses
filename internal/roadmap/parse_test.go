package roadmap

import (
	"os"
	"path/filepath"
	"testing"
)

const testRoadmap = `# Test Project Roadmap

## Phase 0: Foundation (COMPLETE)

- [x] Set up Go module
- [x] Add CLI framework
- [x] Add basic tests

## Phase 1: Core Features

### 1.1 — Parser
- [ ] 1.1.1 — Implement line parser
- [ ] 1.1.2 — Add error handling [BLOCKED BY 1.1.1]
- [x] 1.1.3 — Write unit tests
- **Acceptance:** parser handles all edge cases

### 1.2 — Analyzer
- [ ] 1.2.1 — Walk filesystem
- [ ] 1.2.2 — Compare against roadmap [BLOCKED BY 1.1.1, 1.2.1]
- **Acceptance:** analyzer detects gaps

## Phase 2: Advanced

- [ ] Add documentation
- [ ] Add CI pipeline
`

func writeTestRoadmap(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(path, []byte(testRoadmap), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if rm.Title != "Test Project Roadmap" {
		t.Errorf("Title = %q, want %q", rm.Title, "Test Project Roadmap")
	}

	if len(rm.Phases) != 3 {
		t.Fatalf("got %d phases, want 3", len(rm.Phases))
	}

	// Phase 0: 3 completed tasks
	p0 := rm.Phases[0]
	if p0.Stats.Total != 3 {
		t.Errorf("Phase 0 total = %d, want 3", p0.Stats.Total)
	}
	if p0.Stats.Completed != 3 {
		t.Errorf("Phase 0 completed = %d, want 3", p0.Stats.Completed)
	}

	// Phase 1: 2 sections, 5 tasks (2 done)
	p1 := rm.Phases[1]
	if len(p1.Sections) != 2 {
		t.Fatalf("Phase 1 sections = %d, want 2", len(p1.Sections))
	}
	if p1.Stats.Total != 5 {
		t.Errorf("Phase 1 total = %d, want 5", p1.Stats.Total)
	}
	if p1.Stats.Completed != 1 {
		t.Errorf("Phase 1 completed = %d, want 1", p1.Stats.Completed)
	}

	// Check task IDs
	sec := p1.Sections[0]
	if sec.Tasks[0].ID != "1.1.1" {
		t.Errorf("task ID = %q, want %q", sec.Tasks[0].ID, "1.1.1")
	}

	// Check dependencies
	if len(sec.Tasks[1].DependsOn) != 1 || sec.Tasks[1].DependsOn[0] != "1.1.1" {
		t.Errorf("task 1.1.2 depends_on = %v, want [1.1.1]", sec.Tasks[1].DependsOn)
	}

	// Check multi-dep
	sec2 := p1.Sections[1]
	if len(sec2.Tasks[1].DependsOn) != 2 {
		t.Errorf("task 1.2.2 depends_on = %v, want 2 deps", sec2.Tasks[1].DependsOn)
	}

	// Check acceptance
	if sec.Acceptance != "parser handles all edge cases" {
		t.Errorf("acceptance = %q", sec.Acceptance)
	}

	// Total stats
	if rm.Stats.Total != 10 {
		t.Errorf("total tasks = %d, want 10", rm.Stats.Total)
	}
	if rm.Stats.Completed != 4 {
		t.Errorf("completed = %d, want 4", rm.Stats.Completed)
	}
}

func TestParse_FileNotFound(t *testing.T) {
	t.Parallel()
	_, err := Parse("/nonexistent/ROADMAP.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	_ = os.WriteFile(path, []byte(""), 0644)

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse empty: %v", err)
	}
	if len(rm.Phases) != 0 {
		t.Errorf("expected 0 phases, got %d", len(rm.Phases))
	}
}

func TestParse_MalformedMarkdown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		content    string
		wantPhases int
		wantTasks  int
	}{
		{
			name:       "no_headers_no_checkboxes",
			content:    "This is just plain text.\nNo headers at all.\n",
			wantPhases: 0,
			wantTasks:  0,
		},
		{
			name:       "headers_but_no_checkboxes",
			content:    "# Title\n\n## Phase 1\n\nSome text but no tasks.\n\n## Phase 2\n\nMore text.\n",
			wantPhases: 2,
			wantTasks:  0,
		},
		{
			name:       "checkboxes_but_no_phase_headers",
			content:    "# My Roadmap\n\n- [x] Done thing\n- [ ] Undone thing\n",
			wantPhases: 0,
			wantTasks:  0, // tasks without a phase are dropped
		},
		{
			name:       "single_item_phase",
			content:    "# Plan\n\n## Phase 1\n\n- [ ] Only task\n",
			wantPhases: 1,
			wantTasks:  1,
		},
		{
			name:       "uppercase_X_checkbox",
			content:    "# Plan\n\n## Phase 1\n\n- [X] Done with uppercase X\n- [ ] Not done\n",
			wantPhases: 1,
			wantTasks:  2,
		},
		{
			name:       "title_only",
			content:    "# Just A Title\n",
			wantPhases: 0,
			wantTasks:  0,
		},
		{
			name: "multiple_acceptance_criteria",
			content: `# Plan

## Phase 1

### Section A
- [ ] Task A
- **Acceptance:** A works

### Section B
- [ ] Task B
- **Acceptance:** B works
`,
			wantPhases: 1,
			wantTasks:  2,
		},
		{
			name: "section_without_phase",
			content: `# Plan

### Orphan Section
- [ ] orphan task
`,
			wantPhases: 0,
			wantTasks:  0, // section exists but no phase to attach to
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "ROADMAP.md")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}

			rm, err := Parse(path)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if len(rm.Phases) != tt.wantPhases {
				t.Errorf("phases = %d, want %d", len(rm.Phases), tt.wantPhases)
			}

			if rm.Stats.Total != tt.wantTasks {
				t.Errorf("total tasks = %d, want %d", rm.Stats.Total, tt.wantTasks)
			}
		})
	}
}

func TestParse_TaskIDFormats(t *testing.T) {
	t.Parallel()

	content := `# Roadmap

## Phase 1

### Tasks
- [ ] 1.0.1 — Simple dash separator
- [ ] 2.0.1 – En-dash separator
- [x] 3.0.1 - Hyphen separator
- [ ] No ID just a description
`

	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tasks := rm.Phases[0].Sections[0].Tasks
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}

	// Task with em-dash
	if tasks[0].ID != "1.0.1" {
		t.Errorf("task 0 ID = %q, want %q", tasks[0].ID, "1.0.1")
	}

	// Task with en-dash
	if tasks[1].ID != "2.0.1" {
		t.Errorf("task 1 ID = %q, want %q", tasks[1].ID, "2.0.1")
	}

	// Completed task with hyphen
	if !tasks[2].Done {
		t.Error("task 2 should be done")
	}
	if tasks[2].ID != "3.0.1" {
		t.Errorf("task 2 ID = %q, want %q", tasks[2].ID, "3.0.1")
	}

	// Task without ID
	if tasks[3].ID != "" {
		t.Errorf("task 3 ID = %q, want empty", tasks[3].ID)
	}
}

func TestParse_AcceptanceCriteria(t *testing.T) {
	t.Parallel()

	content := `# Plan

## Phase 1

### Section With Acceptance
- [ ] A task
- **Acceptance:** all tests pass

### Section Without Acceptance
- [ ] Another task
`

	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	sec0 := rm.Phases[0].Sections[0]
	if sec0.Acceptance != "all tests pass" {
		t.Errorf("acceptance = %q, want %q", sec0.Acceptance, "all tests pass")
	}

	sec1 := rm.Phases[0].Sections[1]
	if sec1.Acceptance != "" {
		t.Errorf("expected empty acceptance, got %q", sec1.Acceptance)
	}
}

func TestResolvePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		repo, file, want string
	}{
		{"/repo", "", "/repo/ROADMAP.md"},
		{"/repo", "PLAN.md", "/repo/PLAN.md"},
		{"/repo/ROADMAP.md", "", "/repo/ROADMAP.md"},
	}
	for _, tt := range tests {
		got := ResolvePath(tt.repo, tt.file)
		if got != tt.want {
			t.Errorf("ResolvePath(%q, %q) = %q, want %q", tt.repo, tt.file, got, tt.want)
		}
	}
}
