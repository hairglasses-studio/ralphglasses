package roadmap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExport_RDCycle(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "rdcycle", "", "", 20, true)
	if err != nil {
		t.Fatalf("Export rdcycle: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal rdcycle output: %v", err)
	}

	if spec.Name != "Test Project Roadmap" {
		t.Errorf("spec name = %q", spec.Name)
	}

	if len(spec.Tasks) == 0 {
		t.Error("expected tasks in rdcycle output")
	}

	// Verify completion string
	if !strings.Contains(spec.Completion, "/") {
		t.Errorf("completion = %q, expected fraction", spec.Completion)
	}
}

func TestExport_FixPlan(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "fix_plan", "", "", 20, true)
	if err != nil {
		t.Fatalf("Export fix_plan: %v", err)
	}

	if !strings.Contains(output, "# Fix Plan") {
		t.Error("missing Fix Plan header")
	}
	if !strings.Contains(output, "- [ ]") {
		t.Error("missing unchecked tasks")
	}
	if !strings.Contains(output, "- [x]") {
		t.Error("missing checked tasks")
	}
}

func TestExport_Progress(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "progress", "", "", 20, true)
	if err != nil {
		t.Fatalf("Export progress: %v", err)
	}

	if !strings.Contains(output, "initialized") {
		t.Error("missing initialized status")
	}
	if !strings.Contains(output, "completed_ids") {
		t.Error("missing completed_ids field")
	}
}

