package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	import (
		"context"
		"testing"
		"time"

		"github.com/hairglasses-studio/mcpkit/worktree"
	)

	// ---------------------------------------------------------------------------
	// Manager.SetWorktreePool / WorktreePool (0%)
	// ---------------------------------------------------------------------------

	func TestManager_SetWorktreePool(t *testing.T) {
		t.Parallel()
		m := NewManager()
		if m.WorktreePool() != nil {
			t.Error("WorktreePool should be nil by default")
		}

		pool := &worktree.WorktreePool{} // just a non-nil value
		m.SetWorktreePool(pool)

		if got := m.WorktreePool(); got != pool {
			t.Error("WorktreePool should return the set pool")
		}

		// Reset to nil.
		m.SetWorktreePool(nil)
		if m.WorktreePool() != nil {
			t.Error("WorktreePool should be nil after reset")
		}
	}
// ---------------------------------------------------------------------------
// Manager.SetDepthEstimator / DepthEstimator (0%)
// ---------------------------------------------------------------------------

func TestManager_SetDepthEstimator(t *testing.T) {
	t.Parallel()
	m := NewManager()
	if m.DepthEstimator() != nil {
		t.Error("DepthEstimator should be nil by default")
	}

	de := NewDepthEstimator(nil)
	m.SetDepthEstimator(de)

	if got := m.DepthEstimator(); got != de {
		t.Error("DepthEstimator should return the set estimator")
	}

	m.SetDepthEstimator(nil)
	if m.DepthEstimator() != nil {
		t.Error("DepthEstimator should be nil after reset")
	}
}

// ---------------------------------------------------------------------------
// Manager.effectiveMinSessionDuration (0%)
// ---------------------------------------------------------------------------

func TestManager_EffectiveMinSessionDuration_Default(t *testing.T) {
	t.Parallel()
	m := NewManager()
	d := m.effectiveMinSessionDuration()
	if d != DefaultMinSessionDuration {
		t.Errorf("effectiveMinSessionDuration = %v, want %v", d, DefaultMinSessionDuration)
	}
}

func TestManager_EffectiveMinSessionDuration_Custom(t *testing.T) {
	t.Parallel()
	m := NewManager()
	m.MinSessionDuration = 5 * time.Minute
	d := m.effectiveMinSessionDuration()
	if d != 5*time.Minute {
		t.Errorf("effectiveMinSessionDuration = %v, want 5m", d)
	}
}

func TestManager_EffectiveMinSessionDuration_Negative(t *testing.T) {
	t.Parallel()
	m := NewManager()
	m.MinSessionDuration = -1 * time.Second
	d := m.effectiveMinSessionDuration()
	if d != DefaultMinSessionDuration {
		t.Errorf("effectiveMinSessionDuration = %v, want default %v", d, DefaultMinSessionDuration)
	}
}

// ---------------------------------------------------------------------------
// Manager.IsReapable (0%)
// ---------------------------------------------------------------------------

func TestManager_IsReapable_OldSession(t *testing.T) {
	t.Parallel()
	m := NewManager()
	s := &Session{
		LaunchedAt: time.Now().Add(-5 * time.Minute),
	}
	if !m.IsReapable(s) {
		t.Error("session launched 5 minutes ago should be reapable")
	}
}

func TestManager_IsReapable_NewSession(t *testing.T) {
	t.Parallel()
	m := NewManager()
	s := &Session{
		LaunchedAt: time.Now(),
	}
	if m.IsReapable(s) {
		t.Error("just-launched session should not be reapable")
	}
}

func TestManager_IsReapable_CustomDuration(t *testing.T) {
	t.Parallel()
	m := NewManager()
	m.MinSessionDuration = 1 * time.Hour
	s := &Session{
		LaunchedAt: time.Now().Add(-30 * time.Minute),
	}
	if m.IsReapable(s) {
		t.Error("session launched 30m ago should not be reapable with 1h min duration")
	}
}

// ---------------------------------------------------------------------------
// Manager.SupervisorStatus (0%)
// ---------------------------------------------------------------------------

func TestManager_SupervisorStatus_Nil(t *testing.T) {
	t.Parallel()
	m := NewManager()
	if m.SupervisorStatus() != nil {
		t.Error("SupervisorStatus should be nil when no supervisor is running")
	}
}

// ---------------------------------------------------------------------------
// Manager.ListCycles (0% for the Manager method)
// ---------------------------------------------------------------------------

func TestManager_ListCycles_NoCycles(t *testing.T) {
	t.Parallel()
	m := NewManager()
	dir := t.TempDir()

	cycles, err := m.ListCycles(dir)
	if err != nil {
		t.Fatalf("ListCycles: %v", err)
	}
	if len(cycles) != 0 {
		t.Errorf("expected 0 cycles, got %d", len(cycles))
	}
}

