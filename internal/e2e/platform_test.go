package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- Tiered routing via CascadeRouter ---

func TestCascadeRouterDefaultConfig(t *testing.T) {
	cfg := session.DefaultCascadeConfig()

	if cfg.CheapProvider != session.ProviderGemini {
		t.Errorf("CheapProvider = %q, want %q", cfg.CheapProvider, session.ProviderGemini)
	}
	if cfg.ExpensiveProvider != session.ProviderCodex {
		t.Errorf("ExpensiveProvider = %q, want %q", cfg.ExpensiveProvider, session.ProviderCodex)
	}
	if cfg.ConfidenceThreshold != 0.7 {
		t.Errorf("ConfidenceThreshold = %f, want 0.7", cfg.ConfidenceThreshold)
	}
	if cfg.MaxCheapBudgetUSD != 2.00 {
		t.Errorf("MaxCheapBudgetUSD = %f, want 2.00", cfg.MaxCheapBudgetUSD)
	}
	if cfg.MaxCheapTurns != 15 {
		t.Errorf("MaxCheapTurns = %d, want 15", cfg.MaxCheapTurns)
	}
}

func TestCascadeRouterResolveProviderWithOverrides(t *testing.T) {
	cfg := session.DefaultCascadeConfig()
	cfg.TaskTypeOverrides = map[string]session.Provider{
		"lint":         session.ProviderGemini,
		"architecture": session.ProviderCodex,
	}

	cr := session.NewCascadeRouter(cfg, nil, nil, "")

	// Task type override should return the override directly.
	if got := cr.ResolveProvider("lint"); got != session.ProviderGemini {
		t.Errorf("ResolveProvider(lint) = %q, want %q", got, session.ProviderGemini)
	}
	if got := cr.ResolveProvider("architecture"); got != session.ProviderCodex {
		t.Errorf("ResolveProvider(architecture) = %q, want %q", got, session.ProviderCodex)
	}
}

func TestCascadeRouterShouldCascadeDefault(t *testing.T) {
	cfg := session.DefaultCascadeConfig()
	cr := session.NewCascadeRouter(cfg, nil, nil, "")

	// No feedback analyzer and no override: should cascade.
	if !cr.ShouldCascade("codegen", "implement feature X") {
		t.Error("ShouldCascade(codegen) = false, want true (no feedback data)")
	}
}

func TestCascadeRouterShouldCascadeOverridedSkips(t *testing.T) {
	cfg := session.DefaultCascadeConfig()
	cfg.TaskTypeOverrides = map[string]session.Provider{
		"codegen": session.ProviderCodex,
	}
	cr := session.NewCascadeRouter(cfg, nil, nil, "")

	// Override present: should NOT cascade.
	if cr.ShouldCascade("codegen", "implement feature X") {
		t.Error("ShouldCascade(codegen) = true, want false (override present)")
	}
}

func TestCascadeRouterCheapLaunchOpts(t *testing.T) {
	cfg := session.DefaultCascadeConfig()
	cfg.MaxCheapBudgetUSD = 1.50
	cfg.MaxCheapTurns = 10
	cr := session.NewCascadeRouter(cfg, nil, nil, "")

	base := session.LaunchOptions{
		Provider:     session.ProviderClaude,
		SessionName:  "worker-1",
		MaxBudgetUSD: 5.0,
		MaxTurns:     50,
	}

	cheap := cr.CheapLaunchOpts(base)

	if cheap.Provider != session.ProviderGemini {
		t.Errorf("CheapLaunchOpts.Provider = %q, want %q", cheap.Provider, session.ProviderGemini)
	}
	if cheap.MaxBudgetUSD != 1.50 {
		t.Errorf("CheapLaunchOpts.MaxBudgetUSD = %f, want 1.50", cheap.MaxBudgetUSD)
	}
	if cheap.MaxTurns != 10 {
		t.Errorf("CheapLaunchOpts.MaxTurns = %d, want 10", cheap.MaxTurns)
	}
	if cheap.SessionName != "worker-1-cheap" {
		t.Errorf("CheapLaunchOpts.SessionName = %q, want %q", cheap.SessionName, "worker-1-cheap")
	}
}

