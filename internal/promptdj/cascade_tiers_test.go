package promptdj

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestCascadeTierLevel_String(t *testing.T) {
	tests := []struct {
		tier CascadeTierLevel
		want string
	}{
		{Tier1Fast, "fast"},
		{Tier2Balanced, "balanced"},
		{Tier3Powerful, "powerful"},
		{CascadeTierLevel(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("CascadeTierLevel(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestDefaultCascadeTierConfig(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Verify all 3 tiers are populated.
	for i, tier := range cfg.Tiers {
		if len(tier.Models) == 0 {
			t.Errorf("Tier %d has no models", i+1)
		}
		if tier.Label == "" {
			t.Errorf("Tier %d has no label", i+1)
		}
	}

	// Tier 3 should not escalate.
	if cfg.Tiers[2].EscalationThreshold != 0 {
		t.Errorf("Tier 3 escalation threshold = %v, want 0", cfg.Tiers[2].EscalationThreshold)
	}

	// Should have task-type overrides for architecture and planning.
	if tier, ok := cfg.TaskTypeTierOverrides["architecture"]; !ok || tier != Tier3Powerful {
		t.Error("expected architecture override to Tier3Powerful")
	}
}

func TestClassifyTier_TaskTypeMapping(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	tests := []struct {
		taskType enhancer.TaskType
		want     CascadeTierLevel
	}{
		{enhancer.TaskTypeWorkflow, Tier1Fast},
		{enhancer.TaskTypeCode, Tier2Balanced},
		{enhancer.TaskTypeTroubleshooting, Tier2Balanced},
		{enhancer.TaskTypeCreative, Tier2Balanced},
		{enhancer.TaskTypeAnalysis, Tier3Powerful},
	}
	for _, tt := range tests {
		got := ClassifyTier(tt.taskType, 0, 80, "", cfg)
		if got != tt.want {
			t.Errorf("ClassifyTier(%s) = %v, want %v", tt.taskType, got, tt.want)
		}
	}
}

func TestClassifyTier_TaskTypeOverride(t *testing.T) {
	cfg := DefaultCascadeTierConfig()
	// Architecture is overridden to Tier 3 regardless of other signals.
	got := ClassifyTier(enhancer.TaskTypeGeneral, 1, 90, "", cfg)
	// General with complexity 1 should be Tier 1 (from complexity map).
	if got != Tier1Fast {
		t.Errorf("ClassifyTier(general, complexity=1) = %v, want Tier1Fast", got)
	}
}

func TestClassifyTier_LowQualityEscalation(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Workflow (normally Tier 1) with very low quality score should escalate.
	got := ClassifyTier(enhancer.TaskTypeWorkflow, 0, 30, "", cfg)
	if got != Tier2Balanced {
		t.Errorf("ClassifyTier(workflow, score=30) = %v, want Tier2Balanced", got)
	}

	// Code (normally Tier 2) with very low quality score should escalate.
	got = ClassifyTier(enhancer.TaskTypeCode, 0, 20, "", cfg)
	if got != Tier3Powerful {
		t.Errorf("ClassifyTier(code, score=20) = %v, want Tier3Powerful", got)
	}
}

func TestClassifyTier_HeuristicFallback(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Short prompt -> Tier 1.
	got := ClassifyTier(enhancer.TaskTypeGeneral, 0, 0, "fix this", cfg)
	if got != Tier1Fast {
		t.Errorf("ClassifyTier(short prompt) = %v, want Tier1Fast", got)
	}

	// Complex architecture signal -> Tier 3.
	got = ClassifyTier(enhancer.TaskTypeGeneral, 0, 0,
		"Design a distributed system architecture for handling 10M concurrent users with consensus", cfg)
	if got != Tier3Powerful {
		t.Errorf("ClassifyTier(architecture signal) = %v, want Tier3Powerful", got)
	}

	// Simple formatting signal -> Tier 1.
	got = ClassifyTier(enhancer.TaskTypeGeneral, 0, 0,
		"Format this JSON file and validate the syntax", cfg)
	if got != Tier1Fast {
		t.Errorf("ClassifyTier(format signal) = %v, want Tier1Fast", got)
	}
}

func TestSelectModelForTier_PreferredProvider(t *testing.T) {
	cfg := DefaultCascadeTierConfig()
	tier := cfg.Tiers[1] // Tier 2: Sonnet, GPT-5.4, Flash

	// Prefer Claude -> should get Sonnet.
	model := SelectModelForTier(tier, session.ProviderClaude, nil)
	if model.Provider != session.ProviderClaude {
		t.Errorf("SelectModelForTier(prefer Claude) = %s, want claude", model.Provider)
	}

	// Prefer Codex -> should get GPT-5.4.
	model = SelectModelForTier(tier, session.ProviderCodex, nil)
	if model.Provider != session.ProviderCodex {
		t.Errorf("SelectModelForTier(prefer Codex) = %s, want codex", model.Provider)
	}
}

func TestSelectModelForTier_DomainBased(t *testing.T) {
	cfg := DefaultCascadeTierConfig()
	tier := cfg.Tiers[1] // Tier 2

	// Go domain -> Claude.
	model := SelectModelForTier(tier, "", []string{"go"})
	if model.Provider != session.ProviderClaude {
		t.Errorf("SelectModelForTier(go domain) = %s, want claude", model.Provider)
	}

	// Shader domain -> Gemini.
	model = SelectModelForTier(tier, "", []string{"shader"})
	if model.Provider != session.ProviderGemini {
		t.Errorf("SelectModelForTier(shader domain) = %s, want gemini", model.Provider)
	}
}

func TestSelectModelForTier_DefaultFirst(t *testing.T) {
	cfg := DefaultCascadeTierConfig()
	tier := cfg.Tiers[0] // Tier 1

	// No preference -> first model.
	model := SelectModelForTier(tier, "", nil)
	if model.Model == "" {
		t.Error("SelectModelForTier with no preference should return first model")
	}
}

func TestSelectModelForTier_EmptyTier(t *testing.T) {
	empty := CascadeTierDef{Level: Tier1Fast, Models: nil}
	model := SelectModelForTier(empty, "", nil)
	if model.Model != "" {
		t.Errorf("SelectModelForTier(empty) should return zero model, got %s", model.Model)
	}
}

func TestEscalateTier_BelowThreshold(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Tier 1 with low confidence -> should escalate to Tier 2.
	shouldEsc, next := EscalateTier(Tier1Fast, 0.40, cfg, 0)
	if !shouldEsc {
		t.Error("expected escalation from Tier 1 with confidence 0.40")
	}
	if next != Tier2Balanced {
		t.Errorf("next tier = %v, want Tier2Balanced", next)
	}
}

func TestEscalateTier_AboveThreshold(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Tier 1 with high confidence -> no escalation.
	shouldEsc, next := EscalateTier(Tier1Fast, 0.80, cfg, 0)
	if shouldEsc {
		t.Error("should not escalate from Tier 1 with confidence 0.80")
	}
	if next != Tier1Fast {
		t.Errorf("next tier = %v, want Tier1Fast", next)
	}
}

func TestEscalateTier_AtMaxTier(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Tier 3 never escalates regardless of confidence.
	shouldEsc, next := EscalateTier(Tier3Powerful, 0.10, cfg, 0)
	if shouldEsc {
		t.Error("Tier 3 should never escalate")
	}
	if next != Tier3Powerful {
		t.Errorf("next tier = %v, want Tier3Powerful", next)
	}
}

func TestEscalateTier_BudgetExhausted(t *testing.T) {
	cfg := DefaultCascadeTierConfig()
	cfg.MaxEscalations = 0 // no escalations allowed

	shouldEsc, _ := EscalateTier(Tier1Fast, 0.20, cfg, 0)
	if shouldEsc {
		t.Error("should not escalate when MaxEscalations=0")
	}
}

func TestRouteCascadeTier_NoEscalation(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	result := RouteCascadeTier(
		enhancer.TaskTypeWorkflow, 1, 80, "format this config file",
		0.85, "", nil, cfg,
	)

	if result.InitialTier != Tier1Fast {
		t.Errorf("InitialTier = %v, want Tier1Fast", result.InitialTier)
	}
	if result.Escalated {
		t.Error("should not have escalated")
	}
	if result.SelectedModel.Model == "" {
		t.Error("expected a selected model")
	}
	if result.EstimatedSavings <= 0 {
		t.Errorf("expected positive savings vs Tier 3, got %f", result.EstimatedSavings)
	}
}

func TestRouteCascadeTier_WithEscalation(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	result := RouteCascadeTier(
		enhancer.TaskTypeWorkflow, 1, 80, "format this config file",
		0.30, // below Tier 1 threshold of 0.60
		"", nil, cfg,
	)

	if result.InitialTier != Tier1Fast {
		t.Errorf("InitialTier = %v, want Tier1Fast", result.InitialTier)
	}
	if !result.Escalated {
		t.Error("should have escalated")
	}
	if result.FinalTier < Tier2Balanced {
		t.Errorf("FinalTier = %v, want >= Tier2Balanced", result.FinalTier)
	}
	if result.EscalationCount == 0 {
		t.Error("expected escalation count > 0")
	}
}

func TestRouteCascadeTier_DoubleEscalation(t *testing.T) {
	cfg := DefaultCascadeTierConfig()
	cfg.MaxEscalations = 2 // allow double escalation

	result := RouteCascadeTier(
		enhancer.TaskTypeWorkflow, 1, 80, "format this config file",
		0.20, // very low confidence: below Tier 1 (0.60) and Tier 2 (0.50) thresholds
		"", nil, cfg,
	)

	if result.FinalTier != Tier3Powerful {
		t.Errorf("FinalTier = %v, want Tier3Powerful (double escalation)", result.FinalTier)
	}
	if result.EscalationCount != 2 {
		t.Errorf("EscalationCount = %d, want 2", result.EscalationCount)
	}
}

func TestRouteCascadeTier_PreferredProvider(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	result := RouteCascadeTier(
		enhancer.TaskTypeCode, 2, 75, "Write a Go function",
		0.80, session.ProviderClaude, []string{"go"}, cfg,
	)

	if result.SelectedModel.Provider != session.ProviderClaude {
		t.Errorf("expected Claude for Go code, got %s", result.SelectedModel.Provider)
	}
}

func TestProjectCascadeSavings(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Typical distribution: 40% Tier 1, 45% Tier 2, 15% Tier 3.
	dist := [3]float64{0.40, 0.45, 0.15}
	proj := ProjectCascadeSavings(cfg, dist, 5000) // 5K tokens avg

	if proj.SavingsPercent <= 0 {
		t.Errorf("expected positive savings, got %f%%", proj.SavingsPercent)
	}
	if proj.CascadeCostPer1K >= proj.NaiveCostPer1K {
		t.Errorf("cascade cost ($%.4f) should be < naive cost ($%.4f)",
			proj.CascadeCostPer1K, proj.NaiveCostPer1K)
	}
	if proj.TierDistribution != dist {
		t.Error("tier distribution should be preserved")
	}
}

func TestProjectCascadeSavings_AllTier3(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	// Everything at Tier 3 — savings come from using the cheapest model in Tier 3
	// (o3 at $10/M) vs. the naive baseline (first model: Opus at $15/M).
	dist := [3]float64{0.0, 0.0, 1.0}
	proj := ProjectCascadeSavings(cfg, dist, 5000)

	// Cascade uses cheapest in tier (o3 $10), naive uses first (Opus $15),
	// so there are still savings from model selection within the tier.
	if proj.CascadeCostPer1K > proj.NaiveCostPer1K {
		t.Errorf("cascade cost ($%.4f) should be <= naive cost ($%.4f)",
			proj.CascadeCostPer1K, proj.NaiveCostPer1K)
	}
}

func TestRouteCascadeTier_AnalysisGoesToTier3(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	result := RouteCascadeTier(
		enhancer.TaskTypeAnalysis, 4, 85,
		"Analyze the performance characteristics of this distributed system",
		0.70, "", nil, cfg,
	)

	if result.InitialTier != Tier3Powerful {
		t.Errorf("Analysis should start at Tier3Powerful, got %v", result.InitialTier)
	}
	if result.Escalated {
		t.Error("already at Tier 3, should not escalate")
	}
}

func TestClassifyTier_ComplexityMapping(t *testing.T) {
	cfg := DefaultCascadeTierConfig()

	tests := []struct {
		complexity int
		want       CascadeTierLevel
	}{
		{1, Tier1Fast},
		{2, Tier2Balanced},
		{3, Tier2Balanced},
		{4, Tier3Powerful},
	}
	for _, tt := range tests {
		got := ClassifyTier(enhancer.TaskTypeGeneral, tt.complexity, 80, "", cfg)
		if got != tt.want {
			t.Errorf("ClassifyTier(general, complexity=%d) = %v, want %v",
				tt.complexity, got, tt.want)
		}
	}
}
