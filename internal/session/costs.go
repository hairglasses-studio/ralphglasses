package session

import (
	"github.com/hairglasses-studio/ralphglasses/internal/config"
)

// Model cost rates per 1M tokens (USD). These are re-exported from the config
// package for backward compatibility. The canonical source of truth is now
// internal/config/costs.go — update rates there when provider pricing changes.
const (
	CostGeminiFlashLiteInput = config.CostGeminiFlashLiteInput
	CostGeminiFlashInput     = config.CostGeminiFlashInput
	CostGeminiFlashOutput    = config.CostGeminiFlashOutput
	CostClaudeSonnetInput    = config.CostClaudeSonnetInput
	CostClaudeSonnetOutput   = config.CostClaudeSonnetOutput
	CostClaudeOpusInput      = config.CostClaudeOpusInput
	CostClaudeOpusOutput     = config.CostClaudeOpusOutput
	CostCodexInput           = config.CostCodexInput
	CostCodexOutput          = config.CostCodexOutput
)

// CostRates holds configurable per-model cost rates (USD per 1M tokens).
// This is a thin wrapper around config.ProviderCosts for backward compatibility.
type CostRates struct {
	InputPerMToken  map[string]float64 `json:"input_per_m_token"`
	OutputPerMToken map[string]float64 `json:"output_per_m_token"`
}

// DefaultCostRates returns a CostRates populated from the config package defaults.
func DefaultCostRates() *CostRates {
	pc := config.DefaultProviderCosts()
	return &CostRates{
		InputPerMToken:  pc.InputPerMToken,
		OutputPerMToken: pc.OutputPerMToken,
	}
}

// LoadCostRates reads cost rates from a JSON file at path. Delegates to
// config.LoadProviderCosts and wraps the result for backward compatibility.
func LoadCostRates(path string) (*CostRates, error) {
	pc, err := config.LoadProviderCosts(path)
	if err != nil {
		return nil, err
	}
	return &CostRates{
		InputPerMToken:  pc.InputPerMToken,
		OutputPerMToken: pc.OutputPerMToken,
	}, nil
}

// CostRatesFromConfig converts a config.ProviderCosts to a session CostRates.
// If pc is nil, returns DefaultCostRates().
func CostRatesFromConfig(pc *config.ProviderCosts) *CostRates {
	if pc == nil {
		return DefaultCostRates()
	}
	return &CostRates{
		InputPerMToken:  pc.InputPerMToken,
		OutputPerMToken: pc.OutputPerMToken,
	}
}

// ProviderCostRateFrom returns a CostRate for the given provider using the
// configurable CostRates. Falls back to the compiled-in defaults for any
// missing key.
func (cr *CostRates) ProviderCostRateFrom(p Provider) CostRate {
	var rate CostRate

	switch p {
	case ProviderClaude:
		rate.InputPer1M = cr.lookupInput("claude_sonnet", CostClaudeSonnetInput)
		rate.OutputPer1M = cr.lookupOutput("claude_sonnet", CostClaudeSonnetOutput)
	case ProviderGemini:
		rate.InputPer1M = cr.lookupInput("gemini_flash", CostGeminiFlashInput)
		rate.OutputPer1M = cr.lookupOutput("gemini_flash", CostGeminiFlashOutput)
	case ProviderCodex:
		rate.InputPer1M = cr.lookupInput("codex", CostCodexInput)
		rate.OutputPer1M = cr.lookupOutput("codex", CostCodexOutput)
	}

	return rate
}

func (cr *CostRates) lookupInput(key string, fallback float64) float64 {
	if cr != nil && cr.InputPerMToken != nil {
		if v, ok := cr.InputPerMToken[key]; ok {
			return v
		}
	}
	return fallback
}

func (cr *CostRates) lookupOutput(key string, fallback float64) float64 {
	if cr != nil && cr.OutputPerMToken != nil {
		if v, ok := cr.OutputPerMToken[key]; ok {
			return v
		}
	}
	return fallback
}
