package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper creates a minimal valid repo structure in a temp dir.
func setupValidRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755); err != nil {
		t.Fatal(err)
	}
	roadmap := "# Roadmap\n\n- [ ] First task\n- [x] Done task\n"
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte(roadmap), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestValidateConfig_ValidRepo(t *testing.T) {
	dir := setupValidRepo(t)
	result := ValidateConfig(dir)
	if !result.OK() {
		t.Fatalf("expected OK, got errors: %v", result.Errors)
	}
}

func TestValidateConfig_MissingGit(t *testing.T) {
	dir := setupValidRepo(t)
	if err := os.RemoveAll(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	result := ValidateConfig(dir)
	if result.OK() {
		t.Fatal("expected error for missing .git")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, ".git") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected .git error, got: %v", result.Errors)
	}
}

func TestValidateConfig_MissingRoadmap(t *testing.T) {
	dir := setupValidRepo(t)
	if err := os.Remove(filepath.Join(dir, "ROADMAP.md")); err != nil {
		t.Fatal(err)
	}
	result := ValidateConfig(dir)
	if result.OK() {
		t.Fatal("expected error for missing ROADMAP.md")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "ROADMAP.md") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected ROADMAP.md error, got: %v", result.Errors)
	}
}

func TestValidateConfig_AllItemsChecked(t *testing.T) {
	dir := setupValidRepo(t)
	roadmap := "# Roadmap\n\n- [x] Done task\n- [x] Another done\n"
	if err := os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte(roadmap), 0o644); err != nil {
		t.Fatal(err)
	}
	result := ValidateConfig(dir)
	if result.OK() {
		t.Fatal("expected error for all-checked ROADMAP")
	}
	found := false
	for _, e := range result.Errors {
		if strings.Contains(e, "unchecked") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unchecked-items error, got: %v", result.Errors)
	}
}

func TestValidateConfig_WarningsDontPreventOK(t *testing.T) {
	dir := setupValidRepo(t)
	// No cost_observations.json → should produce a warning but still OK.
	result := ValidateConfig(dir)
	if !result.OK() {
		t.Fatalf("expected OK despite warnings, got errors: %v", result.Errors)
	}
	// There should be at least the cost_observations warning.
	if len(result.Warnings) == 0 {
		t.Fatal("expected at least one warning")
	}
}

func TestValidateConfig_NonexistentPath(t *testing.T) {
	result := ValidateConfig("/tmp/definitely-does-not-exist-validate-test")
	if result.OK() {
		t.Fatal("expected error for nonexistent path")
	}
}

