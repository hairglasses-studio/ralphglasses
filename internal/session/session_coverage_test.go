package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- CostHistory ---

func TestCostHistory_SortByTime(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ch := NewCostHistory(dir)

	now := time.Now()
	ch.Add(CostRecord{Provider: "a", CostUSD: 1.0, Timestamp: now.Add(2 * time.Hour)})
	ch.Add(CostRecord{Provider: "b", CostUSD: 2.0, Timestamp: now})
	ch.Add(CostRecord{Provider: "c", CostUSD: 3.0, Timestamp: now.Add(1 * time.Hour)})

	ch.SortByTime()

	if ch.Records[0].Provider != "b" {
		t.Errorf("first record after sort should be 'b' (oldest), got %q", ch.Records[0].Provider)
	}
	if ch.Records[2].Provider != "a" {
		t.Errorf("last record after sort should be 'a' (newest), got %q", ch.Records[2].Provider)
	}
}

func TestCostHistory_SortByTime_Empty(t *testing.T) {
	t.Parallel()
	ch := NewCostHistory(t.TempDir())
	// Should not panic on empty records.
	ch.SortByTime()
}

func TestCostHistory_RecentRecords_EdgeCases(t *testing.T) {
	t.Parallel()
	ch := NewCostHistory(t.TempDir())
	ch.Add(CostRecord{Provider: "a", CostUSD: 1.0})
	ch.Add(CostRecord{Provider: "b", CostUSD: 2.0})

	// n=0 returns nil.
	if got := ch.RecentRecords(0); got != nil {
		t.Errorf("RecentRecords(0) should be nil, got %d items", len(got))
	}

	// Negative n returns nil.
	if got := ch.RecentRecords(-1); got != nil {
		t.Errorf("RecentRecords(-1) should be nil, got %d items", len(got))
	}

	// n larger than available returns all.
	got := ch.RecentRecords(100)
	if len(got) != 2 {
		t.Errorf("RecentRecords(100) should return 2, got %d", len(got))
	}
}

// --- LoadRoutingConfig ---

func TestLoadRoutingConfig_NoFile(t *testing.T) {
	t.Parallel()
	cfg, err := LoadRoutingConfig(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Rules) != 0 {
		t.Errorf("expected 0 rules for missing config, got %d", len(cfg.Rules))
	}
}

func TestLoadRoutingConfig_ValidFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(configDir, 0755)

	cfg := RoutingConfig{
		Rules: []RoutingRule{
			{Pattern: "*lint*", Provider: "gemini", Model: "gemini-flash"},
			{Pattern: "*architecture*", Provider: "claude", Model: "claude-opus"},
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(configDir, "routing.json"), data, 0644)

	loaded, err := LoadRoutingConfig(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(loaded.Rules))
	}
}

func TestLoadRoutingConfig_InvalidJSON(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(configDir, 0755)
	os.WriteFile(filepath.Join(configDir, "routing.json"), []byte("{invalid"), 0644)

	_, err := LoadRoutingConfig(dir)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- PromptLibrary (additional coverage) ---

func TestPromptLibrary_ListNonexistentDir(t *testing.T) {
	t.Parallel()
	pl := NewPromptLibraryAt("/nonexistent/path")
	names, err := pl.List()
	if err != nil {
		t.Errorf("List on nonexistent dir: %v", err)
	}
	if names != nil {
		t.Errorf("List on nonexistent dir should be nil, got %v", names)
	}
}

func TestPromptLibrary_LoadNonexistent(t *testing.T) {
	t.Parallel()
	pl := NewPromptLibraryAt(t.TempDir())
	_, err := pl.Load("nonexistent")
	if err == nil {
		t.Error("expected error loading nonexistent prompt")
	}
}

func TestPromptLibrary_DeleteNonexistent(t *testing.T) {
	t.Parallel()
	pl := NewPromptLibraryAt(t.TempDir())
	err := pl.Delete("nonexistent")
	if err == nil {
		t.Error("expected error deleting nonexistent prompt")
	}
}

// --- ReflexionStore.Rules ---

func TestReflexionStore_Rules_Empty(t *testing.T) {
	t.Parallel()
	rs := NewReflexionStore(t.TempDir())
	rules := rs.Rules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules for empty store, got %d", len(rules))
	}
}

func TestReflexionStore_Rules_BelowThreshold(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rs := NewReflexionStore(dir)

	// Add fewer than 5 reflections -- should return empty.
	for range 4 {
		rs.Store(Reflection{
			FailureMode: "verify_failed",
			RootCause:   "test error",
		})
	}
	rules := rs.Rules()
	if len(rules) != 0 {
		t.Errorf("expected 0 rules with <5 reflections, got %d", len(rules))
	}
}

func TestReflexionStore_Rules_ExtractsPatterns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	rs := NewReflexionStore(dir)

	// Add 5+ reflections with 3+ of the same failure mode.
	for range 4 {
		rs.Store(Reflection{
			FailureMode: "verify_failed",
			RootCause:   "test assertion failed",
		})
	}
	rs.Store(Reflection{
		FailureMode: "worker_error",
		RootCause:   "timeout",
	})
	rs.Store(Reflection{
		FailureMode: "worker_error",
		RootCause:   "timeout",
	})

	rules := rs.Rules()
	// Should extract "verify_failed" (count=4 >= 3).
	found := false
	for _, r := range rules {
		if r.FailureMode == "verify_failed" {
			found = true
			if r.Count != 4 {
				t.Errorf("verify_failed count = %d, want 4", r.Count)
			}
		}
	}
	if !found {
		t.Error("expected verify_failed rule to be extracted")
	}
}
