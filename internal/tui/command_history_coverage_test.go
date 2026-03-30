package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCommandHistory(t *testing.T) {
	t.Parallel()
	h := NewCommandHistory(50)
	if h == nil {
		t.Fatal("NewCommandHistory returned nil")
	}
	if h.maxLen != 50 {
		t.Errorf("maxLen = %d, want 50", h.maxLen)
	}
	// cursor should be reset to len(entries) or -1 if no history file.
	if len(h.entries) == 0 && h.cursor != 0 && h.cursor != -1 {
		// After load, cursor = len(entries) which is 0 when empty.
		t.Logf("cursor = %d after construction with no history file", h.cursor)
	}
}

func TestCommandHistory_Reset(t *testing.T) {
	t.Parallel()
	h := &CommandHistory{maxLen: 100, cursor: -1}

	h.Add("cmd1")
	h.Add("cmd2")
	h.Add("cmd3")

	// Navigate back.
	h.Previous()
	h.Previous()
	if h.cursor >= len(h.entries) {
		t.Error("cursor should be inside entries after Previous calls")
	}

	h.Reset()
	if h.cursor != len(h.entries) {
		t.Errorf("cursor after Reset = %d, want %d", h.cursor, len(h.entries))
	}
}

func TestCommandHistory_Reset_Empty(t *testing.T) {
	t.Parallel()
	h := &CommandHistory{maxLen: 100, cursor: -1}
	h.Reset()
	if h.cursor != 0 {
		t.Errorf("cursor after Reset on empty = %d, want 0", h.cursor)
	}
}

func TestCommandHistory_Load_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	entries := []string{"first", "second", "third"}
	data, _ := json.Marshal(entries)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	h := &CommandHistory{maxLen: 100, cursor: -1, path: path}
	h.load()

	if len(h.entries) != 3 {
		t.Errorf("loaded entries = %d, want 3", len(h.entries))
	}
	if h.cursor != 3 {
		t.Errorf("cursor after load = %d, want 3", h.cursor)
	}
}

func TestCommandHistory_Load_MissingFile(t *testing.T) {
	t.Parallel()
	h := &CommandHistory{maxLen: 100, cursor: -1, path: "/nonexistent/history.json"}
	h.load()
	if len(h.entries) != 0 {
		t.Errorf("entries after load of missing file = %d, want 0", len(h.entries))
	}
}

func TestCommandHistory_Load_EmptyPath(t *testing.T) {
	t.Parallel()
	h := &CommandHistory{maxLen: 100, cursor: -1}
	h.load() // Should be a no-op.
	if len(h.entries) != 0 {
		t.Errorf("entries after load with empty path = %d, want 0", len(h.entries))
	}
}

func TestCommandHistory_Load_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	os.WriteFile(path, []byte("{invalid json"), 0644)

	h := &CommandHistory{maxLen: 100, cursor: -1, path: path}
	h.load()
	// Should not panic. entries remain empty/nil.
	if len(h.entries) != 0 {
		t.Errorf("entries after bad JSON = %d, want 0", len(h.entries))
	}
}

func TestCommandHistory_Save(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "history.json")

	h := &CommandHistory{maxLen: 100, cursor: -1, path: path}
	h.Add("saved-cmd")

	// Verify the file was written.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved history: %v", err)
	}
	var entries []string
	if err := json.Unmarshal(data, &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 || entries[0] != "saved-cmd" {
		t.Errorf("saved entries = %v, want [saved-cmd]", entries)
	}
}

func TestCommandHistory_Save_EmptyPath(t *testing.T) {
	t.Parallel()
	h := &CommandHistory{maxLen: 100, cursor: -1}
	h.Add("test") // Should not panic even without a path.
}

func TestCommandHistory_AddEmpty(t *testing.T) {
	t.Parallel()
	h := &CommandHistory{maxLen: 100, cursor: -1}
	h.Add("")
	if len(h.entries) != 0 {
		t.Errorf("entries after Add(\"\") = %d, want 0", len(h.entries))
	}
}

func TestCommandHistory_Roundtrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	h1 := &CommandHistory{maxLen: 100, cursor: -1, path: path}
	h1.Add("alpha")
	h1.Add("beta")
	h1.Add("gamma")

	// Load into a new history.
	h2 := &CommandHistory{maxLen: 100, cursor: -1, path: path}
	h2.load()

	if len(h2.entries) != 3 {
		t.Fatalf("roundtrip entries = %d, want 3", len(h2.entries))
	}
	if h2.entries[0] != "alpha" || h2.entries[2] != "gamma" {
		t.Errorf("roundtrip entries = %v", h2.entries)
	}
}
