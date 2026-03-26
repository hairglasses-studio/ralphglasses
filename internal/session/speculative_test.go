package session

import (
	"testing"
)

func makeSession(status SessionStatus, turnCount int, spentUSD float64) *Session {
	return &Session{
		Status:    status,
		TurnCount: turnCount,
		SpentUSD:  spentUSD,
	}
}

func TestResolveSpeculative_BothVerified(t *testing.T) {
	cheap := makeSession(StatusCompleted, 5, 0.50)
	expensive := makeSession(StatusCompleted, 5, 3.00)

	result := ResolveSpeculative(cheap, expensive, true, true)

	if result.Winner != "cheap" {
		t.Errorf("expected winner=cheap, got %s", result.Winner)
	}
	if !result.CheapDone || !result.ExpensiveDone {
		t.Errorf("expected both done, got cheap=%v expensive=%v", result.CheapDone, result.ExpensiveDone)
	}
	if !result.CheapVerified || !result.ExpensiveVerified {
		t.Error("expected both verified")
	}
	if result.CostSavedUSD <= 0 {
		t.Errorf("expected positive cost savings, got %.2f", result.CostSavedUSD)
	}
}

func TestResolveSpeculative_OnlyExpensiveVerified(t *testing.T) {
	cheap := makeSession(StatusCompleted, 5, 0.50)
	expensive := makeSession(StatusCompleted, 5, 3.00)

	result := ResolveSpeculative(cheap, expensive, false, true)

	if result.Winner != "expensive" {
		t.Errorf("expected winner=expensive, got %s", result.Winner)
	}
}

func TestResolveSpeculative_NeitherVerified(t *testing.T) {
	cheap := makeSession(StatusCompleted, 5, 0.50)
	expensive := makeSession(StatusCompleted, 5, 3.00)

	result := ResolveSpeculative(cheap, expensive, false, false)

	if result.Winner != "expensive" {
		t.Errorf("expected winner=expensive (higher capability), got %s", result.Winner)
	}
}

func TestCheapCost_Nil(t *testing.T) {
	if got := cheapCost(nil); got != 0 {
		t.Errorf("cheapCost(nil) = %f, want 0", got)
	}
}

func TestExpensiveCost_Nil(t *testing.T) {
	if got := expensiveCost(nil); got != 0 {
		t.Errorf("expensiveCost(nil) = %f, want 0", got)
	}
}

func TestCheapCost_WithSession(t *testing.T) {
	s := makeSession(StatusCompleted, 5, 1.50)
	if got := cheapCost(s); got != 1.50 {
		t.Errorf("cheapCost = %f, want 1.50", got)
	}
}

func TestResolveSpeculative_OnlyCheapDone(t *testing.T) {
	cheap := makeSession(StatusCompleted, 5, 0.50)
	expensive := makeSession(StatusRunning, 2, 1.00)

	result := ResolveSpeculative(cheap, expensive, false, false)

	if result.Winner != "cheap" {
		t.Errorf("expected winner=cheap (only one done), got %s", result.Winner)
	}
	if !result.CheapDone {
		t.Error("expected cheap to be done")
	}
	if result.ExpensiveDone {
		t.Error("expected expensive to not be done")
	}
}