func TestManager_ListCycles_WithCycles(t *testing.T) {
	t.Parallel()
	m := NewManager()
	dir := t.TempDir()

	// Create some cycles.
	c1, err := m.CreateCycle(dir, "c1", "obj1", nil)
	if err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}
	m.FailCycle(c1, "done")
	_, err = m.CreateCycle(dir, "c2", "obj2", nil)
	if err != nil {
		t.Fatalf("CreateCycle: %v", err)
	}

	cycles, err := m.ListCycles(dir)
	if err != nil {
		t.Fatalf("ListCycles: %v", err)
	}
	if len(cycles) != 2 {
		t.Errorf("expected 2 cycles, got %d", len(cycles))
	}
}

// ---------------------------------------------------------------------------
// NewPromptLibrary (0%)
// ---------------------------------------------------------------------------

func TestNewPromptLibrary(t *testing.T) {
	t.Parallel()
	pl := NewPromptLibrary()
	if pl == nil {
		t.Fatal("NewPromptLibrary returned nil")
	}
	// The dir should be set to something under home.
	if pl.dir == "" {
		// On CI or in some environments, UserHomeDir may fail,
		// which means dir is empty. That's acceptable.
		t.Log("NewPromptLibrary dir is empty (UserHomeDir failed)")
	}
}

// ---------------------------------------------------------------------------
// CycleChainer.SaveLineage (0%)
// ---------------------------------------------------------------------------

func TestCycleChainer_SaveLineage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cc := NewCycleChainer()
	// Record a lineage entry so there's data to save.
	cc.RecordLineage(dir, "parent-1", "child-1")

	if err := cc.SaveLineage(dir); err != nil {
		t.Fatalf("SaveLineage: %v", err)
	}

	// Verify the file exists.
	path := filepath.Join(dir, ".ralph", "cycle_lineage.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lineage file: %v", err)
	}
	if len(data) == 0 {
		t.Error("lineage file is empty")
	}

	// Verify it's valid JSON.
	var lineage []any
	if err := json.Unmarshal(data, &lineage); err != nil {
		t.Fatalf("unmarshal lineage: %v", err)
	}
	if len(lineage) != 1 {
		t.Errorf("expected 1 lineage entry, got %d", len(lineage))
	}
}

func TestCycleChainer_SaveLineage_Empty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	cc := NewCycleChainer()
	if err := cc.SaveLineage(dir); err != nil {
		t.Fatalf("SaveLineage empty: %v", err)
	}
}

// ---------------------------------------------------------------------------
// teamSafetyConfig (0%)
// ---------------------------------------------------------------------------

func TestTeamSafetyConfig_Default(t *testing.T) {
	// Ensure TeamSafety is nil to test default path.
	old := TeamSafety
	TeamSafety = nil
	defer func() { TeamSafety = old }()

	cfg := teamSafetyConfig()
	if cfg.MaxNestingDepth != DefaultTeamSafety.MaxNestingDepth {
		t.Errorf("MaxNestingDepth = %d, want %d", cfg.MaxNestingDepth, DefaultTeamSafety.MaxNestingDepth)
	}
	if cfg.MaxTeamSize != DefaultTeamSafety.MaxTeamSize {
		t.Errorf("MaxTeamSize = %d, want %d", cfg.MaxTeamSize, DefaultTeamSafety.MaxTeamSize)
	}
	if cfg.MaxTotalTeams != DefaultTeamSafety.MaxTotalTeams {
		t.Errorf("MaxTotalTeams = %d, want %d", cfg.MaxTotalTeams, DefaultTeamSafety.MaxTotalTeams)
	}
}

func TestTeamSafetyConfig_Override(t *testing.T) {
	old := TeamSafety
	TeamSafety = &TeamSafetyConfig{
		MaxNestingDepth: 1,
		MaxTeamSize:     5,
		MaxTotalTeams:   3,
	}
	defer func() { TeamSafety = old }()

	cfg := teamSafetyConfig()
	if cfg.MaxNestingDepth != 1 {
		t.Errorf("MaxNestingDepth = %d, want 1", cfg.MaxNestingDepth)
	}
	if cfg.MaxTeamSize != 5 {
		t.Errorf("MaxTeamSize = %d, want 5", cfg.MaxTeamSize)
	}
}

// ---------------------------------------------------------------------------
// Concurrency: SetWorktreePool + SetDepthEstimator under concurrent access
// ---------------------------------------------------------------------------

func TestManager_SubsystemSetters_Concurrent(t *testing.T) {
	t.Parallel()
	m := NewManager()
	de := NewDepthEstimator(nil)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			m.SetDepthEstimator(de)
			_ = m.DepthEstimator()
		}()
		go func() {
			defer wg.Done()
			m.SetWorktreePool(nil)
			_ = m.WorktreePool()
		}()
	}
	wg.Wait()
}
