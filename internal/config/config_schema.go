package config

import (
	"fmt"
	"os"
)

// knownProviders is the set of recognized LLM provider names.
var knownProviders = map[string]bool{
	"claude": true,
	"gemini": true,
	"codex":  true,
	"openai": true,
	"ollama": true,
}

// ValidateConfig checks a Config for common misconfigurations.
// Returns a slice of validation errors (warnings, not fatal).
func ValidateConfig(cfg *Config) []error {
	if cfg == nil {
		return nil
	}
	var errs []error

	// Validate scan paths exist on disk.
	for _, p := range cfg.ScanPaths {
		if _, err := os.Stat(p); err != nil {
			errs = append(errs, fmt.Errorf("scan path %q: %w", p, err))
		}
	}

	// Validate provider names are recognized.
	if cfg.DefaultProvider != "" && !knownProviders[cfg.DefaultProvider] {
		errs = append(errs, fmt.Errorf("unknown default provider %q (known: claude, gemini, codex, openai, ollama)", cfg.DefaultProvider))
	}
	if cfg.WorkerProvider != "" && !knownProviders[cfg.WorkerProvider] {
		errs = append(errs, fmt.Errorf("unknown worker provider %q (known: claude, gemini, codex, openai, ollama)", cfg.WorkerProvider))
	}

	// Validate worker count in sane range (1-50).
	if cfg.MaxWorkers < 0 {
		errs = append(errs, fmt.Errorf("max_workers must be non-negative, got %d", cfg.MaxWorkers))
	} else if cfg.MaxWorkers > 50 {
		errs = append(errs, fmt.Errorf("max_workers %d exceeds maximum of 50", cfg.MaxWorkers))
	}

	// Validate cost-related fields are non-negative.
	if cfg.DefaultBudgetUSD < 0 {
		errs = append(errs, fmt.Errorf("default_budget_usd must be non-negative, got %g", cfg.DefaultBudgetUSD))
	}
	if cfg.CostRateMultiplier < 0 {
		errs = append(errs, fmt.Errorf("cost_rate_multiplier must be non-negative, got %g", cfg.CostRateMultiplier))
	}

	// Validate timeout fields are non-negative.
	if cfg.SessionTimeout < 0 {
		errs = append(errs, fmt.Errorf("session_timeout must be non-negative, got %s", cfg.SessionTimeout))
	}
	if cfg.HealthCheckInterval < 0 {
		errs = append(errs, fmt.Errorf("health_check_interval must be non-negative, got %s", cfg.HealthCheckInterval))
	}

	return errs
}
