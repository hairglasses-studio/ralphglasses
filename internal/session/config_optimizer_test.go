package session

import (
	"encoding/json"
	"testing"
)

func TestDefaultConfigOptimizerConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	if cfg.Exploration != 0.15 {
		t.Fatalf("expected Exploration=0.15, got %f", cfg.Exploration)
	}
	if cfg.MinTrials != 5 {
		t.Fatalf("expected MinTrials=5, got %d", cfg.MinTrials)
	}
	if cfg.WindowSize != 20 {
		t.Fatalf("expected WindowSize=20, got %d", cfg.WindowSize)
	}
}

func TestNewConfigOptimizer_Defaults(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(ConfigOptimizerConfig{})
	if co.exploration != 0.15 {
		t.Fatalf("expected default exploration=0.15, got %f", co.exploration)
	}
	if co.minTrials != 5 {
		t.Fatalf("expected default minTrials=5, got %d", co.minTrials)
	}
}

func TestConfigOptimizer_RecordOutcome(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(DefaultConfigOptimizerConfig())

	co.RecordOutcome("claude", "feature", true, 0.50, 30.0)
	co.RecordOutcome("claude", "feature", false, 0.80, 60.0)

	stats := co.ArmStats()
	arm, ok := stats["claude:feature"]
	if !ok {
		t.Fatal("expected arm claude:feature")
	}
	if arm.Trials != 2 {
		t.Fatalf("expected 2 trials, got %d", arm.Trials)
	}
	if arm.Successes != 1 {
		t.Fatalf("expected 1 success, got %d", arm.Successes)
	}
	if arm.SuccessRate != 0.5 {
		t.Fatalf("expected success rate 0.5, got %f", arm.SuccessRate)
	}
}

func TestConfigOptimizer_RecordOutcome_WindowTrim(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	cfg.WindowSize = 5
	co := NewConfigOptimizer(cfg)

	for range 10 {
		co.RecordOutcome("claude", "test", true, 0.10, 10.0)
	}

	co.mu.Lock()
	arm := co.arms["claude:test"]
	windowLen := len(arm.recentOutcomes)
	co.mu.Unlock()

	// Window should be trimmed, but total trials is 10.
	if windowLen > 5 {
		t.Fatalf("expected window <= 5, got %d", windowLen)
	}
}

func TestConfigOptimizer_SelectProvider_SingleCandidate(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(DefaultConfigOptimizerConfig())
	provider, explored := co.SelectProvider("feature", []string{"claude"})
	if provider != "claude" {
		t.Fatalf("expected claude, got %s", provider)
	}
	if explored {
		t.Fatal("expected explored=false for single candidate")
	}
}

func TestConfigOptimizer_SelectProvider_Empty(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(DefaultConfigOptimizerConfig())
	provider, _ := co.SelectProvider("feature", nil)
	if provider != "" {
		t.Fatalf("expected empty, got %s", provider)
	}
}

func TestConfigOptimizer_SelectProvider_ExploresUnderMinTrials(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	cfg.Exploration = 0 // no random exploration
	co := NewConfigOptimizer(cfg)

	// Record fewer than minTrials for one provider.
	co.RecordOutcome("claude", "feature", true, 0.5, 30)

	// With insufficient data, it should explore.
	_, explored := co.SelectProvider("feature", []string{"claude", "gemini"})
	if !explored {
		t.Fatal("expected exploration when under min trials")
	}
}

func TestConfigOptimizer_SelectProvider_ExploitsBestArm(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	cfg.Exploration = 0 // no random exploration
	cfg.MinTrials = 2
	co := NewConfigOptimizer(cfg)

	// Claude: 100% success, cheap.
	for range 5 {
		co.RecordOutcome("claude", "feature", true, 0.10, 10)
	}
	// Gemini: 20% success, expensive.
	for i := range 5 {
		co.RecordOutcome("gemini", "feature", i == 0, 1.00, 60)
	}

	provider, explored := co.SelectProvider("feature", []string{"claude", "gemini"})
	if explored {
		t.Fatal("expected exploitation, not exploration")
	}
	if provider != "claude" {
		t.Fatalf("expected claude (better arm), got %s", provider)
	}
}

