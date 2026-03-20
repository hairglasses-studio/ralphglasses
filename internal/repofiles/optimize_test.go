package repofiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOptimize_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)

	result, err := Optimize(dir, OptimizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Should have issues for missing files
	if len(result.Issues) == 0 {
		t.Error("expected issues for missing ralph files")
	}

	// Should detect Go project type
	if result.ProjectType != "go" {
		t.Errorf("ProjectType = %q, want go", result.ProjectType)
	}

	// Check for specific missing file issues
	hasRCIssue := false
	hasPromptIssue := false
	for _, issue := range result.Issues {
		if issue.File == ".ralphrc" {
			hasRCIssue = true
		}
		if issue.File == ".ralph/PROMPT.md" {
			hasPromptIssue = true
		}
	}
	if !hasRCIssue {
		t.Error("expected .ralphrc issue")
	}
	if !hasPromptIssue {
		t.Error("expected PROMPT.md issue")
	}
}

func TestOptimize_WithScaffoldedFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)

	// Scaffold first
	_, err := Scaffold(dir, ScaffoldOptions{})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	result, err := Optimize(dir, OptimizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Should still find some info-level issues but no errors
	for _, issue := range result.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error issue after scaffold: %s - %s", issue.File, issue.Issue)
		}
	}
}

func TestOptimize_FocusConfig(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("PROJECT_NAME=\"test\"\n"), 0644)

	result, err := Optimize(dir, OptimizeOptions{Focus: "config"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Should only have config-related issues, not prompt/plan
	for _, issue := range result.Issues {
		if issue.File == ".ralph/PROMPT.md" || issue.File == ".ralph/fix_plan.md" {
			t.Errorf("focus=config should not report %s issues", issue.File)
		}
	}
}

func TestReadKVFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.rc")
	os.WriteFile(path, []byte("KEY1=\"value1\"\nKEY2=value2\n# comment\n\nKEY3=\"value 3\"\n"), 0644)

	values := readKVFile(path)
	if values["KEY1"] != "value1" {
		t.Errorf("KEY1 = %q", values["KEY1"])
	}
	if values["KEY2"] != "value2" {
		t.Errorf("KEY2 = %q", values["KEY2"])
	}
	if values["KEY3"] != "value 3" {
		t.Errorf("KEY3 = %q", values["KEY3"])
	}
}
