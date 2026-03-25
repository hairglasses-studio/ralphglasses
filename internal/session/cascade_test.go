package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldCascade_Default(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	if !cr.ShouldCascade("feature", "implement something") {
		t.Error("expected ShouldCascade=true with nil feedback")
	}
}

func TestShouldCascade_TaskTypeOverride(t *testing.T) {
	config := DefaultCascadeConfig()
	config.TaskTypeOverrides["docs"] = ProviderGemini

	cr := NewCascadeRouter(config, nil, nil, "")

	if cr.ShouldCascade("docs", "write documentation") {
		t.Error("expected ShouldCascade=false for overridden task type")
	}

	// Non-overridden task type should still cascade
	if !cr.ShouldCascade("feature", "implement something") {
		t.Error("expected ShouldCascade=true for non-overridden task type")
	}
}

func TestShouldCascade_ReliableCheapProvider(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 3) // lower min for testing

	// Ingest enough successful journal entries for the cheap provider
	var entries []JournalEntry
	for i := 0; i < 6; i++ {
		entries = append(entries, JournalEntry{
			Timestamp:  time.Now(),
			SessionID:  "sess-" + string(rune('a'+i)),
			Provider:   "gemini",
			RepoName:   "test-repo",
			SpentUSD:   0.10,
			TurnCount:  5,
			ExitReason: "completed",
			TaskFocus:  "add new docs section", // classifies as "docs"
		})
	}
	fa.Ingest(entries)

	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, fa, nil, "")

	if cr.ShouldCascade("docs", "write documentation") {
		t.Error("expected ShouldCascade=false when cheap provider is reliable")
	}

	// Unknown task type should still cascade
	if !cr.ShouldCascade("general", "do something") {
		t.Error("expected ShouldCascade=true for unknown task type")
	}
}

func TestResolveProvider(t *testing.T) {
	t.Run("override", func(t *testing.T) {
		config := DefaultCascadeConfig()
		config.TaskTypeOverrides["docs"] = ProviderCodex
		cr := NewCascadeRouter(config, nil, nil, "")

		if got := cr.ResolveProvider("docs"); got != ProviderCodex {
			t.Errorf("expected ProviderCodex, got %s", got)
		}
	})

	t.Run("cheap_reliable", func(t *testing.T) {
		dir := t.TempDir()
		fa := NewFeedbackAnalyzer(dir, 3)

		var entries []JournalEntry
		for i := 0; i < 6; i++ {
			entries = append(entries, JournalEntry{
				Timestamp:  time.Now(),
				SessionID:  "s-" + string(rune('a'+i)),
				Provider:   "gemini",
				RepoName:   "test",
				SpentUSD:   0.05,
				TurnCount:  3,
				ExitReason: "completed",
				TaskFocus:  "add feature support",
			})
		}
		fa.Ingest(entries)

		config := DefaultCascadeConfig()
		cr := NewCascadeRouter(config, fa, nil, "")

		if got := cr.ResolveProvider("feature"); got != ProviderGemini {
			t.Errorf("expected ProviderGemini (cheap), got %s", got)
		}
	})

	t.Run("default_expensive", func(t *testing.T) {
		config := DefaultCascadeConfig()
		cr := NewCascadeRouter(config, nil, nil, "")

		if got := cr.ResolveProvider("feature"); got != ProviderClaude {
			t.Errorf("expected ProviderClaude (expensive), got %s", got)
		}
	})
}

