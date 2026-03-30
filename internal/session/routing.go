package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// RoutingRule maps task patterns to preferred providers/models.
type RoutingRule struct {
	Pattern  string `json:"pattern"`  // glob-style match on task/prompt keywords
	Provider string `json:"provider"` // preferred provider
	Model    string `json:"model"`    // preferred model (optional)
}

// RoutingConfig holds model routing rules from .ralphrc.
type RoutingConfig struct {
	Rules []RoutingRule `json:"routing_rules"`
}

// LoadRoutingConfig reads routing rules from .ralphrc or .ralph/routing.json.
func LoadRoutingConfig(repoPath string) (*RoutingConfig, error) {
	candidates := []string{
		filepath.Join(repoPath, ".ralph", "routing.json"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var cfg RoutingConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	return &RoutingConfig{}, nil
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
