package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	p := DefaultCostEventPath()
	if p == "" {
		t.Fatal("DefaultCostEventPath returned empty string")
	}
	if filepath.Base(p) != "cost_events.jsonl" {
		t.Errorf("expected cost_events.jsonl, got %s", filepath.Base(p))
	}
}
