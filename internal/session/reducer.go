package session

import "time"

// SessionEventType enumerates the possible lifecycle events for a session.
type SessionEventType string

const (
	EventCreated        SessionEventType = "created"
	EventStarted        SessionEventType = "started"
	EventPaused         SessionEventType = "paused"
	EventResumed        SessionEventType = "resumed"
	EventTurnCompleted  SessionEventType = "turn_completed"
	EventCostUpdated    SessionEventType = "cost_updated"
	EventOutputReceived SessionEventType = "output_received"
	EventErrorOccurred  SessionEventType = "error_occurred"
	EventBudgetExceeded SessionEventType = "budget_exceeded"
	EventCompleted      SessionEventType = "completed"
	EventStopped        SessionEventType = "stopped"
	EventCrashed        SessionEventType = "crashed"
	EventConfigChanged  SessionEventType = "config_changed"
)

// SessionEvent is an immutable record of something that happened to a session.
type SessionEvent struct {
	Type      SessionEventType `json:"type"`
	Timestamp time.Time        `json:"timestamp"`
	SessionID string           `json:"session_id"`

	// Optional fields populated depending on event type.
	Status     SessionStatus `json:"status,omitempty"`
	SpentUSD   float64       `json:"spent_usd,omitempty"`
	TurnCount  int           `json:"turn_count,omitempty"`
	Output     string        `json:"output,omitempty"`
	Error      string        `json:"error,omitempty"`
	ExitReason string        `json:"exit_reason,omitempty"`
	ConfigKey  string        `json:"config_key,omitempty"`
	ConfigVal  string        `json:"config_val,omitempty"`
}

// SideEffectType enumerates effects that the reducer signals but does not execute.
type SideEffectType string

const (
	EffectPersist   SideEffectType = "persist"   // write session state to store
	EffectNotify    SideEffectType = "notify"    // send notification (desktop/webhook)
	EffectEmitEvent SideEffectType = "emit"      // publish to event bus
	EffectKill      SideEffectType = "kill"      // terminate the OS process
	EffectEscalate  SideEffectType = "escalate"  // request human intervention
)

// SideEffect is a description of an action the caller should perform after a reduce step.
type SideEffect struct {
	Type    SideEffectType `json:"type"`
	Message string         `json:"message,omitempty"`
}

// SessionState is the derived state of a session, computed by folding events.
type SessionState struct {
	ID          string        `json:"id"`
	Status      SessionStatus `json:"status"`
	SpentUSD    float64       `json:"spent_usd"`
	TurnCount   int           `json:"turn_count"`
	LastOutput  string        `json:"last_output,omitempty"`
	Error       string        `json:"error,omitempty"`
	ExitReason  string        `json:"exit_reason,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	StartedAt   *time.Time    `json:"started_at,omitempty"`
	EndedAt     *time.Time    `json:"ended_at,omitempty"`
	BudgetUSD   float64       `json:"budget_usd,omitempty"`
	EventCount  int           `json:"event_count"`
}

// Reduce is a pure function: (state, event) -> (newState, sideEffects).
// It contains no I/O and no mutation of the input state.
func Reduce(state SessionState, event SessionEvent) (SessionState, []SideEffect) {
	next := state
	next.EventCount++
	var effects []SideEffect

	switch event.Type {
	case EventCreated:
		next.ID = event.SessionID
		next.Status = StatusLaunching
		next.CreatedAt = event.Timestamp
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventStarted:
		next.Status = StatusRunning
		t := event.Timestamp
		next.StartedAt = &t
		effects = append(effects, SideEffect{Type: EffectEmitEvent, Message: "session.started"})
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventPaused:
		if next.Status == StatusRunning {
			next.Status = "paused"
		}
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventResumed:
		if next.Status == "paused" {
			next.Status = StatusRunning
		}
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventTurnCompleted:
		next.TurnCount = event.TurnCount
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventCostUpdated:
		next.SpentUSD = event.SpentUSD
		if next.BudgetUSD > 0 && next.SpentUSD >= next.BudgetUSD*0.9 {
			effects = append(effects, SideEffect{Type: EffectNotify, Message: "budget 90% reached"})
		}
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventOutputReceived:
		next.LastOutput = event.Output

	case EventErrorOccurred:
		next.Error = event.Error
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventBudgetExceeded:
		next.Status = StatusStopped
		next.ExitReason = "budget_exceeded"
		t := event.Timestamp
		next.EndedAt = &t
		effects = append(effects, SideEffect{Type: EffectKill})
		effects = append(effects, SideEffect{Type: EffectNotify, Message: "budget exceeded"})
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventCompleted:
		next.Status = StatusCompleted
		next.ExitReason = event.ExitReason
		t := event.Timestamp
		next.EndedAt = &t
		effects = append(effects, SideEffect{Type: EffectEmitEvent, Message: "session.completed"})
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventStopped:
		next.Status = StatusStopped
		next.ExitReason = event.ExitReason
		t := event.Timestamp
		next.EndedAt = &t
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventCrashed:
		next.Status = StatusErrored
		next.Error = event.Error
		t := event.Timestamp
		next.EndedAt = &t
		effects = append(effects, SideEffect{Type: EffectEscalate, Message: "session crashed: " + event.Error})
		effects = append(effects, SideEffect{Type: EffectPersist})

	case EventConfigChanged:
		// Config changes don't alter session status but should be persisted.
		effects = append(effects, SideEffect{Type: EffectPersist})
	}

	return next, effects
}

// FoldEvents replays a sequence of events over an initial state to produce
// the final derived state and the combined list of side effects.
func FoldEvents(initial SessionState, events []SessionEvent) (SessionState, []SideEffect) {
	state := initial
	var allEffects []SideEffect
	for _, e := range events {
		var effects []SideEffect
		state, effects = Reduce(state, e)
		allEffects = append(allEffects, effects...)
	}
	return state, allEffects
}
