package loop

import (
	"testing"
)

func TestStateMachine_StartsIdle(t *testing.T) {
	sm := NewStateMachine()
	if sm.Current() != StateIdle {
		t.Fatalf("expected StateIdle, got %s", sm.Current())
	}
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		path []LoopState
	}{
		{
			name: "happy path to complete",
			path: []LoopState{StatePlanning, StateExecuting, StateEvaluating, StateComplete},
		},
		{
			name: "happy path with improvement loop",
			path: []LoopState{StatePlanning, StateExecuting, StateEvaluating, StateImproving, StatePlanning},
		},
		{
			name: "idle to drain to exit",
			path: []LoopState{StateDrain, StateExit},
		},
		{
			name: "cooldown and recovery",
			path: []LoopState{StatePlanning, StateCooldown, StatePlanning},
		},
		{
			name: "cooldown to drain to exit",
			path: []LoopState{StatePlanning, StateCooldown, StateDrain, StateExit},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine()
			for i, state := range tt.path {
				if err := sm.Transition(state); err != nil {
					t.Fatalf("step %d: transition to %s failed: %v", i, state, err)
				}
			}
		})
	}
}

func TestStateMachine_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name   string
		setup  []LoopState // transitions to run before the invalid one
		target LoopState
	}{
		{
			name:   "idle to executing (skip planning)",
			target: StateExecuting,
		},
		{
			name:   "idle to evaluating",
			target: StateEvaluating,
		},
		{
			name:   "idle to exit (skip drain)",
			target: StateExit,
		},
		{
			name:   "planning to complete (skip evaluate)",
			setup:  []LoopState{StatePlanning},
			target: StateComplete,
		},
		{
			name:   "same state transition",
			setup:  []LoopState{StatePlanning},
			target: StatePlanning,
		},
		{
			name:   "exit to idle (terminal)",
			setup:  []LoopState{StateDrain, StateExit},
			target: StateIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := NewStateMachine()
			for _, s := range tt.setup {
				if err := sm.Transition(s); err != nil {
					t.Fatalf("setup transition to %s failed: %v", s, err)
				}
			}
			if err := sm.Transition(tt.target); err == nil {
				t.Fatalf("expected error for transition to %s, got nil", tt.target)
			}
		})
	}
}

func TestStateMachine_HistoryTracking(t *testing.T) {
	sm := NewStateMachine()

	path := []LoopState{StatePlanning, StateExecuting, StateEvaluating, StateComplete}
	for _, s := range path {
		if err := sm.Transition(s); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
	}

	history := sm.History()
	if len(history) != len(path) {
		t.Fatalf("expected %d history entries, got %d", len(path), len(history))
	}

	// Verify first transition: Idle -> Planning
	if history[0].From != StateIdle || history[0].To != StatePlanning {
		t.Errorf("history[0]: expected idle->planning, got %s->%s", history[0].From, history[0].To)
	}

	// Verify timestamps are monotonic
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp.Before(history[i-1].Timestamp) {
			t.Errorf("history[%d] timestamp before history[%d]", i, i-1)
		}
	}
}

func TestStateMachine_OnChangeCallback(t *testing.T) {
	sm := NewStateMachine()

	var changes []StateChange
	sm.OnChange(func(from, to LoopState) {
		changes = append(changes, StateChange{From: from, To: to})
	})

	_ = sm.Transition(StatePlanning)
	_ = sm.Transition(StateExecuting)

	if len(changes) != 2 {
		t.Fatalf("expected 2 onChange calls, got %d", len(changes))
	}
	if changes[0].From != StateIdle || changes[0].To != StatePlanning {
		t.Errorf("change[0]: expected idle->planning, got %s->%s", changes[0].From, changes[0].To)
	}
	if changes[1].From != StatePlanning || changes[1].To != StateExecuting {
		t.Errorf("change[1]: expected planning->executing, got %s->%s", changes[1].From, changes[1].To)
	}
}

func TestStateMachine_GuardRejection(t *testing.T) {
	sm := &StateMachine{
		current: StateIdle,
		transitions: []Transition{
			{From: StateIdle, To: StatePlanning, Guard: func() bool { return false }},
		},
		history: make([]StateChange, 0),
	}

	err := sm.Transition(StatePlanning)
	if err == nil {
		t.Fatal("expected guard rejection error, got nil")
	}
	if sm.Current() != StateIdle {
		t.Fatalf("state should remain idle after guard rejection, got %s", sm.Current())
	}
}

func TestStateMachine_String(t *testing.T) {
	sm := NewStateMachine()
	if sm.String() != "idle" {
		t.Fatalf("expected 'idle', got %q", sm.String())
	}

	_ = sm.Transition(StatePlanning)
	if sm.String() != "planning" {
		t.Fatalf("expected 'planning', got %q", sm.String())
	}
}

func TestLoopState_String(t *testing.T) {
	tests := map[LoopState]string{
		StateIdle:       "idle",
		StatePlanning:   "planning",
		StateExecuting:  "executing",
		StateEvaluating: "evaluating",
		StateImproving:  "improving",
		StateComplete:   "complete",
		StateCooldown:   "cooldown",
		StateDrain:      "drain",
		StateExit:       "exit",
		LoopState(99):   "unknown(99)",
	}
	for state, expected := range tests {
		if got := state.String(); got != expected {
			t.Errorf("LoopState(%d).String() = %q, want %q", int(state), got, expected)
		}
	}
}

func TestStateMachine_HistoryIsCopy(t *testing.T) {
	sm := NewStateMachine()
	_ = sm.Transition(StatePlanning)

	h1 := sm.History()
	h1[0].From = StateExit // mutate the copy

	h2 := sm.History()
	if h2[0].From != StateIdle {
		t.Fatal("History() should return a copy; mutation leaked back")
	}
}
