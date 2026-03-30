package pool

import (
	"testing"
)

func TestSetBudgetCap(t *testing.T) {
	t.Parallel()
	s := NewState(100.0)
	if s.BudgetCapUSD != 100.0 {
		t.Errorf("initial BudgetCapUSD = %f, want 100.0", s.BudgetCapUSD)
	}

	s.SetBudgetCap(250.0)
	if s.BudgetCapUSD != 250.0 {
		t.Errorf("after SetBudgetCap: BudgetCapUSD = %f, want 250.0", s.BudgetCapUSD)
	}
}

func TestSetBudgetCap_Zero(t *testing.T) {
	t.Parallel()
	s := NewState(50.0)
	s.SetBudgetCap(0)
	if s.BudgetCapUSD != 0 {
		t.Errorf("BudgetCapUSD = %f, want 0 (unlimited)", s.BudgetCapUSD)
	}

	// With cap=0, CanSpend should always return true.
	if !s.CanSpend(1000000) {
		t.Error("CanSpend should return true with zero cap (unlimited)")
	}
}

func TestSetBudgetCap_AffectsCanSpend(t *testing.T) {
	t.Parallel()
	s := NewState(10.0)
	s.TotalSpentUSD = 8.0

	if !s.CanSpend(1.0) {
		t.Error("CanSpend(1.0) should be true when 8+1 <= 10")
	}
	if s.CanSpend(3.0) {
		t.Error("CanSpend(3.0) should be false when 8+3 > 10")
	}

	// Raise the cap.
	s.SetBudgetCap(20.0)
	if !s.CanSpend(3.0) {
		t.Error("CanSpend(3.0) should be true after raising cap to 20")
	}
}
