package session

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
)

func TestNewBanditRouter(t *testing.T) {
	t.Parallel()

	tiers := DefaultModelTiers()
	br := NewBanditRouter(tiers, DefaultBanditRouterConfig())

	if br == nil {
		t.Fatal("expected non-nil BanditRouter")
	}
	if len(br.arms) != len(tiers) {
		t.Errorf("expected %d arms, got %d", len(tiers), len(br.arms))
	}
	if br.TotalPulls() != 0 {
		t.Errorf("expected 0 pulls, got %d", br.TotalPulls())
	}
	if br.Ready() {
		t.Error("expected bandit not ready with 0 pulls")
	}
}

func TestBanditRouter_SelectProvider_FallbackBeforeMinSamples(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), BanditRouterConfig{
		MinSamples: 10,
	})

	ctx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.9)
	arm := br.SelectProvider(ctx)

	// Should return empty arm (fall back to static) since we have 0 samples.
	if arm.ID != "" {
		t.Errorf("expected empty arm before min samples, got %+v", arm)
	}
}

func TestBanditRouter_SelectProvider_AfterMinSamples(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), BanditRouterConfig{
		MinSamples:        5,
		SuccessWindowSize: 50,
		LearningRate:      0.1,
	})

	// Record enough samples to pass the threshold.
	for range 6 {
		ctx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.9)
		br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.8, ctx)
	}

	ctx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.9)
	arm := br.SelectProvider(ctx)

	// Should now return a non-empty arm.
	if arm.ID == "" {
		t.Error("expected non-empty arm after min samples")
	}
	if arm.Provider == "" {
		t.Error("expected non-empty provider from bandit selection")
	}
}

func TestBanditRouter_CheapForSimpleTasks(t *testing.T) {
	t.Parallel()

	tiers := []ModelTier{
		{Provider: ProviderGemini, Model: "gemini-3.1-flash-lite", MaxComplexity: 1, CostPer1M: 0.10, Label: "ultra-cheap"},
		{Provider: ProviderClaude, Model: "claude-opus", MaxComplexity: 4, CostPer1M: 15.00, Label: "reasoning"},
	}

	br := NewBanditRouter(tiers, BanditRouterConfig{
		MinSamples:        5,
		SuccessWindowSize: 200,
		LearningRate:      0.3,
		Window:            0, // infinite memory for stable learning
	})

	simpleCtx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.9)
	complexCtx := BuildCascadeContext("architecture", 0.8, TimeInteractive, 0.7)

	// Train: cheap succeeds on simple tasks, fails on complex.
	// Expensive succeeds on complex, mediocre on simple.
	for range 100 {
		br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.85, simpleCtx)
		br.RecordOutcome("gemini", "gemini-3.1-flash-lite", false, 0.01, 0.2, complexCtx)
		br.RecordOutcome("claude", "claude-opus", true, 2.00, 0.95, complexCtx)
		br.RecordOutcome("claude", "claude-opus", true, 2.00, 0.5, simpleCtx)
	}

	// For simple tasks, the bandit should favor the cheap provider.
	cheapCount := 0
	for range 100 {
		arm := br.SelectProvider(simpleCtx)
		if arm.Provider == "gemini" {
			cheapCount++
		}
	}

	// Cheap should be selected the majority of the time for simple tasks.
	// The exact threshold depends on exploration, but it should be over 50%.
	if cheapCount < 50 {
		t.Errorf("expected cheap provider selected >50/100 for simple tasks, got %d", cheapCount)
	}
}

func TestBanditRouter_RecordOutcome(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), DefaultBanditRouterConfig())
	ctx := BuildCascadeContext("feature", 0.5, TimeNormal, 0.7)

	br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.05, 0.8, ctx)
	br.RecordOutcome("claude", "claude-sonnet", true, 1.00, 0.9, ctx)

	if br.TotalPulls() != 2 {
		t.Errorf("expected 2 pulls, got %d", br.TotalPulls())
	}

	stats := br.Stats()
	if len(stats) == 0 {
		t.Error("expected non-empty stats")
	}
}

