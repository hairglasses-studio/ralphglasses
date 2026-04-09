package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAllEventTypes(t *testing.T) {
	types := AllEventTypes()
	if len(types) != 7 {
		t.Errorf("AllEventTypes() returned %d types, want 7", len(types))
	}
	seen := make(map[EventType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}

func TestDefaultPath_UsesStateDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("HOME", "")

	if got, want := DefaultPath(), filepath.Join(xdg, "ralphglasses", "telemetry.jsonl"); got != want {
		t.Fatalf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPath_PrefersExistingLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")
	legacyDir := filepath.Join(home, ".ralphglasses")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, "telemetry.jsonl")
	if err := os.WriteFile(legacyPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write legacy telemetry: %v", err)
	}

	if got := DefaultPath(); got != legacyPath {
		t.Fatalf("DefaultPath() = %q, want %q", got, legacyPath)
	}
}

func TestNewWriter_UsesDefaultPath(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)
	t.Setenv("HOME", "")

	w := NewWriter()
	if got, want := w.path, filepath.Join(xdg, "ralphglasses", "telemetry.jsonl"); got != want {
		t.Fatalf("writer path = %q, want %q", got, want)
	}
}

func TestWriter_WriteCreatesDirAndDefaultsTimestamp(t *testing.T) {
	w := &Writer{path: filepath.Join(t.TempDir(), "state", "telemetry.jsonl")}
	if err := w.Write(Event{Type: EventCrash, RepoName: "alpha"}); err != nil {
		t.Fatalf("Write(): %v", err)
	}

	data, err := os.ReadFile(w.path)
	if err != nil {
		t.Fatalf("ReadFile(): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 JSONL line, got %d", len(lines))
	}

	var ev Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("Unmarshal(): %v", err)
	}
	if ev.Type != EventCrash {
		t.Fatalf("event type = %q, want %q", ev.Type, EventCrash)
	}
	if ev.RepoName != "alpha" {
		t.Fatalf("repo_name = %q, want %q", ev.RepoName, "alpha")
	}
	if ev.Timestamp.IsZero() {
		t.Fatal("expected timestamp to be defaulted")
	}
}
