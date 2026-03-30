package session

import (
	"testing"
)

// TestCascadeRouting_EnabledByDefault verifies QW-2: cascade routing is enabled
// by default for all Manager constructors without any explicit configuration.
func TestCascadeRouting_EnabledByDefault(t *testing.T) {
	t.Parallel()

	t.Run("NewManager", func(t *testing.T) {
		t.Parallel()
		m := NewManager()
		if !m.HasCascadeRouter() {
			t.Fatal("NewManager: cascade routing must be enabled by default (QW-2)")
		}
		cr := m.GetCascadeRouter()
		if cr == nil {
			t.Fatal("NewManager: GetCascadeRouter must return non-nil by default (QW-2)")
		}
		cfg := cr.config
		if !cfg.Enabled {
			t.Errorf("NewManager: CascadeConfig.Enabled = false, want true")
		}
		if cfg.CheapProvider == "" {
			t.Errorf("NewManager: CascadeConfig.CheapProvider is empty")
		}
		if cfg.ExpensiveProvider == "" {
			t.Errorf("NewManager: CascadeConfig.ExpensiveProvider is empty")
		}
	})

	t.Run("NewManagerWithBus", func(t *testing.T) {
		t.Parallel()
		m := NewManagerWithBus(nil)
		if !m.HasCascadeRouter() {
			t.Fatal("NewManagerWithBus: cascade routing must be enabled by default (QW-2)")
		}
		if m.GetCascadeRouter() == nil {
			t.Fatal("NewManagerWithBus: GetCascadeRouter must return non-nil by default (QW-2)")
		}
	})

	t.Run("NewManagerWithStore", func(t *testing.T) {
		t.Parallel()
		m := NewManagerWithStore(nil, nil)
		if !m.HasCascadeRouter() {
			t.Fatal("NewManagerWithStore: cascade routing must be enabled by default (QW-2)")
		}
		if m.GetCascadeRouter() == nil {
			t.Fatal("NewManagerWithStore: GetCascadeRouter must return non-nil by default (QW-2)")
		}
	})
}

// TestDefaultCascadeConfig_Enabled verifies that DefaultCascadeConfig returns
// a config with Enabled=true and sensible non-zero defaults.
func TestDefaultCascadeConfig_Enabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultCascadeConfig()
	if !cfg.Enabled {
		t.Errorf("DefaultCascadeConfig().Enabled = false, want true")
	}
	if cfg.ConfidenceThreshold <= 0 || cfg.ConfidenceThreshold > 1 {
		t.Errorf("DefaultCascadeConfig().ConfidenceThreshold = %v, want 0 < x <= 1", cfg.ConfidenceThreshold)
	}
	if cfg.MaxCheapBudgetUSD <= 0 {
		t.Errorf("DefaultCascadeConfig().MaxCheapBudgetUSD = %v, want > 0", cfg.MaxCheapBudgetUSD)
	}
	if cfg.MaxCheapTurns <= 0 {
		t.Errorf("DefaultCascadeConfig().MaxCheapTurns = %v, want > 0", cfg.MaxCheapTurns)
	}
}
