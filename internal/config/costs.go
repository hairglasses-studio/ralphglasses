package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
)

// Default model cost rates per 1M tokens (USD). These serve as the single
// source of truth for all cost-aware subsystems. Update here when provider
// pricing changes.
const (
	CostGeminiFlashLiteInput float64 = 0.10
	CostGeminiFlashInput     float64 = 0.30
	CostGeminiFlashOutput    float64 = 3.50
	CostClaudeSonnetInput    float64 = 3.00
	CostClaudeSonnetOutput   float64 = 15.00
	CostClaudeOpusInput      float64 = 15.00
	CostClaudeOpusOutput     float64 = 75.00
	CostCodexInput           float64 = 2.50
	CostCodexOutput          float64 = 15.00
)

// ProviderCosts holds configurable per-model cost rates (USD per 1M tokens).
// Keys in the maps are model/tier identifiers (e.g. "gemini_flash", "claude_sonnet").
type ProviderCosts struct {
	InputPerMToken  map[string]float64 `json:"input_per_m_token"`
	OutputPerMToken map[string]float64 `json:"output_per_m_token"`
}

// DefaultProviderCosts returns a ProviderCosts populated from the compiled-in constants.
func DefaultProviderCosts() *ProviderCosts {
	return &ProviderCosts{
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

// LoadProviderCosts reads cost rates from a JSON file at path. If the file does
// not exist, it returns DefaultProviderCosts with no error. Malformed JSON returns
// an error. Partial overrides are merged with defaults so callers only need to
// specify the rates they want to change.
func LoadProviderCosts(path string) (*ProviderCosts, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultProviderCosts(), nil
		}
		return nil, err
	}

	var costs ProviderCosts
	if err := json.Unmarshal(data, &costs); err != nil {
		return nil, err
	}

	// Merge: fill any missing keys from defaults so that partial overrides work.
	defaults := DefaultProviderCosts()
	if costs.InputPerMToken == nil {
		costs.InputPerMToken = defaults.InputPerMToken
	} else {
		for k, v := range defaults.InputPerMToken {
			if _, ok := costs.InputPerMToken[k]; !ok {
				costs.InputPerMToken[k] = v
			}
		}
	}
	if costs.OutputPerMToken == nil {
		costs.OutputPerMToken = defaults.OutputPerMToken
	} else {
		for k, v := range defaults.OutputPerMToken {
			if _, ok := costs.OutputPerMToken[k]; !ok {
				costs.OutputPerMToken[k] = v
			}
		}
	}

	return &costs, nil
}

// InputRate returns the input cost per 1M tokens for the given model key,
// or the fallback value if not found.
func (pc *ProviderCosts) InputRate(model string, fallback float64) float64 {
	if pc != nil && pc.InputPerMToken != nil {
		if v, ok := pc.InputPerMToken[model]; ok {
			return v
		}
	}
	return fallback
}

// OutputRate returns the output cost per 1M tokens for the given model key,
// or the fallback value if not found.
func (pc *ProviderCosts) OutputRate(model string, fallback float64) float64 {
	if pc != nil && pc.OutputPerMToken != nil {
		if v, ok := pc.OutputPerMToken[model]; ok {
			return v
		}
	}
	return fallback
}

// CostRateForProvider returns a (input, output) cost pair for a well-known
// provider name ("claude", "gemini", "codex"). Returns compiled-in defaults
// if the provider or model key is not in the overrides map.
func (pc *ProviderCosts) CostRateForProvider(provider string) (inputPer1M, outputPer1M float64) {
	switch provider {
	case "claude":
		return pc.InputRate("claude_sonnet", CostClaudeSonnetInput),
			pc.OutputRate("claude_sonnet", CostClaudeSonnetOutput)
	case "gemini":
		return pc.InputRate("gemini_flash", CostGeminiFlashInput),
			pc.OutputRate("gemini_flash", CostGeminiFlashOutput)
	case "codex", "openai":
		return pc.InputRate("codex", CostCodexInput),
			pc.OutputRate("codex", CostCodexOutput)
	default:
		// Unknown provider: return Claude rates as a safe default.
		return pc.InputRate("claude_sonnet", CostClaudeSonnetInput),
			pc.OutputRate("claude_sonnet", CostClaudeSonnetOutput)
	}
}
