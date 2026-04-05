package loop

import "sync"

// BudgetAction represents the enforcement action at a given spend threshold.
type BudgetAction int

const (
	ActionContinue BudgetAction = iota // under 80%
	ActionWarn                         // 80% spent
	ActionCooldown                     // 90% spent
	ActionEscalate                     // 95% spent
	ActionStop                         // 100% spent
)

var budgetActionNames = map[BudgetAction]string{
	ActionContinue: "continue",
	ActionWarn:     "warn",
	ActionCooldown: "cooldown",
	ActionEscalate: "escalate",
	ActionStop:     "stop",
}

func (a BudgetAction) String() string {
	if name, ok := budgetActionNames[a]; ok {
		return name
	}
	return "unknown"
}

// BudgetEnforcer tracks spend against a global budget cap with threshold-based actions.
type BudgetEnforcer struct {
	mu      sync.Mutex
	global  float64                  // total budget cap in USD
	spent   float64                  // cumulative spend in USD
	actions map[float64]BudgetAction // threshold percentage (0-1) -> action
}

// NewBudgetEnforcer creates a budget enforcer with the standard threshold table.
// A zero or negative budget means unlimited (Check always returns ActionContinue).
func NewBudgetEnforcer(globalBudget float64) *BudgetEnforcer {
	return &BudgetEnforcer{
		global: globalBudget,
		actions: map[float64]BudgetAction{
			0.80: ActionWarn,
			0.90: ActionCooldown,
			0.95: ActionEscalate,
			1.00: ActionStop,
		},
	}
}

// Record adds a spend amount to the cumulative total.
func (be *BudgetEnforcer) Record(amount float64) {
	be.mu.Lock()
	defer be.mu.Unlock()
	be.spent += amount
}

// Check returns the highest-priority action triggered by current spend.
func (be *BudgetEnforcer) Check() BudgetAction {
	be.mu.Lock()
	defer be.mu.Unlock()

	if be.global <= 0 {
		return ActionContinue
	}

	pct := be.spent / be.global
	highest := ActionContinue

	for threshold, action := range be.actions {
		if pct >= threshold && action > highest {
			highest = action
		}
	}

	return highest
}

// Remaining returns the unspent budget in USD. Returns 0 if overspent.
func (be *BudgetEnforcer) Remaining() float64 {
	be.mu.Lock()
	defer be.mu.Unlock()

	if be.global <= 0 {
		return 0
	}
	rem := be.global - be.spent
	if rem < 0 {
		return 0
	}
	return rem
}

// SpentPercent returns the percentage of budget consumed (0-100+).
func (be *BudgetEnforcer) SpentPercent() float64 {
	be.mu.Lock()
	defer be.mu.Unlock()

	if be.global <= 0 {
		return 0
	}
	return (be.spent / be.global) * 100
}

// Rebalance returns unspent budget by subtracting it from the cumulative spend.
// This is used when a session finishes under its allocated budget.
func (be *BudgetEnforcer) Rebalance(returned float64) {
	be.mu.Lock()
	defer be.mu.Unlock()

	be.spent -= returned
	if be.spent < 0 {
		be.spent = 0
	}
}