func TestBanditRouter_SuccessRateTracking(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), BanditRouterConfig{
		MinSamples:        5,
		SuccessWindowSize: 10,
	})

	ctx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.8)

	// Record 8 successes and 2 failures for gemini.
	for range 8 {
		br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.8, ctx)
	}
	for range 2 {
		br.RecordOutcome("gemini", "gemini-3.1-flash-lite", false, 0.01, 0.3, ctx)
	}

	rate := br.ProviderSuccessRate("gemini")
	if rate < 0.7 || rate > 0.9 {
		t.Errorf("expected success rate ~0.8, got %f", rate)
	}

	// Unknown provider returns 0.5 (neutral).
	unknown := br.ProviderSuccessRate("unknown")
	if unknown != 0.5 {
		t.Errorf("expected 0.5 for unknown provider, got %f", unknown)
	}
}

func TestBanditRouter_Ready(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), BanditRouterConfig{
		MinSamples: 3,
	})

	if br.Ready() {
		t.Error("should not be ready with 0 pulls")
	}

	ctx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.8)
	br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.8, ctx)
	br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.8, ctx)
	if br.Ready() {
		t.Error("should not be ready with 2 pulls (min=3)")
	}

	br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.8, ctx)
	if !br.Ready() {
		t.Error("should be ready with 3 pulls (min=3)")
	}
}

func TestClassifyComplexity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		taskType string
		want     TaskComplexity
	}{
		{"lint", TaskSimple},
		{"format", TaskSimple},
		{"docs", TaskSimple},
		{"config", TaskMedium},
		{"review", TaskMedium},
		{"bug_fix", TaskMedium},
		{"general", TaskMedium},
		{"architecture", TaskComplex},
		{"analysis", TaskComplex},
		{"planning", TaskComplex},
		{"codegen", TaskComplex},
		{"unknown", TaskSimple}, // complexity 0 maps to Simple
	}

	for _, tt := range tests {
		got := ClassifyComplexity(tt.taskType)
		if got != tt.want {
			t.Errorf("ClassifyComplexity(%q) = %d, want %d", tt.taskType, got, tt.want)
		}
	}
}

func TestClassifyBudgetPressure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		remaining float64
		want      BudgetPressure
	}{
		{0.9, BudgetHigh},
		{0.7, BudgetHigh},
		{0.61, BudgetHigh},
		{0.5, BudgetMedium},
		{0.3, BudgetMedium},
		{0.2, BudgetLow},
		{0.1, BudgetLow},
		{0.0, BudgetLow},
	}

	for _, tt := range tests {
		got := ClassifyBudgetPressure(tt.remaining)
		if got != tt.want {
			t.Errorf("ClassifyBudgetPressure(%f) = %d, want %d", tt.remaining, got, tt.want)
		}
	}
}

func TestBuildCascadeContext(t *testing.T) {
	t.Parallel()

	ctx := BuildCascadeContext("architecture", 0.1, TimeInteractive, 0.95)

	if ctx.TaskType != "architecture" {
		t.Errorf("expected task type 'architecture', got %q", ctx.TaskType)
	}
	if ctx.Complexity != TaskComplex {
		t.Errorf("expected TaskComplex, got %d", ctx.Complexity)
	}
	if ctx.BudgetPressure != BudgetLow {
		t.Errorf("expected BudgetLow, got %d", ctx.BudgetPressure)
	}
	if ctx.TimeSensitivity != TimeInteractive {
		t.Errorf("expected TimeInteractive, got %d", ctx.TimeSensitivity)
	}
	if ctx.RecentSuccess != 0.95 {
		t.Errorf("expected RecentSuccess 0.95, got %f", ctx.RecentSuccess)
	}
}

func TestBanditRouter_ComputeReward(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), DefaultBanditRouterConfig())

	// Failed task gets floor reward.
	r := br.computeReward(false, 1.0, 0.9)
	if r != 0.1 {
		t.Errorf("expected 0.1 for failed task, got %f", r)
	}

	// Successful cheap task gets high reward.
	r = br.computeReward(true, 0.01, 0.8)
	// 0.6*0.8 + 0.4*(1/(1+0.01)) = 0.48 + 0.396 = 0.876...
	if r < 0.85 || r > 0.90 {
		t.Errorf("expected reward ~0.876 for cheap+quality task, got %f", r)
	}

	// Successful expensive task gets lower reward due to cost.
	r = br.computeReward(true, 5.0, 0.9)
	// 0.6*0.9 + 0.4*(1/(1+5)) = 0.54 + 0.067 = 0.607
	if r < 0.55 || r > 0.65 {
		t.Errorf("expected reward ~0.607 for expensive task, got %f", r)
	}
}