func TestCheapLaunchOpts(t *testing.T) {
	config := DefaultCascadeConfig()
	config.MaxCheapBudgetUSD = 0.50
	config.MaxCheapTurns = 15

	cr := NewCascadeRouter(config, nil, nil, "")

	base := LaunchOptions{
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/repo",
		Prompt:       "implement feature X",
		MaxBudgetUSD: 2.00,
		MaxTurns:     50,
		SessionName:  "my-session",
	}

	cheap := cr.CheapLaunchOpts(base)

	if cheap.Provider != ProviderGemini {
		t.Errorf("expected provider=gemini, got %s", cheap.Provider)
	}
	if cheap.MaxBudgetUSD != 0.50 {
		t.Errorf("expected budget=0.50, got %f", cheap.MaxBudgetUSD)
	}
	if cheap.MaxTurns != 15 {
		t.Errorf("expected max_turns=15, got %d", cheap.MaxTurns)
	}
	if cheap.SessionName != "my-session-cheap" {
		t.Errorf("expected name=my-session-cheap, got %s", cheap.SessionName)
	}
	// Ensure base is not mutated
	if base.Provider != ProviderClaude {
		t.Error("base provider was mutated")
	}
	if base.MaxBudgetUSD != 2.00 {
		t.Error("base budget was mutated")
	}
}

func TestEvaluateCheapResult_Error(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	s := &Session{
		ID:    "err-sess",
		Error: "provider timeout",
	}

	escalate, conf, reason := cr.EvaluateCheapResult(s, 10, nil)
	if !escalate {
		t.Error("expected escalate=true for errored session")
	}
	if conf != 0 {
		t.Errorf("expected confidence=0, got %f", conf)
	}
	if reason != "error" {
		t.Errorf("expected reason=error, got %s", reason)
	}
}

func TestEvaluateCheapResult_VerifyFailed(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	s := &Session{
		ID:        "verify-fail-sess",
		TurnCount: 5,
	}

	verifications := []LoopVerification{
		{Command: "go test ./...", ExitCode: 1, Output: "FAIL"},
	}

	escalate, _, reason := cr.EvaluateCheapResult(s, 10, verifications)
	if !escalate {
		t.Error("expected escalate=true when verification fails")
	}
	if reason != "verify_failed" {
		t.Errorf("expected reason=verify_failed, got %s", reason)
	}
}

func TestEvaluateCheapResult_LowConfidence(t *testing.T) {
	config := DefaultCascadeConfig()
	config.ConfidenceThreshold = 0.7
	cr := NewCascadeRouter(config, nil, nil, "")

	// Session with lots of hedging words in output
	s := &Session{
		ID:         "hedge-sess",
		TurnCount:  30, // way over expected
		LastOutput: "I'm not sure if this is correct. Maybe the approach is wrong. I think there might be issues. I'm not confident about the implementation. Possibly broken.",
	}

	escalate, conf, reason := cr.EvaluateCheapResult(s, 5, nil)
	if !escalate {
		t.Error("expected escalate=true for low confidence session")
	}
	if conf >= config.ConfidenceThreshold {
		t.Errorf("expected confidence < %f, got %f", config.ConfidenceThreshold, conf)
	}
	if reason != "low_confidence" {
		t.Errorf("expected reason=low_confidence, got %s", reason)
	}
}

func TestEvaluateCheapResult_Success(t *testing.T) {
	config := DefaultCascadeConfig()
	config.ConfidenceThreshold = 0.7
	cr := NewCascadeRouter(config, nil, nil, "")

	s := &Session{
		ID:         "good-sess",
		TurnCount:  5,
		LastOutput: "Successfully implemented the feature. All tests pass.",
	}

	verifications := []LoopVerification{
		{Command: "go test ./...", ExitCode: 0, Output: "ok"},
		{Command: "go vet ./...", ExitCode: 0, Output: ""},
	}

	escalate, conf, reason := cr.EvaluateCheapResult(s, 10, verifications)
	if escalate {
		t.Errorf("expected escalate=false for successful session, got reason=%s confidence=%f", reason, conf)
	}
	if conf < config.ConfidenceThreshold {
		t.Errorf("expected confidence >= %f, got %f", config.ConfidenceThreshold, conf)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %s", reason)
	}
}

