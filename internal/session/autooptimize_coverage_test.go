package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Tests for functions not covered by existing test files.

func TestLoadAutonomyLevel_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "autonomy.json"), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadAutonomyLevel(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestPersistAutonomyLevel_Alias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := PersistAutonomyLevel(1, dir); err != nil {
		t.Fatalf("persist: %v", err)
	}
	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if level != 1 {
		t.Errorf("level = %d, want 1", level)
	}
}

func TestWriteMultipleImprovementNotes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for i := range 3 {
		note := ImprovementNote{
			ID:       "note-" + string(rune('a'+i)),
			Category: "config",
			Priority: i + 1,
			Title:    "Test note",
			Status:   "pending",
		}
		if err := WriteImprovementNote(dir, note); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(notes) != 3 {
		t.Errorf("expected 3 notes, got %d", len(notes))
	}
}

func TestReadPendingNotes_FileNotExist(t *testing.T) {
	t.Parallel()
	notes, err := ReadPendingNotes(t.TempDir())
	if err != nil {
		t.Fatalf("read nonexistent: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes for missing file, got %d", len(notes))
	}
}

func TestReadPendingNotes_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)
	os.WriteFile(filepath.Join(dir, ".ralph", "improvement_notes.jsonl"), []byte(""), 0644)

	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("expected 0 notes for empty file, got %d", len(notes))
	}
}

func TestReadPendingNotes_MixedValidInvalid(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	valid, _ := json.Marshal(ImprovementNote{ID: "valid-1", Status: "pending"})
	content := string(valid) + "\n{invalid json}\n" + string(valid) + "\n"
	os.WriteFile(filepath.Join(dir, ".ralph", "improvement_notes.jsonl"), []byte(content), 0644)

	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Should skip invalid lines.
	if len(notes) != 2 {
		t.Errorf("expected 2 valid notes, got %d", len(notes))
	}
}
