package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPromptCacheConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()

	if !cfg.Enabled {
		t.Error("expected Enabled=true by default")
	}
	if cfg.MinPrefixLen != 1024 {
		t.Errorf("expected MinPrefixLen=1024, got %d", cfg.MinPrefixLen)
	}
	if cfg.MaxCacheEntries != 100 {
		t.Errorf("expected MaxCacheEntries=100, got %d", cfg.MaxCacheEntries)
	}
	if cfg.CacheTTL != 300 {
		t.Errorf("expected CacheTTL=300, got %d", cfg.CacheTTL)
	}
}

func TestShouldCachePrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		provider Provider
		len      int
		want     bool
	}{
		{"claude at threshold", ProviderClaude, 1024, true},
		{"claude above threshold", ProviderClaude, 2000, true},
		{"claude below threshold", ProviderClaude, 1023, false},
		{"claude zero", ProviderClaude, 0, false},
		{"gemini at threshold", ProviderGemini, 2048, true},
		{"gemini above threshold", ProviderGemini, 5000, true},
		{"gemini below threshold", ProviderGemini, 2047, false},
		{"gemini zero", ProviderGemini, 0, false},
		{"codex below threshold", ProviderCodex, 100, false},
		{"codex at threshold", ProviderCodex, 1024, true},
		{"codex above threshold", ProviderCodex, 10000, true},
		{"unknown provider", Provider("unknown"), 10000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldCachePrompt(tt.provider, tt.len)
			if got != tt.want {
				t.Errorf("ShouldCachePrompt(%q, %d) = %v, want %v", tt.provider, tt.len, got, tt.want)
			}
		})
	}
}

func TestPromptCacheTracker_DisabledSkips(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	cfg.Enabled = false
	tracker := NewPromptCacheTracker(cfg)

	prompt := "## Instructions\n" + strings.Repeat("x", 2000)
	result, cacheable := tracker.AnalyzePrompt(t.TempDir(), ProviderClaude, prompt)

	if cacheable {
		t.Error("expected cacheable=false when config disabled")
	}
	if result != prompt {
		t.Error("expected original prompt returned unchanged when disabled")
	}

	stats := tracker.Stats()
	if stats.TotalSessions != 0 {
		t.Errorf("expected 0 total sessions when disabled, got %d", stats.TotalSessions)
	}
}

func TestPromptCacheTracker_ShortPromptSkipped(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	shortPrompt := "do something"
	result, cacheable := tracker.AnalyzePrompt(t.TempDir(), ProviderClaude, shortPrompt)

	if cacheable {
		t.Error("expected cacheable=false for short prompt")
	}
	if result != shortPrompt {
		t.Error("expected original prompt returned unchanged for short prompt")
	}

	stats := tracker.Stats()
	if stats.TotalSessions != 0 {
		t.Errorf("expected 0 total sessions for short prompt, got %d", stats.TotalSessions)
	}
}

func TestPromptCacheTracker_CacheMiss(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	// Build a prompt with stable headers long enough to exceed MinPrefixLen
	stableBlock := "## Instructions\n" + strings.Repeat("- follow rule\n", 200)
	prompt := stableBlock + "\ndo the task now please"

	repoDir := t.TempDir()
	_, cacheable := tracker.AnalyzePrompt(repoDir, ProviderClaude, prompt)

	if !cacheable {
		t.Error("expected cacheable=true for long prompt with stable prefix")
	}

	stats := tracker.Stats()
	if stats.CacheEligible != 1 {
		t.Errorf("expected CacheEligible=1, got %d", stats.CacheEligible)
	}
	if stats.EstimatedHits != 0 {
		t.Errorf("expected EstimatedHits=0 on first call (cache miss), got %d", stats.EstimatedHits)
	}
	if stats.TotalSessions != 1 {
		t.Errorf("expected TotalSessions=1, got %d", stats.TotalSessions)
	}
}

