package fleet

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestFleetRecoveryOrchestrator_EmptyPlan(t *testing.T) {
	coord := newTestCoordinator()
	fro := NewFleetRecoveryOrchestrator(coord, nil)

	n, err := fro.DistributeRecoveryPlan(&session.CrashRecoveryPlan{}, 1.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 submitted, got %d", n)
	}
}

func TestFleetRecoveryOrchestrator_NilPlan(t *testing.T) {
	coord := newTestCoordinator()
	fro := NewFleetRecoveryOrchestrator(coord, nil)

	n, err := fro.DistributeRecoveryPlan(nil, 1.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 submitted, got %d", n)
	}
}

func TestFleetRecoveryOrchestrator_SubmitsSessions(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(100.0) // ensure budget allows work
	fro := NewFleetRecoveryOrchestrator(coord, nil)

	plan := &session.CrashRecoveryPlan{
		DetectedAt: time.Now(),
		Severity:   "major",
		DeadCount:  3,
		SessionsToResume: []session.RecoverableSession{
			{SessionID: "sess-001", RepoPath: "/tmp/repo-a", RepoName: "repo-a", Priority: 1, OpenTasks: 5, ResumePrompt: "resume A"},
			{SessionID: "sess-002", RepoPath: "/tmp/repo-b", RepoName: "repo-b", Priority: 2, OpenTasks: 2, ResumePrompt: "resume B"},
		},
	}

	n, err := fro.DistributeRecoveryPlan(plan, 2.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 submitted, got %d", n)
	}

	dispatched := fro.DispatchedItems()
	if len(dispatched) != 2 {
		t.Errorf("expected 2 dispatched items, got %d", len(dispatched))
	}
	if _, ok := dispatched["sess-001"]; !ok {
		t.Error("expected sess-001 in dispatched")
	}
}

func TestFleetRecoveryOrchestrator_FindWorkerForRepo(t *testing.T) {
	coord := newTestCoordinator()
	fro := NewFleetRecoveryOrchestrator(coord, nil)

	// No workers registered — should return empty.
	id := fro.FindWorkerForRepo("/tmp/repo-a")
	if id != "" {
		t.Errorf("expected empty worker ID, got %q", id)
	}

	// Register a worker with repos.
	coord.mu.Lock()
	coord.workers["worker-1"] = &WorkerInfo{
		ID:     "worker-1",
		Status: WorkerOnline,
		Repos:  []string{"/tmp/repo-a", "/tmp/repo-b"},
	}
	coord.mu.Unlock()

	id = fro.FindWorkerForRepo("/tmp/repo-a")
	if id != "worker-1" {
		t.Errorf("expected worker-1, got %q", id)
	}

	id = fro.FindWorkerForRepo("/tmp/repo-c")
	if id != "" {
		t.Errorf("expected empty for unmatched repo, got %q", id)
	}
}

func TestFleetRecoveryOrchestrator_PriorityMapping(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(100.0)
	fro := NewFleetRecoveryOrchestrator(coord, nil)

	plan := &session.CrashRecoveryPlan{
		SessionsToResume: []session.RecoverableSession{
			{SessionID: "s1", RepoPath: "/tmp/a", RepoName: "a", Priority: 1, ResumePrompt: "p1"},
			{SessionID: "s2", RepoPath: "/tmp/b", RepoName: "b", Priority: 5, ResumePrompt: "p2"},
		},
	}

	fro.DistributeRecoveryPlan(plan, 1.00)

	// Check work items in queue have correct priority mapping.
	coord.mu.RLock()
	defer coord.mu.RUnlock()
	items := coord.queue.All()
	if len(items) < 2 {
		t.Fatalf("expected 2 items in queue, got %d", len(items))
	}

	// Priority 1 recovery → 99 internal, Priority 5 → 95 internal.
	for _, item := range items {
		if item.RepoName == "a" && item.Priority != 99 {
			t.Errorf("expected priority 99 for repo-a (recovery priority 1), got %d", item.Priority)
		}
		if item.RepoName == "b" && item.Priority != 95 {
			t.Errorf("expected priority 95 for repo-b (recovery priority 5), got %d", item.Priority)
		}
	}
}

func TestFleetRecoveryOrchestrator_UsesOriginalProvider(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(100.0)
	fro := NewFleetRecoveryOrchestrator(coord, nil)

	plan := &session.CrashRecoveryPlan{
		DetectedAt: time.Now(),
		Severity:   "minor",
		DeadCount:  2,
		SessionsToResume: []session.RecoverableSession{
			{SessionID: "s1", RepoPath: "/tmp/r1", RepoName: "r1", Priority: 1, Provider: session.ProviderCodex, ResumePrompt: "p1"},
			{SessionID: "s2", RepoPath: "/tmp/r2", RepoName: "r2", Priority: 2, Provider: session.ProviderGemini, ResumePrompt: "p2"},
			{SessionID: "s3", RepoPath: "/tmp/r3", RepoName: "r3", Priority: 3, ResumePrompt: "p3"}, // empty provider → runtime selection
		},
	}

	n, err := fro.DistributeRecoveryPlan(plan, 2.00)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Errorf("expected 3 submitted, got %d", n)
	}

	// Verify providers on submitted work items
	items := coord.queue.All()
	providersByRepo := make(map[string]session.Provider)
	for _, item := range items {
		providersByRepo[item.RepoName] = item.Provider
	}
	if providersByRepo["r1"] != session.ProviderCodex {
		t.Errorf("r1: expected codex, got %s", providersByRepo["r1"])
	}
	if providersByRepo["r2"] != session.ProviderGemini {
		t.Errorf("r2: expected gemini, got %s", providersByRepo["r2"])
	}
	if providersByRepo["r3"] != "" {
		t.Errorf("r3: expected empty provider for runtime selection, got %s", providersByRepo["r3"])
	}
}
