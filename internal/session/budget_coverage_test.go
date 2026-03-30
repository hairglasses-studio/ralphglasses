package session

import (
	"testing"
)

func TestBudgetAlertLevel_NoBudget(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 0, SpentUSD: 100}
	if level := b.BudgetAlertLevel(s); level != nil {
		t.Errorf("expected nil for zero budget, got %+v", level)
	}
}

func TestBudgetAlertLevel_UnderAllThresholds(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 10}
	if level := b.BudgetAlertLevel(s); level != nil {
		t.Errorf("expected nil at 10%%, got %+v", level)
	}
}

func TestBudgetAlertLevel_At50Percent(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 55}
	level := b.BudgetAlertLevel(s)
	if level == nil {
		t.Fatal("expected non-nil at 55%")
	}
	if level.Label != "50%" {
		t.Errorf("expected 50%% threshold, got %s", level.Label)
	}
}

func TestBudgetAlertLevel_At75Percent(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 80}
	level := b.BudgetAlertLevel(s)
	if level == nil {
		t.Fatal("expected non-nil at 80%")
	}
	if level.Label != "75%" {
		t.Errorf("expected 75%% threshold, got %s", level.Label)
	}
}

func TestBudgetAlertLevel_At90Percent(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 95}
	level := b.BudgetAlertLevel(s)
	if level == nil {
		t.Fatal("expected non-nil at 95%")
	}
	if level.Label != "90%" {
		t.Errorf("expected 90%% threshold, got %s", level.Label)
	}
}

func TestCheckThresholds_NoBudget(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 0, SpentUSD: 999}
	crossed := b.CheckThresholds(s)
	if len(crossed) != 0 {
		t.Errorf("expected no thresholds for zero budget, got %d", len(crossed))
	}
}

func TestCheckThresholds_UnderAll(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 10}
	crossed := b.CheckThresholds(s)
	if len(crossed) != 0 {
		t.Errorf("expected no crossed thresholds at 10%%, got %d", len(crossed))
	}
}

func TestCheckThresholds_OneCrossed(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 55}
	crossed := b.CheckThresholds(s)
	if len(crossed) != 1 {
		t.Errorf("expected 1 crossed threshold at 55%%, got %d", len(crossed))
	}
}

func TestCheckThresholds_TwoCrossed(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 80}
	crossed := b.CheckThresholds(s)
	if len(crossed) != 2 {
		t.Errorf("expected 2 crossed thresholds at 80%%, got %d", len(crossed))
	}
}

func TestCheckThresholds_AllCrossed(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	s := &Session{BudgetUSD: 100, SpentUSD: 95}
	crossed := b.CheckThresholds(s)
	if len(crossed) != 3 {
		t.Errorf("expected 3 crossed thresholds at 95%%, got %d", len(crossed))
	}
}

func TestCheckThresholds_AtExactBoundary(t *testing.T) {
	t.Parallel()
	b := NewBudgetEnforcer()
	// Exactly at 50%.
	s := &Session{BudgetUSD: 100, SpentUSD: 50}
	crossed := b.CheckThresholds(s)
	if len(crossed) != 1 {
		t.Errorf("expected 1 crossed threshold at exactly 50%%, got %d", len(crossed))
	}
	if crossed[0].Label != "50%" {
		t.Errorf("expected 50%% label, got %s", crossed[0].Label)
	}
}
