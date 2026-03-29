package pool

import (
	"sync"
	"testing"
	"time"
)

func TestNewState(t *testing.T) {
	t.Run("no cap", func(t *testing.T) {
		s := NewState(0)
		if s.BudgetCapUSD != 0 {
			t.Fatalf("expected 0 cap, got %f", s.BudgetCapUSD)
		}
		if s.ProviderCounts == nil {
			t.Fatal("ProviderCounts should be initialized")
		}
	})
	t.Run("with cap", func(t *testing.T) {
		s := NewState(100.0)
		if s.BudgetCapUSD != 100.0 {
			t.Fatalf("expected 100 cap, got %f", s.BudgetCapUSD)
		}
	})
}

func TestUpdate(t *testing.T) {
	s := NewState(50.0)
	sessions := []SessionSnapshot{
		{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 1.50, BudgetUSD: 5.0, StartedAt: time.Now()},
		{ID: "s2", Provider: "gemini", Status: "running", SpentUSD: 0.75, BudgetUSD: 3.0, StartedAt: time.Now()},
		{ID: "s3", Provider: "claude", Status: "completed", SpentUSD: 2.00, BudgetUSD: 5.0, StartedAt: time.Now()},
		{ID: "s4", Provider: "codex", Status: "launching", SpentUSD: 0.0, BudgetUSD: 2.0, StartedAt: time.Now()},
	}

	s.Update(sessions)

	if s.TotalSessions != 4 {
		t.Errorf("TotalSessions: got %d, want 4", s.TotalSessions)
	}
	if s.ActiveSessions != 3 {
		t.Errorf("ActiveSessions: got %d, want 3", s.ActiveSessions)
	}
	wantSpent := 4.25
	if s.TotalSpentUSD != wantSpent {
		t.Errorf("TotalSpentUSD: got %f, want %f", s.TotalSpentUSD, wantSpent)
	}
	wantBudget := 15.0
	if s.TotalBudgetUSD != wantBudget {
		t.Errorf("TotalBudgetUSD: got %f, want %f", s.TotalBudgetUSD, wantBudget)
	}
	if s.ProviderCounts["claude"] != 2 {
		t.Errorf("claude count: got %d, want 2", s.ProviderCounts["claude"])
	}
	if s.ProviderCounts["gemini"] != 1 {
		t.Errorf("gemini count: got %d, want 1", s.ProviderCounts["gemini"])
	}
	if s.ProviderCounts["codex"] != 1 {
		t.Errorf("codex count: got %d, want 1", s.ProviderCounts["codex"])
	}
	if s.LastUpdate.IsZero() {
		t.Error("LastUpdate should be set after Update")
	}
	if len(s.CostHistory) != 1 {
		t.Errorf("CostHistory length: got %d, want 1", len(s.CostHistory))
	}
}

func TestCanSpend(t *testing.T) {
	t.Run("no cap allows any spend", func(t *testing.T) {
		s := NewState(0)
		s.Update([]SessionSnapshot{
			{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 999.0},
		})
		if !s.CanSpend(1000.0) {
			t.Error("should allow spend when no cap set")
		}
	})
	t.Run("under cap", func(t *testing.T) {
		s := NewState(10.0)
		s.Update([]SessionSnapshot{
			{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 5.0},
		})
		if !s.CanSpend(4.0) {
			t.Error("should allow spend under cap")
		}
	})
	t.Run("at cap", func(t *testing.T) {
		s := NewState(10.0)
		s.Update([]SessionSnapshot{
			{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 10.0},
		})
		if s.CanSpend(0.01) {
			t.Error("should deny spend at cap")
		}
	})
	t.Run("over cap", func(t *testing.T) {
		s := NewState(10.0)
		s.Update([]SessionSnapshot{
			{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 8.0},
		})
		if s.CanSpend(3.0) {
			t.Error("should deny spend that would exceed cap")
		}
	})
}

func TestCostRate(t *testing.T) {
	s := NewState(0)

	s.mu.Lock()
	now := time.Now()
	s.CostHistory = append(s.CostHistory, CostSample{
		Time:     now.Add(-30 * time.Minute),
		SpentUSD: 1.00,
	})
	s.CostHistory = append(s.CostHistory, CostSample{
		Time:     now,
		SpentUSD: 3.00,
	})
	s.mu.Unlock()

	// $2.00 over 30 minutes = $4.00/hour
	rate := s.CostRate(time.Hour)
	if rate < 3.9 || rate > 4.1 {
		t.Errorf("CostRate: got %f, want ~4.0", rate)
	}
}

func TestCostRateEmpty(t *testing.T) {
	s := NewState(0)
	rate := s.CostRate(time.Hour)
	if rate != 0 {
		t.Errorf("CostRate with no history should be 0, got %f", rate)
	}
}

func TestGetSummary(t *testing.T) {
	s := NewState(20.0)
	s.Update([]SessionSnapshot{
		{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 15.0, BudgetUSD: 10.0},
		{ID: "s2", Provider: "gemini", Status: "completed", SpentUSD: 4.0, BudgetUSD: 5.0},
	})

	sum := s.GetSummary()

	if sum.TotalSpentUSD != 19.0 {
		t.Errorf("TotalSpentUSD: got %f, want 19.0", sum.TotalSpentUSD)
	}
	if sum.BudgetCapUSD != 20.0 {
		t.Errorf("BudgetCapUSD: got %f, want 20.0", sum.BudgetCapUSD)
	}
	if sum.BudgetPct != 95.0 {
		t.Errorf("BudgetPct: got %f, want 95.0", sum.BudgetPct)
	}
	if !sum.AtCapacity {
		t.Error("AtCapacity should be true at 95%")
	}
	if sum.ActiveSessions != 1 {
		t.Errorf("ActiveSessions: got %d, want 1", sum.ActiveSessions)
	}
	if sum.ProviderCounts["claude"] != 1 {
		t.Errorf("claude count: got %d, want 1", sum.ProviderCounts["claude"])
	}
}

func TestGetSummaryNoCap(t *testing.T) {
	s := NewState(0)
	s.Update([]SessionSnapshot{
		{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 5.0},
	})
	sum := s.GetSummary()
	if sum.BudgetPct != 0 {
		t.Errorf("BudgetPct with no cap should be 0, got %f", sum.BudgetPct)
	}
	if sum.AtCapacity {
		t.Error("AtCapacity should be false with no cap")
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewState(100.0)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s.Update([]SessionSnapshot{
				{ID: "s1", Provider: "claude", Status: "running", SpentUSD: float64(i)},
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.CanSpend(1.0)
			_ = s.CostRate(time.Hour)
			_ = s.GetSummary()
		}()
	}

	wg.Wait()
}

func TestCostHistoryPruning(t *testing.T) {
	s := NewState(0)
	s.mu.Lock()
	base := time.Now().Add(-400 * 10 * time.Second)
	for i := 0; i < 400; i++ {
		s.CostHistory = append(s.CostHistory, CostSample{
			Time:     base.Add(time.Duration(i) * 10 * time.Second),
			SpentUSD: float64(i) * 0.01,
		})
	}
	s.mu.Unlock()

	s.Update([]SessionSnapshot{
		{ID: "s1", Provider: "claude", Status: "running", SpentUSD: 5.0},
	})

	s.mu.RLock()
	histLen := len(s.CostHistory)
	s.mu.RUnlock()

	if histLen > 360 {
		t.Errorf("CostHistory should be pruned to <=360, got %d", histLen)
	}
}
