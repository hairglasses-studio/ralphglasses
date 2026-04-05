package session

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// CacheManagerConfig configures the cross-session cache manager.
type CacheManagerConfig struct {
	// MinPrefixLen is the minimum character length for a prefix to be cacheable.
	MinPrefixLen int `json:"min_prefix_len"`
	// MaxEntries caps the number of tracked prefixes.
	MaxEntries int `json:"max_entries"`
	// TTL is how long a prefix stays "warm" without being reused.
	TTL time.Duration `json:"ttl"`
	// SavingsRate is the fraction of input cost saved on a cache hit (0.0-1.0).
	// Keep this conservative. Claude resumed-session cache reads have regressed
	// in the field, so default assumed savings are disabled until live reads are observed.
	SavingsRate map[Provider]float64 `json:"savings_rate"`
}

// DefaultCacheManagerConfig returns production defaults.
func DefaultCacheManagerConfig() CacheManagerConfig {
	return CacheManagerConfig{
		MinPrefixLen: 512,
		MaxEntries:   200,
		TTL:          5 * time.Minute,
		SavingsRate: map[Provider]float64{
			ProviderClaude: 0.0,
			ProviderGemini: 0.75,
			ProviderCodex:  0.50,
		},
	}
}

// CachedPrefix represents a tracked prompt prefix.
type CachedPrefix struct {
	Hash       string    `json:"hash"`
	Provider   Provider  `json:"provider"`
	Length     int       `json:"length"`      // character length of the prefix
	TokenCount int64     `json:"token_count"` // estimated tokens
	HitCount   int       `json:"hit_count"`
	FirstSeen  time.Time `json:"first_seen"`
	LastHit    time.Time `json:"last_hit"`
}

// CacheManagerStats tracks aggregate cache metrics.
type CacheManagerStats struct {
	TotalLookups     int     `json:"total_lookups"`
	CacheHits        int     `json:"cache_hits"`
	CacheMisses      int     `json:"cache_misses"`
	HitRatePct       float64 `json:"hit_rate_pct"`
	EstimatedSavings float64 `json:"estimated_savings_usd"`
	ActivePrefixes   int     `json:"active_prefixes"`
	ExpiredPrefixes  int     `json:"expired_prefixes"`
}

// CacheManager manages prompt prefix caching across sessions. It identifies
// commonly reused prompt prefixes, tracks hit rates, and estimates cost savings.
type CacheManager struct {
	mu       sync.Mutex
	config   CacheManagerConfig
	prefixes map[string]*CachedPrefix // hash -> prefix
	stats    CacheManagerStats
}

func assumedCacheSavingsRate(provider Provider) float64 {
	switch provider {
	case ProviderGemini:
		return 0.75
	case ProviderCodex:
		return 0.50
	default:
		return 0.0
	}
}

// NewCacheManager creates a new cross-session cache manager.
func NewCacheManager(config CacheManagerConfig) *CacheManager {
	return &CacheManager{
		config:   config,
		prefixes: make(map[string]*CachedPrefix),
	}
}

// LookupPrefix checks whether the prompt has a cacheable prefix that has been
// seen before. Returns the prefix length suitable for caching and whether it
// was a hit (previously seen and still warm).
func (cm *CacheManager) LookupPrefix(provider Provider, prompt string) (prefixLen int, hit bool) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.stats.TotalLookups++

	prefix := extractCacheablePrefix(prompt, cm.config.MinPrefixLen)
	if prefix == "" {
		cm.stats.CacheMisses++
		return 0, false
	}

	hash := hashCachePrefix(prefix)
	entry, exists := cm.prefixes[hash]
	now := time.Now()

	if exists && now.Sub(entry.LastHit) < cm.config.TTL {
		// Cache hit — prefix is still warm.
		entry.HitCount++
		entry.LastHit = now
		cm.stats.CacheHits++
		cm.updateHitRate()

		// Estimate savings.
		rate := cm.config.SavingsRate[provider]
		inputPrice, _ := providerTokenPricing(provider)
		savedTokens := entry.TokenCount
		cm.stats.EstimatedSavings += float64(savedTokens) / 1_000_000 * inputPrice * rate

		return entry.Length, true
	}

	// Miss — register or refresh the prefix.
	cm.stats.CacheMisses++
	tokenEst := EstimateTokensForProvider(prefix, provider)

	if exists {
		entry.LastHit = now
		entry.HitCount = 0 // reset after expiry
		entry.TokenCount = tokenEst
	} else {
		cm.evictIfNeeded()
		cm.prefixes[hash] = &CachedPrefix{
			Hash:       hash,
			Provider:   provider,
			Length:     len(prefix),
			TokenCount: tokenEst,
			HitCount:   0,
			FirstSeen:  now,
			LastHit:    now,
		}
	}

	cm.updateHitRate()
	return len(prefix), false
}

