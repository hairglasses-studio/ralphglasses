package enhancer

import "context"

// ProviderName identifies which LLM API backend to use for prompt improvement.
type ProviderName string

const (
	ProviderClaude ProviderName = "claude"
	ProviderGemini ProviderName = "gemini"
	ProviderOpenAI ProviderName = "openai"
)

// PromptImprover is implemented by each LLM API client for prompt improvement.
type PromptImprover interface {
	// Improve sends a prompt to the LLM with a provider-specific meta-prompt
	// and returns the improved version.
	Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error)
	// Provider returns the provider name for cache keying and logging.
	Provider() ProviderName
}

// NewPromptImprover creates the appropriate client for the configured provider.
// Returns nil if no API key is available.
func NewPromptImprover(cfg LLMConfig) PromptImprover {
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
	default:
		c := NewLLMClient(cfg)
		if c == nil {
			return nil
		}
		return c
	}
}