func TestRecordResult_Persistence(t *testing.T) {
	dir := t.TempDir()
	config := DefaultCascadeConfig()

	cr1 := NewCascadeRouter(config, nil, nil, dir)

	r1 := CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "feature",
		TaskTitle:    "add auth",
		UsedProvider: ProviderGemini,
		Escalated:    false,
		CheapCostUSD: 0.10,
		TotalCostUSD: 0.10,
	}
	r2 := CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "bug_fix",
		TaskTitle:    "fix crash",
		UsedProvider: ProviderClaude,
		Escalated:    true,
		CheapCostUSD: 0.20,
		TotalCostUSD: 1.50,
		Reason:       "low_confidence",
	}

	cr1.RecordResult(r1)
	cr1.RecordResult(r2)

	// Verify file exists
	path := filepath.Join(dir, "cascade_results.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("results file not created: %v", err)
	}

	// Create new router from same dir and verify loaded
	cr2 := NewCascadeRouter(config, nil, nil, dir)

	results := cr2.RecentResults(10)
	if len(results) != 2 {
		t.Fatalf("expected 2 loaded results, got %d", len(results))
	}
	if results[0].TaskTitle != "add auth" {
		t.Errorf("expected first result title=add auth, got %s", results[0].TaskTitle)
	}
	if results[1].TaskTitle != "fix crash" {
		t.Errorf("expected second result title=fix crash, got %s", results[1].TaskTitle)
	}
	if !results[1].Escalated {
		t.Error("expected second result to be escalated")
	}
}

func TestDefaultModelTiers(t *testing.T) {
	tiers := DefaultModelTiers()
	if len(tiers) != 4 {
		t.Fatalf("expected 4 default tiers, got %d", len(tiers))
	}

	// Verify sorted by cost ascending
	for i := 1; i < len(tiers); i++ {
		if tiers[i].CostPer1M < tiers[i-1].CostPer1M {
			t.Errorf("tiers not sorted by cost: tier[%d]=%f < tier[%d]=%f",
				i, tiers[i].CostPer1M, i-1, tiers[i-1].CostPer1M)
		}
	}

	// Check labels
	labels := []string{"ultra-cheap", "worker", "coding", "reasoning"}
	for i, want := range labels {
		if tiers[i].Label != want {
			t.Errorf("tier[%d].Label = %q, want %q", i, tiers[i].Label, want)
		}
	}
}

func TestTaskTypeComplexity(t *testing.T) {
	cases := []struct {
		taskType   string
		wantLevel  int
	}{
		{"lint", 1},
		{"format", 1},
		{"classify", 1},
		{"codegen", 3},
		{"test", 3},
		{"architecture", 4},
		{"analysis", 4},
		{"planning", 4},
		{"unknown", 0},
	}

	for _, tc := range cases {
		if got := TaskTypeComplexity(tc.taskType); got != tc.wantLevel {
			t.Errorf("TaskTypeComplexity(%q) = %d, want %d", tc.taskType, got, tc.wantLevel)
		}
	}
}

func TestSelectTier_ByTaskType(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	tests := []struct {
		taskType  string
		wantLabel string
	}{
		{"lint", "ultra-cheap"},
		{"format", "ultra-cheap"},
		{"classify", "ultra-cheap"},
		{"codegen", "coding"},
		{"test", "coding"},
		{"architecture", "reasoning"},
		{"analysis", "reasoning"},
		{"planning", "reasoning"},
	}

	for _, tc := range tests {
		tier := cr.SelectTier(tc.taskType, 0)
		if tier.Label != tc.wantLabel {
			t.Errorf("SelectTier(%q, 0): label = %q, want %q", tc.taskType, tier.Label, tc.wantLabel)
		}
	}
}

func TestSelectTier_ByComplexity(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	tests := []struct {
		complexity int
		wantLabel  string
	}{
		{1, "ultra-cheap"},
		{2, "worker"},
		{3, "coding"},
		{4, "reasoning"},
	}

	for _, tc := range tests {
		// Use empty task type so complexity arg is used directly
		tier := cr.SelectTier("", tc.complexity)
		if tier.Label != tc.wantLabel {
			t.Errorf("SelectTier(\"\", %d): label = %q, want %q", tc.complexity, tier.Label, tc.wantLabel)
		}
	}
}

func TestSelectTier_UnknownTaskDefaultsToHighest(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	tier := cr.SelectTier("unknown_task", 0)
	if tier.Label != "reasoning" {
		t.Errorf("SelectTier for unknown task: label = %q, want %q", tier.Label, "reasoning")
	}
}

