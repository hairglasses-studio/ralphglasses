package repofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffold(t *testing.T) {
	dir := t.TempDir()

	// Create a go.mod to trigger Go detection
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644)

	result, err := Scaffold(dir, ScaffoldOptions{})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	if len(result.Created) != 4 {
		t.Errorf("expected 4 created files, got %d: %v", len(result.Created), result.Created)
	}

	// Verify files exist
	for _, relPath := range []string{".ralphrc", ".ralph/PROMPT.md", ".ralph/AGENT.md", ".ralph/fix_plan.md"} {
		full := filepath.Join(dir, relPath)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected %s to exist", relPath)
		}
	}

	// Verify .ralph/logs/ dir
	if _, err := os.Stat(filepath.Join(dir, ".ralph", "logs")); err != nil {
		t.Error("expected .ralph/logs/ to exist")
	}

	// Verify .ralphrc content
	data, _ := os.ReadFile(filepath.Join(dir, ".ralphrc"))
	content := string(data)
	if !strings.Contains(content, "PROJECT_TYPE=\"go\"") {
		t.Error("expected PROJECT_TYPE=go in .ralphrc")
	}
	if !strings.Contains(content, "go build") {
		t.Error("expected go build command in .ralphrc")
	}
}

func TestScaffold_SkipsExisting(t *testing.T) {
	dir := t.TempDir()

	// Create existing .ralphrc
	os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("EXISTING=true\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	result, err := Scaffold(dir, ScaffoldOptions{})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	// .ralphrc should be skipped
	found := false
	for _, s := range result.Skipped {
		if s == ".ralphrc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .ralphrc to be skipped")
	}

	// Verify original content preserved
	data, _ := os.ReadFile(filepath.Join(dir, ".ralphrc"))
	if !strings.Contains(string(data), "EXISTING=true") {
		t.Error("existing .ralphrc content was overwritten")
	}
}

func TestScaffold_Force(t *testing.T) {
	dir := t.TempDir()

	// Create existing files
	os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("OLD=true\n"), 0644)
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	result, err := Scaffold(dir, ScaffoldOptions{Force: true})
	if err != nil {
		t.Fatalf("Scaffold force: %v", err)
	}

	if len(result.Skipped) != 0 {
		t.Errorf("expected 0 skipped with force, got %d", len(result.Skipped))
	}

	// Verify .ralphrc was overwritten
	data, _ := os.ReadFile(filepath.Join(dir, ".ralphrc"))
	if strings.Contains(string(data), "OLD=true") {
		t.Error("expected .ralphrc to be overwritten with force")
	}
}

func TestDetectProjectType(t *testing.T) {
	tests := []struct {
		file string
		want string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
	}

	for _, tt := range tests {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, tt.file), []byte(""), 0644)

		got := detectProjectType(dir)
		if got != tt.want {
			t.Errorf("detectProjectType(%s) = %q, want %q", tt.file, got, tt.want)
		}
	}
}