// RecordPrefix explicitly registers a prefix as cached for a provider.
// Useful when the caller knows the exact cacheable boundary.
func (cm *CacheManager) RecordPrefix(provider Provider, prefix string) {
	if len(prefix) < cm.config.MinPrefixLen {
		return
	}

	hash := hashCachePrefix(prefix)
	tokenEst := EstimateTokensForProvider(prefix, provider)
	now := time.Now()

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if entry, ok := cm.prefixes[hash]; ok {
		entry.LastHit = now
		entry.TokenCount = tokenEst
		return
	}

	cm.evictIfNeeded()
	cm.prefixes[hash] = &CachedPrefix{
		Hash:       hash,
		Provider:   provider,
		Length:     len(prefix),
		TokenCount: tokenEst,
		FirstSeen:  now,
		LastHit:    now,
	}
}

// Stats returns current cache statistics.
func (cm *CacheManager) Stats() CacheManagerStats {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.stats.ActivePrefixes = cm.countActive()
	cm.stats.ExpiredPrefixes = len(cm.prefixes) - cm.stats.ActivePrefixes
	return cm.stats
}

// TopPrefixes returns the N most-hit cached prefixes, sorted by hit count.
func (cm *CacheManager) TopPrefixes(n int) []CachedPrefix {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	entries := make([]CachedPrefix, 0, len(cm.prefixes))
	for _, p := range cm.prefixes {
		entries = append(entries, *p)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].HitCount > entries[j].HitCount
	})

	if n > len(entries) {
		n = len(entries)
	}
	return entries[:n]
}

// EstimateSavings calculates the projected USD savings if a prompt with
// the given token count hits the cache for the specified provider.
func (cm *CacheManager) EstimateSavings(provider Provider, tokenCount int64) float64 {
	rate := cm.config.SavingsRate[provider]
	inputPrice, _ := providerTokenPricing(provider)
	return float64(tokenCount) / 1_000_000 * inputPrice * rate
}

// Purge removes all expired entries.
func (cm *CacheManager) Purge() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	removed := 0
	for hash, entry := range cm.prefixes {
		if now.Sub(entry.LastHit) >= cm.config.TTL {
			delete(cm.prefixes, hash)
			removed++
		}
	}
	cm.stats.ExpiredPrefixes = 0
	return removed
}

// Reset clears all cache state.
func (cm *CacheManager) Reset() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.prefixes = make(map[string]*CachedPrefix)
	cm.stats = CacheManagerStats{}
}

// updateHitRate recalculates hit rate. Must be called with mu held.
func (cm *CacheManager) updateHitRate() {
	if cm.stats.TotalLookups > 0 {
		cm.stats.HitRatePct = float64(cm.stats.CacheHits) / float64(cm.stats.TotalLookups) * 100
	}
}

// countActive returns the number of non-expired prefixes. Must be called with mu held.
func (cm *CacheManager) countActive() int {
	now := time.Now()
	count := 0
	for _, entry := range cm.prefixes {
		if now.Sub(entry.LastHit) < cm.config.TTL {
			count++
		}
	}
	return count
}

// evictIfNeeded removes the oldest prefix if at capacity. Must be called with mu held.
func (cm *CacheManager) evictIfNeeded() {
	if len(cm.prefixes) < cm.config.MaxEntries {
		return
	}

	// Evict the least recently hit entry.
	var oldestHash string
	var oldestTime time.Time
	first := true
	for hash, entry := range cm.prefixes {
		if first || entry.LastHit.Before(oldestTime) {
			oldestHash = hash
			oldestTime = entry.LastHit
			first = false
		}
	}
	if oldestHash != "" {
		delete(cm.prefixes, oldestHash)
	}
}

// extractCacheablePrefix extracts the stable, cacheable portion of a prompt.
// It looks for system instructions, tool definitions, and project context
// that are unlikely to change between requests.
func extractCacheablePrefix(prompt string, minLen int) string {
	if len(prompt) < minLen {
		return ""
	}

	lines := strings.Split(prompt, "\n")
	var stableLines []string
	inStableSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect stable content: headers, instructions, constraints.
		if isStableLine(trimmed) {
			inStableSection = true
		} else if trimmed == "" {
			// Blank lines preserve section membership.
		} else if !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "*") && !strings.HasPrefix(trimmed, "#") {
			inStableSection = false
		}

		if inStableSection || trimmed == "" {
			stableLines = append(stableLines, line)
		}
	}

	prefix := strings.TrimSpace(strings.Join(stableLines, "\n"))
	if len(prefix) < minLen {
		// If we couldn't extract enough stable content, use the first
		// minLen characters as-is (many prompts have stable prefixes).
		if len(prompt) >= minLen {
			return prompt[:minLen]
		}
		return ""
	}
	return prefix
}

// isStableLine returns true if a line likely belongs to stable prompt content.
func isStableLine(trimmed string) bool {
	if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
		return true
	}
	stableMarkers := []string{
		"Instructions:", "Constraints:", "System:", "Context:",
		"You are", "Your role", "CLAUDE.md", "AGENTS.md",
		"<system>", "</system>", "<instructions>", "</instructions>",
	}
	for _, marker := range stableMarkers {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	return false
}

// hashCachePrefix produces a hex hash for cache lookup.
func hashCachePrefix(prefix string) string {
	h := sha256.Sum256([]byte(prefix))
	return hex.EncodeToString(h[:10])
}
