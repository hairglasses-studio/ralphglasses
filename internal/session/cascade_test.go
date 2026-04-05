package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestDefaultCascadeConfig_EnabledByDefault(t *testing.T) {
	config := DefaultCascadeConfig()
	if !config.Enabled {
		t.Error("expected DefaultCascadeConfig().Enabled to be true")
	}
	// Verify cascade works with default config (no opt-in required)
	cr := NewCascadeRouter(config, nil, nil, "")
	if !cr.ShouldCascade("feature", "implement something") {
		t.Error("expected cascade to work by default with DefaultCascadeConfig()")
	}
}

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
	for i := range 6 {
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
		for i := range 6 {
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

		if got := cr.ResolveProvider("feature"); got != ProviderCodex {
			t.Errorf("expected ProviderCodex (primary expensive lane), got %s", got)
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
		taskType  string
		wantLevel int
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

	// Coding should use Codex; highest-complexity reasoning should still use Claude.
	if tier := cr.SelectTier("codegen", 0); tier.Provider != ProviderCodex {
		t.Errorf("SelectTier(%q): provider = %q, want codex", "codegen", tier.Provider)
	}
	for _, tt := range []string{"architecture"} {
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

func TestRecordLatency_TracksP50P95(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// Record 20 latencies: 10ms, 20ms, ..., 200ms
	for i := 1; i <= 20; i++ {
		cr.RecordLatency("gemini", time.Duration(i*10)*time.Millisecond)
	}

	lat := cr.GetProviderLatency("gemini")
	if lat == nil {
		t.Fatal("expected non-nil latency")
	}
	if lat.Samples != 20 {
		t.Errorf("expected 20 samples, got %d", lat.Samples)
	}

	// P50 of [10..200] step 10 => 10th value = 100ms
	if lat.P50 != 100*time.Millisecond {
		t.Errorf("expected P50=100ms, got %v", lat.P50)
	}

	// P95 of 20 items => rank = ceil(0.95*20)-1 = 18 => 190ms
	if lat.P95 != 190*time.Millisecond {
		t.Errorf("expected P95=190ms, got %v", lat.P95)
	}
}

func TestLatencyAwareRouting_SkipsSlow(t *testing.T) {
	config := DefaultCascadeConfig()
	config.LatencyThresholdMs = 500 // 500ms threshold
	cr := NewCascadeRouter(config, nil, nil, "")

	// Record high latencies for cheap provider (gemini)
	for range 20 {
		cr.RecordLatency("gemini", 800*time.Millisecond)
	}

	// ShouldCascade should return false (skip cheap, too slow)
	if cr.ShouldCascade("feature", "do something") {
		t.Error("expected ShouldCascade=false when cheap provider is slow")
	}

	// ResolveProvider should return expensive
	if got := cr.ResolveProvider("feature"); got != ProviderCodex {
		t.Errorf("expected expensive provider (codex), got %s", got)
	}
}

func TestLatencyAwareRouting_UsesCheapWhenFast(t *testing.T) {
	config := DefaultCascadeConfig()
	config.LatencyThresholdMs = 500
	cr := NewCascadeRouter(config, nil, nil, "")

	// Record low latencies for cheap provider
	for range 20 {
		cr.RecordLatency("gemini", 100*time.Millisecond)
	}

	// ShouldCascade should return true (cheap is fast, try it)
	if !cr.ShouldCascade("feature", "do something") {
		t.Error("expected ShouldCascade=true when cheap provider is fast")
	}
}

func TestLatencyAwareRouting_Disabled(t *testing.T) {
	config := DefaultCascadeConfig()
	config.LatencyThresholdMs = 0 // disabled
	cr := NewCascadeRouter(config, nil, nil, "")

	// Record extremely high latencies
	for range 20 {
		cr.RecordLatency("gemini", 5*time.Second)
	}

	// ShouldCascade should still return true — latency routing disabled
	if !cr.ShouldCascade("feature", "do something") {
		t.Error("expected ShouldCascade=true when latency threshold is disabled")
	}
}

func TestRecordLatency_SlidingWindow(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// Fill window with 100 low-latency samples
	for range 100 {
		cr.RecordLatency("gemini", 50*time.Millisecond)
	}

	lat := cr.GetProviderLatency("gemini")
	if lat.Samples != 100 {
		t.Fatalf("expected 100 samples, got %d", lat.Samples)
	}
	if lat.P95 != 50*time.Millisecond {
		t.Errorf("expected P95=50ms, got %v", lat.P95)
	}

	// Add 100 more high-latency samples — old ones should be evicted
	for range 100 {
		cr.RecordLatency("gemini", 900*time.Millisecond)
	}

	lat = cr.GetProviderLatency("gemini")
	if lat.Samples != 100 {
		t.Fatalf("expected 100 samples after overflow, got %d", lat.Samples)
	}
	if lat.P95 != 900*time.Millisecond {
		t.Errorf("expected P95=900ms after old samples dropped, got %v", lat.P95)
	}
}

func TestSetBanditHooks(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	if cr.BanditConfigured() {
		t.Error("expected BanditConfigured=false initially")
	}

	called := false
	cr.SetBanditHooks(
		func() (string, string) { return "gemini", "gemini-2.5-flash" },
		func(provider string, reward float64) { called = true },
	)

	if !cr.BanditConfigured() {
		t.Error("expected BanditConfigured=true after SetBanditHooks")
	}

	// Record enough results to trigger bandit usage
	dir := t.TempDir()
	cr2 := NewCascadeRouter(config, nil, nil, dir)
	cr2.SetBanditHooks(
		func() (string, string) { return "gemini", "gemini-2.5-flash" },
		func(provider string, reward float64) { called = true },
	)
	for range 12 {
		cr2.RecordResult(CascadeResult{
			Timestamp:    time.Now(),
			UsedProvider: ProviderGemini,
			CheapCostUSD: 0.01,
		})
	}
	if !called {
		t.Error("expected bandit update to be called on RecordResult")
	}

	// SelectTier should consult bandit when enough history
	tier := cr2.SelectTier("lint", 1)
	if tier.Provider != ProviderGemini {
		t.Errorf("expected bandit-selected provider gemini, got %s", tier.Provider)
	}
}

func TestSetDecisionModel(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// DecisionModelStats returns nil when no model set
	if stats := cr.DecisionModelStats(); stats != nil {
		t.Errorf("expected nil stats, got %v", stats)
	}

	// Set a mock decision model
	mock := &mockDecisionModel{
		trained:    true,
		confidence: 0.95,
		stats:      map[string]any{"accuracy": 0.92},
	}
	cr.SetDecisionModel(mock)

	stats := cr.DecisionModelStats()
	if stats == nil {
		t.Fatal("expected non-nil stats after SetDecisionModel")
	}
	if stats["accuracy"] != 0.92 {
		t.Errorf("expected accuracy=0.92, got %v", stats["accuracy"])
	}

	// EvaluateCheapResult should use the decision model
	s := &Session{
		ID:         "dm-sess",
		TurnCount:  5,
		LastOutput: "Done successfully",
	}
	escalate, conf, reason := cr.EvaluateCheapResult(s, 10, nil)
	if escalate {
		t.Error("expected no escalation with high-confidence model")
	}
	if conf != 0.95 {
		t.Errorf("expected confidence=0.95 from model, got %f", conf)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %s", reason)
	}
}

type mockDecisionModel struct {
	trained    bool
	confidence float64
	stats      map[string]any
}

func (m *mockDecisionModel) IsTrained() bool { return m.trained }
func (m *mockDecisionModel) PredictConfidence(turnCount, expectedTurns int, lastOutput string, verifyPassed bool) float64 {
	return m.confidence
}
func (m *mockDecisionModel) Stats() map[string]any { return m.stats }

func TestSpeculativeLaunchOpts(t *testing.T) {
	config := DefaultCascadeConfig()
	config.MaxCheapBudgetUSD = 0.50
	config.MaxCheapTurns = 10
	cr := NewCascadeRouter(config, nil, nil, "")

	base := LaunchOptions{
		Provider:    ProviderClaude,
		RepoPath:    "/tmp/repo",
		Prompt:      "implement feature",
		SessionName: "my-session",
	}

	cheap, expensive := cr.SpeculativeLaunchOpts(base)

	if cheap.Provider != ProviderGemini {
		t.Errorf("cheap provider = %s, want gemini", cheap.Provider)
	}
	if cheap.SessionName != "my-session-cheap" {
		t.Errorf("cheap session name = %s, want my-session-cheap", cheap.SessionName)
	}
	if cheap.MaxBudgetUSD != 0.50 {
		t.Errorf("cheap budget = %f, want 0.50", cheap.MaxBudgetUSD)
	}
	if expensive.SessionName != "my-session-speculative" {
		t.Errorf("expensive session name = %s, want my-session-speculative", expensive.SessionName)
	}
	if expensive.Provider != ProviderClaude {
		t.Errorf("expensive provider = %s, want claude", expensive.Provider)
	}
}

func TestLogDecision(t *testing.T) {
	// nil decision log — always allows
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")
	if !cr.logDecision("test", "route", "testing", nil) {
		t.Error("expected logDecision=true with nil decisions")
	}

	// With a decision log at sufficient level
	dl := NewDecisionLog(t.TempDir(), LevelAutoOptimize)
	cr2 := NewCascadeRouter(config, nil, dl, "")
	if !cr2.logDecision("test", "route", "testing", map[string]any{"task": "lint"}) {
		t.Error("expected logDecision=true at AutoOptimize level")
	}
}

func TestSelectTier_BanditFallback(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// Set bandit that returns unknown provider
	cr.SetBanditHooks(
		func() (string, string) { return "unknown-provider", "" },
		func(string, float64) {},
	)

	// Add enough results for bandit to be consulted
	for range 12 {
		cr.mu.Lock()
		cr.results = append(cr.results, CascadeResult{})
		cr.mu.Unlock()
	}

	// Should fall through to static selection since bandit returns unknown
	tier := cr.SelectTier("lint", 0)
	if tier.Label != "ultra-cheap" {
		t.Errorf("expected fallback to static selection, got label=%q", tier.Label)
	}
}

func TestSelectTier_BanditEmptyProvider(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	cr.SetBanditHooks(
		func() (string, string) { return "", "" },
		func(string, float64) {},
	)

	for range 12 {
		cr.mu.Lock()
		cr.results = append(cr.results, CascadeResult{})
		cr.mu.Unlock()
	}

	tier := cr.SelectTier("lint", 0)
	if tier.Label != "ultra-cheap" {
		t.Errorf("expected static selection for empty bandit result, got label=%q", tier.Label)
	}
}

func TestSelectTier_HighComplexityExceedsAllTiers(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// Set tiers with max complexity=2
	cr.SetTiers([]ModelTier{
		{Provider: ProviderGemini, Model: "small", MaxComplexity: 1, CostPer1M: 0.1, Label: "tiny"},
		{Provider: ProviderGemini, Model: "medium", MaxComplexity: 2, CostPer1M: 0.5, Label: "medium"},
	})

	// Complexity 4 exceeds all tiers — should return highest-capability tier
	tier := cr.SelectTier("", 4)
	if tier.Label != "medium" {
		t.Errorf("expected highest tier for exceeding complexity, got label=%q", tier.Label)
	}
}

func TestRecentResults_LimitZero(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	for range 30 {
		cr.mu.Lock()
		cr.results = append(cr.results, CascadeResult{TaskTitle: "task"})
		cr.mu.Unlock()
	}

	// limit <= 0 defaults to 20
	results := cr.RecentResults(0)
	if len(results) != 20 {
		t.Errorf("expected 20 results for limit=0, got %d", len(results))
	}
}

func TestRecentResults_LessThanLimit(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	cr.mu.Lock()
	cr.results = append(cr.results, CascadeResult{TaskTitle: "only-one"})
	cr.mu.Unlock()

	results := cr.RecentResults(10)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].TaskTitle != "only-one" {
		t.Errorf("expected task title only-one, got %s", results[0].TaskTitle)
	}
}

func TestCascadeStats_Empty(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	stats := cr.Stats()
	if stats.TotalDecisions != 0 {
		t.Errorf("expected 0 total decisions, got %d", stats.TotalDecisions)
	}
	if stats.EscalationRate != 0 {
		t.Errorf("expected 0 escalation rate, got %f", stats.EscalationRate)
	}
}

func TestRecordResult_BanditUpdateCalledWithReward(t *testing.T) {
	dir := t.TempDir()
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, dir)

	var lastProvider string
	var lastReward float64
	cr.SetBanditHooks(
		func() (string, string) { return "gemini", "" },
		func(provider string, reward float64) {
			lastProvider = provider
			lastReward = reward
		},
	)

	// Non-escalated result should have reward 1.0
	cr.RecordResult(CascadeResult{
		UsedProvider: ProviderGemini,
		Escalated:    false,
	})
	if lastProvider != "gemini" {
		t.Errorf("expected provider=gemini, got %s", lastProvider)
	}
	if lastReward != 1.0 {
		t.Errorf("expected reward=1.0 for non-escalated, got %f", lastReward)
	}

	// Escalated result should have reward 0.2
	cr.RecordResult(CascadeResult{
		UsedProvider: ProviderClaude,
		Escalated:    true,
	})
	if lastProvider != "claude" {
		t.Errorf("expected provider=claude, got %s", lastProvider)
	}
	if lastReward != 0.2 {
		t.Errorf("expected reward=0.2 for escalated, got %f", lastReward)
	}
}

func TestComputeConfidence(t *testing.T) {
	tests := []struct {
		name          string
		turnCount     int
		expectedTurns int
		lastOutput    string
		verifyPassed  bool
		wantMin       float64
		wantMax       float64
	}{
		{
			name:          "all_signals_positive",
			turnCount:     5,
			expectedTurns: 10,
			lastOutput:    "Successfully completed the task",
			verifyPassed:  true,
			wantMin:       0.9,
			wantMax:       1.0,
		},
		{
			name:          "high_hedging",
			turnCount:     20,
			expectedTurns: 5,
			lastOutput:    "I'm not sure this is right. Maybe the approach is wrong. I think there might be issues. I'm not confident. Possibly broken.",
			verifyPassed:  false,
			wantMin:       0.0,
			wantMax:       0.30,
		},
		{
			name:          "error_in_output",
			turnCount:     5,
			expectedTurns: 10,
			lastOutput:    "error: compilation failed",
			verifyPassed:  true,
			wantMin:       0.4,
			wantMax:       0.8,
		},
		{
			name:          "empty_output_no_turns",
			turnCount:     0,
			expectedTurns: 0,
			lastOutput:    "",
			verifyPassed:  true,
			wantMin:       0.6,
			wantMax:       1.0,
		},
		{
			name:          "over_2x_expected_turns",
			turnCount:     25,
			expectedTurns: 10,
			lastOutput:    "Done.",
			verifyPassed:  true,
			wantMin:       0.4,
			wantMax:       0.8,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			conf := computeConfidence(tc.turnCount, tc.expectedTurns, tc.lastOutput, tc.verifyPassed)
			if conf < tc.wantMin || conf > tc.wantMax {
				t.Errorf("computeConfidence() = %f, want in [%f, %f]", conf, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestComputePercentile_EdgeCases(t *testing.T) {
	// Empty slice
	if got := computePercentile(nil, 50); got != 0 {
		t.Errorf("expected 0 for empty slice, got %v", got)
	}

	// Single element
	if got := computePercentile([]time.Duration{100 * time.Millisecond}, 50); got != 100*time.Millisecond {
		t.Errorf("expected 100ms for single element, got %v", got)
	}

	// P0 and P100
	samples := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	if got := computePercentile(samples, 100); got != 30*time.Millisecond {
		t.Errorf("P100 = %v, want 30ms", got)
	}
}

func TestGetProviderLatency_NoSamples(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	if lat := cr.GetProviderLatency("nonexistent"); lat != nil {
		t.Errorf("expected nil for unknown provider, got %v", lat)
	}
}

func TestAppendResult_NoStateDir(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	// Should not panic with empty stateDir
	cr.RecordResult(CascadeResult{
		Timestamp:    time.Now(),
		UsedProvider: ProviderGemini,
	})

	// Verify in-memory results still tracked
	results := cr.RecentResults(10)
	if len(results) != 1 {
		t.Errorf("expected 1 result in memory, got %d", len(results))
	}
}

func TestComputeConfidence_EmptyOutputVerifyFailed(t *testing.T) {
	// Empty output with verify failed — should be low confidence
	conf := computeConfidence(5, 10, "", false)
	// verify=0, hedge=1.0, error-free=1.0, turn efficiency=1.0 => (0+1+1+1)/4 = 0.75
	// But verify failed, so second component = 0 => (1+0+1+1)/4 = 0.75
	if conf >= 1.0 || conf < 0 {
		t.Errorf("computeConfidence empty output + verify failed = %f, out of range", conf)
	}
}

func TestComputeConfidence_PanicInOutput(t *testing.T) {
	conf := computeConfidence(5, 10, "panic: runtime error", true)
	// error signals in output => no error-free score
	if conf >= 1.0 {
		t.Errorf("expected confidence < 1.0 with panic in output, got %f", conf)
	}
}

func TestComputeConfidence_FailedInOutput(t *testing.T) {
	conf := computeConfidence(5, 10, "failed: compilation error in main.go", true)
	if conf >= 1.0 {
		t.Errorf("expected confidence < 1.0 with failed: in output, got %f", conf)
	}
}

func TestComputeConfidence_MidRangeTurnRatio(t *testing.T) {
	// turnCount=15, expectedTurns=10 => ratio 1.5, so turn score = 0.5
	conf := computeConfidence(15, 10, "Done.", true)
	if conf <= 0 || conf >= 1.0 {
		t.Errorf("expected mid-range confidence for 1.5x turn ratio, got %f", conf)
	}
}

func TestComputeConfidence_ZeroTurnsZeroExpected(t *testing.T) {
	// 0 turns, 0 expected => no turn component
	conf := computeConfidence(0, 0, "Output here", true)
	if conf <= 0 || conf > 1.0 {
		t.Errorf("expected valid confidence for zero turns/expected, got %f", conf)
	}
}

func TestComputeConfidence_NoExpectedTurns(t *testing.T) {
	// Non-zero turns but 0 expected => skip turn component
	conf := computeConfidence(5, 0, "All done successfully.", true)
	// verify=1, hedge=1, error-free=1 => 3/3 = 1.0
	if conf != 1.0 {
		t.Errorf("expected 1.0 for all positive signals with no expected turns, got %f", conf)
	}
}

func TestShouldCascade_LatencyThresholdNoSamples(t *testing.T) {
	config := DefaultCascadeConfig()
	config.LatencyThresholdMs = 500
	cr := NewCascadeRouter(config, nil, nil, "")

	// No latency samples recorded — should cascade (don't skip cheap)
	if !cr.ShouldCascade("feature", "something") {
		t.Error("expected ShouldCascade=true with no latency samples")
	}
}

func TestResolveProvider_LatencyHighSkipsCheap(t *testing.T) {
	config := DefaultCascadeConfig()
	config.LatencyThresholdMs = 200
	cr := NewCascadeRouter(config, nil, nil, "")

	// Record high latencies
	for range 20 {
		cr.RecordLatency("gemini", 500*time.Millisecond)
	}

	got := cr.ResolveProvider("feature")
	if got != ProviderCodex {
		t.Errorf("expected expensive provider when cheap is slow, got %s", got)
	}
}

func TestCascadeStats_AllEscalated(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	for range 5 {
		cr.mu.Lock()
		cr.results = append(cr.results, CascadeResult{
			Escalated:    true,
			CheapCostUSD: 0.10,
			TotalCostUSD: 1.00,
		})
		cr.mu.Unlock()
	}

	stats := cr.Stats()
	if stats.Escalations != 5 {
		t.Errorf("Escalations = %d, want 5", stats.Escalations)
	}
	if stats.EscalationRate != 1.0 {
		t.Errorf("EscalationRate = %f, want 1.0", stats.EscalationRate)
	}
	if stats.CostSavedUSD != 0 {
		t.Errorf("CostSavedUSD = %f, want 0 when all escalated", stats.CostSavedUSD)
	}
}

func TestCascadeStats_NoneEscalated(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	for range 3 {
		cr.mu.Lock()
		cr.results = append(cr.results, CascadeResult{
			Escalated:    false,
			CheapCostUSD: 0.20,
			TotalCostUSD: 0.20,
		})
		cr.mu.Unlock()
	}

	stats := cr.Stats()
	if stats.EscalationRate != 0 {
		t.Errorf("EscalationRate = %f, want 0", stats.EscalationRate)
	}
	if diff := stats.CostSavedUSD - 0.60; diff > 0.01 || diff < -0.01 {
		t.Errorf("CostSavedUSD = %f, want 0.60", stats.CostSavedUSD)
	}
}

func TestEvaluateCheapResult_DecisionModelUntrained(t *testing.T) {
	config := DefaultCascadeConfig()
	config.ConfidenceThreshold = 0.7
	cr := NewCascadeRouter(config, nil, nil, "")

	// Set an untrained model — should fall back to heuristic
	mock := &mockDecisionModel{
		trained:    false,
		confidence: 0.99, // would pass if used, but model isn't trained
	}
	cr.SetDecisionModel(mock)

	s := &Session{
		ID:         "untrained-dm-sess",
		TurnCount:  5,
		LastOutput: "Successfully done.",
	}

	escalate, conf, _ := cr.EvaluateCheapResult(s, 10, nil)
	// Should use heuristic, not the model's 0.99
	if conf == 0.99 {
		t.Error("should use heuristic when decision model is untrained")
	}
	// With good signals, heuristic should give high confidence
	if escalate {
		t.Errorf("expected no escalation with good signals, conf=%f", conf)
	}
}

func TestRecordResult_PersistenceIntegrity(t *testing.T) {
	dir := t.TempDir()
	config := DefaultCascadeConfig()

	cr := NewCascadeRouter(config, nil, nil, dir)

	// Record multiple results with varying fields
	for i := range 5 {
		cr.RecordResult(CascadeResult{
			Timestamp:       time.Now(),
			TaskType:        "feature",
			TaskTitle:       "task-" + string(rune('A'+i)),
			UsedProvider:    ProviderGemini,
			Escalated:       i%2 == 0,
			CheapConfidence: float64(i) * 0.2,
			CheapCostUSD:    float64(i) * 0.05,
			TotalCostUSD:    float64(i) * 0.10,
		})
	}

	// Reload and verify
	cr2 := NewCascadeRouter(config, nil, nil, dir)
	results := cr2.RecentResults(10)
	if len(results) != 5 {
		t.Fatalf("expected 5 loaded results, got %d", len(results))
	}
	for i, r := range results {
		want := "task-" + string(rune('A'+i))
		if r.TaskTitle != want {
			t.Errorf("result[%d].TaskTitle = %q, want %q", i, r.TaskTitle, want)
		}
	}
}

func TestSelectTier_BanditNotEnoughHistory(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	called := false
	cr.SetBanditHooks(
		func() (string, string) { called = true; return "gemini", "" },
		func(string, float64) {},
	)

	// Only 5 results (< 10 minimum)
	for range 5 {
		cr.mu.Lock()
		cr.results = append(cr.results, CascadeResult{})
		cr.mu.Unlock()
	}

	tier := cr.SelectTier("lint", 0)
	if called {
		t.Error("bandit should not be consulted with < 10 history items")
	}
	if tier.Label != "ultra-cheap" {
		t.Errorf("expected static selection, got label=%q", tier.Label)
	}
}

func TestRecordLatency_MultipleProviders(t *testing.T) {
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	cr.RecordLatency("gemini", 100*time.Millisecond)
	cr.RecordLatency("gemini", 200*time.Millisecond)
	cr.RecordLatency("claude", 500*time.Millisecond)
	cr.RecordLatency("claude", 600*time.Millisecond)

	gemLat := cr.GetProviderLatency("gemini")
	claudeLat := cr.GetProviderLatency("claude")

	if gemLat == nil || claudeLat == nil {
		t.Fatal("expected non-nil latencies for both providers")
	}
	if gemLat.Samples != 2 {
		t.Errorf("gemini samples = %d, want 2", gemLat.Samples)
	}
	if claudeLat.Samples != 2 {
		t.Errorf("claude samples = %d, want 2", claudeLat.Samples)
	}
	// Gemini P50 should be lower than Claude P50
	if gemLat.P50 >= claudeLat.P50 {
		t.Errorf("expected gemini P50 < claude P50: %v >= %v", gemLat.P50, claudeLat.P50)
	}
}

func TestDefaultCascadeConfig(t *testing.T) {
	config := DefaultCascadeConfig()
	if config.CheapProvider != ProviderGemini {
		t.Errorf("CheapProvider = %s, want gemini", config.CheapProvider)
	}
	if config.ExpensiveProvider != ProviderCodex {
		t.Errorf("ExpensiveProvider = %s, want codex", config.ExpensiveProvider)
	}
	if config.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7", config.ConfidenceThreshold)
	}
	if config.MaxCheapBudgetUSD != 2.00 {
		t.Errorf("MaxCheapBudgetUSD = %f, want 2.00", config.MaxCheapBudgetUSD)
	}
	if config.MaxCheapTurns != 15 {
		t.Errorf("MaxCheapTurns = %d, want 15", config.MaxCheapTurns)
	}
	if config.TaskTypeOverrides == nil {
		t.Error("TaskTypeOverrides should be initialized")
	}
	if config.LatencyThresholdMs != 0 {
		t.Errorf("LatencyThresholdMs = %d, want 0 (disabled)", config.LatencyThresholdMs)
	}
}

func TestCheapLaunchOpts_ZeroBudgetAndTurns(t *testing.T) {
	config := DefaultCascadeConfig()
	config.MaxCheapBudgetUSD = 0
	config.MaxCheapTurns = 0

	cr := NewCascadeRouter(config, nil, nil, "")

	base := LaunchOptions{
		Provider:     ProviderClaude,
		MaxBudgetUSD: 5.00,
		MaxTurns:     50,
		SessionName:  "test",
	}

	cheap := cr.CheapLaunchOpts(base)
	// With zero config values, base values should be preserved
	if cheap.MaxBudgetUSD != 5.00 {
		t.Errorf("expected base budget preserved when config is 0, got %f", cheap.MaxBudgetUSD)
	}
	if cheap.MaxTurns != 50 {
		t.Errorf("expected base max turns preserved when config is 0, got %d", cheap.MaxTurns)
	}
}

func TestDefaultCascadeFromConfig_NilConfig(t *testing.T) {
	result := DefaultCascadeFromConfig(nil)
	if result != nil {
		t.Error("expected nil for nil config")
	}
}

func TestDefaultCascadeFromConfig_EmptyConfig(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{})
	if result != nil {
		t.Error("expected nil when CASCADE_ENABLED is not set")
	}
}

func TestDefaultCascadeFromConfig_DisabledExplicit(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED": "false",
	})
	if result != nil {
		t.Error("expected nil when CASCADE_ENABLED=false")
	}
}

func TestDefaultCascadeFromConfig_EnabledDefaults(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED": "true",
	})
	if result == nil {
		t.Fatal("expected non-nil config when CASCADE_ENABLED=true")
	}
	if result.CheapProvider != ProviderGemini {
		t.Errorf("CheapProvider = %q, want %q", result.CheapProvider, ProviderGemini)
	}
	if result.ExpensiveProvider != ProviderCodex {
		t.Errorf("ExpensiveProvider = %q, want %q", result.ExpensiveProvider, ProviderCodex)
	}
	if result.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7", result.ConfidenceThreshold)
	}
	if result.MaxCheapBudgetUSD != 2.00 {
		t.Errorf("MaxCheapBudgetUSD = %f, want 2.00", result.MaxCheapBudgetUSD)
	}
	if result.MaxCheapTurns != 15 {
		t.Errorf("MaxCheapTurns = %d, want 15", result.MaxCheapTurns)
	}
}

