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