func TestExport_PhaseFilter(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "rdcycle", "Phase 1", "", 20, true)
	if err != nil {
		t.Fatalf("Export with filter: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should only have Phase 1 tasks
	for _, task := range spec.Tasks {
		if strings.Contains(task.ID, "Phase 0") {
			t.Error("phase filter didn't exclude Phase 0")
		}
	}
}

func TestExport_MaxTasks(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "rdcycle", "", "", 2, false)
	if err != nil {
		t.Fatalf("Export max_tasks: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(spec.Tasks) > 2 {
		t.Errorf("expected max 2 tasks, got %d", len(spec.Tasks))
	}
}

func TestExport_EmptyFormat(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Empty format defaults to rdcycle
	output, err := Export(rm, "", "", "", 20, true)
	if err != nil {
		t.Fatalf("Export empty format: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if spec.Name == "" {
		t.Error("expected non-empty spec name")
	}
}

func TestExport_SectionFilter(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "rdcycle", "", "Parser", 20, true)
	if err != nil {
		t.Fatalf("Export section filter: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Should only have tasks from the Parser section
	for _, task := range spec.Tasks {
		if !strings.Contains(task.ID, "1.1") && task.ID != "" {
			t.Errorf("section filter leaked task %q", task.ID)
		}
	}
}

func TestExport_RespectDeps(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// With respectDeps=true, tasks with unmet deps should be excluded
	output, err := Export(rm, "rdcycle", "Phase 1", "", 20, true)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var specWithDeps TaskSpec
	if err := json.Unmarshal([]byte(output), &specWithDeps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// With respectDeps=false, all tasks collected
	output2, err := Export(rm, "rdcycle", "Phase 1", "", 20, false)
	if err != nil {
		t.Fatalf("Export no deps: %v", err)
	}

	var specNoDeps TaskSpec
	if err := json.Unmarshal([]byte(output2), &specNoDeps); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Without deps filter, should have at least as many tasks
	if len(specNoDeps.Tasks) < len(specWithDeps.Tasks) {
		t.Errorf("no-deps (%d) should have >= tasks than with-deps (%d)",
			len(specNoDeps.Tasks), len(specWithDeps.Tasks))
	}
}

func TestExport_DefaultMaxTasks(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// maxTasks=0 defaults to 20
	output, err := Export(rm, "rdcycle", "", "", 0, false)
	if err != nil {
		t.Fatalf("Export default max: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(spec.Tasks) == 0 {
		t.Error("expected tasks with default max")
	}
}

func TestExport_EmptyRoadmap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	_ = os.WriteFile(path, []byte("# Empty Project\n"), 0644)

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "rdcycle", "", "", 20, true)
	if err != nil {
		t.Fatalf("Export empty: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(spec.Tasks) != 0 {
		t.Errorf("expected 0 tasks for empty roadmap, got %d", len(spec.Tasks))
	}
}

func TestExport_FixPlanWithIDs(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "fix_plan", "Phase 1", "", 20, false)
	if err != nil {
		t.Fatalf("Export fix_plan: %v", err)
	}

	// Should contain task IDs in output
	if !strings.Contains(output, "1.1.1") {
		t.Error("fix plan should contain task ID 1.1.1")
	}
}

func TestExport_ProgressWithCompletedTasks(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Export all tasks (not just incomplete) by not filtering deps
	output, err := Export(rm, "progress", "", "", 20, false)
	if err != nil {
		t.Fatalf("Export progress: %v", err)
	}

	if !strings.Contains(output, "total_tasks") {
		t.Error("missing total_tasks in progress output")
	}
}

func TestExport_TaskWithoutID(t *testing.T) {
	t.Parallel()
	// Phase 2 has tasks without IDs
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "rdcycle", "Phase 2", "", 20, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	var spec TaskSpec
	if err := json.Unmarshal([]byte(output), &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Tasks without IDs should get Phase/Section as ID
	for _, task := range spec.Tasks {
		if task.ID == "" {
			t.Error("expected non-empty ID even for tasks without explicit IDs")
		}
	}
}

func TestExport_UnknownFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	_ = os.WriteFile(path, []byte(testRoadmap), 0644)

	rm, _ := Parse(path)
	_, err := Export(rm, "xml", "", "", 20, true)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestExport_LaunchReady(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "launch_ready", "", "", 20, false)
	if err != nil {
		t.Fatalf("Export launch_ready: %v", err)
	}

	var tasks []LaunchTask
	if err := json.Unmarshal([]byte(output), &tasks); err != nil {
		t.Fatalf("unmarshal launch_ready output: %v", err)
	}

	if len(tasks) == 0 {
		t.Fatal("expected tasks in launch_ready output")
	}

	for _, task := range tasks {
		if task.Prompt == "" {
			t.Error("task prompt should not be empty")
		}
		if task.Provider == "" {
			t.Error("task provider should not be empty")
		}
		if task.BudgetUSD <= 0 {
			t.Errorf("task budget_usd should be positive, got %f", task.BudgetUSD)
		}
		if task.DifficultyScore < 0 || task.DifficultyScore > 1.0 {
			t.Errorf("difficulty_score should be 0-1, got %f", task.DifficultyScore)
		}
		if task.SuggestedProvider == "" {
			t.Error("suggested_provider should not be empty")
		}
		if task.EstimatedBudget <= 0 {
			t.Errorf("estimated_budget_usd should be positive, got %f", task.EstimatedBudget)
		}
	}
}

func TestExport_LaunchReady_PhaseFilter(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "launch_ready", "Phase 1", "", 20, false)
	if err != nil {
		t.Fatalf("Export launch_ready: %v", err)
	}

	var tasks []LaunchTask
	if err := json.Unmarshal([]byte(output), &tasks); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, task := range tasks {
		if task.Phase != "Phase 1: Core Features" {
			t.Errorf("phase filter leaked task from phase %q", task.Phase)
		}
	}
}

func TestExport_Checkpoint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "checkpoint-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	path := filepath.Join(repoDir, "ROADMAP.md")
	if err := os.WriteFile(path, []byte(testRoadmap), 0o644); err != nil {
		t.Fatalf("write roadmap: %v", err)
	}

	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "checkpoint", "Phase 1", "Parser", 20, true)
	if err != nil {
		t.Fatalf("Export checkpoint: %v", err)
	}

	if !strings.Contains(output, "# Tranche Checkpoint") {
		t.Fatal("expected checkpoint header")
	}
	if !strings.Contains(output, "Repo: `checkpoint-repo`") {
		t.Errorf("expected repo name in checkpoint output, got: %s", output)
	}
	if !strings.Contains(output, "Component: `Phase 1 / Parser`") {
		t.Errorf("expected component label in checkpoint output, got: %s", output)
	}
	if !strings.Contains(output, "1.1.3 — Write unit tests") {
		t.Errorf("expected completed tranche task in checkpoint output, got: %s", output)
	}
	if !strings.Contains(output, "Parser: parser handles all edge cases") {
		t.Errorf("expected acceptance criteria in checkpoint output, got: %s", output)
	}
	if !strings.Contains(output, "1.1.1 — Implement line parser") {
		t.Errorf("expected next-wave ready task in checkpoint output, got: %s", output)
	}
	if !strings.Contains(output, "1.1.2 — Add error handling") || !strings.Contains(output, "[blocked by 1.1.1]") {
		t.Errorf("expected blocked follow-up task in checkpoint output, got: %s", output)
	}
}