func TestDefaultCascadeFromConfig_CustomValues(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED":              "true",
		"CASCADE_CHEAP_PROVIDER":       "codex",
		"CASCADE_EXPENSIVE_PROVIDER":   "gemini",
		"CASCADE_CONFIDENCE_THRESHOLD": "0.85",
		"CASCADE_MAX_CHEAP_BUDGET":     "5.50",
	})
	if result == nil {
		t.Fatal("expected non-nil config")
	}
	if result.CheapProvider != ProviderCodex {
		t.Errorf("CheapProvider = %q, want %q", result.CheapProvider, ProviderCodex)
	}
	if result.ExpensiveProvider != ProviderGemini {
		t.Errorf("ExpensiveProvider = %q, want %q", result.ExpensiveProvider, ProviderGemini)
	}
	if result.ConfidenceThreshold != 0.85 {
		t.Errorf("ConfidenceThreshold = %f, want 0.85", result.ConfidenceThreshold)
	}
	if result.MaxCheapBudgetUSD != 5.50 {
		t.Errorf("MaxCheapBudgetUSD = %f, want 5.50", result.MaxCheapBudgetUSD)
	}
}

func TestDefaultCascadeFromConfig_InvalidNumericsFallback(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED":              "true",
		"CASCADE_CONFIDENCE_THRESHOLD": "notanumber",
		"CASCADE_MAX_CHEAP_BUDGET":     "garbage",
	})
	if result == nil {
		t.Fatal("expected non-nil config despite invalid numerics")
	}
	// Should fall back to defaults
	if result.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7 (default fallback)", result.ConfidenceThreshold)
	}
	if result.MaxCheapBudgetUSD != 2.00 {
		t.Errorf("MaxCheapBudgetUSD = %f, want 2.00 (default fallback)", result.MaxCheapBudgetUSD)
	}
}

