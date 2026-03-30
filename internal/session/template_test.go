package session

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TemplateDir (0%)
// ---------------------------------------------------------------------------

func TestTemplateDir(t *testing.T) {
	t.Parallel()
	dir := TemplateDir()
	// On any normal system HOME is set, so this should be non-empty.
	if dir == "" {
		t.Skip("UserHomeDir failed — likely CI without HOME set")
	}
	if filepath.Base(dir) != "templates" {
		t.Errorf("TemplateDir base = %q, want templates", filepath.Base(dir))
	}
}

// ---------------------------------------------------------------------------
// SaveTemplate / LoadTemplate / ListTemplates / DeleteTemplate (all 0%)
// These tests override HOME to a temp directory so we don't pollute real config.
// ---------------------------------------------------------------------------

func setTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

func TestSaveTemplate_HappyPath(t *testing.T) {
	setTempHome(t)

	tmpl := LaunchTemplate{
		Name:      "test-tmpl",
		Provider:  "claude",
		Model:     "opus",
		Prompt:    "do something",
		BudgetUSD: 5.0,
		MaxTurns:  100,
	}
	if err := SaveTemplate(tmpl); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	// Verify the file exists.
	dir := TemplateDir()
	path := filepath.Join(dir, "test-tmpl.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("template file should exist after save")
	}
}

func TestLoadTemplate_HappyPath(t *testing.T) {
	setTempHome(t)

	original := LaunchTemplate{
		Name:      "load-test",
		Provider:  "gemini",
		BudgetUSD: 2.5,
	}
	if err := SaveTemplate(original); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	loaded, err := LoadTemplate("load-test")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	if loaded.Name != "load-test" {
		t.Errorf("Name = %q, want load-test", loaded.Name)
	}
	if loaded.Provider != "gemini" {
		t.Errorf("Provider = %q, want gemini", loaded.Provider)
	}
	if loaded.BudgetUSD != 2.5 {
		t.Errorf("BudgetUSD = %f, want 2.5", loaded.BudgetUSD)
	}
}

func TestLoadTemplate_NotFound(t *testing.T) {
	setTempHome(t)

	_, err := LoadTemplate("nonexistent")
	if err == nil {
		t.Error("LoadTemplate should return error for nonexistent template")
	}
}

func TestListTemplates_Empty(t *testing.T) {
	setTempHome(t)

	names, err := ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 templates, got %d", len(names))
	}
}

func TestListTemplates_Multiple(t *testing.T) {
	setTempHome(t)

	for _, name := range []string{"beta", "alpha", "gamma"} {
		if err := SaveTemplate(LaunchTemplate{Name: name, Provider: "claude"}); err != nil {
			t.Fatalf("SaveTemplate(%s): %v", name, err)
		}
	}

	names, err := ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 templates, got %d", len(names))
	}
	// Should be sorted.
	if names[0] != "alpha" || names[1] != "beta" || names[2] != "gamma" {
		t.Errorf("templates not sorted: %v", names)
	}
}

func TestListTemplates_IgnoresSubdirs(t *testing.T) {
	setTempHome(t)

	// Save one template.
	if err := SaveTemplate(LaunchTemplate{Name: "real", Provider: "claude"}); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}
	// Create a subdirectory in the templates dir.
	dir := TemplateDir()
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	names, err := ListTemplates()
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if len(names) != 1 {
		t.Errorf("expected 1 template, got %d: %v", len(names), names)
	}
}

func TestDeleteTemplate_HappyPath(t *testing.T) {
	setTempHome(t)

	if err := SaveTemplate(LaunchTemplate{Name: "delete-me", Provider: "codex"}); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	if err := DeleteTemplate("delete-me"); err != nil {
		t.Fatalf("DeleteTemplate: %v", err)
	}

	// Verify it's gone.
	names, _ := ListTemplates()
	for _, n := range names {
		if n == "delete-me" {
			t.Error("template should be deleted")
		}
	}
}

func TestDeleteTemplate_NotFound(t *testing.T) {
	setTempHome(t)

	// Create the template dir so DeleteTemplate can attempt the remove.
	os.MkdirAll(TemplateDir(), 0755)

	err := DeleteTemplate("nonexistent")
	if err == nil {
		t.Error("DeleteTemplate should return error for nonexistent template")
	}
}

func TestTemplate_Roundtrip(t *testing.T) {
	setTempHome(t)

	original := LaunchTemplate{
		Name:      "roundtrip",
		Provider:  "claude",
		Model:     "sonnet",
		Prompt:    "improve test coverage",
		BudgetUSD: 10.0,
		MaxTurns:  500,
		RepoPath:  "/tmp/repo",
	}
	if err := SaveTemplate(original); err != nil {
		t.Fatalf("SaveTemplate: %v", err)
	}

	loaded, err := LoadTemplate("roundtrip")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}

	if loaded.Name != original.Name ||
		loaded.Provider != original.Provider ||
		loaded.Model != original.Model ||
		loaded.Prompt != original.Prompt ||
		loaded.BudgetUSD != original.BudgetUSD ||
		loaded.MaxTurns != original.MaxTurns ||
		loaded.RepoPath != original.RepoPath {
		t.Errorf("roundtrip mismatch:\n  original: %+v\n  loaded:   %+v", original, *loaded)
	}
}