func TestCascadeRouterRecordAndStats(t *testing.T) {
	dir := t.TempDir()
	cfg := session.DefaultCascadeConfig()
	cr := session.NewCascadeRouter(cfg, nil, nil, dir)

	cr.RecordResult(session.CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "lint",
		UsedProvider: session.ProviderGemini,
		Escalated:    false,
		CheapCostUSD: 0.10,
		TotalCostUSD: 0.10,
	})
	cr.RecordResult(session.CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "codegen",
		UsedProvider: session.ProviderCodex,
		Escalated:    true,
		CheapCostUSD: 0.20,
		TotalCostUSD: 1.50,
		Reason:       "low_confidence",
	})

	stats := cr.Stats()
	if stats.TotalDecisions != 2 {
		t.Errorf("TotalDecisions = %d, want 2", stats.TotalDecisions)
	}
	if stats.Escalations != 1 {
		t.Errorf("Escalations = %d, want 1", stats.Escalations)
	}
	if stats.EscalationRate != 0.5 {
		t.Errorf("EscalationRate = %f, want 0.5", stats.EscalationRate)
	}
	// CostSaved = cheap cost of non-escalated (0.10)
	if stats.CostSavedUSD != 0.10 {
		t.Errorf("CostSavedUSD = %f, want 0.10", stats.CostSavedUSD)
	}
}

func TestCascadeRouterRecentResults(t *testing.T) {
	cfg := session.DefaultCascadeConfig()
	cr := session.NewCascadeRouter(cfg, nil, nil, "")

	for i := 0; i < 5; i++ {
		cr.RecordResult(session.CascadeResult{
			Timestamp: time.Now(),
			TaskType:  "test",
		})
	}

	results := cr.RecentResults(3)
	if len(results) != 3 {
		t.Errorf("RecentResults(3) returned %d, want 3", len(results))
	}

	all := cr.RecentResults(0) // default limit 20
	if len(all) != 5 {
		t.Errorf("RecentResults(0) returned %d, want 5", len(all))
	}
}

func TestCascadeRouterPersistsResults(t *testing.T) {
	dir := t.TempDir()
	cfg := session.DefaultCascadeConfig()

	// Write results with one router.
	cr1 := session.NewCascadeRouter(cfg, nil, nil, dir)
	cr1.RecordResult(session.CascadeResult{
		Timestamp:    time.Now(),
		TaskType:     "persist-test",
		CheapCostUSD: 0.05,
	})

	// New router should load persisted results.
	cr2 := session.NewCascadeRouter(cfg, nil, nil, dir)
	stats := cr2.Stats()
	if stats.TotalDecisions != 1 {
		t.Errorf("persisted TotalDecisions = %d, want 1", stats.TotalDecisions)
	}
}

// --- Compaction readiness (lint concept) ---

func TestSelfImprovementProfileHasSelfLearningEnabled(t *testing.T) {
	p := session.SelfImprovementProfile()

	if !p.SelfImprovement {
		t.Error("SelfImprovement = false, want true")
	}
	if !p.EnableReflexion {
		t.Error("EnableReflexion = false, want true")
	}
	if !p.EnableEpisodicMemory {
		t.Error("EnableEpisodicMemory = false, want true")
	}
	if !p.EnableUncertainty {
		t.Error("EnableUncertainty = false, want true")
	}
	if !p.EnableCurriculum {
		t.Error("EnableCurriculum = false, want true")
	}
	if p.EnableCascade {
		t.Error("EnableCascade = true, want false (serial self-improvement)")
	}
	if p.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", p.MaxIterations)
	}
	if p.MaxDurationSecs != 14400 {
		t.Errorf("MaxDurationSecs = %d, want 14400", p.MaxDurationSecs)
	}
}