func TestDefaultCascadeFromConfig_PartialConfig(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED":        "true",
		"CASCADE_CHEAP_PROVIDER": "codex",
		// everything else should use defaults
	})
	if result == nil {
		t.Fatal("expected non-nil config")
	}
	if result.CheapProvider != ProviderCodex {
		t.Errorf("CheapProvider = %q, want codex", result.CheapProvider)
	}
	if result.ExpensiveProvider != ProviderCodex {
		t.Errorf("ExpensiveProvider = %q, want codex (default)", result.ExpensiveProvider)
	}
	if result.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7 (default)", result.ConfidenceThreshold)
	}
}

func TestDefaultCascadeFromConfig_EnabledVariants(t *testing.T) {
	for _, val := range []string{"true", "TRUE", "True", "1", "yes"} {
		result := DefaultCascadeFromConfig(map[string]string{
			"CASCADE_ENABLED": val,
		})
		if result == nil {
			t.Errorf("expected non-nil for CASCADE_ENABLED=%q", val)
		}
	}
}

func TestDefaultCascadeFromConfig_OutOfRangeConfidence(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED":              "true",
		"CASCADE_CONFIDENCE_THRESHOLD": "1.5", // out of 0-1 range
	})
	if result == nil {
		t.Fatal("expected non-nil config")
	}
	// Out-of-range should fall back to default
	if result.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7 (default for out-of-range)", result.ConfidenceThreshold)
	}
}

