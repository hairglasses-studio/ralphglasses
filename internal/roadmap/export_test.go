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
	json.Unmarshal([]byte(output), &spec)

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
	json.Unmarshal([]byte(output), &spec)

	if len(spec.Tasks) > 2 {
		t.Errorf("expected max 2 tasks, got %d", len(spec.Tasks))
	}
}

func TestExport_UnknownFormat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "ROADMAP.md")
	os.WriteFile(path, []byte(testRoadmap), 0644)

	rm, _ := Parse(path)
	_, err := Export(rm, "xml", "", "", 20, true)
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
}
