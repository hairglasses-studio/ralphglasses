package model

import (
	"fmt"
	"math"
	"slices"
	"sort"
)

// Provider identifies which LLM provider family a model belongs to.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderGemini Provider = "gemini"
	ProviderOpenAI Provider = "openai"
)

// Capability describes a model's functional abilities.
type Capability string

const (
	CapCode      Capability = "code"
	CapReasoning Capability = "reasoning"
	CapVision    Capability = "vision"
	CapAgents    Capability = "agents"
)

// ModelCost holds per-token pricing and metadata for a single LLM model.
type ModelCost struct {
	// ID is the canonical API model identifier (e.g. "claude-sonnet-4-20250514").
	ID string

	// Provider is the vendor family (claude, gemini, openai).
	Provider Provider

	// DisplayName is a human-friendly label.
	DisplayName string

	// InputPer1MTok is the cost in USD per 1 million input tokens.
	InputPer1MTok float64

	// OutputPer1MTok is the cost in USD per 1 million output tokens.
	OutputPer1MTok float64

	// ContextWindow is the maximum input context length in tokens.
	ContextWindow int

	// MaxOutputTok is the maximum output length in tokens.
	MaxOutputTok int

	// Capabilities lists the functional abilities of this model.
	Capabilities []Capability
}

// HasCapability reports whether the model supports the given capability.
func (m *ModelCost) HasCapability(cap Capability) bool {
	return slices.Contains(m.Capabilities, cap)
}

// BlendedCostPer1MTok returns the weighted average cost per 1M tokens assuming
// the given input/output ratio. A ratio of 0.8 means 80% input, 20% output.
// Panics if ratio is outside [0, 1].
func (m *ModelCost) BlendedCostPer1MTok(inputRatio float64) float64 {
	if inputRatio < 0 || inputRatio > 1 {
		panic("inputRatio must be in [0, 1]")
	}
	return m.InputPer1MTok*inputRatio + m.OutputPer1MTok*(1-inputRatio)
}

// costTable is the canonical registry of model costs. Pricing reflects publicly
// available rates as of early 2026. Update this table when providers publish
// new pricing.
//
// Sources:
//   - Claude: https://docs.anthropic.com/en/docs/about-claude/models
//   - Gemini: https://ai.google.dev/pricing
//   - OpenAI: https://openai.com/api/pricing
var costTable = []ModelCost{
	// ── Claude (Anthropic) ──────────────────────────────────────────────
	{
		ID: "claude-opus-4-20250514", Provider: ProviderClaude,
		DisplayName:   "Claude Opus 4",
		InputPer1MTok: 15.00, OutputPer1MTok: 75.00,
		ContextWindow: 200_000, MaxOutputTok: 32_000,
		Capabilities: []Capability{CapCode, CapReasoning, CapVision, CapAgents},
	},
	{
		ID: "claude-sonnet-4-20250514", Provider: ProviderClaude,
		DisplayName:   "Claude Sonnet 4",
		InputPer1MTok: 3.00, OutputPer1MTok: 15.00,
		ContextWindow: 200_000, MaxOutputTok: 16_000,
		Capabilities: []Capability{CapCode, CapReasoning, CapVision, CapAgents},
	},
	{
		ID: "claude-haiku-3-5-20241022", Provider: ProviderClaude,
		DisplayName:   "Claude Haiku 3.5",
		InputPer1MTok: 0.80, OutputPer1MTok: 4.00,
		ContextWindow: 200_000, MaxOutputTok: 8_192,
		Capabilities: []Capability{CapCode, CapVision},
	},

	// ── Gemini (Google) ─────────────────────────────────────────────────
	{
		ID: "gemini-2.5-pro", Provider: ProviderGemini,
		DisplayName:   "Gemini 2.5 Pro",
		InputPer1MTok: 1.25, OutputPer1MTok: 10.00,
		ContextWindow: 1_000_000, MaxOutputTok: 65_536,
		Capabilities: []Capability{CapCode, CapReasoning, CapVision},
	},
	{
		ID: "gemini-2.5-flash", Provider: ProviderGemini,
		DisplayName:   "Gemini 2.5 Flash",
		InputPer1MTok: 0.15, OutputPer1MTok: 0.60,
		ContextWindow: 1_000_000, MaxOutputTok: 65_536,
		Capabilities: []Capability{CapCode, CapVision},
	},
	{
		ID: "gemini-2.0-flash-lite", Provider: ProviderGemini,
		DisplayName:   "Gemini 2.0 Flash Lite",
		InputPer1MTok: 0.075, OutputPer1MTok: 0.30,
		ContextWindow: 1_000_000, MaxOutputTok: 8_192,
		Capabilities: []Capability{CapCode},
	},

	// ── OpenAI ──────────────────────────────────────────────────────────
	{
		ID: "gpt-4o", Provider: ProviderOpenAI,
		DisplayName:   "GPT-4o",
		InputPer1MTok: 2.50, OutputPer1MTok: 10.00,
		ContextWindow: 128_000, MaxOutputTok: 16_384,
		Capabilities: []Capability{CapCode, CapReasoning, CapVision},
	},
	{
		ID: "gpt-4o-mini", Provider: ProviderOpenAI,
		DisplayName:   "GPT-4o Mini",
		InputPer1MTok: 0.15, OutputPer1MTok: 0.60,
		ContextWindow: 128_000, MaxOutputTok: 16_384,
		Capabilities: []Capability{CapCode, CapVision},
	},
	{
		ID: "o3", Provider: ProviderOpenAI,
		DisplayName:   "OpenAI o3",
		InputPer1MTok: 2.00, OutputPer1MTok: 8.00,
		ContextWindow: 200_000, MaxOutputTok: 100_000,
		Capabilities: []Capability{CapCode, CapReasoning},
	},
	{
		ID: "o4-mini", Provider: ProviderOpenAI,
		DisplayName:   "OpenAI o4-mini",
		InputPer1MTok: 1.10, OutputPer1MTok: 4.40,
		ContextWindow: 200_000, MaxOutputTok: 100_000,
		Capabilities: []Capability{CapCode, CapReasoning},
	},
	{
		ID: "o1", Provider: ProviderOpenAI,
		DisplayName:   "OpenAI o1",
		InputPer1MTok: 15.00, OutputPer1MTok: 60.00,
		ContextWindow: 200_000, MaxOutputTok: 100_000,
		Capabilities: []Capability{CapCode, CapReasoning},
	},
}

