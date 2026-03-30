package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// EventType identifies the category of telemetry event.
type EventType string

const (
	EventSessionStart  EventType = "session_start"
	EventSessionStop   EventType = "session_stop"
	EventCrash         EventType = "crash"
	EventBudgetHit     EventType = "budget_hit"
	EventCircuitTrip   EventType = "circuit_trip"
)

// Event is a single telemetry record.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	SessionID string         `json:"session_id,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	RepoName  string         `json:"repo_name,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// AllEventTypes returns all defined telemetry event types.
func AllEventTypes() []EventType {
	return []EventType{
		EventSessionStart,
		EventSessionStop,
		EventCrash,
		EventBudgetHit,
		EventCircuitTrip,
	}
}

// Writer appends telemetry events to a local JSONL file.
type Writer struct {
	mu   sync.Mutex
	path string
}

// NewWriter creates a writer targeting ~/.ralphglasses/telemetry.jsonl.
func NewWriter() *Writer {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "/tmp"
	}
	return &Writer{path: filepath.Join(home, ".ralphglasses", "telemetry.jsonl")}
}

// Write appends an event to the JSONL file.
func (w *Writer) Write(ev Event) error {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(w.path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
