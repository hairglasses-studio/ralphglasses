package session

import (
	"strings"
	"sync"
	"time"
)

// TokenRate defines the estimated tokens-per-word ratio for a provider.
// These are heuristic approximations based on typical tokenizer behavior.
var tokenRates = map[Provider]float64{
	ProviderClaude: 1.3,
	ProviderGemini: 1.2,
	ProviderCodex:  1.3,
}

// DefaultTokenRate is used when the provider is unknown.
const DefaultTokenRate = 1.3

// TokenUsage records cumulative token counts for a session.
type TokenUsage struct {
	SessionID    string   `json:"session_id"`
	Provider     Provider `json:"provider"`
	InputTokens  int64    `json:"input_tokens"`
	OutputTokens int64    `json:"output_tokens"`
	TurnCount    int      `json:"turn_count"`
	LastUpdated  time.Time `json:"last_updated"`
}

// TotalTokens returns the sum of input and output tokens.
func (u *TokenUsage) TotalTokens() int64 {
	return u.InputTokens + u.OutputTokens
}

// TokenCounter estimates and tracks token usage per session using
// heuristic word-based counting. No external tokenizer dependencies.
type TokenCounter struct {
	mu       sync.Mutex
	sessions map[string]*TokenUsage
}

// NewTokenCounter creates a new token counter.
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{
		sessions: make(map[string]*TokenUsage),
	}
}

// EstimateTokens returns an estimated token count for the given text
// using the provider's tokens-per-word ratio.
func EstimateTokensForProvider(text string, provider Provider) int64 {
	if text == "" {
		return 0
	}
	words := countWords(text)
	rate := tokenRates[provider]
	if rate == 0 {
		rate = DefaultTokenRate
	}
	return int64(float64(words) * rate)
}

// RecordInput adds estimated input tokens for a session turn.
func (tc *TokenCounter) RecordInput(sessionID string, provider Provider, prompt string) int64 {
	tokens := EstimateTokensForProvider(prompt, provider)

	tc.mu.Lock()
	defer tc.mu.Unlock()

	usage := tc.getOrCreate(sessionID, provider)
	usage.InputTokens += tokens
	usage.TurnCount++
	usage.LastUpdated = time.Now()
	return tokens
}

// RecordOutput adds estimated output tokens for a session turn.
func (tc *TokenCounter) RecordOutput(sessionID string, provider Provider, output string) int64 {
	tokens := EstimateTokensForProvider(output, provider)

	tc.mu.Lock()
	defer tc.mu.Unlock()

	usage := tc.getOrCreate(sessionID, provider)
	usage.OutputTokens += tokens
	usage.LastUpdated = time.Now()
	return tokens
}

// GetUsage returns the token usage for a session, or nil if not tracked.
func (tc *TokenCounter) GetUsage(sessionID string) *TokenUsage {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	usage, ok := tc.sessions[sessionID]
	if !ok {
		return nil
	}
	// Return a copy.
	u := *usage
	return &u
}

// AllUsage returns token usage for all tracked sessions.
func (tc *TokenCounter) AllUsage() []TokenUsage {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	result := make([]TokenUsage, 0, len(tc.sessions))
	for _, u := range tc.sessions {
		result = append(result, *u)
	}
	return result
}

// TotalTokens returns the aggregate token count across all sessions.
func (tc *TokenCounter) TotalTokens() int64 {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	var total int64
	for _, u := range tc.sessions {
		total += u.TotalTokens()
	}
	return total
}

// EstimateCostUSD returns a rough cost estimate based on token counts
// and provider pricing. Uses approximate per-million-token rates.
func (tc *TokenCounter) EstimateCostUSD(sessionID string) float64 {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	usage, ok := tc.sessions[sessionID]
	if !ok {
		return 0
	}

	inputRate, outputRate := providerTokenPricing(usage.Provider)
	inputCost := float64(usage.InputTokens) / 1_000_000 * inputRate
	outputCost := float64(usage.OutputTokens) / 1_000_000 * outputRate
	return inputCost + outputCost
}

// Reset removes all tracked sessions.
func (tc *TokenCounter) Reset() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.sessions = make(map[string]*TokenUsage)
}

// RemoveSession removes tracking data for a specific session.
func (tc *TokenCounter) RemoveSession(sessionID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	delete(tc.sessions, sessionID)
}

// getOrCreate returns or creates a usage entry. Must be called with mu held.
func (tc *TokenCounter) getOrCreate(sessionID string, provider Provider) *TokenUsage {
	usage, ok := tc.sessions[sessionID]
	if !ok {
		usage = &TokenUsage{
			SessionID: sessionID,
			Provider:  provider,
		}
		tc.sessions[sessionID] = usage
	}
	return usage
}

// providerTokenPricing returns approximate input and output per-million-token
// prices in USD. These are rough heuristics, not live pricing.
func providerTokenPricing(provider Provider) (inputPerM, outputPerM float64) {
	switch provider {
	case ProviderClaude:
		return 3.00, 15.00 // Claude 3.5 Sonnet-class pricing
	case ProviderGemini:
		return 0.50, 1.50 // Gemini 1.5 Pro pricing
	case ProviderCodex:
		return 2.50, 10.00 // GPT-4o-class pricing
	default:
		return 3.00, 15.00
	}
}

// countWords splits text on whitespace and returns the word count.
func countWords(text string) int {
	return len(strings.Fields(text))
}