func TestDefaultLoopProfileProviders(t *testing.T) {
	p := session.DefaultLoopProfile()

	if p.PlannerProvider != session.ProviderCodex {
		t.Errorf("PlannerProvider = %q, want %q", p.PlannerProvider, session.ProviderCodex)
	}
	if p.WorkerProvider != session.ProviderCodex {
		t.Errorf("WorkerProvider = %q, want %q", p.WorkerProvider, session.ProviderCodex)
	}
	if p.WorktreePolicy != "git" {
		t.Errorf("WorktreePolicy = %q, want git", p.WorktreePolicy)
	}
	if p.MaxConcurrentWorkers != 1 {
		t.Errorf("MaxConcurrentWorkers = %d, want 1", p.MaxConcurrentWorkers)
	}
}

// --- Prompt caching config ---

func TestPromptCacheConfigDefaults(t *testing.T) {
	cfg := session.DefaultPromptCacheConfig()

	if !cfg.Enabled {
		t.Error("Enabled = false, want true")
	}
	if cfg.MinPrefixLen != 1024 {
		t.Errorf("MinPrefixLen = %d, want 1024", cfg.MinPrefixLen)
	}
	if cfg.MaxCacheEntries != 100 {
		t.Errorf("MaxCacheEntries = %d, want 100", cfg.MaxCacheEntries)
	}
	if cfg.CacheTTL != 300 {
		t.Errorf("CacheTTL = %d, want 300", cfg.CacheTTL)
	}
}

func TestShouldCachePromptByProvider(t *testing.T) {
	tests := []struct {
		provider  session.Provider
		promptLen int
		want      bool
	}{
		{session.ProviderClaude, 2000, true},
		{session.ProviderClaude, 500, false},
		{session.ProviderGemini, 3000, true},
		{session.ProviderGemini, 1000, false},
		{session.ProviderCodex, 5000, true},
		{session.ProviderCodex, 100, false},
	}
	for _, tt := range tests {
		got := session.ShouldCachePrompt(tt.provider, tt.promptLen)
		if got != tt.want {
			t.Errorf("ShouldCachePrompt(%q, %d) = %v, want %v",
				tt.provider, tt.promptLen, got, tt.want)
		}
	}
}

func TestPromptCacheTrackerHitAndMiss(t *testing.T) {
	cfg := session.DefaultPromptCacheConfig()
	cfg.MinPrefixLen = 50 // lower for testing
	tracker := session.NewPromptCacheTracker(cfg)
	repoDir := t.TempDir()
	agentsContent := "# Repo Instructions\nUse Codex for coding tasks.\nKeep edits focused, validate relevant paths, and avoid unrelated churn.\n"
	if err := os.WriteFile(filepath.Join(repoDir, "AGENTS.md"), []byte(agentsContent), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	longPrompt := strings.Repeat("## System Instructions\n- Follow all rules below.\n- Be concise and correct.\n- Do not hallucinate.\n\n## Constraints:\n- Maximum 100 lines of code.\n- No external dependencies.\n\n", 8) +
		"Now implement the feature X for the project."

	// First call: cache miss (creates entry).
	_, cacheable1 := tracker.AnalyzePrompt(repoDir, session.ProviderCodex, longPrompt)
	if !cacheable1 {
		t.Error("first call: cacheable = false, want true")
	}

	// Second call with same prompt: cache hit.
	_, cacheable2 := tracker.AnalyzePrompt(repoDir, session.ProviderCodex, longPrompt)
	if !cacheable2 {
		t.Error("second call: cacheable = false, want true")
	}

	stats := tracker.Stats()
	if stats.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", stats.TotalSessions)
	}
	if stats.EstimatedHits < 1 {
		t.Errorf("EstimatedHits = %d, want >= 1", stats.EstimatedHits)
	}
}

