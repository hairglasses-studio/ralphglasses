package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// BreakglassConfig defines circuit breaker thresholds at all granularities.
type BreakglassConfig struct {
	// Per-loop limits
	LoopTokenBudget  int     `json:"loop_token_budget"`
	LoopCostCeiling  float64 `json:"loop_cost_ceiling"`
	LoopTimeLimitMin int     `json:"loop_time_limit_min"`

	// Per-session limits
	SessionTokenBudget    int     `json:"session_token_budget"`
	SessionCostCeiling    float64 `json:"session_cost_ceiling"`
	SessionTimeLimitHours int     `json:"session_time_limit_hours"`
	MaxConsecutiveNoProg  int     `json:"max_consecutive_no_progress"`

	// Per-hour limits
	MaxCallsPerHour   int     `json:"max_calls_per_hour"`
	HourlyTokenBudget int     `json:"hourly_token_budget"`
	HourlyCostCeiling float64 `json:"hourly_cost_ceiling"`

	// Per-day limits
	DailyTokenBudget  int     `json:"daily_token_budget"`
	DailyCostCeiling  float64 `json:"daily_cost_ceiling"`
	DailyTimeLimitHrs int     `json:"daily_time_limit_hours"`
}

// DefaultBreakglass returns sane defaults for all breakglass thresholds.
func DefaultBreakglass() *BreakglassConfig {
	return &BreakglassConfig{
		LoopTokenBudget:       200000,
		LoopCostCeiling:       2.00,
		LoopTimeLimitMin:      15,
		SessionTokenBudget:    2000000,
		SessionCostCeiling:    10.00,
		SessionTimeLimitHours: 4,
		MaxConsecutiveNoProg:  3,
		MaxCallsPerHour:       80,
		HourlyTokenBudget:     500000,
		HourlyCostCeiling:     5.00,
		DailyTokenBudget:      10000000,
		DailyCostCeiling:      100.00,
		DailyTimeLimitHrs:     24,
	}
}

// BreakglassCheck evaluates current metrics against thresholds.
// Returns the first violated criterion, or "" if all OK.
func (bg *BreakglassConfig) Check(metrics *LoopMetrics) string {
	if metrics == nil {
		return ""
	}

	// Per-loop checks
	if bg.LoopTokenBudget > 0 && metrics.LoopTokens > bg.LoopTokenBudget {
		return "LOOP_TOKEN_BUDGET"
	}
	if bg.LoopCostCeiling > 0 && metrics.LoopCost > bg.LoopCostCeiling {
		return "LOOP_COST_CEILING"
	}

	// Per-session checks
	if bg.SessionTokenBudget > 0 && metrics.SessionTokens > bg.SessionTokenBudget {
		return "SESSION_TOKEN_BUDGET"
	}
	if bg.SessionCostCeiling > 0 && metrics.SessionCost > bg.SessionCostCeiling {
		return "SESSION_COST_CEILING"
	}
	if bg.MaxConsecutiveNoProg > 0 && metrics.ConsecutiveNoProgress >= bg.MaxConsecutiveNoProg {
		return "MAX_CONSECUTIVE_NO_PROGRESS"
	}

	// Time checks
	if bg.SessionTimeLimitHours > 0 && metrics.SessionDuration >= time.Duration(bg.SessionTimeLimitHours)*time.Hour {
		return "SESSION_TIME_LIMIT"
	}

	// Per-hour checks
	if bg.MaxCallsPerHour > 0 && metrics.HourlyCalls > bg.MaxCallsPerHour {
		return "MAX_CALLS_PER_HOUR"
	}

	return ""
}

// LoopMetrics holds current metrics for breakglass evaluation.
type LoopMetrics struct {
	LoopTokens             int
	LoopCost               float64
	SessionTokens          int
	SessionCost            float64
	SessionDuration        time.Duration
	HourlyCalls            int
	ConsecutiveNoProgress  int
}

// SaveBreakglass writes the breakglass config to .ralph/breakglass.json.
func SaveBreakglass(repoPath string, bg *BreakglassConfig) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(bg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "breakglass.json"), data, 0644)
}

// LoadBreakglass reads breakglass config, returning defaults if not found.
func LoadBreakglass(repoPath string) *BreakglassConfig {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", "breakglass.json"))
	if err != nil {
		return DefaultBreakglass()
	}
	var bg BreakglassConfig
	if err := json.Unmarshal(data, &bg); err != nil {
		return DefaultBreakglass()
	}
	return &bg
}
