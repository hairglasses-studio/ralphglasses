package loop

import "fmt"

// ProgressSnapshot captures the loop's current state for external consumers (TUI, API).
type ProgressSnapshot struct {
	State       string  `json:"state"`
	SpentUSD    float64 `json:"spent_usd"`
	BudgetUSD   float64 `json:"budget_usd"`
	Transitions int     `json:"transitions"`
}

// Controller ties the state machine to budget enforcement, providing
// a high-level API for driving the Ralph Loop v2.
type Controller struct {
	sm     *StateMachine
	budget *BudgetEnforcer
}

// NewController creates a controller with a fresh state machine and budget enforcer.
func NewController(budget float64) *Controller {
	return &Controller{
		sm:     NewStateMachine(),
		budget: NewBudgetEnforcer(budget),
	}
}

// Start transitions from Idle to Planning.
func (c *Controller) Start() error {
	return c.sm.Transition(StatePlanning)
}

// Advance moves the state machine to the next logical state in the happy path:
// Planning->Executing, Executing->Evaluating, Evaluating->Improving, Improving->Planning.
// Callers use Complete() or Cancel() for terminal transitions.
func (c *Controller) Advance() error {
	current := c.sm.Current()
	next, ok := nextState(current)
	if !ok {
		return fmt.Errorf("no automatic advance from state %s", current)
	}
	return c.sm.Transition(next)
}

// nextState returns the default next state in the happy-path cycle.
func nextState(s LoopState) (LoopState, bool) {
	switch s {
	case StatePlanning:
		return StateExecuting, true
	case StateExecuting:
		return StateEvaluating, true
	case StateEvaluating:
		return StateImproving, true
	case StateImproving:
		return StatePlanning, true
	default:
		return 0, false
	}
}

// Complete transitions from Evaluating to Complete.
func (c *Controller) Complete() error {
	return c.sm.Transition(StateComplete)
}

// RecordSpend records a cost and may trigger a Cooldown transition if spend
// thresholds are breached.
func (c *Controller) RecordSpend(amount float64) {
	c.budget.Record(amount)

	action := c.budget.Check()
	if action >= ActionCooldown {
		current := c.sm.Current()
		// Only transition to cooldown from active states; ignore if already
		// in cooldown, drain, exit, or complete.
		switch current {
		case StateCooldown, StateDrain, StateExit, StateComplete:
			return
		}
		// Best-effort: if transition fails (e.g. concurrent change), that's OK.
		_ = c.sm.Transition(StateCooldown)
	}
}

// Cancel initiates graceful shutdown: current state -> Drain -> Exit.
func (c *Controller) Cancel() {
	current := c.sm.Current()
	if current == StateExit {
		return
	}
	if current != StateDrain {
		// Best-effort transition to Drain; may fail from Complete, that's fine.
		_ = c.sm.Transition(StateDrain)
	}
	_ = c.sm.Transition(StateExit)
}

// State returns the current loop state.
func (c *Controller) State() LoopState {
	return c.sm.Current()
}

// Progress returns a snapshot of current loop progress.
func (c *Controller) Progress() ProgressSnapshot {
	return ProgressSnapshot{
		State:       c.sm.String(),
		SpentUSD:    c.budget.global - c.budget.Remaining(),
		BudgetUSD:   c.budget.global,
		Transitions: len(c.sm.History()),
	}
}
