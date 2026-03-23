package session

// CostRate holds input/output token pricing for a provider (USD per 1M tokens).
type CostRate struct {
	InputPer1M  float64 `json:"input_per_1m_usd"`
	OutputPer1M float64 `json:"output_per_1m_usd"`
}

// ProviderCostRates maps each provider to its approximate token pricing.
// These are reference rates for cross-provider cost normalization; update when
// provider pricing changes.
var ProviderCostRates = map[Provider]CostRate{
	ProviderClaude: {InputPer1M: 3.00, OutputPer1M: 15.00}, // claude-sonnet-4
	ProviderGemini: {InputPer1M: 1.25, OutputPer1M: 5.00},  // gemini-2.5-pro
	ProviderCodex:  {InputPer1M: 2.50, OutputPer1M: 10.00}, // gpt-5 estimate
}

// claudeBaseRate is the reference rate used for normalization.
var claudeBaseRate = ProviderCostRates[ProviderClaude]

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

	rate, hasRate := ProviderCostRates[p]

	if inputTokens > 0 || outputTokens > 0 {
		// Exact normalization using token counts.
		n.NormalizedUSD = (float64(inputTokens)/1_000_000)*claudeBaseRate.InputPer1M +
			(float64(outputTokens)/1_000_000)*claudeBaseRate.OutputPer1M
	} else if hasRate {
		// Estimate: scale raw cost by the ratio of Claude's blended rate to provider's blended rate.
		providerBlended := (rate.InputPer1M + rate.OutputPer1M) / 2
		claudeBlended := (claudeBaseRate.InputPer1M + claudeBaseRate.OutputPer1M) / 2
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
