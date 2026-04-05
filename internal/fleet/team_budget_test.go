package fleet

import (
	"testing"
)

func TestBudgetStore_SetAndGet(t *testing.T) {
	t.Parallel()
	s := NewBudgetStore()
	s.SetBudget("team-a", 100.0)

	b, ok := s.GetBudget("team-a")
	if !ok {
		t.Fatal("budget not found")
	}
	if b.CapUSD != 100.0 {
		t.Errorf("CapUSD = %v, want 100", b.CapUSD)
	}
	if b.SpentUSD != 0 {
		t.Errorf("SpentUSD = %v, want 0", b.SpentUSD)
	}
}

func TestBudgetStore_RecordSpend(t *testing.T) {
	t.Parallel()
	s := NewBudgetStore()
	s.SetBudget("team-a", 100.0)

	// Spend 45 — no alert
	alerts := s.RecordSpend("team-a", 45.0)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts at 45%%, got %d", len(alerts))
	}

	// Spend 10 more (55 total) — 50% alert
	alerts = s.RecordSpend("team-a", 10.0)
	if len(alerts) != 1 || alerts[0].Threshold != 50 {
		t.Errorf("expected 50%% alert, got %v", alerts)
	}

	// Spend 25 more (80 total) — 75% alert
	alerts = s.RecordSpend("team-a", 25.0)
	if len(alerts) != 1 || alerts[0].Threshold != 75 {
		t.Errorf("expected 75%% alert, got %v", alerts)
	}

	// Spend 15 more (95 total) — 90% alert
	alerts = s.RecordSpend("team-a", 15.0)
	if len(alerts) != 1 || alerts[0].Threshold != 90 {
		t.Errorf("expected 90%% alert, got %v", alerts)
	}

	// No duplicate alerts
	alerts = s.RecordSpend("team-a", 1.0)
	if len(alerts) != 0 {
		t.Errorf("expected no duplicate alerts, got %d", len(alerts))
	}

	// 99% alert
	alerts = s.RecordSpend("team-a", 3.5)
	if len(alerts) != 1 || alerts[0].Threshold != 99 {
		t.Errorf("expected 99%% alert, got %v", alerts)
	}
}

func TestBudgetStore_CanSpend(t *testing.T) {
	t.Parallel()
	s := NewBudgetStore()
	s.SetBudget("team-a", 50.0)

	if !s.CanSpend("team-a", 30.0) {
		t.Error("should be able to spend 30 of 50")
	}
	s.RecordSpend("team-a", 30.0)
	if !s.CanSpend("team-a", 20.0) {
		t.Error("should be able to spend exactly to cap")
	}
	if s.CanSpend("team-a", 21.0) {
		t.Error("should NOT be able to exceed cap")
	}
}

func TestBudgetStore_UnknownTeam(t *testing.T) {
	t.Parallel()
	s := NewBudgetStore()

	if !s.CanSpend("unknown", 1000.0) {
		t.Error("unknown team should have no limit")
	}
	alerts := s.RecordSpend("unknown", 100.0)
	if len(alerts) != 0 {
		t.Error("unknown team should produce no alerts")
	}
	_, ok := s.GetBudget("unknown")
	if ok {
		t.Error("unknown team should not exist")
	}
}

func TestBudgetStore_ListBudgets(t *testing.T) {
	t.Parallel()
	s := NewBudgetStore()
	s.SetBudget("a", 10.0)
	s.SetBudget("b", 20.0)

	budgets := s.ListBudgets()
	if len(budgets) != 2 {
		t.Errorf("ListBudgets = %d, want 2", len(budgets))
	}
}

func TestBudgetStore_UpdateExisting(t *testing.T) {
	t.Parallel()
	s := NewBudgetStore()
	s.SetBudget("team", 50.0)
	s.RecordSpend("team", 30.0)
	s.SetBudget("team", 100.0) // Update cap

	b, _ := s.GetBudget("team")
	if b.CapUSD != 100.0 {
		t.Errorf("CapUSD = %v, want 100 after update", b.CapUSD)
	}
	if b.SpentUSD != 30.0 {
		t.Errorf("SpentUSD = %v, want 30 (preserved)", b.SpentUSD)
	}
}
