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
	Enabled         bool `json:"enabled"`
	MinPrefixLen    int  `json:"min_prefix_len"`    // minimum cacheable prefix length (default 1024)
	MaxCacheEntries int  `json:"max_cache_entries"` // max entries in the hit-rate tracker
	CacheTTL        int  `json:"cache_ttl_seconds"` // how long to consider a prefix "warm" (default 300)
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
	TotalSessions  int     `json:"total_sessions"`
	CacheEligible  int     `json:"cache_eligible"` // sessions with cacheable prefixes
	EstimatedHits  int     `json:"estimated_hits"` // sessions that reused a warm prefix
	HitRate        float64 `json:"hit_rate_pct"`
	EstimatedSaved float64 `json:"estimated_saved_usd"`
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
// the optimized prompt with stable sections first for maximum cache hits.
// Returns the reordered prompt and whether caching is beneficial.
func (t *PromptCacheTracker) AnalyzePrompt(repoPath string, provider Provider, prompt string) (string, bool) {
	if !t.config.Enabled || len(prompt) < t.config.MinPrefixLen || !ShouldCachePrompt(provider, len(prompt)) {
		return prompt, false
	}

	optimizedPrompt, cacheBoundaryOffset := buildSectionedPrompt(repoPath, prompt)
	if cacheBoundaryOffset < t.config.MinPrefixLen {
		return prompt, false
	}

	hash := hashPrefix(optimizedPrompt[:cacheBoundaryOffset])

	t.mu.Lock()
	defer t.mu.Unlock()

	t.stats.TotalSessions++
	t.stats.CacheEligible++

	entry, exists := t.entries[hash]
	if exists && time.Since(entry.lastSeen) < time.Duration(t.config.CacheTTL)*time.Second {
		entry.hitCount++
		entry.lastSeen = time.Now()
		t.stats.EstimatedHits++
		if savingsRate := assumedCacheSavingsRate(provider); savingsRate > 0 {
			estimatedTokens := float64(cacheBoundaryOffset) / 4.0 // rough token estimate
			inputPrice, _ := providerTokenPricing(provider)
			t.stats.EstimatedSaved += estimatedTokens / 1_000_000 * inputPrice * savingsRate
		}
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

	return optimizedPrompt, true
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
	switch provider {
	case ProviderClaude:
		return promptLen >= 1024
	case ProviderGemini:
		return promptLen >= 2048
	case ProviderCodex:
		return promptLen >= 1024
	default:
		return false
	}
}

type promptCacheSections struct {
	repoInstructions string
	stablePrompt     string
	variablePrompt   string
}

// buildSectionedPrompt preserves the legacy stable-prefix byte layout while
// making the cacheable and variable sections explicit for hashing.
func buildSectionedPrompt(repoPath string, prompt string) (string, int) {
	sections := classifyPromptSections(repoPath, prompt)
	var stableParts []string
	if sections.repoInstructions != "" {
		stableParts = append(stableParts, sections.repoInstructions)
	}
	if sections.stablePrompt != "" {
		stableParts = append(stableParts, sections.stablePrompt)
	}

	stablePrefix := strings.TrimSpace(strings.Join(stableParts, "\n"))
	if stablePrefix == "" {
		return prompt, 0
	}
	if sections.variablePrompt == "" {
		return stablePrefix, len(stablePrefix)
	}
	return stablePrefix + "\n\n" + sections.variablePrompt, len(stablePrefix)
}

// classifyPromptSections separates the stable (cacheable) prefix from variable
// content. Stable content includes repo instruction files, system prompts, and
// tool/schema-style sections. Variable content includes task-specific context.
func classifyPromptSections(repoPath string, prompt string) promptCacheSections {
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

	return promptCacheSections{
		repoInstructions: readRepoInstructionPrefix(repoPath, prompt),
		stablePrompt:     strings.TrimSpace(strings.Join(stableParts, "\n")),
		variablePrompt:   strings.TrimSpace(strings.Join(variableParts, "\n")),
	}
}

// readRepoInstructionPrefix prepends repo-scoped instruction files when their
// contents are not already present in the prompt.
func readRepoInstructionPrefix(repoPath string, prompt string) string {
	var repoInstructions []string
	for _, name := range []string{"AGENTS.md", "CLAUDE.md", "GEMINI.md"} {
		path := filepath.Join(repoPath, name)
		content, err := os.ReadFile(path)
		if err != nil || len(content) == 0 {
			continue
		}
		snippet := string(content[:min(100, len(content))])
		if !strings.Contains(prompt, snippet) {
			repoInstructions = append([]string{string(content)}, repoInstructions...)
		}
	}
	return strings.TrimSpace(strings.Join(repoInstructions, "\n"))
}

func hashPrefix(prefix string) string {
	h := sha256.Sum256([]byte(prefix))
	return hex.EncodeToString(h[:8])
}
