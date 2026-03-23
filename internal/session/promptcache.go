package session

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PromptCacheConfig configures prompt caching behavior.
type PromptCacheConfig struct {
	Enabled         bool    `json:"enabled"`
	MinPrefixLen    int     `json:"min_prefix_len"`    // minimum cacheable prefix length (default 1024)
	MaxCacheEntries int     `json:"max_cache_entries"` // max entries in the hit-rate tracker
	CacheTTL        int     `json:"cache_ttl_seconds"` // how long to consider a prefix "warm" (default 300)
}

// DefaultPromptCacheConfig returns the default caching configuration.
func DefaultPromptCacheConfig() PromptCacheConfig {
	return PromptCacheConfig{
		Enabled:         true,
		MinPrefixLen:    1024,
		MaxCacheEntries: 100,
		CacheTTL:        300,
	}
}

// PromptCacheStats tracks cache hit rates.
type PromptCacheStats struct {
	TotalSessions   int     `json:"total_sessions"`
	CacheEligible   int     `json:"cache_eligible"`   // sessions with cacheable prefixes
	EstimatedHits   int     `json:"estimated_hits"`    // sessions that reused a warm prefix
	HitRate         float64 `json:"hit_rate_pct"`
	EstimatedSaved  float64 `json:"estimated_saved_usd"`
}

// PromptCacheTracker tracks cacheable prompt prefixes across sessions.
type PromptCacheTracker struct {
	mu      sync.Mutex
	config  PromptCacheConfig
	entries map[string]*cacheEntry // prefix hash → entry
	stats   PromptCacheStats
}

type cacheEntry struct {
	prefixHash string
	repoPath   string
	provider   Provider
	lastSeen   time.Time
	hitCount   int
}

// NewPromptCacheTracker creates a cache tracker.
func NewPromptCacheTracker(config PromptCacheConfig) *PromptCacheTracker {
	return &PromptCacheTracker{
		config:  config,
		entries: make(map[string]*cacheEntry),
	}
}

// AnalyzePrompt checks if a prompt has a cacheable prefix and returns
// the optimized prompt with stable prefix first for maximum cache hits.
// Returns the (potentially reordered) prompt and whether caching is beneficial.
func (t *PromptCacheTracker) AnalyzePrompt(repoPath string, provider Provider, prompt string) (string, bool) {
	if !t.config.Enabled || len(prompt) < t.config.MinPrefixLen {
		return prompt, false
	}

	// Extract cacheable components (system prompt, tool definitions, CLAUDE.md)
	prefix, variable := splitCacheablePrefix(repoPath, prompt)
	if len(prefix) < t.config.MinPrefixLen {
		return prompt, false
	}

	hash := hashPrefix(prefix)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats.TotalSessions++
	t.stats.CacheEligible++

	entry, exists := t.entries[hash]
	if exists && time.Since(entry.lastSeen) < time.Duration(t.config.CacheTTL)*time.Second {
		entry.hitCount++
		entry.lastSeen = time.Now()
		t.stats.EstimatedHits++
		// 90% savings on cached tokens at $3/M input → ~$2.70/M saved
		estimatedTokens := float64(len(prefix)) / 4.0 // rough token estimate
		t.stats.EstimatedSaved += estimatedTokens / 1_000_000 * 2.70
	} else {
		t.entries[hash] = &cacheEntry{
			prefixHash: hash,
			repoPath:   repoPath,
			provider:   provider,
			lastSeen:   time.Now(),
			hitCount:   0,
		}
	}

	if t.stats.CacheEligible > 0 {
		t.stats.HitRate = float64(t.stats.EstimatedHits) / float64(t.stats.CacheEligible) * 100
	}

	// Reorder: stable prefix first, then variable content
	if variable != "" {
		return prefix + "\n\n" + variable, true
	}
	return prompt, true
}

// Stats returns current cache statistics.
func (t *PromptCacheTracker) Stats() PromptCacheStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stats
}

// ShouldCachePrompt returns true if the provider supports prompt caching
// and the prompt is likely to benefit from it.
func ShouldCachePrompt(provider Provider, promptLen int) bool {
	// Claude supports prompt caching natively
	// Gemini has implicit caching
	// Codex doesn't support it
	switch provider {
	case ProviderClaude:
		return promptLen >= 1024
	case ProviderGemini:
		return promptLen >= 2048
	default:
		return false
	}
}

// splitCacheablePrefix separates the stable (cacheable) prefix from variable content.
// Stable content: CLAUDE.md, system prompts, tool schemas.
// Variable content: user-specific prompt, dynamic context.
func splitCacheablePrefix(repoPath string, prompt string) (prefix, variable string) {
	var stableParts []string
	var variableParts []string

	lines := strings.Split(prompt, "\n")
	inStableBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect stable content markers
		isStable := false
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "# ") {
			// Section headers from CLAUDE.md or system prompts are stable
			isStable = true
			inStableBlock = true
		}
		if strings.HasPrefix(trimmed, "- ") && inStableBlock {
			isStable = true
		}
		if trimmed == "" {
			// Empty lines preserve block membership
			if inStableBlock {
				isStable = true
			}
		}
		if strings.Contains(trimmed, "Constraints:") || strings.Contains(trimmed, "Instructions:") {
			isStable = true
			inStableBlock = true
		}

		// Non-stable content breaks the stable block
		if !isStable && !strings.HasPrefix(trimmed, "#") {
			inStableBlock = false
		}

		if isStable {
			stableParts = append(stableParts, line)
		} else {
			variableParts = append(variableParts, line)
		}
	}

	// Also try to include CLAUDE.md content as stable prefix
	claudeMDPath := filepath.Join(repoPath, "CLAUDE.md")
	if claudeContent, err := os.ReadFile(claudeMDPath); err == nil && len(claudeContent) > 0 {
		// If CLAUDE.md content appears in the prompt, it's already accounted for
		// Otherwise, prepend it as stable prefix
		if !strings.Contains(prompt, string(claudeContent[:min(100, len(claudeContent))])) {
			stableParts = append([]string{string(claudeContent)}, stableParts...)
		}
	}

	prefix = strings.TrimSpace(strings.Join(stableParts, "\n"))
	variable = strings.TrimSpace(strings.Join(variableParts, "\n"))
	return prefix, variable
}

func hashPrefix(prefix string) string {
	h := sha256.Sum256([]byte(prefix))
	return hex.EncodeToString(h[:8])
}
