// Package events provides an in-process pub/sub event bus with optional
// SQLite persistence. Decouples modules so task completions, mood logs,
// focus sessions, etc. can trigger reactions without direct imports.
package events

import "time"

// EventType identifies the kind of event.
type EventType string

const (
	TaskCompleted     EventType = "task.completed"
	MoodLogged        EventType = "mood.logged"
	FocusStarted      EventType = "focus.started"
	FocusEnded        EventType = "focus.ended"
	ReplySent         EventType = "reply.sent"
	ChoreDone         EventType = "chore.done"
	HabitCompleted    EventType = "habit.completed"
	EnergyRecorded    EventType = "energy.recorded"
	OverwhelmDetected EventType = "overwhelm.detected"
	AchievementEarned EventType = "achievement.earned"
	ReviewGenerated   EventType = "review.generated"
)

// Event is a single occurrence published through the bus.
type Event struct {
	Type      EventType      `json:"type"`
	Payload   map[string]any `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
	Source    string         `json:"source"` // originating module/worker task
}

// New creates an event with the current timestamp.
func New(t EventType, source string, payload map[string]any) Event {
	if payload == nil {
		payload = make(map[string]any)
	}
	return Event{
		Type:      t,
		Payload:   payload,
		Timestamp: time.Now(),
		Source:    source,
	}
}

// Handler processes an event. Implementations must not block.
type Handler func(Event)
