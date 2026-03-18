package model

import (
	"testing"
	"time"
)

func TestDefaultBreakglass(t *testing.T) {
	bg := DefaultBreakglass()
	if bg.LoopTokenBudget != 200000 {
		t.Errorf("LoopTokenBudget = %d, want 200000", bg.LoopTokenBudget)
	}
	if bg.MaxConsecutiveNoProg != 3 {
		t.Errorf("MaxConsecutiveNoProg = %d, want 3", bg.MaxConsecutiveNoProg)
	}
	if bg.DailyCostCeiling != 100.00 {
		t.Errorf("DailyCostCeiling = %f, want 100.00", bg.DailyCostCeiling)
	}
}

func TestBreakglassCheckPasses(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{
		LoopTokens:    100000,
		LoopCost:      1.00,
		SessionTokens: 500000,
		SessionCost:   5.00,
		HourlyCalls:   40,
	}
	if reason := bg.Check(metrics); reason != "" {
		t.Errorf("expected no violation, got %q", reason)
	}
}

func TestBreakglassCheckTokenBudget(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{LoopTokens: 300000}
	if reason := bg.Check(metrics); reason != "LOOP_TOKEN_BUDGET" {
		t.Errorf("expected LOOP_TOKEN_BUDGET, got %q", reason)
	}
}

func TestBreakglassCheckCostCeiling(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{LoopCost: 3.00}
	if reason := bg.Check(metrics); reason != "LOOP_COST_CEILING" {
		t.Errorf("expected LOOP_COST_CEILING, got %q", reason)
	}
}

func TestBreakglassCheckSessionCost(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{SessionCost: 15.00}
	if reason := bg.Check(metrics); reason != "SESSION_COST_CEILING" {
		t.Errorf("expected SESSION_COST_CEILING, got %q", reason)
	}
}

func TestBreakglassCheckNoProgress(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{ConsecutiveNoProgress: 3}
	if reason := bg.Check(metrics); reason != "MAX_CONSECUTIVE_NO_PROGRESS" {
		t.Errorf("expected MAX_CONSECUTIVE_NO_PROGRESS, got %q", reason)
	}
}

func TestBreakglassCheckTimeLimit(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{SessionDuration: 5 * time.Hour}
	if reason := bg.Check(metrics); reason != "SESSION_TIME_LIMIT" {
		t.Errorf("expected SESSION_TIME_LIMIT, got %q", reason)
	}
}

func TestBreakglassCheckHourlyCalls(t *testing.T) {
	bg := DefaultBreakglass()
	metrics := &LoopMetrics{HourlyCalls: 100}
	if reason := bg.Check(metrics); reason != "MAX_CALLS_PER_HOUR" {
		t.Errorf("expected MAX_CALLS_PER_HOUR, got %q", reason)
	}
}

func TestBreakglassCheckNilMetrics(t *testing.T) {
	bg := DefaultBreakglass()
	if reason := bg.Check(nil); reason != "" {
		t.Errorf("expected empty for nil metrics, got %q", reason)
	}
}

func TestSaveLoadBreakglass(t *testing.T) {
	dir := t.TempDir()
	bg := DefaultBreakglass()
	bg.SessionCostCeiling = 50.00

	if err := SaveBreakglass(dir, bg); err != nil {
		t.Fatalf("SaveBreakglass: %v", err)
	}

	loaded := LoadBreakglass(dir)
	if loaded.SessionCostCeiling != 50.00 {
		t.Errorf("SessionCostCeiling = %f, want 50.00", loaded.SessionCostCeiling)
	}
}

func TestLoadBreakglassDefaults(t *testing.T) {
	dir := t.TempDir()
	loaded := LoadBreakglass(dir)
	if loaded.LoopTokenBudget != 200000 {
		t.Errorf("expected defaults, got LoopTokenBudget=%d", loaded.LoopTokenBudget)
	}
}
