package events

import (
	"encoding/json"
	"testing"
)

func TestMigrateEvent_V0(t *testing.T) {
	// v0 event has no "v" field — version defaults to 0.
	raw := json.RawMessage(`{"type":"session.started","timestamp":"2025-01-01T00:00:00Z","session_id":"abc"}`)

	ev, err := MigrateEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Version != 1 {
		t.Errorf("expected Version=1 after migration, got %d", ev.Version)
	}
	if ev.Type != SessionStarted {
		t.Errorf("expected Type=%q, got %q", SessionStarted, ev.Type)
	}
	if ev.SessionID != "abc" {
		t.Errorf("expected SessionID=%q, got %q", "abc", ev.SessionID)
	}
}

func TestMigrateEvent_V1(t *testing.T) {
	raw := json.RawMessage(`{"type":"cost.update","v":1,"timestamp":"2025-06-01T12:00:00Z","session_id":"xyz","data":{"amount":1.5}}`)

	ev, err := MigrateEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Version != 1 {
		t.Errorf("expected Version=1, got %d", ev.Version)
	}
	if ev.Type != CostUpdate {
		t.Errorf("expected Type=%q, got %q", CostUpdate, ev.Type)
	}
	if ev.Data["amount"] != 1.5 {
		t.Errorf("expected data amount=1.5, got %v", ev.Data["amount"])
	}
}

func TestMigrateEvent_FutureVersion(t *testing.T) {
	// Future versions are forward-compatible: deserialized as-is without migration.
	raw := json.RawMessage(`{"type":"session.started","v":99,"session_id":"future"}`)

	ev, err := MigrateEvent(raw)
	if err != nil {
		t.Fatalf("unexpected error for future version: %v", err)
	}
	if ev.Version != 99 {
		t.Errorf("expected Version=99, got %d", ev.Version)
	}
	if ev.SessionID != "future" {
		t.Errorf("expected SessionID=%q, got %q", "future", ev.SessionID)
	}
}

func TestMigrateEvent_InvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{not valid json`)

	_, err := MigrateEvent(raw)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
