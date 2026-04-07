package session

import (
	"context"
	"fmt"
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
	if rec.Provider != ProviderCodex {
		t.Fatalf("expected codex default, got %s", rec.Provider)
	}
	if rec.Confidence != "low" {
		t.Fatalf("expected low confidence, got %s", rec.Confidence)
	}
	if len(rec.FallbackChain) == 0 {
		t.Fatal("expected fallback chain to be populated")
	}
	if rec.DataSource != "default" {
		t.Fatalf("expected default data source, got %s", rec.DataSource)
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
	if rec.DataSource != "feedback_data" {
		t.Fatalf("expected feedback_data, got %s", rec.DataSource)
	}
	if len(rec.CapabilityConstraints) == 0 {
		t.Fatal("expected capability constraints for selected provider")
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

func TestAutoOptimizer_GateChange(t *testing.T) {
	t.Run("gate_disabled", func(t *testing.T) {
		ao := NewAutoOptimizer(nil, nil, nil, nil)
		oldGate := GateEnabled.Load()
		GateEnabled.Store(false)
		defer func() { GateEnabled.Store(oldGate) }()

		change := GatedChange{ChangeType: "config"}
		result := ao.GateChange("/tmp", change)
		if result.Verdict != "skip" {
			t.Errorf("expected verdict=skip when disabled, got %s", result.Verdict)
		}
	})

	t.Run("gate_pass", func(t *testing.T) {
		ao := NewAutoOptimizer(nil, nil, nil, nil)
		oldGate := GateEnabled.Load()
		oldRunner := GetRunTestGate()
		GateEnabled.Store(true)
		SetRunTestGate(func(string) (string, error) { return "pass", nil })
		defer func() { GateEnabled.Store(oldGate); SetRunTestGate(oldRunner) }()

		change := GatedChange{ChangeType: "config"}
		result := ao.GateChange("/tmp", change)
		if result.Verdict != "pass" {
			t.Errorf("expected verdict=pass, got %s", result.Verdict)
		}
		if result.RolledBack {
			t.Error("expected no rollback on pass")
		}
	})

	t.Run("gate_fail_verdict", func(t *testing.T) {
		ao := NewAutoOptimizer(nil, nil, nil, nil)
		oldGate := GateEnabled.Load()
		oldRunner := GetRunTestGate()
		GateEnabled.Store(true)
		SetRunTestGate(func(string) (string, error) { return "fail", nil })
		defer func() { GateEnabled.Store(oldGate); SetRunTestGate(oldRunner) }()

		change := GatedChange{ChangeType: "config"}
		result := ao.GateChange("/tmp", change)
		if result.Verdict != "fail" {
			t.Errorf("expected verdict=fail, got %s", result.Verdict)
		}
		if !result.RolledBack {
			t.Error("expected rollback on fail verdict")
		}
	})

	t.Run("gate_error", func(t *testing.T) {
		dir := t.TempDir()
		dl := NewDecisionLog(dir, LevelAutoOptimize)
		ao := NewAutoOptimizer(nil, dl, nil, nil)
		oldGate := GateEnabled.Load()
		oldRunner := GetRunTestGate()
		GateEnabled.Store(true)
		SetRunTestGate(func(string) (string, error) {
			return "", fmt.Errorf("test gate error")
		})
		defer func() { GateEnabled.Store(oldGate); SetRunTestGate(oldRunner) }()

		change := GatedChange{ChangeType: "config"}
		result := ao.GateChange("/tmp", change)
		if result.Verdict != "fail" {
			t.Errorf("expected verdict=fail on error, got %s", result.Verdict)
		}
		if !result.RolledBack {
			t.Error("expected rollback on gate error")
		}
	})
}

func TestAutoOptimizer_GenerateNotes(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)

	t.Run("nil_patterns", func(t *testing.T) {
		notes := ao.GenerateNotes(nil)
		if notes != nil {
			t.Errorf("expected nil for nil patterns, got %v", notes)
		}
	})

	t.Run("with_rules_and_negatives", func(t *testing.T) {
		patterns := &ConsolidatedPatterns{
			UpdatedAt: time.Now(),
			Rules: []Rule{
				{ID: "apply-1", Pattern: "use gemini for lint tasks", Action: "use gemini for lint tasks"},
				{ID: "apply-2", Pattern: "adjust budget limits", Action: "adjust budget limits"},
			},
			Negative: []ConsolidatedItem{
				{Text: "timeout errors", Count: 5, Category: "error"},
			},
		}
		notes := ao.GenerateNotes(patterns)
		if len(notes) < 3 {
			t.Errorf("expected at least 3 notes (2 rules + 1 negative), got %d", len(notes))
		}
	})

	t.Run("rules_not_actionable", func(t *testing.T) {
		patterns := &ConsolidatedPatterns{
			Rules: []Rule{{ID: "apply-3", Pattern: "some generic advice", Action: "some generic advice"}},
		}
		notes := ao.GenerateNotes(patterns)
		if len(notes) != 0 {
			t.Errorf("expected 0 notes for non-actionable rules, got %d", len(notes))
		}
	})
}

func TestAutoOptimizer_RuleToNote(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)

	tests := []struct {
		name   string
		rule   string
		wantOK bool
	}{
		{"provider_suggestion", "use gemini for lint tasks", true},
		{"budget_suggestion", "increase budget for complex tasks", true},
		{"generic_advice", "write better prompts", false},
		{"empty_rule", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			note := ao.ruleToNote(tt.rule)
			if (note != nil) != tt.wantOK {
				t.Errorf("ruleToNote(%q) returned note=%v, wantOK=%v", tt.rule, note != nil, tt.wantOK)
			}
		})
	}
}

