package loop

import (
	"math"
	"testing"
)

func TestBudgetEnforcer_ThresholdsProgression(t *testing.T) {
	be := NewBudgetEnforcer(100.0)

	// Under 80% -> Continue
	be.Record(79.0)
	if got := be.Check(); got != ActionContinue {
		t.Fatalf("at 79%%: expected continue, got %s", got)
	}

	// At 80% -> Warn
	be.Record(1.0)
	if got := be.Check(); got != ActionWarn {
		t.Fatalf("at 80%%: expected warn, got %s", got)
	}

	// At 90% -> Cooldown
	be.Record(10.0)
	if got := be.Check(); got != ActionCooldown {
		t.Fatalf("at 90%%: expected cooldown, got %s", got)
	}

	// At 95% -> Escalate
	be.Record(5.0)
	if got := be.Check(); got != ActionEscalate {
		t.Fatalf("at 95%%: expected escalate, got %s", got)
	}

	// At 100% -> Stop
	be.Record(5.0)
	if got := be.Check(); got != ActionStop {
		t.Fatalf("at 100%%: expected stop, got %s", got)
	}
}

func TestBudgetEnforcer_Remaining(t *testing.T) {
	be := NewBudgetEnforcer(50.0)
	be.Record(30.0)

	if got := be.Remaining(); got != 20.0 {
		t.Fatalf("expected remaining 20.0, got %f", got)
	}

	// Overspend: remaining should be 0, not negative.
	be.Record(25.0)
	if got := be.Remaining(); got != 0 {
		t.Fatalf("expected remaining 0 when overspent, got %f", got)
	}
}

func TestBudgetEnforcer_SpentPercent(t *testing.T) {
	be := NewBudgetEnforcer(200.0)
	be.Record(100.0)

	if got := be.SpentPercent(); got != 50.0 {
		t.Fatalf("expected 50%%, got %f%%", got)
	}

	be.Record(100.0)
	if got := be.SpentPercent(); got != 100.0 {
		t.Fatalf("expected 100%%, got %f%%", got)
	}
}

func TestBudgetEnforcer_Rebalance(t *testing.T) {
	be := NewBudgetEnforcer(100.0)
	be.Record(90.0)

	if got := be.Check(); got != ActionCooldown {
		t.Fatalf("pre-rebalance: expected cooldown, got %s", got)
	}

	// Return 20 -> now at 70% -> should be Continue
	be.Rebalance(20.0)
	if got := be.Check(); got != ActionContinue {
		t.Fatalf("post-rebalance: expected continue, got %s", got)
	}
	if got := be.Remaining(); got != 30.0 {
		t.Fatalf("expected remaining 30.0, got %f", got)
	}
}

func TestBudgetEnforcer_RebalanceFloor(t *testing.T) {
	be := NewBudgetEnforcer(100.0)
	be.Record(10.0)
	be.Rebalance(50.0) // return more than spent

	if got := be.Remaining(); got != 100.0 {
		t.Fatalf("rebalance beyond spent should floor at 0 spent, remaining=100, got %f", got)
	}
}

func TestBudgetEnforcer_ZeroBudget(t *testing.T) {
	be := NewBudgetEnforcer(0)
	be.Record(100.0)

	// Zero budget = unlimited, always continue.
	if got := be.Check(); got != ActionContinue {
		t.Fatalf("zero budget: expected continue, got %s", got)
	}
	if got := be.Remaining(); got != 0 {
		t.Fatalf("zero budget: expected remaining 0, got %f", got)
	}
	if got := be.SpentPercent(); got != 0 {
		t.Fatalf("zero budget: expected spent percent 0, got %f", got)
	}
}

func TestBudgetEnforcer_NegativeBudget(t *testing.T) {
	be := NewBudgetEnforcer(-10.0)
	be.Record(50.0)

	if got := be.Check(); got != ActionContinue {
		t.Fatalf("negative budget: expected continue, got %s", got)
	}
}

func TestBudgetEnforcer_OverspendPercent(t *testing.T) {
	be := NewBudgetEnforcer(100.0)
	be.Record(150.0)

	if got := be.SpentPercent(); got != 150.0 {
		t.Fatalf("expected 150%% when overspent, got %f%%", got)
	}
}

func TestBudgetAction_String(t *testing.T) {
	tests := map[BudgetAction]string{
		ActionContinue: "continue",
		ActionWarn:     "warn",
		ActionCooldown: "cooldown",
		ActionEscalate: "escalate",
		ActionStop:     "stop",
		BudgetAction(99): "unknown",
	}
	for action, expected := range tests {
		if got := action.String(); got != expected {
			t.Errorf("BudgetAction(%d).String() = %q, want %q", int(action), got, expected)
		}
	}
}

func TestBudgetEnforcer_PrecisionEdgeCases(t *testing.T) {
	be := NewBudgetEnforcer(100.0)

	// Just under 80%
	be.Record(79.99)
	if got := be.Check(); got != ActionContinue {
		t.Fatalf("at 79.99%%: expected continue, got %s", got)
	}

	// Tiny push over 80%
	be.Record(0.01)
	if got := be.Check(); got != ActionWarn {
		t.Fatalf("at 80.00%%: expected warn, got %s", got)
	}

	// Verify float precision is reasonable
	remaining := be.Remaining()
	if math.Abs(remaining-20.0) > 0.001 {
		t.Fatalf("expected ~20.0 remaining, got %f", remaining)
	}
}
