package model

import (
	"errors"
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// EstimateCost
// ---------------------------------------------------------------------------

func TestEstimateCost_ClaudeSonnet(t *testing.T) {
	est, err := EstimateCost("claude-sonnet-4-20250514", 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.InputCost != 3.0 {
		t.Errorf("input cost = %f, want 3.0", est.InputCost)
	}
	if est.OutputCost != 15.0 {
		t.Errorf("output cost = %f, want 15.0", est.OutputCost)
	}
	if est.TotalCost != 18.0 {
		t.Errorf("total cost = %f, want 18.0", est.TotalCost)
	}
}

func TestEstimateCost_ClaudeOpus(t *testing.T) {
	est, err := EstimateCost("claude-opus-4-20250514", 500_000, 100_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 0.5M * 15/M = 7.50 input
	// 0.1M * 75/M = 7.50 output
	wantIn := 7.50
	wantOut := 7.50
	if math.Abs(est.InputCost-wantIn) > 0.001 {
		t.Errorf("input cost = %f, want %f", est.InputCost, wantIn)
	}
	if math.Abs(est.OutputCost-wantOut) > 0.001 {
		t.Errorf("output cost = %f, want %f", est.OutputCost, wantOut)
	}
	if math.Abs(est.TotalCost-(wantIn+wantOut)) > 0.001 {
		t.Errorf("total cost = %f, want %f", est.TotalCost, wantIn+wantOut)
	}
}

func TestEstimateCost_ClaudeHaiku(t *testing.T) {
	est, err := EstimateCost("claude-haiku-3-5-20241022", 2_000_000, 500_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2M * 0.80/M = 1.60
	// 0.5M * 4.0/M = 2.00
	if math.Abs(est.InputCost-1.60) > 0.001 {
		t.Errorf("input cost = %f, want 1.60", est.InputCost)
	}
	if math.Abs(est.OutputCost-2.00) > 0.001 {
		t.Errorf("output cost = %f, want 2.00", est.OutputCost)
	}
}

func TestEstimateCost_GeminiFlash(t *testing.T) {
	est, err := EstimateCost("gemini-2.5-flash", 10_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 10M * 0.15/M = 1.50
	// 1M * 0.60/M = 0.60
	if math.Abs(est.InputCost-1.50) > 0.001 {
		t.Errorf("input cost = %f, want 1.50", est.InputCost)
	}
	if math.Abs(est.OutputCost-0.60) > 0.001 {
		t.Errorf("output cost = %f, want 0.60", est.OutputCost)
	}
}

func TestEstimateCost_GeminiPro(t *testing.T) {
	est, err := EstimateCost("gemini-2.5-pro", 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.InputCost != 1.25 {
		t.Errorf("input cost = %f, want 1.25", est.InputCost)
	}
	if est.OutputCost != 10.0 {
		t.Errorf("output cost = %f, want 10.0", est.OutputCost)
	}
}

func TestEstimateCost_GPT4o(t *testing.T) {
	est, err := EstimateCost("gpt-4o", 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.InputCost != 2.50 {
		t.Errorf("input cost = %f, want 2.50", est.InputCost)
	}
	if est.OutputCost != 10.0 {
		t.Errorf("output cost = %f, want 10.0", est.OutputCost)
	}
}

func TestEstimateCost_O3(t *testing.T) {
	est, err := EstimateCost("o3", 1_000_000, 500_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1M * 2.0/M = 2.0
	// 0.5M * 8.0/M = 4.0
	if math.Abs(est.InputCost-2.0) > 0.001 {
		t.Errorf("input cost = %f, want 2.0", est.InputCost)
	}
	if math.Abs(est.OutputCost-4.0) > 0.001 {
		t.Errorf("output cost = %f, want 4.0", est.OutputCost)
	}
}

func TestEstimateCost_O1(t *testing.T) {
	est, err := EstimateCost("o1", 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.InputCost != 15.0 {
		t.Errorf("input cost = %f, want 15.0", est.InputCost)
	}
	if est.OutputCost != 60.0 {
		t.Errorf("output cost = %f, want 60.0", est.OutputCost)
	}
}

func TestEstimateCost_O4Mini(t *testing.T) {
	est, err := EstimateCost("o4-mini", 2_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2M * 1.10/M = 2.20
	// 1M * 4.40/M = 4.40
	if math.Abs(est.InputCost-2.20) > 0.001 {
		t.Errorf("input cost = %f, want 2.20", est.InputCost)
	}
	if math.Abs(est.OutputCost-4.40) > 0.001 {
		t.Errorf("output cost = %f, want 4.40", est.OutputCost)
	}
}

func TestEstimateCost_ZeroTokens(t *testing.T) {
	est, err := EstimateCost("claude-sonnet-4-20250514", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.TotalCost != 0 {
		t.Errorf("total cost = %f, want 0", est.TotalCost)
	}
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	_, err := EstimateCost("nonexistent-model-9000", 1000, 1000)
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestEstimateCost_NegativeTokens(t *testing.T) {
	_, err := EstimateCost("claude-sonnet-4-20250514", -1, 100)
	if err == nil {
		t.Fatal("expected error for negative tokens")
	}
	if !errors.Is(err, ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams, got: %v", err)
	}

	_, err = EstimateCost("claude-sonnet-4-20250514", 100, -1)
	if err == nil {
		t.Fatal("expected error for negative output tokens")
	}
}

func TestEstimateCost_SmallTokenCount(t *testing.T) {
	est, err := EstimateCost("claude-sonnet-4-20250514", 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1 token * 3.0 / 1M = 0.000003
	// 1 token * 15.0 / 1M = 0.000015
	if est.InputCost != 0.000003 {
		t.Errorf("input cost = %f, want 0.000003", est.InputCost)
	}
	if est.OutputCost != 0.000015 {
		t.Errorf("output cost = %f, want 0.000015", est.OutputCost)
	}
}

func TestEstimateCost_GeminiFlashLite(t *testing.T) {
	est, err := EstimateCost("gemini-2.0-flash-lite", 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.InputCost != 0.075 {
		t.Errorf("input cost = %f, want 0.075", est.InputCost)
	}
	if est.OutputCost != 0.30 {
		t.Errorf("output cost = %f, want 0.30", est.OutputCost)
	}
}

func TestEstimateCost_GPT4oMini(t *testing.T) {
	est, err := EstimateCost("gpt-4o-mini", 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if est.InputCost != 0.15 {
		t.Errorf("input cost = %f, want 0.15", est.InputCost)
	}
	if est.OutputCost != 0.60 {
		t.Errorf("output cost = %f, want 0.60", est.OutputCost)
	}
}

// ---------------------------------------------------------------------------
// CheapestModelForProvider
// ---------------------------------------------------------------------------

func TestCheapestModelForProvider_Claude(t *testing.T) {
	m := CheapestModelForProvider(ProviderClaude, "")
	if m == nil {
		t.Fatal("expected a model")
	}
	if m.ID != "claude-haiku-3-5-20241022" {
		t.Errorf("cheapest Claude = %q, want claude-haiku-3-5-20241022", m.ID)
	}
}

func TestCheapestModelForProvider_Claude_Reasoning(t *testing.T) {
	m := CheapestModelForProvider(ProviderClaude, CapReasoning)
	if m == nil {
		t.Fatal("expected a model")
	}
	// Haiku lacks reasoning, so Sonnet should be cheapest with reasoning.
	if m.ID != "claude-sonnet-4-20250514" {
		t.Errorf("cheapest Claude with reasoning = %q, want claude-sonnet-4-20250514", m.ID)
	}
}

func TestCheapestModelForProvider_Gemini(t *testing.T) {
	m := CheapestModelForProvider(ProviderGemini, "")
	if m == nil {
		t.Fatal("expected a model")
	}
	if m.ID != "gemini-2.0-flash-lite" {
		t.Errorf("cheapest Gemini = %q, want gemini-2.0-flash-lite", m.ID)
	}
}

func TestCheapestModelForProvider_Gemini_Reasoning(t *testing.T) {
	m := CheapestModelForProvider(ProviderGemini, CapReasoning)
	if m == nil {
		t.Fatal("expected a model")
	}
	if m.ID != "gemini-2.5-pro" {
		t.Errorf("cheapest Gemini with reasoning = %q, want gemini-2.5-pro", m.ID)
	}
}

func TestCheapestModelForProvider_OpenAI(t *testing.T) {
	m := CheapestModelForProvider(ProviderOpenAI, "")
	if m == nil {
		t.Fatal("expected a model")
	}
	if m.ID != "gpt-4o-mini" {
		t.Errorf("cheapest OpenAI = %q, want gpt-4o-mini", m.ID)
	}
}

func TestCheapestModelForProvider_OpenAI_Reasoning(t *testing.T) {
	m := CheapestModelForProvider(ProviderOpenAI, CapReasoning)
	if m == nil {
		t.Fatal("expected a model")
	}
	// o4-mini ($1.10) < o3 ($2.00) < gpt-4o ($2.50) < o1 ($15.00)
	if m.ID != "o4-mini" {
		t.Errorf("cheapest OpenAI with reasoning = %q, want o4-mini", m.ID)
	}
}

func TestCheapestModelForProvider_UnknownProvider(t *testing.T) {
	m := CheapestModelForProvider("anthropic-v2", "")
	if m != nil {
		t.Errorf("expected nil for unknown provider, got %q", m.ID)
	}
}

func TestCheapestModelForProvider_NoMatch(t *testing.T) {
	// Agents capability only on Claude models.
	m := CheapestModelForProvider(ProviderOpenAI, CapAgents)
	if m != nil {
		t.Errorf("expected nil when no model has agents cap for OpenAI, got %q", m.ID)
	}
}

// ---------------------------------------------------------------------------
// CheapestModelAny
// ---------------------------------------------------------------------------

func TestCheapestModelAny_NoFilter(t *testing.T) {
	m := CheapestModelAny("")
	if m == nil {
		t.Fatal("expected a model")
	}
	// Gemini 2.0 Flash Lite at $0.075/MTok is cheapest overall.
	if m.ID != "gemini-2.0-flash-lite" {
		t.Errorf("cheapest any = %q, want gemini-2.0-flash-lite", m.ID)
	}
}

func TestCheapestModelAny_Reasoning(t *testing.T) {
	m := CheapestModelAny(CapReasoning)
	if m == nil {
		t.Fatal("expected a model")
	}
	// o4-mini at $1.10 is cheapest reasoning model.
	if m.InputPer1MTok > 1.25 {
		t.Errorf("expected cheap reasoning model, got cost %f", m.InputPer1MTok)
	}
}

func TestCheapestModelAny_Agents(t *testing.T) {
	m := CheapestModelAny(CapAgents)
	if m == nil {
		t.Fatal("expected a model")
	}
	// Only Claude models have agents capability; Sonnet is cheapest.
	if m.Provider != ProviderClaude {
		t.Errorf("expected Claude provider for agents, got %s", m.Provider)
	}
}

// ---------------------------------------------------------------------------
// LookupModelCost
// ---------------------------------------------------------------------------

func TestLookupModelCost_Found(t *testing.T) {
	m := LookupModelCost("gpt-4o")
	if m == nil {
		t.Fatal("expected to find gpt-4o")
	}
	if m.DisplayName != "GPT-4o" {
		t.Errorf("display name = %q, want GPT-4o", m.DisplayName)
	}
	if m.Provider != ProviderOpenAI {
		t.Errorf("provider = %q, want openai", m.Provider)
	}
}

func TestLookupModelCost_NotFound(t *testing.T) {
	if LookupModelCost("missing-model") != nil {
		t.Error("expected nil for missing model")
	}
}

// ---------------------------------------------------------------------------
// AllModelCosts
// ---------------------------------------------------------------------------

func TestAllModelCosts(t *testing.T) {
	all := AllModelCosts()
	if len(all) != len(costTable) {
		t.Errorf("len = %d, want %d", len(all), len(costTable))
	}
	// Verify it is a copy.
	all[0].ID = "mutated"
	if costTable[0].ID == "mutated" {
		t.Error("AllModelCosts returned a reference, not a copy")
	}
}

// ---------------------------------------------------------------------------
// ModelCostsByProvider
// ---------------------------------------------------------------------------

func TestModelCostsByProvider(t *testing.T) {
	claude := ModelCostsByProvider(ProviderClaude)
	if len(claude) != 3 {
		t.Errorf("Claude models = %d, want 3", len(claude))
	}
	for _, m := range claude {
		if m.Provider != ProviderClaude {
			t.Errorf("unexpected provider %q in Claude list", m.Provider)
		}
	}

	gemini := ModelCostsByProvider(ProviderGemini)
	if len(gemini) != 3 {
		t.Errorf("Gemini models = %d, want 3", len(gemini))
	}

	openai := ModelCostsByProvider(ProviderOpenAI)
	if len(openai) != 6 {
		t.Errorf("OpenAI models = %d, want 6", len(openai))
	}
}

// ---------------------------------------------------------------------------
// RankedByCost
// ---------------------------------------------------------------------------

func TestRankedByCost_AllProviders(t *testing.T) {
	ranked := RankedByCost("", "")
	if len(ranked) != len(costTable) {
		t.Errorf("len = %d, want %d", len(ranked), len(costTable))
	}
	for i := 1; i < len(ranked); i++ {
		if ranked[i].InputPer1MTok < ranked[i-1].InputPer1MTok {
			t.Errorf("not sorted: [%d] %f > [%d] %f", i-1, ranked[i-1].InputPer1MTok, i, ranked[i].InputPer1MTok)
		}
	}
}

func TestRankedByCost_ReasoningOnly(t *testing.T) {
	ranked := RankedByCost("", CapReasoning)
	for _, m := range ranked {
		if !m.HasCapability(CapReasoning) {
			t.Errorf("model %q lacks reasoning capability", m.ID)
		}
	}
	if len(ranked) == 0 {
		t.Fatal("expected at least one reasoning model")
	}
}

func TestRankedByCost_ProviderFilter(t *testing.T) {
	ranked := RankedByCost(ProviderClaude, "")
	for _, m := range ranked {
		if m.Provider != ProviderClaude {
			t.Errorf("unexpected provider %q", m.Provider)
		}
	}
}

// ---------------------------------------------------------------------------
// HasCapability
// ---------------------------------------------------------------------------

func TestModelCost_HasCapability(t *testing.T) {
	m := LookupModelCost("claude-opus-4-20250514")
	if m == nil {
		t.Fatal("expected to find opus")
	}
	if !m.HasCapability(CapCode) {
		t.Error("opus should have code capability")
	}
	if !m.HasCapability(CapReasoning) {
		t.Error("opus should have reasoning capability")
	}
	if !m.HasCapability(CapVision) {
		t.Error("opus should have vision capability")
	}
	if !m.HasCapability(CapAgents) {
		t.Error("opus should have agents capability")
	}
}

// ---------------------------------------------------------------------------
// BlendedCostPer1MTok
// ---------------------------------------------------------------------------

func TestBlendedCostPer1MTok(t *testing.T) {
	m := LookupModelCost("claude-sonnet-4-20250514")
	if m == nil {
		t.Fatal("expected to find sonnet")
	}
	// 80% input, 20% output: 3.0*0.8 + 15.0*0.2 = 2.4 + 3.0 = 5.4
	blended := m.BlendedCostPer1MTok(0.8)
	if math.Abs(blended-5.4) > 0.001 {
		t.Errorf("blended cost = %f, want 5.4", blended)
	}

	// 100% input
	if m.BlendedCostPer1MTok(1.0) != 3.0 {
		t.Errorf("100%% input cost = %f, want 3.0", m.BlendedCostPer1MTok(1.0))
	}

	// 100% output
	if m.BlendedCostPer1MTok(0.0) != 15.0 {
		t.Errorf("100%% output cost = %f, want 15.0", m.BlendedCostPer1MTok(0.0))
	}
}

func TestBlendedCostPer1MTok_Panic(t *testing.T) {
	m := LookupModelCost("claude-sonnet-4-20250514")
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for ratio > 1")
		}
	}()
	m.BlendedCostPer1MTok(1.5)
}

// ---------------------------------------------------------------------------
// CostIndex integrity
// ---------------------------------------------------------------------------

func TestCostIndex_AllModelsIndexed(t *testing.T) {
	for _, m := range costTable {
		if _, ok := costIndex[m.ID]; !ok {
			t.Errorf("model %q not in cost index", m.ID)
		}
	}
}

func TestCostIndex_NoDuplicateIDs(t *testing.T) {
	seen := make(map[string]bool)
	for _, m := range costTable {
		if seen[m.ID] {
			t.Errorf("duplicate model ID: %q", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestCostTable_AllHavePositiveCosts(t *testing.T) {
	for _, m := range costTable {
		if m.InputPer1MTok <= 0 {
			t.Errorf("model %q has non-positive input cost: %f", m.ID, m.InputPer1MTok)
		}
		if m.OutputPer1MTok <= 0 {
			t.Errorf("model %q has non-positive output cost: %f", m.ID, m.OutputPer1MTok)
		}
	}
}

func TestCostTable_AllHaveProvider(t *testing.T) {
	for _, m := range costTable {
		switch m.Provider {
		case ProviderClaude, ProviderGemini, ProviderOpenAI:
			// ok
		default:
			t.Errorf("model %q has unknown provider %q", m.ID, m.Provider)
		}
	}
}

func TestCostTable_AllHaveCapabilities(t *testing.T) {
	for _, m := range costTable {
		if len(m.Capabilities) == 0 {
			t.Errorf("model %q has no capabilities", m.ID)
		}
	}
}

func TestCostTable_AllHaveContextWindow(t *testing.T) {
	for _, m := range costTable {
		if m.ContextWindow <= 0 {
			t.Errorf("model %q has invalid context window: %d", m.ID, m.ContextWindow)
		}
		if m.MaxOutputTok <= 0 {
			t.Errorf("model %q has invalid max output: %d", m.ID, m.MaxOutputTok)
		}
	}
}

func TestCostTable_OutputMoreExpensiveThanInput(t *testing.T) {
	for _, m := range costTable {
		if m.OutputPer1MTok < m.InputPer1MTok {
			t.Errorf("model %q: output (%f) cheaper than input (%f), unexpected",
				m.ID, m.OutputPer1MTok, m.InputPer1MTok)
		}
	}
}
