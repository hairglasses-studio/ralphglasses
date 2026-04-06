package promptdj

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// CascadeTierLevel identifies a tier in the 3-tier model cascade.
type CascadeTierLevel int

const (
	// Tier1Fast is for simple classification, routing, formatting, and lint.
	// Models: Haiku, Flash Lite — cheapest, lowest latency.
	Tier1Fast CascadeTierLevel = 1

	// Tier2Balanced is for analysis, code review, moderate reasoning.
	// Models: Sonnet, GPT-4o/5.4, Flash — good cost/quality tradeoff.
	Tier2Balanced CascadeTierLevel = 2

	// Tier3Powerful is for complex reasoning, architecture, planning.
	// Models: Opus, o3 — highest capability, highest cost.
	Tier3Powerful CascadeTierLevel = 3
)

// String returns the human-readable tier name.
func (t CascadeTierLevel) String() string {
	switch t {
	case Tier1Fast:
		return "fast"
	case Tier2Balanced:
		return "balanced"
	case Tier3Powerful:
		return "powerful"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// CascadeTierModel defines a model within a cascade tier.
type CascadeTierModel struct {
	Provider    session.Provider `json:"provider"`
	Model       string           `json:"model"`
	CostPer1M   float64          `json:"cost_per_1m"`   // input cost per 1M tokens (USD)
	MaxTokens   int              `json:"max_tokens"`     // typical max output tokens
	LatencyClass string          `json:"latency_class"` // "fast", "medium", "slow"
}

// CascadeTierDef defines a complete tier with its models and routing policy.
type CascadeTierDef struct {
	Level       CascadeTierLevel   `json:"level"`
	Label       string             `json:"label"`
	Description string             `json:"description"`
	Models      []CascadeTierModel `json:"models"`
	// EscalationThreshold is the confidence score below which this tier
	// escalates to the next tier. 0.0 means never escalate (Tier 3).
	EscalationThreshold float64 `json:"escalation_threshold"`
}

// CascadeTierConfig holds the full 3-tier cascade configuration.
type CascadeTierConfig struct {
	Tiers [3]CascadeTierDef `json:"tiers"`

	// TaskTypeTierOverrides maps specific task types to a forced tier level.
	// Overrides the classifier when set.
	TaskTypeTierOverrides map[string]CascadeTierLevel `json:"task_type_tier_overrides,omitempty"`

	// MaxEscalations is the maximum number of tier escalations allowed per
	// routing decision. Default 1 (Tier 1 -> Tier 2 or Tier 2 -> Tier 3).
	MaxEscalations int `json:"max_escalations"`
}

// DefaultCascadeTierConfig returns the standard 3-tier cascade layout.
func DefaultCascadeTierConfig() CascadeTierConfig {
	return CascadeTierConfig{
		Tiers: [3]CascadeTierDef{
			{
				Level:       Tier1Fast,
				Label:       "fast",
				Description: "Simple classification, routing, formatting, lint, config",
				Models: []CascadeTierModel{
					{Provider: session.ProviderClaude, Model: "claude-haiku", CostPer1M: 0.80, MaxTokens: 4096, LatencyClass: "fast"},
					{Provider: session.ProviderGemini, Model: "gemini-2.0-flash-lite", CostPer1M: 0.075, MaxTokens: 8192, LatencyClass: "fast"},
				},
				EscalationThreshold: 0.60,
			},
			{
				Level:       Tier2Balanced,
				Label:       "balanced",
				Description: "Analysis, code review, moderate code generation, bug fixes",
				Models: []CascadeTierModel{
					{Provider: session.ProviderClaude, Model: "claude-sonnet", CostPer1M: 3.0, MaxTokens: 8192, LatencyClass: "medium"},
					{Provider: session.ProviderCodex, Model: "gpt-5.4", CostPer1M: 2.5, MaxTokens: 16384, LatencyClass: "medium"},
					{Provider: session.ProviderGemini, Model: "gemini-2.5-flash", CostPer1M: 0.15, MaxTokens: 65536, LatencyClass: "medium"},
				},
				EscalationThreshold: 0.50,
			},
			{
				Level:       Tier3Powerful,
				Label:       "powerful",
				Description: "Complex reasoning, architecture, planning, ambiguous prompts",
				Models: []CascadeTierModel{
					{Provider: session.ProviderClaude, Model: "claude-opus", CostPer1M: 15.0, MaxTokens: 32768, LatencyClass: "slow"},
					{Provider: session.ProviderCodex, Model: "o3", CostPer1M: 10.0, MaxTokens: 100000, LatencyClass: "slow"},
				},
				EscalationThreshold: 0.0, // no escalation from Tier 3
			},
		},
		TaskTypeTierOverrides: map[string]CascadeTierLevel{
			"architecture": Tier3Powerful,
			"planning":     Tier3Powerful,
		},
		MaxEscalations: 1,
	}
}

// CascadeTierResult is the outcome of a multi-tier cascade routing decision.
type CascadeTierResult struct {
	// InitialTier is the tier selected by the task-type classifier.
	InitialTier CascadeTierLevel `json:"initial_tier"`
	// FinalTier is the tier after any escalations.
	FinalTier CascadeTierLevel `json:"final_tier"`
	// Escalated is true if the initial tier was not confident enough.
	Escalated bool `json:"escalated"`
	// EscalationCount is how many tiers were escalated through.
	EscalationCount int `json:"escalation_count"`
	// SelectedModel is the model chosen from the final tier.
	SelectedModel CascadeTierModel `json:"selected_model"`
	// Confidence is the routing confidence at the final tier.
	Confidence float64 `json:"confidence"`
	// Rationale explains the routing decision.
	Rationale string `json:"rationale"`
	// EstimatedSavings is the estimated cost savings vs. always using Tier 3,
	// expressed as a ratio (0.0 = no savings, 1.0 = 100% savings).
	EstimatedSavings float64 `json:"estimated_savings"`
}

// taskTypeTierMap maps enhancer task types to their default cascade tier.
// This is the core classifier: it determines where a prompt starts in the cascade.
var taskTypeTierMap = map[enhancer.TaskType]CascadeTierLevel{
	// Tier 1: Simple, low-complexity tasks
	enhancer.TaskTypeWorkflow: Tier1Fast, // routing, formatting, config editing

	// Tier 2: Moderate complexity
	enhancer.TaskTypeCode:            Tier2Balanced, // code generation, bug fixes
	enhancer.TaskTypeTroubleshooting: Tier2Balanced, // debugging, error analysis
	enhancer.TaskTypeCreative:        Tier2Balanced, // creative writing, docs

	// Tier 3: High complexity
	enhancer.TaskTypeAnalysis: Tier3Powerful, // deep analysis, architecture
}

// complexityTierMap maps numeric complexity (1-4) to cascade tiers.
var complexityTierMap = map[int]CascadeTierLevel{
	1: Tier1Fast,
	2: Tier2Balanced,
	3: Tier2Balanced,
	4: Tier3Powerful,
}

// ClassifyTier determines the appropriate cascade tier for a prompt based on
// task type, complexity, quality score, and prompt characteristics.
func ClassifyTier(
	taskType enhancer.TaskType,
	complexity int,
	qualityScore int,
	prompt string,
	cfg CascadeTierConfig,
) CascadeTierLevel {
	// Check explicit task-type overrides first.
	if override, ok := cfg.TaskTypeTierOverrides[string(taskType)]; ok {
		return override
	}

	// Use task-type mapping.
	if tier, ok := taskTypeTierMap[taskType]; ok {
		// Adjust tier based on prompt quality: low-quality prompts need
		// stronger models to compensate for ambiguity.
		if qualityScore > 0 && qualityScore < 40 && tier < Tier3Powerful {
			return tier + 1 // escalate one tier for low-quality prompts
		}
		return tier
	}

	// Use complexity mapping for TaskTypeGeneral or unknown types.
	if tier, ok := complexityTierMap[complexity]; ok {
		return tier
	}

	// Heuristic: check prompt length and signal words.
	return classifyByHeuristics(prompt)
}

// classifyByHeuristics uses prompt text signals to pick a tier when no
// task-type or complexity mapping applies.
func classifyByHeuristics(prompt string) CascadeTierLevel {
	lower := strings.ToLower(prompt)
	wordCount := len(strings.Fields(prompt))

	// Very short prompts (< 10 words) are typically simple.
	if wordCount < 10 {
		return Tier1Fast
	}

	// Signal words for Tier 3 (complex reasoning).
	tier3Signals := []string{
		"architecture", "design system", "trade-off", "tradeoff",
		"compare and contrast", "migration strategy", "refactor the entire",
		"redesign", "evaluate alternatives", "system design",
		"scalability", "distributed", "consensus",
	}
	for _, sig := range tier3Signals {
		if strings.Contains(lower, sig) {
			return Tier3Powerful
		}
	}

	// Signal words for Tier 1 (simple tasks).
	tier1Signals := []string{
		"format", "lint", "rename", "list all", "show me",
		"what is", "explain briefly", "summarize",
		"check syntax", "validate", "convert",
	}
	for _, sig := range tier1Signals {
		if strings.Contains(lower, sig) {
			return Tier1Fast
		}
	}

	// Default to Tier 2 for everything else.
	return Tier2Balanced
}

// SelectModelForTier picks the best model within a tier, considering provider
// affinity and domain tags. Returns the first model if no preference applies.
func SelectModelForTier(tier CascadeTierDef, preferredProvider session.Provider, domainTags []string) CascadeTierModel {
	if len(tier.Models) == 0 {
		return CascadeTierModel{}
	}

	// If a preferred provider has a model in this tier, use it.
	if preferredProvider != "" {
		for _, m := range tier.Models {
			if m.Provider == preferredProvider {
				return m
			}
		}
	}

	// Apply domain-based selection: Go/MCP/agents favor Claude, shaders favor Gemini.
	for _, tag := range domainTags {
		switch tag {
		case "go", "mcp", "agents", "security":
			for _, m := range tier.Models {
				if m.Provider == session.ProviderClaude {
					return m
				}
			}
		case "shader", "rice", "deployment":
			for _, m := range tier.Models {
				if m.Provider == session.ProviderGemini {
					return m
				}
			}
		}
	}

	// Default to first model (cheapest in the tier by convention).
	return tier.Models[0]
}

// EscalateTier checks whether the current tier's confidence warrants escalation
// and returns the escalation result.
func EscalateTier(
	currentTier CascadeTierLevel,
	confidence float64,
	cfg CascadeTierConfig,
	escalationsSoFar int,
) (shouldEscalate bool, nextTier CascadeTierLevel) {
	// Already at highest tier — nowhere to escalate.
	if currentTier >= Tier3Powerful {
		return false, currentTier
	}

	// Check escalation budget.
	if escalationsSoFar >= cfg.MaxEscalations {
		return false, currentTier
	}

	tierDef := cfg.Tiers[currentTier-1] // tiers are 1-indexed
	if tierDef.EscalationThreshold <= 0 {
		return false, currentTier
	}

	if confidence < tierDef.EscalationThreshold {
		return true, currentTier + 1
	}

	return false, currentTier
}

// RouteCascadeTier performs the full 3-tier cascade routing: classify, select
// model, evaluate confidence, and escalate if needed.
func RouteCascadeTier(
	taskType enhancer.TaskType,
	complexity int,
	qualityScore int,
	prompt string,
	confidence float64,
	preferredProvider session.Provider,
	domainTags []string,
	cfg CascadeTierConfig,
) CascadeTierResult {
	initialTier := ClassifyTier(taskType, complexity, qualityScore, prompt, cfg)
	currentTier := initialTier
	escalations := 0

	// Escalation loop: keep escalating while confidence is below threshold.
	for {
		shouldEscalate, nextTier := EscalateTier(currentTier, confidence, cfg, escalations)
		if !shouldEscalate {
			break
		}
		currentTier = nextTier
		escalations++
		// Each escalation slightly boosts confidence (we have a more capable model).
		confidence += 0.10
		if confidence > 1.0 {
			confidence = 1.0
		}
	}

	tierDef := cfg.Tiers[currentTier-1]
	model := SelectModelForTier(tierDef, preferredProvider, domainTags)

	// Estimate savings vs. always using Tier 3.
	tier3Cost := cfg.Tiers[2].Models[0].CostPer1M
	if tier3Cost > 0 {
		savings := 1.0 - (model.CostPer1M / tier3Cost)
		if savings < 0 {
			savings = 0
		}
		return CascadeTierResult{
			InitialTier:      initialTier,
			FinalTier:        currentTier,
			Escalated:        escalations > 0,
			EscalationCount:  escalations,
			SelectedModel:    model,
			Confidence:       confidence,
			Rationale:        buildCascadeRationale(initialTier, currentTier, escalations, model, confidence),
			EstimatedSavings: savings,
		}
	}

	return CascadeTierResult{
		InitialTier:     initialTier,
		FinalTier:       currentTier,
		Escalated:       escalations > 0,
		EscalationCount: escalations,
		SelectedModel:   model,
		Confidence:      confidence,
		Rationale:       buildCascadeRationale(initialTier, currentTier, escalations, model, confidence),
	}
}

func buildCascadeRationale(initial, final CascadeTierLevel, escalations int, model CascadeTierModel, confidence float64) string {
	if escalations == 0 {
		return fmt.Sprintf("Routed to %s tier (%s/%s) — confidence %.2f, no escalation needed",
			final, model.Provider, model.Model, confidence)
	}
	return fmt.Sprintf("Classified as %s tier, escalated %dx to %s tier (%s/%s) — confidence %.2f",
		initial, escalations, final, model.Provider, model.Model, confidence)
}

// CostProjection estimates cost savings from using the 3-tier cascade vs.
// always routing to Tier 3 (the most expensive tier).
type CostProjection struct {
	// CascadeCostPer1K is the estimated cost per 1,000 prompts using the cascade.
	CascadeCostPer1K float64 `json:"cascade_cost_per_1k"`
	// NaiveCostPer1K is the estimated cost per 1,000 prompts using only Tier 3.
	NaiveCostPer1K float64 `json:"naive_cost_per_1k"`
	// SavingsPercent is the estimated percentage savings.
	SavingsPercent float64 `json:"savings_percent"`
	// TierDistribution shows the expected percentage of prompts at each tier.
	TierDistribution [3]float64 `json:"tier_distribution"`
}

// ProjectCascadeSavings estimates cost savings based on expected prompt
// distribution across tiers. Distribution is [Tier1%, Tier2%, Tier3%] and
// must sum to 1.0.
func ProjectCascadeSavings(cfg CascadeTierConfig, distribution [3]float64, avgTokensPerPrompt int) CostProjection {
	tokens := float64(avgTokensPerPrompt)

	var cascadeCost float64
	for i, pct := range distribution {
		if len(cfg.Tiers[i].Models) > 0 {
			// Use the cheapest model in each tier for projection.
			cheapest := cfg.Tiers[i].Models[0].CostPer1M
			for _, m := range cfg.Tiers[i].Models[1:] {
				if m.CostPer1M < cheapest {
					cheapest = m.CostPer1M
				}
			}
			cascadeCost += pct * (tokens / 1_000_000.0) * cheapest * 1000 // per 1K prompts
		}
	}

	// Naive: always Tier 3, cheapest model.
	var naiveCost float64
	if len(cfg.Tiers[2].Models) > 0 {
		naiveCost = (tokens / 1_000_000.0) * cfg.Tiers[2].Models[0].CostPer1M * 1000
	}

	var savingsPct float64
	if naiveCost > 0 {
		savingsPct = (1.0 - cascadeCost/naiveCost) * 100
	}

	return CostProjection{
		CascadeCostPer1K: cascadeCost,
		NaiveCostPer1K:   naiveCost,
		SavingsPercent:   savingsPct,
		TierDistribution: distribution,
	}
}