func TestDefaultCascadeFromConfig_NegativeBudget(t *testing.T) {
	result := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED":          "true",
		"CASCADE_MAX_CHEAP_BUDGET": "-1.00",
	})
	if result == nil {
		t.Fatal("expected non-nil config")
	}
	// Negative should fall back to default
	if result.MaxCheapBudgetUSD != 2.00 {
		t.Errorf("MaxCheapBudgetUSD = %f, want 2.00 (default for negative)", result.MaxCheapBudgetUSD)
	}
}

func TestCascadeRouter_ConcurrentShouldCascade(t *testing.T) {
	t.Parallel()
	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, t.TempDir())

	const N = 10
	var wg sync.WaitGroup

	// 10 goroutines calling ShouldCascade and SelectTier concurrently
	for i := range N {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := range 50 {
				taskTypes := []string{"feature", "docs", "refactor", "lint", "test"}
				tt := taskTypes[j%len(taskTypes)]
				_ = cr.ShouldCascade(tt, "do some work")
				_ = cr.SelectTier(tt, idx%4+1)
			}
		}(i)
	}

	// 1 goroutine modifying state: recording latencies and results
	wg.Go(func() {
		for j := range 100 {
			cr.RecordLatency("gemini", time.Duration(j+100)*time.Millisecond)
			cr.RecordLatency("claude", time.Duration(j+200)*time.Millisecond)
		}
	})

	wg.Wait()

	// After concurrent access, methods should still work correctly.
	got := cr.ShouldCascade("feature", "implement something")
	if !got {
		t.Error("expected ShouldCascade=true with nil feedback")
	}
	tier := cr.SelectTier("lint", 1)
	if tier.Provider == "" {
		t.Error("expected non-empty provider from SelectTier")
	}
}

