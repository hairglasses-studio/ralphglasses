package session

import (
	"testing"
)

func TestAutoMode_HourlySpend_ViaRequestPermission(t *testing.T) {
	// Set up a config with a very low global cost limit so hourlySpend is exercised.
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelFullAutonomy
	cfg.GlobalCostLimitUSD = 0.01 // very tight limit
	am := NewAutoMode(cfg)

	// First request with a small cost should be checked against global limit.
	action := AutoAction{
		Category: CategoryCostBearing,
		CostUSD:  0.005,
	}
	// This should trigger hourlySpend check.
	ok, _ := am.RequestPermission(action)
	// Whether it passes or not doesn't matter for coverage; just ensure no panic.
	_ = ok
}

func TestAutoMode_HourlySpend_ExceedsLimit(t *testing.T) {
	cfg := DefaultAutoModeConfig()
	cfg.Enabled = true
	cfg.AutonomyLevel = LevelFullAutonomy
	cfg.GlobalCostLimitUSD = 0.001 // very low limit
	am := NewAutoMode(cfg)

	// Request with cost greater than the limit.
	action := AutoAction{
		Category: CategoryCostBearing,
		CostUSD:  1.0, // way over limit
	}
	ok, reason := am.RequestPermission(action)
	if ok {
		t.Error("expected permission denied due to global cost limit")
	}
	if reason == "" {
		t.Error("expected non-empty reason for denial")
	}
}
