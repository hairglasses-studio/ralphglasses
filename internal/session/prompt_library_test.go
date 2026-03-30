package session

import "testing"

func TestPromptLibrary_SaveLoadList(t *testing.T) {
	dir := t.TempDir()
	pl := NewPromptLibraryAt(dir)

	entry := PromptEntry{
		Name:        "fix-lint",
		Description: "Fix all lint issues in the codebase",
		Template:    "Run golangci-lint and fix all reported issues in {{.Repo}}",
		Variables:   map[string]string{"Repo": "ralphglasses"},
		Tags:        []string{"lint", "quality"},
	}

	if err := pl.Save(entry); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := pl.Load("fix-lint")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Template != entry.Template {
		t.Errorf("template mismatch: %s", loaded.Template)
	}
	if loaded.Description != entry.Description {
		t.Errorf("description mismatch: %s", loaded.Description)
	}

	names, err := pl.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 1 || names[0] != "fix-lint" {
		t.Errorf("unexpected names: %v", names)
	}
}

func TestPromptLibrary_Delete(t *testing.T) {
	dir := t.TempDir()
	pl := NewPromptLibraryAt(dir)

	pl.Save(PromptEntry{Name: "temp", Template: "test"})
	if err := pl.Delete("temp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := pl.Load("temp"); err == nil {
		t.Error("expected error loading deleted prompt")
	}
}

func TestPromptLibrary_EmptyDir(t *testing.T) {
	pl := &PromptLibrary{}
	if err := pl.Save(PromptEntry{Name: "x"}); err == nil {
		t.Error("expected error with empty dir")
	}
	names, err := pl.List()
	if err != nil || len(names) != 0 {
		t.Errorf("unexpected: names=%v err=%v", names, err)
	}
}