// TestCascadeConfig_MalformedThreshold verifies that DefaultCascadeFromConfig
// falls back to the default ConfidenceThreshold (0.7) when the config value
// is malformed (non-numeric).
func TestCascadeConfig_MalformedThreshold(t *testing.T) {
	t.Parallel()

	cfg := DefaultCascadeFromConfig(map[string]string{
		"CASCADE_ENABLED":              "true",
		"CASCADE_CONFIDENCE_THRESHOLD": "abc",
	})
	if cfg == nil {
		t.Fatal("expected non-nil config when CASCADE_ENABLED=true")
	}
	if cfg.ConfidenceThreshold != 0.7 {
		t.Errorf("expected default threshold 0.7 for malformed value, got %f", cfg.ConfidenceThreshold)
	}
}

// TestCascadeRouter_NilRouterNoPanic verifies that when a Manager has no
// QW-2: Cascade routing is now enabled by default, so a fresh manager
// should always have a cascade router configured.
func TestCascadeRouter_DefaultEnabled(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// HasCascadeRouter should return true — cascade is enabled by default.
	if !m.HasCascadeRouter() {
		t.Error("expected HasCascadeRouter=true for fresh manager (QW-2)")
	}

	// GetCascadeRouter should return a non-nil default router.
	cr := m.GetCascadeRouter()
	if cr == nil {
		t.Error("expected GetCascadeRouter=non-nil for fresh manager (QW-2)")
	}

	// The default router should allow cascading for unknown task types.
	if !cr.ShouldCascade("feature", "implement something") {
		t.Error("expected default cascade router to allow cascading")
	}
}

