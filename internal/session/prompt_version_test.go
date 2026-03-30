package session

import "testing"

func TestPromptVersionStore_SaveAndGet(t *testing.T) {
	dir := t.TempDir()
	pvs := NewPromptVersionStore(dir)

	v1, err := pvs.Save("fix-lint", "Fix lint issues v1", "initial version")
	if err != nil {
		t.Fatalf("Save v1: %v", err)
	}
	if v1 != 1 {
		t.Errorf("expected version 1, got %d", v1)
	}

	v2, err := pvs.Save("fix-lint", "Fix lint issues v2 — improved", "added scope")
	if err != nil {
		t.Fatalf("Save v2: %v", err)
	}
	if v2 != 2 {
		t.Errorf("expected version 2, got %d", v2)
	}

	// Get latest
	latest, err := pvs.Get("fix-lint", 0)
	if err != nil {
		t.Fatalf("Get latest: %v", err)
	}
	if latest.Version != 2 {
		t.Errorf("expected latest version 2, got %d", latest.Version)
	}
	if latest.Template != "Fix lint issues v2 — improved" {
		t.Errorf("wrong template: %s", latest.Template)
	}

	// Get specific version
	first, err := pvs.Get("fix-lint", 1)
	if err != nil {
		t.Fatalf("Get v1: %v", err)
	}
	if first.Template != "Fix lint issues v1" {
		t.Errorf("wrong v1 template: %s", first.Template)
	}
}

func TestPromptVersionStore_ListVersions(t *testing.T) {
	dir := t.TempDir()
	pvs := NewPromptVersionStore(dir)

	pvs.Save("test", "v1", "")
	pvs.Save("test", "v2", "")
	pvs.Save("test", "v3", "")

	versions := pvs.ListVersions("test")
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
}

func TestPromptVersionStore_NonexistentPrompt(t *testing.T) {
	dir := t.TempDir()
	pvs := NewPromptVersionStore(dir)

	if _, err := pvs.Get("nope", 0); err == nil {
		t.Error("expected error for nonexistent prompt")
	}
}