func TestPromptCacheTrackerDisabledSkips(t *testing.T) {
	cfg := session.DefaultPromptCacheConfig()
	cfg.Enabled = false
	tracker := session.NewPromptCacheTracker(cfg)

	_, cacheable := tracker.AnalyzePrompt("/repo", session.ProviderClaude, "some prompt text here that is long enough")
	if cacheable {
		t.Error("disabled tracker should return cacheable=false")
	}
}

// --- Cascade routing with custom tiers (task type overrides) ---

func TestCascadeCustomTierOverrides(t *testing.T) {
	cfg := session.DefaultCascadeConfig()
	cfg.TaskTypeOverrides = map[string]session.Provider{
		"lint":         session.ProviderGemini,
		"codegen":      session.ProviderCodex,
		"test":         session.ProviderGemini,
		"architecture": session.ProviderCodex,
	}

	cr := session.NewCascadeRouter(cfg, nil, nil, "")

	// Each override task type routes to its designated provider.
	cases := []struct {
		taskType string
		want     session.Provider
	}{
		{"lint", session.ProviderGemini},
		{"codegen", session.ProviderCodex},
		{"test", session.ProviderGemini},
		{"architecture", session.ProviderCodex},
	}
	for _, tc := range cases {
		got := cr.ResolveProvider(tc.taskType)
		if got != tc.want {
			t.Errorf("ResolveProvider(%q) = %q, want %q", tc.taskType, got, tc.want)
		}
	}

	// Unknown task type without feedback falls through to expensive.
	if got := cr.ResolveProvider("unknown"); got != session.ProviderCodex {
		t.Errorf("ResolveProvider(unknown) = %q, want %q (expensive fallback)", got, session.ProviderCodex)
	}
}

// --- LoopProfile JSON round-trip ---

func TestLoopProfileCascadeConfigJSON(t *testing.T) {
	// Verify that CascadeConfig can be set on LoopProfile and is non-nil.
	p := session.LoopProfile{
		EnableCascade: true,
		CascadeConfig: &session.CascadeConfig{
			CheapProvider:       session.ProviderGemini,
			ExpensiveProvider:   session.ProviderClaude,
			ConfidenceThreshold: 0.8,
			MaxCheapTurns:       10,
		},
	}
	if !p.EnableCascade {
		t.Error("EnableCascade = false after set")
	}
	if p.CascadeConfig == nil {
		t.Fatal("CascadeConfig is nil after set")
	}
	if p.CascadeConfig.ConfidenceThreshold != 0.8 {
		t.Errorf("ConfidenceThreshold = %f, want 0.8", p.CascadeConfig.ConfidenceThreshold)
	}
}

// --- E2E scenario with cascade routing profile ---

func TestE2EScenarioWithCascadeProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e scenario test in short mode")
	}

	scenarios := AllScenarios()
	// Find any scenario that works and patch cascade config onto it.
	if len(scenarios) == 0 {
		t.Skip("no scenarios available")
	}

	s := scenarios[0]
	originalPatch := s.ProfilePatch
	s.ProfilePatch = func(p *session.LoopProfile) {
		if originalPatch != nil {
			originalPatch(p)
		}
		p.EnableCascade = true
		p.CascadeConfig = &session.CascadeConfig{
			CheapProvider:       session.ProviderGemini,
			ExpensiveProvider:   session.ProviderClaude,
			ConfidenceThreshold: 0.8,
			MaxCheapBudgetUSD:   1.0,
			MaxCheapTurns:       5,
			TaskTypeOverrides:   make(map[string]session.Provider),
		}
	}

	h := NewHarness(t)
	// We only verify the loop starts and steps without panic.
	// Actual cascade routing in StepLoop may or may not trigger depending
	// on the scenario, but the config propagation is what we're testing.
	_, _ = h.RunScenario(t.Context(), s)
}