func TestConfigOptimizer_SuggestChanges(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	cfg.MinTrials = 2
	co := NewConfigOptimizer(cfg)

	// Create a clear winner.
	for range 10 {
		co.RecordOutcome("claude", "feature", true, 0.10, 10)
	}
	for range 10 {
		co.RecordOutcome("gemini", "feature", false, 2.00, 120)
	}

	suggestions := co.SuggestChanges()
	if len(suggestions) == 0 {
		t.Fatal("expected at least one suggestion")
	}

	found := false
	for _, s := range suggestions {
		if s.Category == "provider" && s.Suggested == "claude" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected provider suggestion to switch to claude")
	}
}

func TestConfigOptimizer_SuggestChanges_NoSuggestionWhenSimilar(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	cfg.MinTrials = 2
	co := NewConfigOptimizer(cfg)

	// Both providers perform similarly.
	for range 5 {
		co.RecordOutcome("claude", "feature", true, 0.50, 30)
		co.RecordOutcome("gemini", "feature", true, 0.50, 30)
	}

	suggestions := co.SuggestChanges()
	if len(suggestions) != 0 {
		t.Fatalf("expected 0 suggestions for similar arms, got %d", len(suggestions))
	}
}

func TestConfigOptimizer_PendingSuggestions(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfigOptimizerConfig()
	cfg.MinTrials = 2
	co := NewConfigOptimizer(cfg)

	for range 5 {
		co.RecordOutcome("claude", "fix", true, 0.10, 10)
		co.RecordOutcome("gemini", "fix", false, 2.00, 120)
	}
	co.SuggestChanges()

	pending := co.PendingSuggestions()
	if len(pending) == 0 {
		t.Fatal("expected pending suggestions")
	}
	// Second call should be empty.
	if len(co.PendingSuggestions()) != 0 {
		t.Fatal("expected empty after drain")
	}
}

func TestConfigOptimizer_ArmStats(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(DefaultConfigOptimizerConfig())
	co.RecordOutcome("claude", "test", true, 1.0, 30)

	stats := co.ArmStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 arm, got %d", len(stats))
	}
	arm := stats["claude:test"]
	if arm.Provider != "claude" {
		t.Fatalf("expected provider=claude, got %s", arm.Provider)
	}
	if arm.TaskType != "test" {
		t.Fatalf("expected task_type=test, got %s", arm.TaskType)
	}
}

func TestConfigOptimizer_MarshalJSON(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(DefaultConfigOptimizerConfig())
	co.RecordOutcome("claude", "test", true, 0.50, 30)

	data, err := json.Marshal(co)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["arms"]; !ok {
		t.Fatal("expected 'arms' key in JSON")
	}
}

func TestArmScore_NoOutcomes(t *testing.T) {
	t.Parallel()
	co := NewConfigOptimizer(DefaultConfigOptimizerConfig())

	arm := &ConfigArm{Trials: 0}
	score := co.armScore(arm)
	if score != 0.5 {
		t.Fatalf("expected 0.5 for zero trials, got %f", score)
	}

	// With trials but no windowed outcomes, falls back to overall rate.
	arm2 := &ConfigArm{Trials: 10, Successes: 8}
	score2 := co.armScore(arm2)
	if score2 != 0.8 {
		t.Fatalf("expected 0.8, got %f", score2)
	}
}

func TestSafeDiv(t *testing.T) {
	t.Parallel()
	if safeDiv(10, 0) != 0 {
		t.Fatal("expected 0 for division by zero")
	}
	if safeDiv(10, 2) != 5 {
		t.Fatal("expected 5")
	}
}
