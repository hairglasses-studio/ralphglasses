package session

import (
	"context"
	"testing"
	"time"
)

func TestProductivitySnapshot_Productive(t *testing.T) {
	repoPath := setupLoopRepo(t)
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	run := startLoopForProductivityTest(t, mgr, repoPath, LoopProfile{
		VerifyCommands:   []string{"true"},
		NoopPlateauLimit: 3,
	})
	run.Lock()
	run.Status = "idle"
	run.Iterations = []LoopIteration{{
		Number:           0,
		Status:           "idle",
		StartedAt:        time.Now(),
		AcceptanceReason: "auto_merged",
		StagedFilesCount: 2,
		Verification:     []LoopVerification{{Status: "completed", ExitCode: 0}},
		Acceptance:       &AcceptanceResult{AutoMerged: true, SafePaths: []string{"docs/research.md"}},
	}}
	run.UpdatedAt = time.Now()
	run.Unlock()
	mgr.PersistLoop(run)

	gw := newMockGateway(&ResearchEntry{
		Topic: "productive-topic", Domain: "mcp", Source: "manual",
		PriorityScore: 0.6, ModelTier: "sonnet", BudgetUSD: 3.0,
	})
	gw.dedupConfidence = 0.5
	rd := NewResearchDaemon(gw, ResearchDaemonConfig{
		Enabled:         true,
		TickInterval:    1,
		MaxTopicsPerRun: 1,
		BudgetPerRunUSD: 10,
		BudgetDailyUSD:  25,
		MaxComplexity:   3,
		ClaimTTLSecs:    60,
		AgentID:         "test-research-daemon",
	})
	rd.Tick(context.Background())

	snapshot := mgr.productivitySnapshot(repoPath, time.Time{}, rd)
	if !snapshot.Productive {
		t.Fatalf("expected productive snapshot, got %+v", snapshot)
	}
	if snapshot.Score != 100 {
		t.Fatalf("score = %d, want 100", snapshot.Score)
	}
	if snapshot.ResearchOutputs != 1 || snapshot.TopicsCompleted != 1 {
		t.Fatalf("expected 1 research output/topic, got %+v", snapshot)
	}
	if snapshot.DevelopmentOutputs != 1 {
		t.Fatalf("development_outputs = %d, want 1", snapshot.DevelopmentOutputs)
	}
	if snapshot.VerificationFailures != 0 {
		t.Fatalf("verification_failures = %d, want 0", snapshot.VerificationFailures)
	}

	sup := NewSupervisor(mgr, repoPath)
	sup.SetResearchDaemon(rd)
	status := sup.Status()
	if !status.Productivity.Productive {
		t.Fatalf("supervisor productivity = %+v, want productive", status.Productivity)
	}
	if status.ResearchDaemonStats == nil || status.ResearchDaemonStats.ResearchOutputs != 1 {
		t.Fatalf("expected research daemon stats with one output, got %+v", status.ResearchDaemonStats)
	}
}

func TestProductivitySnapshot_DedupSkipAndNoopRemainUnproductive(t *testing.T) {
	repoPath := setupLoopRepo(t)
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	run := startLoopForProductivityTest(t, mgr, repoPath, LoopProfile{
		VerifyCommands:   []string{"true"},
		NoopPlateauLimit: 2,
	})
	now := time.Now()
	run.Lock()
	run.Status = "converged"
	run.LastError = "no-op plateau: 2 consecutive iterations with no changes"
	run.Iterations = []LoopIteration{
		{Number: 0, Status: "idle", StartedAt: now.Add(-time.Minute), AcceptanceReason: "worker_no_changes"},
		{Number: 1, Status: "idle", StartedAt: now, AcceptanceReason: "no_staged_files"},
	}
	run.UpdatedAt = now
	run.Unlock()
	mgr.PersistLoop(run)

	gw := newMockGateway(&ResearchEntry{
		Topic: "already-covered", Domain: "mcp", Source: "manual",
		PriorityScore: 0.3, ModelTier: "sonnet", BudgetUSD: 1.0,
	})
	gw.dedupConfidence = 0.9
	gw.dedupRecommend = "exists"

	rd := NewResearchDaemon(gw, ResearchDaemonConfig{
		Enabled:         true,
		TickInterval:    1,
		MaxTopicsPerRun: 1,
		BudgetPerRunUSD: 10,
		BudgetDailyUSD:  25,
		MaxComplexity:   3,
		ClaimTTLSecs:    60,
		AgentID:         "test-research-daemon",
	})
	rd.Tick(context.Background())

	snapshot := mgr.productivitySnapshot(repoPath, time.Time{}, rd)
	if snapshot.Productive {
		t.Fatalf("expected unproductive snapshot, got %+v", snapshot)
	}
	if snapshot.DedupSkips != 1 {
		t.Fatalf("dedup_skips = %d, want 1", snapshot.DedupSkips)
	}
	if !snapshot.NoopPlateau {
		t.Fatalf("expected noop plateau, got %+v", snapshot)
	}
	if snapshot.Score >= 80 {
		t.Fatalf("score = %d, want below 80", snapshot.Score)
	}
}

func TestProductivitySnapshot_VerificationFailurePreventsProductive(t *testing.T) {
	repoPath := setupLoopRepo(t)
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	run := startLoopForProductivityTest(t, mgr, repoPath, LoopProfile{
		VerifyCommands:   []string{"false"},
		NoopPlateauLimit: 3,
	})
	run.Lock()
	run.Status = "failed"
	run.Iterations = []LoopIteration{{
		Number:           0,
		Status:           "failed",
		StartedAt:        time.Now(),
		AcceptanceReason: "auto_merged",
		StagedFilesCount: 1,
		Acceptance:       &AcceptanceResult{AutoMerged: true, SafePaths: []string{"docs/research.md"}},
		Verification:     []LoopVerification{{Status: "failed", ExitCode: 1}},
	}}
	run.UpdatedAt = time.Now()
	run.Unlock()
	mgr.PersistLoop(run)

	gw := newMockGateway(&ResearchEntry{
		Topic: "research-topic", Domain: "mcp", Source: "manual",
		PriorityScore: 0.5, ModelTier: "sonnet", BudgetUSD: 2.0,
	})
	gw.dedupConfidence = 0.5
	rd := NewResearchDaemon(gw, ResearchDaemonConfig{
		Enabled:         true,
		TickInterval:    1,
		MaxTopicsPerRun: 1,
		BudgetPerRunUSD: 10,
		BudgetDailyUSD:  25,
		MaxComplexity:   3,
		ClaimTTLSecs:    60,
		AgentID:         "test-research-daemon",
	})
	rd.Tick(context.Background())

	snapshot := mgr.productivitySnapshot(repoPath, time.Time{}, rd)
	if snapshot.Productive {
		t.Fatalf("expected verifier failure to prevent productive snapshot, got %+v", snapshot)
	}
	if snapshot.Score != 80 {
		t.Fatalf("score = %d, want 80", snapshot.Score)
	}
	if snapshot.VerificationFailures != 1 {
		t.Fatalf("verification_failures = %d, want 1", snapshot.VerificationFailures)
	}
}

func startLoopForProductivityTest(t *testing.T, mgr *Manager, repoPath string, profile LoopProfile) *LoopRun {
	t.Helper()
	run, err := mgr.StartLoop(context.Background(), repoPath, profile)
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}
	return run
}