func TestSelectTier_TaskTypeOverridesComplexityArg(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// "lint" maps to complexity 1, so even if we pass complexity=4 it should pick ultra-cheap
	tier := cr.SelectTier("lint", 4)
	if tier.Label != "ultra-cheap" {
		t.Errorf("SelectTier(\"lint\", 4): label = %q, want %q", tier.Label, "ultra-cheap")
	}
}

func TestSelectTier_CustomTiers(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	custom := []ModelTier{
		{Provider: ProviderGemini, Model: "custom-small", MaxComplexity: 2, CostPer1M: 0.05, Label: "small"},
		{Provider: ProviderClaude, Model: "custom-large", MaxComplexity: 4, CostPer1M: 5.00, Label: "large"},
	}
	cr.SetTiers(custom)

	tier := cr.SelectTier("", 1)
	if tier.Label != "small" {
		t.Errorf("expected small tier for complexity 1, got %q", tier.Label)
	}

	tier = cr.SelectTier("", 3)
	if tier.Label != "large" {
		t.Errorf("expected large tier for complexity 3, got %q", tier.Label)
	}
}

func TestSelectTier_EmptyTiers(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")
	cr.SetTiers(nil)

	tier := cr.SelectTier("lint", 1)
	if tier.Label != "" {
		t.Errorf("expected empty tier for nil tiers, got label=%q", tier.Label)
	}
}

func TestSelectTier_ProviderAlignment(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// Ultra-cheap and worker should be Gemini
	for _, tt := range []string{"lint", "format"} {
		tier := cr.SelectTier(tt, 0)
		if tier.Provider != ProviderGemini {
			t.Errorf("SelectTier(%q): provider = %q, want gemini", tt, tier.Provider)
		}
	}

	// Coding and reasoning should be Claude
	for _, tt := range []string{"codegen", "architecture"} {
		tier := cr.SelectTier(tt, 0)
		if tier.Provider != ProviderClaude {
			t.Errorf("SelectTier(%q): provider = %q, want claude", tt, tier.Provider)
		}
	}
}

func TestTiers_ReturnsCopy(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	tiers := cr.Tiers()
	tiers[0].Label = "mutated"

	original := cr.Tiers()
	if original[0].Label == "mutated" {
		t.Error("Tiers() should return a copy, not a reference")
	}
}

func TestCascadeStats(t *testing.T) {
	dir := t.TempDir()
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, dir)

	// Record mix of escalated and non-escalated
	cr.RecordResult(CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "feature",
		Escalated:    false,
		CheapCostUSD: 0.10,
		TotalCostUSD: 0.10,
	})
	cr.RecordResult(CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "feature",
		Escalated:    false,
		CheapCostUSD: 0.15,
		TotalCostUSD: 0.15,
	})
	cr.RecordResult(CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "bug_fix",
		Escalated:    true,
		CheapCostUSD: 0.20,
		TotalCostUSD: 1.50,
		Reason:       "low_confidence",
	})

	stats := cr.Stats()

	if stats.TotalDecisions != 3 {
		t.Errorf("expected 3 total decisions, got %d", stats.TotalDecisions)
	}
	if stats.Escalations != 1 {
		t.Errorf("expected 1 escalation, got %d", stats.Escalations)
	}

	// Escalation rate = 1/3
	expectedRate := 1.0 / 3.0
	if diff := stats.EscalationRate - expectedRate; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected escalation rate ~%.3f, got %.3f", expectedRate, stats.EscalationRate)
	}

	// Cost saved = sum of cheap costs for non-escalated (0.10 + 0.15 = 0.25)
	if diff := stats.CostSavedUSD - 0.25; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected cost saved ~0.25, got %.3f", stats.CostSavedUSD)
	}

	// Avg cheap cost = (0.10 + 0.15 + 0.20) / 3 = 0.15
	if diff := stats.AvgCheapCost - 0.15; diff > 0.01 || diff < -0.01 {
		t.Errorf("expected avg cheap cost ~0.15, got %.3f", stats.AvgCheapCost)
	}
}