func TestWireBanditRouter(t *testing.T) {
	t.Parallel()

	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	br := NewBanditRouter(DefaultModelTiers(), BanditRouterConfig{
		MinSamples:        3,
		SuccessWindowSize: 50,
		LearningRate:      0.1,
	})

	// Wire the bandit router.
	WireBanditRouter(cr, br)

	if !cr.BanditConfigured() {
		t.Error("expected bandit to be configured after WireBanditRouter")
	}

	// Before min samples, static routing should still work.
	tier := cr.SelectTier("lint", 1)
	if tier.Provider == "" {
		t.Error("expected valid tier from static selection")
	}
}

func TestWireBanditRouter_UsesContextAfterTraining(t *testing.T) {
	t.Parallel()

	config := DefaultCascadeConfig()
	cr := NewCascadeRouter(config, nil, nil, "")

	br := NewBanditRouter(DefaultModelTiers(), BanditRouterConfig{
		MinSamples:        5,
		SuccessWindowSize: 50,
		LearningRate:      0.1,
	})

	WireBanditRouter(cr, br)

	// Seed enough data for the bandit to be ready.
	ctx := BuildCascadeContext("lint", 0.8, TimeBatch, 0.9)
	for range 20 {
		br.RecordOutcome("gemini", "gemini-3.1-flash-lite", true, 0.01, 0.8, ctx)
	}

	// Also need enough cascade results for SelectTier to consult bandit.
	for range 15 {
		cr.RecordResult(CascadeResult{
			UsedProvider: ProviderGemini,
			Escalated:    false,
		})
	}

	// SelectTier should now consult the bandit.
	tier := cr.SelectTier("lint", 1)
	if tier.Provider == "" {
		t.Error("expected valid provider after bandit training")
	}
}

func TestBanditRouter_FindArmID(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), DefaultBanditRouterConfig())

	// Exact match.
	id := br.findArmID("gemini", "gemini-3.1-flash-lite")
	if id != "ultra-cheap" {
		t.Errorf("expected 'ultra-cheap', got %q", id)
	}

	// Provider-only match.
	id = br.findArmID("claude", "")
	if id == "" {
		t.Error("expected non-empty arm ID for provider-only match")
	}

	// No match.
	id = br.findArmID("unknown", "unknown-model")
	if id != "" {
		t.Errorf("expected empty arm ID for unknown provider, got %q", id)
	}
}

func TestBanditRouter_ContextToFeatures(t *testing.T) {
	t.Parallel()

	br := NewBanditRouter(DefaultModelTiers(), DefaultBanditRouterConfig())

	ctx := CascadeContext{
		Complexity:      TaskComplex,
		BudgetRemaining: 0.3,
		TimeSensitivity: TimeInteractive,
		RecentSuccess:   0.75,
	}

	features := br.contextToFeatures(ctx)
	if len(features) != int(bandit.NumContextualFeatures) {
		t.Fatalf("expected %d features, got %d", bandit.NumContextualFeatures, len(features))
	}

	if features[bandit.FeatureComplexity] != 1.0 {
		t.Errorf("expected complexity=1.0, got %f", features[bandit.FeatureComplexity])
	}
	// Budget 0.3 -> centered: 2*0.3 - 1 = -0.4
	expectedBudget := 2.0*0.3 - 1.0
	if features[bandit.FeatureBudgetPressure] != expectedBudget {
		t.Errorf("expected budget=%f, got %f", expectedBudget, features[bandit.FeatureBudgetPressure])
	}
	if features[bandit.FeatureTimeSensitivity] != 1.0 {
		t.Errorf("expected time=1.0, got %f", features[bandit.FeatureTimeSensitivity])
	}
	// Success 0.75 -> centered: 2*0.75 - 1 = 0.5
	expectedSuccess := 2.0*0.75 - 1.0
	if features[bandit.FeatureRecentSuccess] != expectedSuccess {
		t.Errorf("expected success=%f, got %f", expectedSuccess, features[bandit.FeatureRecentSuccess])
	}
}
