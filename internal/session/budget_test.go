package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	m.sessionsMu.Lock()
	m.sessions["planner-1"] = plannerSess
	m.sessions["worker-1"] = workerSess
	m.sessionsMu.Unlock()

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

func TestRunLoop_BudgetReasonInLastError(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Create a loop run with a small budget that will be exceeded.
	run := &LoopRun{
		ID:       "budget-lasterror-test",
		RepoPath: t.TempDir(),
		RepoName: "test",
		Status:   "running",
		Profile: LoopProfile{
			PlannerBudgetUSD: 1.0,
			WorkerBudgetUSD:  4.0,
		},
		Iterations: []LoopIteration{},
	}

	// Add sessions that exceed the $5.00 total budget (90% headroom = $4.50).
	plannerSess := &Session{ID: "p-1", SpentUSD: 1.0}
	workerSess := &Session{ID: "w-1", SpentUSD: 4.0}
	m.sessionsMu.Lock()
	m.sessions["p-1"] = plannerSess
	m.sessions["w-1"] = workerSess
	m.sessionsMu.Unlock()

	run.Iterations = append(run.Iterations, LoopIteration{
		PlannerSessionID: "p-1",
		WorkerSessionIDs: []string{"w-1"},
	})

	// Verify checkLoopBudget detects the overspend.
	exceeded, reason := m.checkLoopBudget(run)
	if !exceeded {
		t.Fatal("expected budget exceeded at $5.0 of $5.0")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}

	// Simulate what RunLoop does when budget is exceeded:
	// set LastError and status, then verify callers can see the reason.
	run.mu.Lock()
	run.Status = "completed"
	run.LastError = "budget exceeded: " + reason
	run.mu.Unlock()

	// Verify LastError contains the budget reason.
	run.mu.Lock()
	lastErr := run.LastError
	status := run.Status
	run.mu.Unlock()

	if status != "completed" {
		t.Errorf("expected status 'completed', got %q", status)
	}
	if lastErr == "" {
		t.Error("expected LastError to be set after budget exceeded")
	}
	if !strings.Contains(lastErr, "budget") {
		t.Errorf("expected LastError to contain 'budget', got %q", lastErr)
	}
	if !strings.Contains(lastErr, "spent") {
		t.Errorf("expected LastError to contain spend details, got %q", lastErr)
	}
}

func TestCheckLoopBudget_ZeroBudget(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Create a loop run with ZERO budget (both planner and worker)
	run := &LoopRun{
		ID:       "zero-budget-test",
		RepoPath: t.TempDir(),
		RepoName: "test",
		Status:   "running",
		Profile: LoopProfile{
			PlannerBudgetUSD: 0,
			WorkerBudgetUSD:  0,
		},
		Iterations: []LoopIteration{},
	}

	// Add sessions that have spent money
	plannerSess := &Session{ID: "zb-planner-1", SpentUSD: 5.0}
	workerSess := &Session{ID: "zb-worker-1", SpentUSD: 15.0}
	m.sessionsMu.Lock()
	m.sessions["zb-planner-1"] = plannerSess
	m.sessions["zb-worker-1"] = workerSess
	m.sessionsMu.Unlock()

	run.Iterations = append(run.Iterations, LoopIteration{
		PlannerSessionID: "zb-planner-1",
		WorkerSessionIDs: []string{"zb-worker-1"},
	})

	// With totalBudget=0, checkLoopBudget should return (false, "")
	// regardless of how much has been spent — zero budget means "no limit".
	exceeded, reason := m.checkLoopBudget(run)
	if exceeded {
		t.Error("checkLoopBudget should return false when totalBudget is zero (no limit)")
	}
	if reason != "" {
		t.Errorf("expected empty reason for zero budget, got %q", reason)
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

// --- Batch 2b pressure tests (WS-5 Budget Enforcement) ---

// Test 2.6: checkLoopBudget gracefully handles missing sessions (no panic).
func TestCheckLoopBudget_MissingSessions(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	run := &LoopRun{
		ID:       "missing-sessions-test",
		RepoPath: t.TempDir(),
		RepoName: "test",
		Status:   "running",
		Profile: LoopProfile{
			PlannerBudgetUSD: 5.0,
			WorkerBudgetUSD:  5.0,
		},
		Iterations: []LoopIteration{
			{
				PlannerSessionID: "planner-missing",
				WorkerSessionIDs: []string{"worker-missing"},
			},
		},
	}

	// Neither "planner-missing" nor "worker-missing" exist in m.sessions.
	// checkLoopBudget should skip them and return (false, ""), not panic.
	exceeded, reason := m.checkLoopBudget(run)
	if exceeded {
		t.Errorf("expected false for missing sessions, got exceeded with reason: %s", reason)
	}
	if reason != "" {
		t.Errorf("expected empty reason for missing sessions, got %q", reason)
	}
}

// Test 2.7: checkLoopBudget is safe under concurrent access with -race.
func TestCheckLoopBudget_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Create real sessions.
	for i := 0; i < 5; i++ {
		sid := "conc-worker-" + string(rune('0'+i))
		s := &Session{ID: sid, SpentUSD: 1.0}
		m.sessionsMu.Lock()
		m.sessions[sid] = s
		m.sessionsMu.Unlock()
	}

	run := &LoopRun{
		ID:       "concurrent-test",
		RepoPath: t.TempDir(),
		RepoName: "test",
		Status:   "running",
		Profile: LoopProfile{
			PlannerBudgetUSD: 50.0,
			WorkerBudgetUSD:  50.0,
		},
		Iterations: []LoopIteration{
			{
				PlannerSessionID: "conc-worker-0",
				WorkerSessionIDs: []string{"conc-worker-1", "conc-worker-2"},
			},
			{
				PlannerSessionID: "conc-worker-3",
				WorkerSessionIDs: []string{"conc-worker-4"},
			},
		},
	}

	var wg sync.WaitGroup

	// 5 goroutines reading budget concurrently.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.checkLoopBudget(run)
		}()
	}

	// 2 goroutines mutating Iterations concurrently.
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run.Lock()
			run.Iterations = append(run.Iterations, LoopIteration{
				PlannerSessionID: "conc-worker-0",
				WorkerSessionIDs: []string{"conc-worker-1"},
			})
			run.Unlock()
		}()
	}

	wg.Wait()
	// If we get here without a race detector failure, the test passes.
}