func TestPromptCacheTracker_CacheHit(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	stableBlock := "## Instructions\n" + strings.Repeat("- follow rule\n", 200)
	prompt := stableBlock + "\ndo the task now please"

	repoDir := t.TempDir()

	// First call: cache miss
	tracker.AnalyzePrompt(repoDir, ProviderClaude, prompt)

	// Second call with same prompt: cache hit
	_, cacheable := tracker.AnalyzePrompt(repoDir, ProviderClaude, prompt)
	if !cacheable {
		t.Error("expected cacheable=true on second call")
	}

	stats := tracker.Stats()
	if stats.EstimatedHits != 1 {
		t.Errorf("expected EstimatedHits=1 after second call, got %d", stats.EstimatedHits)
	}
	if stats.TotalSessions != 2 {
		t.Errorf("expected TotalSessions=2, got %d", stats.TotalSessions)
	}
	if stats.CacheEligible != 2 {
		t.Errorf("expected CacheEligible=2, got %d", stats.CacheEligible)
	}
	if stats.HitRate == 0 {
		t.Error("expected non-zero hit rate after cache hit")
	}
	if stats.EstimatedSaved != 0 {
		t.Errorf("expected no assumed savings for Claude without live cache-read evidence, got %f", stats.EstimatedSaved)
	}
}

func TestPromptCacheTracker_StatsAccumulate(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	repoDir := t.TempDir()

	// Three different long prompts with stable headers
	for i := range 3 {
		stableBlock := "## Instructions\n" + strings.Repeat("- rule line\n", 200)
		variable := strings.Repeat("x", i+1) // different variable content each time
		prompt := stableBlock + "\n" + variable
		tracker.AnalyzePrompt(repoDir, ProviderClaude, prompt)
	}

	stats := tracker.Stats()
	if stats.TotalSessions != 3 {
		t.Errorf("expected TotalSessions=3, got %d", stats.TotalSessions)
	}
	if stats.CacheEligible != 3 {
		t.Errorf("expected CacheEligible=3, got %d", stats.CacheEligible)
	}
}

func TestNewPromptCacheTracker(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	stats := tracker.Stats()
	if stats.TotalSessions != 0 {
		t.Errorf("expected TotalSessions=0 on new tracker, got %d", stats.TotalSessions)
	}
	if stats.CacheEligible != 0 {
		t.Errorf("expected CacheEligible=0 on new tracker, got %d", stats.CacheEligible)
	}
	if stats.EstimatedHits != 0 {
		t.Errorf("expected EstimatedHits=0 on new tracker, got %d", stats.EstimatedHits)
	}
	if stats.HitRate != 0 {
		t.Errorf("expected HitRate=0 on new tracker, got %f", stats.HitRate)
	}
	if stats.EstimatedSaved != 0 {
		t.Errorf("expected EstimatedSaved=0 on new tracker, got %f", stats.EstimatedSaved)
	}
}

func TestPromptCacheTracker_CLAUDEmdPrefix(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	// Create a repo dir with a CLAUDE.md file
	repoDir := t.TempDir()
	claudeContent := "# Project\n" + strings.Repeat("- important rule\n", 200)
	err := os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), []byte(claudeContent), 0644)
	if err != nil {
		t.Fatalf("failed to write CLAUDE.md: %v", err)
	}

	// Prompt that doesn't include CLAUDE.md content — it should get prepended as stable prefix
	prompt := "## Task\n" + strings.Repeat("- step\n", 200) + "\ndo the work"

	result, cacheable := tracker.AnalyzePrompt(repoDir, ProviderClaude, prompt)
	if !cacheable {
		t.Error("expected cacheable=true with CLAUDE.md prefix")
	}
	if !strings.Contains(result, "# Project") {
		t.Error("expected CLAUDE.md content to appear in result")
	}
}

func TestPromptCacheTracker_AGENTSmdPrefixForCodex(t *testing.T) {
	t.Parallel()

	cfg := DefaultPromptCacheConfig()
	tracker := NewPromptCacheTracker(cfg)

	repoDir := t.TempDir()
	agentsContent := "# Codex Instructions\n" + strings.Repeat("- keep plans concise\n", 200)
	err := os.WriteFile(filepath.Join(repoDir, "AGENTS.md"), []byte(agentsContent), 0644)
	if err != nil {
		t.Fatalf("failed to write AGENTS.md: %v", err)
	}

	prompt := "## Task\n" + strings.Repeat("- implement carefully\n", 200) + "\nfinish the work"
	result, cacheable := tracker.AnalyzePrompt(repoDir, ProviderCodex, prompt)
	if !cacheable {
		t.Error("expected cacheable=true with AGENTS.md prefix for codex")
	}
	if !strings.Contains(result, "# Codex Instructions") {
		t.Error("expected AGENTS.md content to appear in result")
	}
}