func TestImprovementNotes_ReadWrite(t *testing.T) {
	dir := t.TempDir()

	note := ImprovementNote{
		ID:        "test-note-1",
		Timestamp: time.Now(),
		Category:  "config",
		Priority:  2,
		Title:     "Use gemini for lint",
		Status:    "pending",
		AutoApply: true,
	}

	if err := WriteImprovementNote(dir, note); err != nil {
		t.Fatalf("WriteImprovementNote: %v", err)
	}

	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("ReadPendingNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].ID != "test-note-1" {
		t.Errorf("note ID = %q, want test-note-1", notes[0].ID)
	}
}

func TestImprovementNotes_ReadMissing(t *testing.T) {
	dir := t.TempDir()

	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("ReadPendingNotes on missing file: %v", err)
	}
	if notes != nil {
		t.Errorf("expected nil for missing file, got %v", notes)
	}
}

func TestAutoOptimizer_ApplyPendingNotes(t *testing.T) {
	dir := t.TempDir()

	// Write a pending auto-apply note
	note := ImprovementNote{
		ID:        "apply-1",
		Timestamp: time.Now(),
		Category:  "config",
		Priority:  2,
		Title:     "Test note",
		Status:    "pending",
		AutoApply: true,
	}
	if err := WriteImprovementNote(dir, note); err != nil {
		t.Fatalf("WriteImprovementNote: %v", err)
	}

	ao := NewAutoOptimizer(nil, nil, nil, nil)
	// Gate disabled — notes should be applied
	oldGate := GateEnabled.Load()
	GateEnabled.Store(false)
	defer func() { GateEnabled.Store(oldGate) }()

	applied, rejected, err := ao.ApplyPendingNotes(dir)
	if err != nil {
		t.Fatalf("ApplyPendingNotes: %v", err)
	}
	if applied != 1 {
		t.Errorf("expected 1 applied, got %d", applied)
	}
	if rejected != 0 {
		t.Errorf("expected 0 rejected, got %d", rejected)
	}

	// Read back — status should be updated
	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("ReadPendingNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Status != "applied" {
		t.Errorf("expected status=applied, got %v", notes)
	}
}

func TestAutoOptimizer_HandleSessionCompleteWithOutcome(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoOptimize)
	ao := NewAutoOptimizer(nil, dl, nil, nil)

	s := &Session{
		ID:       "outcome-sess",
		Status:   StatusCompleted,
		RepoName: "repo",
	}
	ao.HandleSessionCompleteWithOutcome(context.Background(), s)
	// Should not panic; verification is that it completes
}

func TestAutoOptimizer_HandleSessionCompleteWithOutcome_NilDecisions(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	s := &Session{
		ID:       "nil-sess",
		Status:   StatusCompleted,
		RepoName: "repo",
	}
	ao.HandleSessionCompleteWithOutcome(context.Background(), s)
	// Should not panic
}

func TestAutonomyPersistence_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	if err := SaveAutonomyLevel(dir, 2); err != nil {
		t.Fatalf("SaveAutonomyLevel: %v", err)
	}

	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("LoadAutonomyLevel: %v", err)
	}
	if level != 2 {
		t.Errorf("level = %d, want 2", level)
	}
}

func TestLoadAutonomyLevel_Missing(t *testing.T) {
	dir := t.TempDir()
	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("LoadAutonomyLevel on missing file: %v", err)
	}
	if level != 0 {
		t.Errorf("level = %d, want 0 for missing file", level)
	}
}

func TestPersistAutonomyLevel(t *testing.T) {
	dir := t.TempDir()

	if err := PersistAutonomyLevel(3, dir); err != nil {
		t.Fatalf("PersistAutonomyLevel: %v", err)
	}

	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("LoadAutonomyLevel: %v", err)
	}
	if level != 3 {
		t.Errorf("level = %d, want 3", level)
	}
}

func TestAutonomyPersistence_EmptyDir(t *testing.T) {
	err := SaveAutonomyLevel("", 1)
	if err == nil {
		t.Fatal("expected error for empty ralph dir")
	}
}
