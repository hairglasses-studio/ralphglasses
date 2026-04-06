package session

import (
	"log/slog"
	"path/filepath"
	"sync"
)

// CostRate holds input/output token pricing for a provider (USD per 1M tokens).
type CostRate struct {
	InputPer1M  float64 `json:"input_per_1m_usd"`
	OutputPer1M float64 `json:"output_per_1m_usd"`
}

// costRateMu protects ProviderCostRates and claudeBaseRate from concurrent access.
var costRateMu sync.RWMutex

// ProviderCostRates maps each provider to its approximate token pricing.
// These are reference rates for cross-provider cost normalization; update when
// provider pricing changes. Call LoadCostRatesFromDir to override from a
// .ralph/cost_rates.json file. Access must be guarded by costRateMu.
var ProviderCostRates = map[Provider]CostRate{
	ProviderClaude: {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput}, // claude-sonnet-4.6
	ProviderGemini: {InputPer1M: CostGeminiFlashInput, OutputPer1M: CostGeminiFlashOutput},   // gemini-2.5-flash
	ProviderCodex:  {InputPer1M: CostCodexInput, OutputPer1M: CostCodexOutput},               // gpt-5.4
	ProviderCrush:  {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput}, // crush multi-model, default to Claude rates
	ProviderGoose:  {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput}, // goose multi-model, default to Claude rates
	ProviderAmp:    {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput}, // amp managed models, default to Claude rates
	ProviderA2A:    {InputPer1M: CostClaudeSonnetInput, OutputPer1M: CostClaudeSonnetOutput}, // a2a remote agents, default to Claude rates (actual cost depends on remote)
}

// claudeBaseRate is the reference rate used for normalization.
var claudeBaseRate = ProviderCostRates[ProviderClaude]

// getProviderCostRate returns the cost rate for a provider, safe for concurrent use.
func getProviderCostRate(p Provider) (CostRate, bool) {
	costRateMu.RLock()
	defer costRateMu.RUnlock()
	rate, ok := ProviderCostRates[p]
	return rate, ok
}

// getClaudeBaseRate returns the Claude base rate, safe for concurrent use.
func getClaudeBaseRate() CostRate {
	costRateMu.RLock()
	defer costRateMu.RUnlock()
	return claudeBaseRate
}

// LoadCostRatesFromDir loads cost rate overrides from ralphDir/cost_rates.json.
// If the file doesn't exist, the compiled-in defaults remain unchanged.
// On successful load, ProviderCostRates and claudeBaseRate are updated.
// Safe for concurrent use.
func LoadCostRatesFromDir(ralphDir string) {
	path := filepath.Join(ralphDir, "cost_rates.json")
	cr, err := LoadCostRates(path)
	if err != nil {
		slog.Warn("failed to load cost rates override", "path", path, "error", err)
		return
	}

	costRateMu.Lock()
	defer costRateMu.Unlock()

	// Apply overrides to ProviderCostRates.
	for _, provider := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex, ProviderCrush, ProviderGoose, ProviderAmp, ProviderA2A} {
		rate := cr.ProviderCostRateFrom(provider)
		if rate.InputPer1M > 0 || rate.OutputPer1M > 0 {
			ProviderCostRates[provider] = rate
		}
	}
	claudeBaseRate = ProviderCostRates[ProviderClaude]
}

// NormalizedCost holds a cost breakdown with cross-provider normalization.
type NormalizedCost struct {
	Provider      Provider `json:"provider"`
	RawCostUSD    float64  `json:"raw_cost_usd"`
	InputTokens   int      `json:"input_tokens"`
	OutputTokens  int      `json:"output_tokens"`
	NormalizedUSD float64  `json:"normalized_usd"` // cost at Claude-sonnet-4 rates
	EfficiencyPct float64  `json:"efficiency_pct"` // raw/normalized × 100 (<100 = cheaper than Claude)
}

// NormalizeProviderCost normalizes a provider's reported cost to the Claude baseline.
//
// If inputTokens/outputTokens are known, the normalized cost is computed directly
// from token counts at Claude rates. When token counts are zero, it estimates them
// from the raw cost at the provider's blended rate (50/50 input/output heuristic).
func NormalizeProviderCost(p Provider, rawCostUSD float64, inputTokens, outputTokens int) NormalizedCost {
	n := NormalizedCost{
		Provider:     p,
		RawCostUSD:   rawCostUSD,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}

	if rawCostUSD <= 0 {
		n.EfficiencyPct = 100
		return n
	}

	rate, hasRate := getProviderCostRate(p)
	baseRate := getClaudeBaseRate()

	if inputTokens > 0 || outputTokens > 0 {
		// Exact normalization using token counts.
		n.NormalizedUSD = (float64(inputTokens)/1_000_000)*baseRate.InputPer1M +
			(float64(outputTokens)/1_000_000)*baseRate.OutputPer1M
	} else if hasRate {
		// Estimate: scale raw cost by the ratio of Claude's blended rate to provider's blended rate.
		providerBlended := (rate.InputPer1M + rate.OutputPer1M) / 2
		claudeBlended := (baseRate.InputPer1M + baseRate.OutputPer1M) / 2
		if providerBlended > 0 {
			n.NormalizedUSD = rawCostUSD * (claudeBlended / providerBlended)
		} else {
			n.NormalizedUSD = rawCostUSD
		}
	} else {
		n.NormalizedUSD = rawCostUSD
	}

	if n.NormalizedUSD > 0 {
		n.EfficiencyPct = (n.RawCostUSD / n.NormalizedUSD) * 100
	}
	return n
}

// NormalizeSessionCost normalizes a session's cumulative spend.
func NormalizeSessionCost(s *Session) NormalizedCost {
	s.mu.Lock()
	provider := s.Provider
	spent := s.SpentUSD
	s.mu.Unlock()
	return NormalizeProviderCost(provider, spent, 0, 0)
}
