package session

import "fmt"

// ContextBudget tracks token allocation across different sections of the
// context window. It enforces that the sum of all sections never exceeds
// the provider's context limit, reserving space for the model's response.
type ContextBudget struct {
	MaxTokens      int64 `json:"max_tokens"`
	ResponseReserve int64 `json:"response_reserve"`

	SystemPrompt   int64 `json:"system_prompt_tokens"`
	History        int64 `json:"history_tokens"`
	ToolResults    int64 `json:"tool_results_tokens"`
	PrefetchCtx    int64 `json:"prefetch_ctx_tokens"`
	ErrorContext   int64 `json:"error_context_tokens"`
	UserPrompt     int64 `json:"user_prompt_tokens"`
}

// DefaultContextBudget returns a budget sized for a typical provider context window.
// Claude: 200K, Gemini: 1M, Codex: 200K. Uses conservative 128K as default.
func DefaultContextBudget(provider Provider) ContextBudget {
	maxTokens := int64(128000) // conservative default
	switch provider {
	case ProviderClaude:
		maxTokens = 200000
	case ProviderGemini:
		maxTokens = 1000000
	case ProviderCodex:
		maxTokens = 200000
	}
	return ContextBudget{
		MaxTokens:       maxTokens,
		ResponseReserve: maxTokens / 10, // 10% reserved for response
	}
}

// Used returns the total tokens currently allocated across all sections.
func (b *ContextBudget) Used() int64 {
	return b.SystemPrompt + b.History + b.ToolResults +
		b.PrefetchCtx + b.ErrorContext + b.UserPrompt
}

// Available returns the number of tokens that can still be added
// before reaching the limit (accounting for response reserve).
func (b *ContextBudget) Available() int64 {
	limit := b.MaxTokens - b.ResponseReserve
	used := b.Used()
	if used >= limit {
		return 0
	}
	return limit - used
}

// Fits returns true if additionalTokens can be added without exceeding the budget.
func (b *ContextBudget) Fits(additionalTokens int64) bool {
	return additionalTokens <= b.Available()
}

// Allocate attempts to add tokens to the given section. Returns an error
// if the allocation would exceed the budget.
func (b *ContextBudget) Allocate(section string, tokens int64) error {
	if !b.Fits(tokens) {
		return fmt.Errorf("context budget exceeded: need %d tokens for %s, only %d available",
			tokens, section, b.Available())
	}
	switch section {
	case "system_prompt":
		b.SystemPrompt += tokens
	case "history":
		b.History += tokens
	case "tool_results":
		b.ToolResults += tokens
	case "prefetch":
		b.PrefetchCtx += tokens
	case "errors":
		b.ErrorContext += tokens
	case "user_prompt":
		b.UserPrompt += tokens
	default:
		return fmt.Errorf("unknown context section: %s", section)
	}
	return nil
}

// UtilizationPct returns the percentage of the usable context window that is filled.
func (b *ContextBudget) UtilizationPct() float64 {
	limit := b.MaxTokens - b.ResponseReserve
	if limit <= 0 {
		return 100.0
	}
	return float64(b.Used()) / float64(limit) * 100.0
}

// Summary returns a human-readable breakdown of the context budget.
func (b *ContextBudget) Summary() map[string]any {
	return map[string]any{
		"max_tokens":       b.MaxTokens,
		"response_reserve": b.ResponseReserve,
		"used":             b.Used(),
		"available":        b.Available(),
		"utilization_pct":  b.UtilizationPct(),
		"sections": map[string]int64{
			"system_prompt": b.SystemPrompt,
			"history":       b.History,
			"tool_results":  b.ToolResults,
			"prefetch":      b.PrefetchCtx,
			"errors":        b.ErrorContext,
			"user_prompt":   b.UserPrompt,
		},
	}
}
