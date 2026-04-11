package enhancer

import (
	"context"
	"strings"
)

// ProviderName identifies which LLM API backend to use for prompt improvement.
type ProviderName string

const (
	ProviderClaude ProviderName = "claude"
	ProviderGemini ProviderName = "gemini"
	ProviderOpenAI ProviderName = "openai"
)

func normalizeLLMProviderAlias(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "claude", "gemini", "openai":
		return strings.ToLower(strings.TrimSpace(provider))
	case "codex":
		return "openai"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

// NormalizeTargetProviderName resolves user-facing provider aliases down to the
// prompt-enhancer's structural provider families.
func NormalizeTargetProviderName(provider string) ProviderName {
	switch normalizeLLMProviderAlias(provider) {
	case "claude":
		return ProviderClaude
	case "gemini":
		return ProviderGemini
	case "openai":
		return ProviderOpenAI
	default:
		return ""
	}
}

// PromptImprover is implemented by each LLM API client for prompt improvement.
type PromptImprover interface {
	// Improve sends a prompt to the LLM with a provider-specific meta-prompt
	// and returns the improved version.
	Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error)
	// Provider returns the provider name for cache keying and logging.
	Provider() ProviderName
}

// DefaultTargetProviderForLLM returns the TargetProvider that matches the given
// LLM provider name. Returns ProviderOpenAI for unknown or empty values.
func DefaultTargetProviderForLLM(provider string) ProviderName {
	return defaultTargetProviderForLLM(provider)
}

func defaultTargetProviderForLLM(provider string) ProviderName {
	if target := NormalizeTargetProviderName(provider); target != "" {
		return target
	}
	return ProviderOpenAI
}

func normalizeTargetProvider(provider ProviderName) ProviderName {
	if normalized := NormalizeTargetProviderName(string(provider)); normalized != "" {
		return normalized
	}
	return ProviderOpenAI
}

// NewPromptImprover creates the appropriate client for the configured provider.
// Returns nil if no API key is available.
func NewPromptImprover(cfg LLMConfig) PromptImprover {
	if cfg.Provider == "" {
		for _, provider := range []string{"openai", "gemini", "claude"} {
			cfg.Provider = provider
			if client := NewPromptImprover(cfg); client != nil {
				return client
			}
		}
		return nil
	}

	cfg.Provider = normalizeLLMProviderAlias(cfg.Provider)

	switch cfg.Provider {
	case "gemini":
		c := NewGeminiClient(cfg)
		if c == nil {
			return nil
		}
		return c
	case "openai":
		c := NewOpenAIClient(cfg)
		if c == nil {
			return nil
		}
		return c
	case "claude":
		c := NewLLMClient(cfg)
		if c == nil {
			return nil
		}
		return c
	default:
		return nil
	}
}