// costIndex is built once at init for O(1) lookups by model ID.
var costIndex map[string]*ModelCost

func init() {
	costIndex = make(map[string]*ModelCost, len(costTable))
	for i := range costTable {
		costIndex[costTable[i].ID] = &costTable[i]
	}
}

// CostEstimate is the result of EstimateCost.
type CostEstimate struct {
	Model        string  `json:"model"`
	InputCost    float64 `json:"input_cost"`
	OutputCost   float64 `json:"output_cost"`
	TotalCost    float64 `json:"total_cost"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
}

// EstimateCost computes the USD cost for the given model and token counts.
// Returns an error if the model ID is not in the cost table.
func EstimateCost(modelID string, inputTokens, outputTokens int) (*CostEstimate, error) {
	m, ok := costIndex[modelID]
	if !ok {
		return nil, fmt.Errorf("unknown model %q: %w", modelID, ErrNotFound)
	}
	if inputTokens < 0 || outputTokens < 0 {
		return nil, fmt.Errorf("token counts must be non-negative: %w", ErrInvalidParams)
	}

	inCost := float64(inputTokens) / 1_000_000 * m.InputPer1MTok
	outCost := float64(outputTokens) / 1_000_000 * m.OutputPer1MTok

	return &CostEstimate{
		Model:        modelID,
		InputCost:    roundUSD(inCost),
		OutputCost:   roundUSD(outCost),
		TotalCost:    roundUSD(inCost + outCost),
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

// LookupModelCost returns the cost entry for a model ID, or nil if not found.
func LookupModelCost(modelID string) *ModelCost {
	return costIndex[modelID]
}

// AllModelCosts returns a copy of the full cost table.
func AllModelCosts() []ModelCost {
	result := make([]ModelCost, len(costTable))
	copy(result, costTable)
	return result
}

// ModelCostsByProvider returns all models for the given provider.
func ModelCostsByProvider(p Provider) []ModelCost {
	var result []ModelCost
	for _, m := range costTable {
		if m.Provider == p {
			result = append(result, m)
		}
	}
	return result
}

// CheapestModelForProvider returns the model with the lowest input cost per 1M
// tokens for the given provider, optionally filtered by a minimum capability.
// If minCapability is empty, no capability filter is applied.
// Returns nil if no model matches.
func CheapestModelForProvider(provider Provider, minCapability Capability) *ModelCost {
	var best *ModelCost
	for i := range costTable {
		m := &costTable[i]
		if m.Provider != provider {
			continue
		}
		if minCapability != "" && !m.HasCapability(minCapability) {
			continue
		}
		if best == nil || m.InputPer1MTok < best.InputPer1MTok {
			best = m
		}
	}
	return best
}

// CheapestModelAny returns the cheapest model across all providers that has
// the given capability. If minCapability is empty, returns the overall cheapest.
// Returns nil if no model matches.
func CheapestModelAny(minCapability Capability) *ModelCost {
	var best *ModelCost
	for i := range costTable {
		m := &costTable[i]
		if minCapability != "" && !m.HasCapability(minCapability) {
			continue
		}
		if best == nil || m.InputPer1MTok < best.InputPer1MTok {
			best = m
		}
	}
	return best
}

// RankedByCost returns all models matching the provider and capability filters,
// sorted by input cost ascending (cheapest first). Pass empty provider to
// include all providers; pass empty capability to skip capability filtering.
func RankedByCost(provider Provider, minCapability Capability) []ModelCost {
	var result []ModelCost
	for _, m := range costTable {
		if provider != "" && m.Provider != provider {
			continue
		}
		if minCapability != "" && !m.HasCapability(minCapability) {
			continue
		}
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].InputPer1MTok < result[j].InputPer1MTok
	})
	return result
}

// roundUSD rounds to 6 decimal places to avoid floating-point dust.
func roundUSD(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}
