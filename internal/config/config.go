// Package config provides the top-level runtime configuration for ralphglasses.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"
)

// Config holds the top-level runtime configuration for ralphglasses.
type Config struct {
	// ScanPaths lists directories to scan for ralph-enabled repos.
	ScanPaths []string `json:"scan_paths,omitempty"`

	// DefaultProvider is the default LLM provider (claude, gemini, codex, openai, ollama).
	DefaultProvider string `json:"default_provider,omitempty"`

	// WorkerProvider is the default provider for worker/delegate tasks.
	WorkerProvider string `json:"worker_provider,omitempty"`

	// MaxWorkers is the maximum number of concurrent worker sessions.
	MaxWorkers int `json:"max_workers,omitempty"`

	// DefaultBudgetUSD is the default per-session budget in USD.
	DefaultBudgetUSD float64 `json:"default_budget_usd,omitempty"`

	// CostRateMultiplier scales cost estimates (e.g. 1.0 = normal, 0.5 = half).
	CostRateMultiplier float64 `json:"cost_rate_multiplier,omitempty"`

	// SessionTimeout is the default session timeout.
	SessionTimeout time.Duration `json:"session_timeout,omitempty"`

	// HealthCheckInterval controls how often provider health is polled.
	HealthCheckInterval time.Duration `json:"health_check_interval,omitempty"`

	// ProviderCosts holds configurable per-model cost rates. When nil,
	// DefaultProviderCosts() is used. Load from .ralph/cost_rates.json
	// to override specific model rates without recompiling.
	ProviderCosts *ProviderCosts `json:"provider_costs,omitempty"`
}

// Load reads a config JSON file from path. If the file does not exist,
// it returns a zero-value Config with no error. After loading, it runs
// validation and logs any warnings.
func Load(path string) (*Config, error) {
	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if warnings := cfg.Validate(); len(warnings) > 0 {
		for _, w := range warnings {
			slog.Warn("config validation", "warning", w)
		}
	}

	return cfg, nil
}

// Validate checks the Config for common misconfigurations.
// Returns a slice of validation errors (warnings, not fatal).
func (c *Config) Validate() []error {
	return ValidateConfig(c)
}