func TestExport_Checkpoint_NoAcceptanceFallback(t *testing.T) {
	t.Parallel()
	path := writeTestRoadmap(t)
	rm, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	output, err := Export(rm, "checkpoint", "Phase 2", "", 20, true)
	if err != nil {
		t.Fatalf("Export checkpoint: %v", err)
	}

	if !strings.Contains(output, "No explicit acceptance criteria found") {
		t.Errorf("expected verification fallback in checkpoint output, got: %s", output)
	}
	if !strings.Contains(output, "Add documentation") {
		t.Errorf("expected phase 2 next-wave task in checkpoint output, got: %s", output)
	}
}

func TestComputeDifficulty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		desc       string
		deps       int
		hasSection bool
		wantMin    float64
		wantMax    float64
	}{
		{
			name:       "simple_docs_task",
			desc:       "Add docs",
			deps:       0,
			hasSection: true,
			wantMin:    0.0,
			wantMax:    0.3,
		},
		{
			name:       "medium_implement_task",
			desc:       "Implement line parser with error handling",
			deps:       1,
			hasSection: true,
			wantMin:    0.3,
			wantMax:    0.7,
		},
		{
			name:       "complex_refactor_task",
			desc:       "Refactor the entire session architecture to support multi-provider cascade routing with automatic failover and budget tracking across all providers in the fleet",
			deps:       3,
			hasSection: false,
			wantMin:    0.7,
			wantMax:    1.0,
		},
		{
			name:       "no_deps_no_section_short",
			desc:       "Fix typo",
			deps:       0,
			hasSection: true,
			wantMin:    0.0,
			wantMax:    0.3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := ComputeDifficulty(tt.desc, tt.deps, tt.hasSection)
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("ComputeDifficulty(%q, %d, %v) = %f, want [%f, %f]",
					tt.desc, tt.deps, tt.hasSection, score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestSuggestedProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		difficulty float64
		want       string
	}{
		{0.1, "gemini/flash"},
		{0.29, "gemini/flash"},
		{0.3, "claude/sonnet"},
		{0.5, "claude/sonnet"},
		{0.7, "claude/sonnet"},
		{0.71, "claude/opus"},
		{0.9, "claude/opus"},
	}

	for _, tt := range tests {
		got := SuggestedProvider(tt.difficulty)
		if got != tt.want {
			t.Errorf("SuggestedProvider(%f) = %q, want %q", tt.difficulty, got, tt.want)
		}
	}
}

func TestEstimatedBudget(t *testing.T) {
	t.Parallel()

	// Low difficulty: $0.25-0.50
	lowBudget := EstimatedBudget(0.1)
	if lowBudget < 0.25 || lowBudget > 0.50 {
		t.Errorf("EstimatedBudget(0.1) = %f, want [0.25, 0.50]", lowBudget)
	}

	// Medium difficulty: $0.50-2.00
	medBudget := EstimatedBudget(0.5)
	if medBudget < 0.50 || medBudget > 2.00 {
		t.Errorf("EstimatedBudget(0.5) = %f, want [0.50, 2.00]", medBudget)
	}

	// High difficulty: $2.00-5.00
	highBudget := EstimatedBudget(0.9)
	if highBudget < 2.00 || highBudget > 5.00 {
		t.Errorf("EstimatedBudget(0.9) = %f, want [2.00, 5.00]", highBudget)
	}
}
