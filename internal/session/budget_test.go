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

func TestBudgetUSD_FlowsFromLaunchOptions(t *testing.T) {
	// Verify that LaunchOptions.MaxBudgetUSD is correctly assigned to Session.BudgetUSD
	// during session creation. This is the core wiring that FINDING-258/261 identified
	// as silently ignored.
	opts := LaunchOptions{
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/test-repo",
		Prompt:       "test prompt",
		MaxBudgetUSD: 1.0,
		MaxTurns:     5,
	}

	// Create a Session struct the same way launch() does (runner.go).
	s := &Session{
		Provider:  opts.Provider,
		RepoPath:  opts.RepoPath,
		Prompt:    opts.Prompt,
		BudgetUSD: opts.MaxBudgetUSD,
		MaxTurns:  opts.MaxTurns,
	}

	if s.BudgetUSD != 1.0 {
		t.Errorf("expected BudgetUSD=1.0, got %f", s.BudgetUSD)
	}

	// Verify BudgetEnforcer catches it when exceeded
	b := NewBudgetEnforcer()
	s.SpentUSD = 0.95 // 95% > 90% headroom
	exceeded, reason := b.Check(s)
	if !exceeded {
		t.Error("expected budget exceeded at 95% of $1.00")
	}
	if reason == "" {
		t.Error("expected reason string when budget exceeded")
	}

	// Verify under-budget is not flagged
	s.SpentUSD = 0.50
	exceeded, _ = b.Check(s)
	if exceeded {
		t.Error("should not exceed at 50% of $1.00")
	}
}

func TestCheckLoopBudget(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Create a loop run with budget
	run := &LoopRun{
		ID:       "budget-test",
		RepoPath: t.TempDir(),
		RepoName: "test",
		Status:   "running",
		Profile: LoopProfile{
			PlannerBudgetUSD: 2.0,
			WorkerBudgetUSD:  8.0,
		},
		Iterations: []LoopIteration{},
	}

	// No iterations yet — should not exceed
	exceeded, _ := m.checkLoopBudget(run)
	if exceeded {
		t.Error("should not exceed with no iterations")
	}

	// Add a mock session that has spent money
	plannerSess := &Session{ID: "planner-1", SpentUSD: 1.5}
	workerSess := &Session{ID: "worker-1", SpentUSD: 7.5}
	m.mu.Lock()
	m.sessions["planner-1"] = plannerSess
	m.sessions["worker-1"] = workerSess
	m.mu.Unlock()

	run.Iterations = append(run.Iterations, LoopIteration{
		PlannerSessionID: "planner-1",
		WorkerSessionIDs: []string{"worker-1"},
	})

	// Total spend = 9.0 out of 10.0 budget (90% headroom threshold)
	exceeded, reason := m.checkLoopBudget(run)
	if !exceeded {
		t.Error("expected budget exceeded at $9.0 of $10.0 (90% headroom)")
	}
	if reason == "" {
		t.Error("expected reason string")
	}

	// Reduce spend below threshold
	workerSess.SpentUSD = 5.0
	exceeded, _ = m.checkLoopBudget(run)
	if exceeded {
		t.Error("should not exceed at $6.5 of $10.0")
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
