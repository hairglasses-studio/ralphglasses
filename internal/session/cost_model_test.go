package session

import (
	"testing"
	"time"
)

func TestCostHistory_AddAndAverage(t *testing.T) {
	dir := t.TempDir()
	ch := NewCostHistory(dir)

	ch.Add(CostRecord{Provider: "claude", CostUSD: 0.10, Turns: 5})
	ch.Add(CostRecord{Provider: "claude", CostUSD: 0.20, Turns: 10})
	ch.Add(CostRecord{Provider: "claude", CostUSD: 0.15, Turns: 7})

	avgSession := ch.AverageCostPerSession()
	if avgSession < 0.14 || avgSession > 0.16 {
		t.Errorf("unexpected avg cost per session: %f", avgSession)
	}

	avgTurn := ch.AverageCostPerTurn()
	if avgTurn <= 0 {
		t.Error("expected positive avg cost per turn")
	}
}

func TestCostHistory_ProjectBudget(t *testing.T) {
	dir := t.TempDir()
	ch := NewCostHistory(dir)

	ch.Add(CostRecord{CostUSD: 0.50, Turns: 10})
	ch.Add(CostRecord{CostUSD: 0.50, Turns: 10})

	proj := ch.ProjectBudget(5.00)
	if proj.EstimatedSessions != 10 {
		t.Errorf("expected 10 sessions, got %d", proj.EstimatedSessions)
	}
}

func TestCostHistory_EmptyProjection(t *testing.T) {
	dir := t.TempDir()
	ch := NewCostHistory(dir)

	proj := ch.ProjectBudget(5.00)
	if proj.EstimatedSessions != -1 {
		t.Errorf("expected -1 for unknown, got %d", proj.EstimatedSessions)
	}
}

func TestCostHistory_Persistence(t *testing.T) {
	dir := t.TempDir()
	ch := NewCostHistory(dir)
	ch.Add(CostRecord{Provider: "claude", CostUSD: 1.00, Turns: 5, Timestamp: time.Now()})

	// Reload from same path
	ch2 := NewCostHistory(dir)
	if len(ch2.Records) != 1 {
		t.Fatalf("expected 1 record after reload, got %d", len(ch2.Records))
	}
	if ch2.Records[0].CostUSD != 1.00 {
		t.Errorf("wrong cost: %f", ch2.Records[0].CostUSD)
	}
}

func TestCostHistory_ByProvider(t *testing.T) {
	dir := t.TempDir()
	ch := NewCostHistory(dir)
	ch.Add(CostRecord{Provider: "claude", CostUSD: 0.10})
	ch.Add(CostRecord{Provider: "gemini", CostUSD: 0.05})
	ch.Add(CostRecord{Provider: "claude", CostUSD: 0.20})

	claude := ch.ByProvider("claude")
	if len(claude) != 2 {
		t.Errorf("expected 2 claude records, got %d", len(claude))
	}
}

func TestCostHistory_RecentRecords(t *testing.T) {
	dir := t.TempDir()
	ch := NewCostHistory(dir)
	for i := 0; i < 10; i++ {
		ch.Add(CostRecord{CostUSD: float64(i) * 0.01})
	}

	recent := ch.RecentRecords(3)
	if len(recent) != 3 {
		t.Fatalf("expected 3 recent, got %d", len(recent))
	}
}
