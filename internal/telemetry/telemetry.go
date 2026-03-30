package telemetry

import "time"

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
