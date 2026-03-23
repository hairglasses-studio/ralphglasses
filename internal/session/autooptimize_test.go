package session

import (
	"context"
	"testing"
	"time"
)

func TestAutoOptimizer_OptimizedLaunchOptions_NoFeedback(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	opts := LaunchOptions{Prompt: "fix the bug"}
	result, changed := ao.OptimizedLaunchOptions(opts)
	if changed {
		t.Fatal("expected no changes with nil feedback")
	}
	if result.Provider != opts.Provider {
		t.Fatalf("provider changed without feedback")
	}
}

func TestAutoOptimizer_OptimizedLaunchOptions_WithFeedback(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1) // minSessions=1 for testing
	dl := NewDecisionLog(dir, LevelAutoOptimize)

	// Ingest entries that make gemini the best for bug_fix tasks
	fa.Ingest([]JournalEntry{
		{Provider: "gemini", TaskFocus: "fix bug in parser", SpentUSD: 0.01, TurnCount: 5, ExitReason: "completed"},
		{Provider: "claude", TaskFocus: "fix crash on startup", SpentUSD: 0.50, TurnCount: 20, ExitReason: "completed"},
	})

	ao := NewAutoOptimizer(fa, dl, nil, nil)

	opts := LaunchOptions{Prompt: "fix the null pointer bug"}
	result, changed := ao.OptimizedLaunchOptions(opts)
	if !changed {
		t.Fatal("expected provider to be changed based on feedback")
	}
	if result.Provider != ProviderGemini {
		t.Fatalf("expected gemini, got %s", result.Provider)
	}
}

func TestAutoOptimizer_OptimizedLaunchOptions_BudgetSuggestion(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	dl := NewDecisionLog(dir, LevelAutoOptimize)

	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "add new feature", SpentUSD: 2.0, TurnCount: 10, ExitReason: "completed"},
	})

	ao := NewAutoOptimizer(fa, dl, nil, nil)
	opts := LaunchOptions{Prompt: "add search functionality"}
	result, changed := ao.OptimizedLaunchOptions(opts)
	if !changed {
		t.Fatal("expected budget to be set")
	}
	if result.MaxBudgetUSD <= 0 {
		t.Fatal("expected positive budget suggestion")
	}
}

func TestAutoOptimizer_OptimizedLaunchOptions_RespectExplicitProvider(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	dl := NewDecisionLog(dir, LevelAutoOptimize)

	fa.Ingest([]JournalEntry{
		{Provider: "gemini", TaskFocus: "fix bug", SpentUSD: 0.01, TurnCount: 5, ExitReason: "completed"},
	})

	ao := NewAutoOptimizer(fa, dl, nil, nil)
	// Codex explicitly set — should not be overridden
	opts := LaunchOptions{Provider: ProviderCodex, Prompt: "fix the bug"}
	result, _ := ao.OptimizedLaunchOptions(opts)
	if result.Provider != ProviderCodex {
		t.Fatalf("explicit provider should not be overridden, got %s", result.Provider)
	}
}

func TestAutoOptimizer_OptimizedLaunchOptions_LevelTooLow(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	dl := NewDecisionLog(dir, LevelObserve) // Level 0 — too low for auto-optimize

	fa.Ingest([]JournalEntry{
		{Provider: "gemini", TaskFocus: "fix bug", SpentUSD: 0.01, TurnCount: 5, ExitReason: "completed"},
	})

	ao := NewAutoOptimizer(fa, dl, nil, nil)
	opts := LaunchOptions{Prompt: "fix the bug"}
	_, changed := ao.OptimizedLaunchOptions(opts)
	if changed {
		t.Fatal("should not optimize at observe level")
	}
}

func TestAutoOptimizer_BuildSmartFailoverChain(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	// Ingest data: gemini best for refactoring
	fa.Ingest([]JournalEntry{
		{Provider: "gemini", TaskFocus: "refactor parser", SpentUSD: 0.05, TurnCount: 3, ExitReason: "completed"},
		{Provider: "gemini", TaskFocus: "refactor handler", SpentUSD: 0.03, TurnCount: 2, ExitReason: "completed"},
		{Provider: "claude", TaskFocus: "refactor utils", SpentUSD: 0.50, TurnCount: 15, ExitReason: "completed"},
	})

	ao := NewAutoOptimizer(fa, nil, nil, nil)
	chain := ao.BuildSmartFailoverChain("refactor the database layer")

	if len(chain.Providers) != 3 {
		t.Fatalf("expected 3 providers, got %d", len(chain.Providers))
	}
	// Gemini should be first due to better cost efficiency
	if chain.Providers[0] != ProviderGemini {
		t.Fatalf("expected gemini first, got %s", chain.Providers[0])
	}
}

func TestAutoOptimizer_RecommendProvider_NoData(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	rec := ao.RecommendProvider("fix the bug")
	if rec.Provider != ProviderClaude {
		t.Fatalf("expected claude default, got %s", rec.Provider)
	}
	if rec.Confidence != "low" {
		t.Fatalf("expected low confidence, got %s", rec.Confidence)
	}
}

func TestAutoOptimizer_RecommendProvider_WithData(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	fa.Ingest([]JournalEntry{
		{Provider: "gemini", TaskFocus: "add feature", SpentUSD: 0.10, TurnCount: 5, ExitReason: "completed"},
		{Provider: "claude", TaskFocus: "add feature", SpentUSD: 1.00, TurnCount: 20, ExitReason: "completed"},
	})

	ao := NewAutoOptimizer(fa, nil, nil, nil)
	rec := ao.RecommendProvider("add search functionality")

	if rec.TaskType != "feature" {
		t.Fatalf("expected feature task type, got %s", rec.TaskType)
	}
	if rec.EstimatedBudget <= 0 {
		t.Fatal("expected positive budget estimate")
	}
}

func TestAutoOptimizer_IngestSessionJournal(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	ao := NewAutoOptimizer(fa, nil, nil, nil)

	now := time.Now()
	ended := now.Add(5 * time.Minute)
	s := &Session{
		ID:         "test-123",
		Provider:   ProviderClaude,
		RepoName:   "myrepo",
		Model:      "sonnet",
		SpentUSD:   0.25,
		TurnCount:  10,
		Prompt:     "fix the parser bug",
		ExitReason: "completed",
		LaunchedAt: now,
		EndedAt:    &ended,
	}

	ao.IngestSessionJournal(s)

	profile, ok := fa.GetPromptProfile("bug_fix")
	if !ok {
		t.Fatal("expected bug_fix profile after ingestion")
	}
	if profile.SampleCount != 1 {
		t.Fatalf("expected 1 sample, got %d", profile.SampleCount)
	}
}

func TestAutoOptimizer_HandleSessionComplete_Recovery(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)
	hitl := NewHITLTracker(dir)
	mgr := NewManager()
	mgr.SetStateDir(dir)
	recovery := NewAutoRecovery(mgr, dl, hitl, DefaultAutoRecoveryConfig())

	ao := NewAutoOptimizer(nil, dl, hitl, recovery)

	// Completed session should record HITL metric
	s := &Session{
		ID:       "test-ok",
		Status:   StatusCompleted,
		RepoName: "repo",
	}
	ao.HandleSessionComplete(context.Background(), s)

	// Check that HITL event was recorded
	events := hitl.History(time.Hour, 10)
	if len(events) == 0 {
		t.Fatal("expected HITL event for completed session")
	}
	if events[0].MetricType != MetricSessionCompleted {
		t.Fatalf("expected session_completed metric, got %s", events[0].MetricType)
	}
}
