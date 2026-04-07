package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// RoutingRule maps task patterns to preferred providers/models.
type RoutingRule struct {
	Pattern  string   `json:"pattern"`  // glob-style match on task/prompt keywords
	Provider Provider `json:"provider"` // preferred provider
	Model    string   `json:"model"`    // preferred model (optional)
}

// FallbackChain defines the sequence of providers to try when one fails or is over capacity.
var DefaultFallbackChain = []Provider{
	ProviderClaude,
	ProviderGemini,
	ProviderCodex,
}

// CapacityFactors defines concurrent load-balancing limits per provider.
type CapacityFactors map[Provider]int

// DefaultCapacityLimits sets conservative defaults for concurrent sessions.
var DefaultCapacityLimits = CapacityFactors{
	ProviderClaude: 5,
	ProviderGemini: 10,
	ProviderCodex:  20,
}

// RoutingConfig holds model routing rules from .ralphrc.
type RoutingConfig struct {
	Rules         []RoutingRule   `json:"routing_rules"`
	FallbackChain []Provider      `json:"fallback_chain"`
	Capacity      CapacityFactors `json:"capacity_factors"`
}

// LoadRoutingConfig reads routing rules from .ralphrc or .ralph/routing.json.
func LoadRoutingConfig(repoPath string) (*RoutingConfig, error) {
	candidates := []string{
		filepath.Join(repoPath, ".ralph", "routing.json"),
	}
	var cfg *RoutingConfig
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var parsed RoutingConfig
		if err := json.Unmarshal(data, &parsed); err != nil {
			return nil, err
		}
		cfg = &parsed
		break
	}
	if cfg == nil {
		cfg = &RoutingConfig{}
	}

	if len(cfg.FallbackChain) == 0 {
		cfg.FallbackChain = DefaultFallbackChain
	}
	if cfg.Capacity == nil {
		cfg.Capacity = make(CapacityFactors)
		for k, v := range DefaultCapacityLimits {
			cfg.Capacity[k] = v
		}
	}
	return cfg, nil
}

// Match finds the first routing rule that matches the given prompt.
// Returns nil if no rule matches.
func (rc *RoutingConfig) Match(prompt string) *RoutingRule {
	lower := strings.ToLower(prompt)
	for i := range rc.Rules {
		if matchGlob(strings.ToLower(rc.Rules[i].Pattern), lower) {
			return &rc.Rules[i]
		}
	}
	return nil
}

// matchGlob performs simple glob matching with * wildcards.
func matchGlob(pattern, text string) bool {
	if pattern == "*" {
		return true
	}
	// Simple contains match for patterns like "*keyword*"
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		return strings.Contains(text, pattern[1:len(pattern)-1])
	}
	// Prefix match for "keyword*"
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(text, pattern[:len(pattern)-1])
	}
	// Suffix match for "*keyword"
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(text, pattern[1:])
	}
	// Exact match
	return pattern == text
}

// RouteByContextLength selects the optimal provider based on the number of input tokens.
// Returns ProviderGemini if inputTokens > 100000.
func RouteByContextLength(inputTokens int) Provider {
	if inputTokens > 100000 {
		return ProviderGemini
	}
	return DefaultPrimaryProvider()
}

// IsOverCapacity checks if a given provider has reached its concurrent load-balancing limit.
func (rc *RoutingConfig) IsOverCapacity(p Provider, currentActive int) bool {
	if rc == nil || rc.Capacity == nil {
		return false
	}
	limit, exists := rc.Capacity[p]
	if !exists {
		// If not configured, fall back to default limit
		limit, exists = DefaultCapacityLimits[p]
		if !exists {
			return false // No limit configured
		}
	}
	return currentActive >= limit
}