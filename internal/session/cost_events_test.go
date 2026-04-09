package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

func TestNewCostEventWriter_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "events.jsonl")

	w, err := NewCostEventWriter(path)
	if err != nil {
		t.Fatalf("NewCostEventWriter: %v", err)
	}
	defer w.Close()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist at %s: %v", path, err)
	}
}

func TestCostEventWriter_WriteAppendsValidJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	w, err := NewCostEventWriter(path)
	if err != nil {
		t.Fatalf("NewCostEventWriter: %v", err)
	}

	evt := CostEvent{
		SessionID:    "sess-001",
		Provider:     "anthropic",
		Model:        "claude-opus-4-20250514",
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0.045,
		Repo:         "ralphglasses",
		Timestamp:    time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC),
	}
	if err := w.Write(evt); err != nil {
		t.Fatalf("Write: %v", err)
	}
	w.Close()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var decoded CostEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.SessionID != "sess-001" {
		t.Errorf("SessionID = %q, want %q", decoded.SessionID, "sess-001")
	}
	if decoded.CostUSD != 0.045 {
		t.Errorf("CostUSD = %f, want %f", decoded.CostUSD, 0.045)
	}
}

func TestCostEventWriter_MultipleWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	w, err := NewCostEventWriter(path)
	if err != nil {
		t.Fatalf("NewCostEventWriter: %v", err)
	}

	for i := range 3 {
		evt := CostEvent{
			SessionID: "sess-multi",
			CostUSD:   float64(i) * 0.01,
			Timestamp: time.Now(),
		}
		if err := w.Write(evt); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	w.Close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lines := 0
	for scanner.Scan() {
		lines++
		var evt CostEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			t.Errorf("line %d: invalid JSON: %v", lines, err)
		}
	}
	if lines != 3 {
		t.Errorf("got %d lines, want 3", lines)
	}
}

func TestCostEventWriter_CloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	w, err := NewCostEventWriter(path)
	if err != nil {
		t.Fatalf("NewCostEventWriter: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDefaultCostEventPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	if got, want := DefaultCostEventPath(), filepath.Join(home, ".ralphglasses", "cost_events.jsonl"); got != want {
		t.Fatalf("DefaultCostEventPath() = %q, want %q", got, want)
	}
}

func TestDefaultCostEventPath_PrefersExistingLegacyFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	legacyDir := filepath.Join(home, ".ralph")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	legacyPath := filepath.Join(legacyDir, "cost_events.jsonl")
	if err := os.WriteFile(legacyPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write legacy cost events: %v", err)
	}

	if got := DefaultCostEventPath(); got != legacyPath {
		t.Fatalf("DefaultCostEventPath() = %q, want %q", got, legacyPath)
	}
	if got := DefaultCostEventPath(); got != ralphpath.CostEventsPath() {
		t.Fatalf("DefaultCostEventPath() = %q, want helper path %q", got, ralphpath.CostEventsPath())
	}
}
