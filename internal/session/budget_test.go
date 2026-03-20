package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBudgetEnforcerCheck(t *testing.T) {
	b := NewBudgetEnforcer()

	// No budget set — never exceeded
	s := &Session{BudgetUSD: 0, SpentUSD: 100}
	exceeded, _ := b.Check(s)
	if exceeded {
		t.Error("should not exceed when no budget set")
	}

	// Under budget
	s = &Session{BudgetUSD: 10, SpentUSD: 5}
	exceeded, _ = b.Check(s)
	if exceeded {
		t.Error("should not exceed at 50%")
	}

	// At headroom threshold (90%)
	s = &Session{BudgetUSD: 10, SpentUSD: 9}
	exceeded, reason := b.Check(s)
	if !exceeded {
		t.Error("should exceed at 90%")
	}
	if reason == "" {
		t.Error("expected reason string")
	}

	// Over budget
	s = &Session{BudgetUSD: 10, SpentUSD: 11}
	exceeded, _ = b.Check(s)
	if !exceeded {
		t.Error("should exceed when over budget")
	}
}

func TestBudgetEnforcerWriteLedger(t *testing.T) {
	root := t.TempDir()
	b := NewBudgetEnforcer()

	s := &Session{
		ID:       "test-session",
		SpentUSD: 1.50,
		Model:    "sonnet",
		Status:   StatusRunning,
	}

	if err := b.WriteLedgerEntry(s, root); err != nil {
		t.Fatalf("WriteLedgerEntry: %v", err)
	}

	ledgerPath := filepath.Join(root, ".ralph", "logs", "cost_ledger.jsonl")
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("read ledger: %v", err)
	}
	if len(data) == 0 {
		t.Error("ledger file is empty")
	}
}

func TestBudgetEnforcerWriteCostSummary(t *testing.T) {
	root := t.TempDir()
	b := NewBudgetEnforcer()

	s := &Session{
		ID:        "test-session",
		RepoName:  "myrepo",
		SpentUSD:  2.50,
		BudgetUSD: 10,
		TurnCount: 5,
		Model:     "opus",
		Status:    StatusCompleted,
	}

	if err := b.WriteCostSummary(s, root); err != nil {
		t.Fatalf("WriteCostSummary: %v", err)
	}

	summaryPath := filepath.Join(root, ".ralph", "cost_summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	if len(data) == 0 {
		t.Error("summary file is empty")
	}
}
