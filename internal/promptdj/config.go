package promptdj

import (
	"os"
	"strconv"
	"strings"
)

// PromptDJConfig holds routing configuration.
type PromptDJConfig struct {
	Enabled            bool    `json:"enabled"`
	EnhanceThreshold   int     `json:"enhance_threshold"`    // score < this triggers auto-enhancement (default 50)
	SuggestThreshold   int     `json:"suggest_threshold"`    // score < this suggests enhancement (default 80)
	MinConfidence      float64 `json:"min_confidence"`       // below this, route to highest tier (default 0.50)
	EnhanceMaxPerHour  int     `json:"enhance_max_per_hour"` // rate limit (default 50)
	EnhanceMode        string  `json:"enhance_mode"`         // "local" or "hybrid" (default "local")
	LogDecisions       bool    `json:"log_decisions"`        // persist decisions to JSONL (default true)
	LearnFromOutcomes  bool    `json:"learn_from_outcomes"`  // update weights from feedback (default true)
	LearningRate       float64 `json:"learning_rate"`        // EMA learning rate (default 0.05)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() PromptDJConfig {
	return PromptDJConfig{
		Enabled:           false, // opt-in
		EnhanceThreshold:  50,
		SuggestThreshold:  80,
		MinConfidence:     0.50,
		EnhanceMaxPerHour: 50,
		EnhanceMode:       "local",
		LogDecisions:      true,
		LearnFromOutcomes: true,
		LearningRate:      0.05,
	}
}

// ConfigFromEnv loads config from environment variables (.ralphrc keys).
func ConfigFromEnv() PromptDJConfig {
	cfg := DefaultConfig()

	if v := os.Getenv("PROMPT_ROUTER_ENABLED"); v != "" {
		cfg.Enabled = strings.EqualFold(v, "true") || v == "1"
	}
	if v := os.Getenv("PROMPT_ROUTER_ENHANCE_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.EnhanceThreshold = n
		}
	}
	if v := os.Getenv("PROMPT_ROUTER_SUGGEST_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SuggestThreshold = n
		}
	}
	if v := os.Getenv("PROMPT_ROUTER_MIN_CONFIDENCE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.MinConfidence = f
		}
	}
	if v := os.Getenv("PROMPT_ROUTER_ENHANCE_MAX_HOUR"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.EnhanceMaxPerHour = n
		}
	}
	if v := os.Getenv("PROMPT_ROUTER_ENHANCE_MODE"); v != "" {
		cfg.EnhanceMode = v
	}
	if v := os.Getenv("PROMPT_ROUTER_LOG_DECISIONS"); v != "" {
		cfg.LogDecisions = strings.EqualFold(v, "true") || v == "1"
	}

	return cfg
}
