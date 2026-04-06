package session

import (
	"sync"
)

// ContextBudget tracks per-session token usage against a model's context
// window limit. It is thread-safe and supports warning/critical thresholds
// for proactive context management (12-Factor Agents Factor 3).
type ContextBudget struct {
	mu                sync.RWMutex
	ModelLimit        int     `json:"model_limit"`
	UsedTokens        int     `json:"used_tokens"`
	WarningThreshold  float64 `json:"warning_threshold"`
	CriticalThreshold float64 `json:"critical_threshold"`
}

// Default context window sizes by provider.
const (
	DefaultClaudeLimit = 200000
	DefaultGeminiLimit = 1000000
	DefaultCodexLimit  = 128000
)

// ModelLimitForProvider returns the default context window size for a provider.
func ModelLimitForProvider(p Provider) int {
	switch p {
	case ProviderClaude:
		return DefaultClaudeLimit
	case ProviderGemini:
		return DefaultGeminiLimit
	case ProviderCodex:
		return DefaultCodexLimit
	default:
		return DefaultCodexLimit
	}
}

// NewContextBudget creates a ContextBudget for the given model token limit
// with default warning (0.8) and critical (0.95) thresholds.
func NewContextBudget(modelLimit int) *ContextBudget {
	return &ContextBudget{
		ModelLimit:        modelLimit,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
	}
}

// Record adds tokens to the usage counter.
func (b *ContextBudget) Record(tokens int) {
	b.mu.Lock()
	b.UsedTokens += tokens
	b.mu.Unlock()
}

// Usage returns the current used tokens, model limit, and usage percentage.
func (b *ContextBudget) Usage() (used int, limit int, percent float64) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	used = b.UsedTokens
	limit = b.ModelLimit
	if limit > 0 {
		percent = float64(used) / float64(limit)
	}
	return
}

// IsWarning returns true if usage exceeds the warning threshold.
func (b *ContextBudget) IsWarning() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.ModelLimit <= 0 {
		return false
	}
	return float64(b.UsedTokens)/float64(b.ModelLimit) > b.WarningThreshold
}

// IsCritical returns true if usage exceeds the critical threshold.
func (b *ContextBudget) IsCritical() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.ModelLimit <= 0 {
		return false
	}
	return float64(b.UsedTokens)/float64(b.ModelLimit) > b.CriticalThreshold
}

// Reset zeroes out the usage counter.
func (b *ContextBudget) Reset() {
	b.mu.Lock()
	b.UsedTokens = 0
	b.mu.Unlock()
}

// Remaining returns the number of tokens remaining before the model limit.
func (b *ContextBudget) Remaining() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	rem := b.ModelLimit - b.UsedTokens
	if rem < 0 {
		return 0
	}
	return rem
}

// Status returns "ok", "warning", or "critical" based on current usage.
func (b *ContextBudget) Status() string {
	if b.IsCritical() {
		return "critical"
	}
	if b.IsWarning() {
		return "warning"
	}
	return "ok"
}