// TestCascadeRouter_DefaultEnabledAllConstructors verifies all Manager
// constructors create a cascade router by default (QW-2).
func TestCascadeRouter_DefaultEnabledAllConstructors(t *testing.T) {
	t.Parallel()

	t.Run("NewManager", func(t *testing.T) {
		m := NewManager()
		if !m.HasCascadeRouter() {
			t.Error("NewManager: expected cascade router by default")
		}
	})

	t.Run("NewManagerWithBus", func(t *testing.T) {
		m := NewManagerWithBus(nil)
		if !m.HasCascadeRouter() {
			t.Error("NewManagerWithBus: expected cascade router by default")
		}
	})

	t.Run("NewManagerWithStore", func(t *testing.T) {
		m := NewManagerWithStore(nil, nil)
		if !m.HasCascadeRouter() {
			t.Error("NewManagerWithStore: expected cascade router by default")
		}
	})
}

// TestCascadeRouter_ProfileDefaultEnabled verifies all default loop profiles
// have EnableCascade=true (QW-2: cascade routing on by default).
func TestCascadeRouter_ProfileDefaultEnabled(t *testing.T) {
	t.Parallel()

	t.Run("DefaultLoopProfile", func(t *testing.T) {
		p := DefaultLoopProfile()
		if !p.EnableCascade {
			t.Error("DefaultLoopProfile: expected EnableCascade=true (QW-2)")
		}
	})

	t.Run("BudgetOptimizedSelfImprovementProfile", func(t *testing.T) {
		p := BudgetOptimizedSelfImprovementProfile(100)
		if !p.EnableCascade {
			t.Error("BudgetOptimizedSelfImprovementProfile: expected EnableCascade=true (QW-2)")
		}
	})
}

// TestCascadeRouter_ConfigExplicitDisable verifies that CASCADE_ENABLED=false
// in config removes the default cascade router.
func TestCascadeRouter_ConfigExplicitDisable(t *testing.T) {
	t.Parallel()

	m := NewManager()
	if !m.HasCascadeRouter() {
		t.Fatal("precondition: expected cascade router by default")
	}

	cfg := &model.RalphConfig{Values: map[string]string{"CASCADE_ENABLED": "false"}}
	m.ApplyConfig(cfg)
	if m.HasCascadeRouter() {
		t.Error("expected cascade router disabled after CASCADE_ENABLED=false")
	}
}
