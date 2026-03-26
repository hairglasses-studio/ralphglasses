package session

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// Model cost rates per 1M tokens (USD). Single source of truth for all
// cost-aware subsystems (budget normalization, cascade routing, tier selection).
// Update here when provider pricing changes.
const (
	CostGeminiFlashLiteInput = 0.10
	CostGeminiFlashInput     = 0.30
	CostGeminiFlashOutput    = 2.50
	CostClaudeSonnetInput    = 3.00
	CostClaudeSonnetOutput   = 15.00
	CostClaudeOpusInput      = 15.00
	CostClaudeOpusOutput     = 75.00
	CostCodexInput           = 2.50
	CostCodexOutput          = 15.00
)

// CostRates holds configurable per-model cost rates (USD per 1M tokens).
// Keys in the maps are model/tier identifiers matching the const names
// (e.g. "gemini_flash_input", "claude_sonnet_input").
type CostRates struct {
	InputPerMToken  map[string]float64 `json:"input_per_m_token"`
	OutputPerMToken map[string]float64 `json:"output_per_m_token"`
}

// DefaultCostRates returns a CostRates populated from the compiled-in constants.
func DefaultCostRates() *CostRates {
	return &CostRates{
		InputPerMToken: map[string]float64{
			"gemini_flash_lite": CostGeminiFlashLiteInput,
			"gemini_flash":      CostGeminiFlashInput,
			"claude_sonnet":     CostClaudeSonnetInput,
			"claude_opus":       CostClaudeOpusInput,
			"codex":             CostCodexInput,
		},
		OutputPerMToken: map[string]float64{
			"gemini_flash": CostGeminiFlashOutput,
			"claude_sonnet": CostClaudeSonnetOutput,
			"claude_opus":   CostClaudeOpusOutput,
			"codex":         CostCodexOutput,
		},
	}
}

// LoadCostRates reads cost rates from a JSON file at path. If the file does
// not exist, it returns DefaultCostRates with no error. Malformed JSON returns
// an error.
func LoadCostRates(path string) (*CostRates, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultCostRates(), nil
		}
		return nil, err
	}

	var rates CostRates
	if err := json.Unmarshal(data, &rates); err != nil {
		return nil, err
	}

	// Merge: fill any missing keys from defaults so that partial overrides work.
	defaults := DefaultCostRates()
	if rates.InputPerMToken == nil {
		rates.InputPerMToken = defaults.InputPerMToken
	} else {
		for k, v := range defaults.InputPerMToken {
			if _, ok := rates.InputPerMToken[k]; !ok {
				rates.InputPerMToken[k] = v
			}
		}
	}
	if rates.OutputPerMToken == nil {
		rates.OutputPerMToken = defaults.OutputPerMToken
	} else {
		for k, v := range defaults.OutputPerMToken {
			if _, ok := rates.OutputPerMToken[k]; !ok {
				rates.OutputPerMToken[k] = v
			}
		}
	}

	return &rates, nil
}

// ProviderCostRateFrom returns a CostRate for the given provider using the
// configurable CostRates. Falls back to the compiled-in ProviderCostRates
// for any missing key.
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
