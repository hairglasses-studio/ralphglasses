package session

import (
	"fmt"
	"strings"
	"time"
)

// LoopConfig captures the user-facing configuration for a loop run,
// consolidating fields from LoopProfile and LaunchOptions that benefit
// from pre-flight validation.
type LoopConfig struct {
	Provider        Provider      `json:"provider"`
	Model           string        `json:"model"`
	BudgetUSD       float64       `json:"budget_usd,omitempty"`
	EnhancePrompt   bool          `json:"enhance_prompt,omitempty"`
	EnhancerProvider string       `json:"enhancer_provider,omitempty"` // provider configured for enhancement
	Timeout         time.Duration `json:"timeout,omitempty"`
}

// LoopValidationWarning describes a single validation issue with a loop config.
type LoopValidationWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// String returns a human-readable representation of the warning.
func (w LoopValidationWarning) String() string {
	return fmt.Sprintf("%s: %s", w.Field, w.Message)
}

// knownModelPrefixes maps each provider to valid model name prefixes.
var knownModelPrefixes = map[Provider][]string{
	ProviderClaude: {"claude-"},
	ProviderGemini: {"gemini-"},
	ProviderCodex:  {"gpt-", "o1-", "o3-", "codex-"},
}

// ValidateLoopConfig checks a LoopConfig for common misconfigurations and
// returns warnings. An empty slice means the config looks valid.
func ValidateLoopConfig(cfg LoopConfig) []LoopValidationWarning {
	var warnings []LoopValidationWarning

	// 1. Model name validation: check model matches known patterns for provider.
	if cfg.Provider != "" && cfg.Model != "" {
		prefixes, known := knownModelPrefixes[cfg.Provider]
		if known {
			matched := false
			for _, p := range prefixes {
				if strings.HasPrefix(cfg.Model, p) {
					matched = true
					break
				}
			}
			if !matched {
				warnings = append(warnings, LoopValidationWarning{
					Field:   "model",
					Message: fmt.Sprintf("model %q does not match expected prefixes for provider %q (%s)", cfg.Model, cfg.Provider, strings.Join(prefixes, ", ")),
				})
			}
		}
	}

	// 2. Budget range validation.
	if cfg.BudgetUSD != 0 {
		if cfg.BudgetUSD < 0.01 {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "budget_usd",
				Message: fmt.Sprintf("budget $%.4f is below minimum recommended $0.01", cfg.BudgetUSD),
			})
		}
		if cfg.BudgetUSD > 100 {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "budget_usd",
				Message: fmt.Sprintf("budget $%.2f exceeds maximum recommended $100.00", cfg.BudgetUSD),
			})
		}
	}

	// 3. Enhancement flag checks.
	if cfg.EnhancePrompt && cfg.EnhancerProvider == "" {
		warnings = append(warnings, LoopValidationWarning{
			Field:   "enhance_prompt",
			Message: "prompt enhancement enabled but no enhancer provider is configured",
		})
	}

	// 4. Timeout validation.
	if cfg.Timeout != 0 {
		if cfg.Timeout < 10*time.Second {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "timeout",
				Message: fmt.Sprintf("timeout %s is below minimum recommended 10s", cfg.Timeout),
			})
		}
		if cfg.Timeout > 3600*time.Second {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "timeout",
				Message: fmt.Sprintf("timeout %s exceeds maximum recommended 1h", cfg.Timeout),
			})
		}
	}

	return warnings
}
