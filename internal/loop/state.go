package loop

import (
	"fmt"
	"sync"
	"time"
)

// LoopState represents a discrete state in the Ralph Loop v2 state machine.
type LoopState int

const (
	StateIdle      LoopState = iota // initial state, no work in progress
	StatePlanning                   // generating plan for next iteration
	StateExecuting                  // running agent workers
	StateEvaluating                 // assessing worker output
	StateImproving                  // refining based on evaluation feedback
	StateComplete                   // terminal: work finished successfully
	StateCooldown                   // budget warning, reduced activity
	StateDrain                      // graceful shutdown in progress
	StateExit                       // terminal: shutdown complete
)

var stateNames = map[LoopState]string{
	StateIdle:       "idle",
	StatePlanning:   "planning",
	StateExecuting:  "executing",
	StateEvaluating: "evaluating",
	StateImproving:  "improving",
	StateComplete:   "complete",
	StateCooldown:   "cooldown",
	StateDrain:      "drain",
	StateExit:       "exit",
}

func (s LoopState) String() string {
	if name, ok := stateNames[s]; ok {
		return name
	}
	return fmt.Sprintf("unknown(%d)", int(s))
}

// Transition represents a valid state transition with an optional guard condition.
type Transition struct {
	From  LoopState
	To    LoopState
	Guard func() bool // nil means always allowed
}

// StateChange records a completed state transition with its timestamp.
type StateChange struct {
	From      LoopState
	To        LoopState
	Timestamp time.Time
}

// StateMachine manages loop state with validated transitions and change notifications.
type StateMachine struct {
	mu          sync.RWMutex
	current     LoopState
	transitions []Transition
	history     []StateChange
	onChange    func(from, to LoopState)
}

// standardTransitions returns the default set of valid transitions for the Ralph Loop v2.
func standardTransitions() []Transition {
	return []Transition{
		// Happy path
		{From: StateIdle, To: StatePlanning},
		{From: StatePlanning, To: StateExecuting},
		{From: StateExecuting, To: StateEvaluating},
		{From: StateEvaluating, To: StateImproving},
		{From: StateEvaluating, To: StateComplete},
		{From: StateImproving, To: StatePlanning}, // loop back

		// Budget: any state can enter cooldown
		{From: StateIdle, To: StateCooldown},
		{From: StatePlanning, To: StateCooldown},
		{From: StateExecuting, To: StateCooldown},
		{From: StateEvaluating, To: StateCooldown},
		{From: StateImproving, To: StateCooldown},

		// Recovery from cooldown
		{From: StateCooldown, To: StatePlanning},

		// Drain: any non-terminal state can enter drain
		{From: StateIdle, To: StateDrain},
		{From: StatePlanning, To: StateDrain},
		{From: StateExecuting, To: StateDrain},
		{From: StateEvaluating, To: StateDrain},
		{From: StateImproving, To: StateDrain},
		{From: StateCooldown, To: StateDrain},

		// Drain to exit (terminal)
		{From: StateDrain, To: StateExit},
	}
}

// NewStateMachine creates a state machine starting in StateIdle with the standard transition table.
func NewStateMachine() *StateMachine {
	return &StateMachine{
		current:     StateIdle,
		transitions: standardTransitions(),
		history:     make([]StateChange, 0, 32),
	}
}

// Current returns the current state.
func (sm *StateMachine) Current() LoopState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

// Transition validates and applies a state transition. Returns an error if the
// transition is not allowed or its guard condition returns false.
func (sm *StateMachine) Transition(to LoopState) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	from := sm.current
	if from == to {
		return fmt.Errorf("already in state %s", from)
	}

	allowed := false
	for _, t := range sm.transitions {
		if t.From == from && t.To == to {
			if t.Guard != nil && !t.Guard() {
				return fmt.Errorf("transition %s -> %s: guard rejected", from, to)
			}
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("invalid transition: %s -> %s", from, to)
	}

	sm.current = to
	sm.history = append(sm.history, StateChange{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
	})

	if sm.onChange != nil {
		sm.onChange(from, to)
	}

	return nil
}

// OnChange registers a callback invoked after every successful state transition.
// The callback is called while the state machine lock is held, so it must not
// call back into the state machine.
func (sm *StateMachine) OnChange(fn func(from, to LoopState)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onChange = fn
}

// History returns a copy of all recorded state transitions.
func (sm *StateMachine) History() []StateChange {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make([]StateChange, len(sm.history))
	copy(out, sm.history)
	return out
}

// String returns the human-readable name of the current state.
func (sm *StateMachine) String() string {
	return sm.Current().String()
}