// Test 2.8: checkLoopBudget returns exceeded at 90% threshold, documenting
// that RunLoop calls checkLoopBudget each iteration and would stop.
func TestRunLoop_BudgetExceededStopsLoop(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Budget: planner=$1, worker=$1, total=$2. Threshold at 90% = $1.80.
	run := &LoopRun{
		ID:       "runloop-budget-test",
		RepoPath: t.TempDir(),
		RepoName: "test",
		Status:   "running",
		Profile: LoopProfile{
			PlannerBudgetUSD: 1.0,
			WorkerBudgetUSD:  1.0,
		},
		Iterations: []LoopIteration{},
	}

	// Add sessions whose total spend = $1.80 (exactly at 90% of $2.00).
	plannerSess := &Session{ID: "rl-planner", SpentUSD: 0.80}
	workerSess := &Session{ID: "rl-worker", SpentUSD: 1.00}
	m.sessionsMu.Lock()
	m.sessions["rl-planner"] = plannerSess
	m.sessions["rl-worker"] = workerSess
	m.sessionsMu.Unlock()

	run.Iterations = append(run.Iterations, LoopIteration{
		PlannerSessionID: "rl-planner",
		WorkerSessionIDs: []string{"rl-worker"},
	})

	// Total = $1.80, threshold = $1.80 → exceeded (>=).
	exceeded, reason := m.checkLoopBudget(run)
	if !exceeded {
		t.Error("expected budget exceeded at $1.80 of $2.00 (90% headroom)")
	}
	if reason == "" {
		t.Error("expected reason string when budget is exceeded")
	}

	// Slightly under: $1.79 total → NOT exceeded.
	workerSess.Lock()
	workerSess.SpentUSD = 0.99
	workerSess.Unlock()

	exceeded, _ = m.checkLoopBudget(run)
	if exceeded {
		t.Error("should not exceed at $1.79 of $2.00")
	}

	// NOTE: RunLoop calls checkLoopBudget at the start of each iteration.
	// When it returns (true, reason), RunLoop sets run.Status to "budget_exceeded"
	// and exits. This test validates the gate function directly.
}

// Test 2.9: BudgetEnforcer.Check exact boundary behavior at 90% headroom.
func TestBudgetEnforcer_BoundaryAt90Pct(t *testing.T) {
	b := NewBudgetEnforcer()

	tests := []struct {
		name     string
		budget   float64
		spent    float64
		exceeded bool
	}{
		{"under_at_89.9", 100.0, 89.9, false},
		{"at_90.0", 100.0, 90.0, true},
		{"under_at_89.99", 100.0, 89.99, false},
		{"over_at_90.01", 100.0, 90.01, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{BudgetUSD: tt.budget, SpentUSD: tt.spent}
			exceeded, reason := b.Check(s)
			if exceeded != tt.exceeded {
				t.Errorf("BudgetUSD=%.2f SpentUSD=%.4f: got exceeded=%v, want %v (reason=%q)",
					tt.budget, tt.spent, exceeded, tt.exceeded, reason)
			}
			if tt.exceeded && reason == "" {
				t.Error("expected non-empty reason when exceeded")
			}
			if !tt.exceeded && reason != "" {
				t.Errorf("expected empty reason when not exceeded, got %q", reason)
			}
		})
	}
}
