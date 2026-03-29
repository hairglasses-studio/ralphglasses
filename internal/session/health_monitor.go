package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// HealthSignal represents a health metric evaluation from HealthMonitor.
type HealthSignal struct {
	Category        DecisionCategory `json:"category"`
	Metric          string           `json:"metric"`
	Value           float64          `json:"value"`
	Threshold       float64          `json:"threshold"`
	Rationale       string           `json:"rationale"`
	SuggestedAction string           `json:"suggested_action"`
}

// HealthThresholds configures the health evaluation parameters.
type HealthThresholds struct {
	MinCompletionRate  float64       // below → launch investigation cycle
	MaxCostRatePerHour float64       // above → reduce budget
	MinVerifyPassRate  float64       // below → launch fix cycle
	MaxIdleTime        time.Duration // above → launch exploration cycle
	MinIterationRate   float64       // iterations/hour; below → consolidation

	// L2 spec thresholds (docs/AUTONOMY.md)
	MinCoverage         float64 // test coverage percentage; below → launch coverage cycle
	MaxMeanCycleCostUSD float64 // per-cycle average; above → budget adjustment
	MaxHITLRate         float64 // manual/(manual+auto) ratio; above → investigate
	MaxCriticalFindings int     // open critical findings; above → launch fix cycle
}

// DefaultHealthThresholds returns production defaults.
func DefaultHealthThresholds() HealthThresholds {
	return HealthThresholds{
		MinCompletionRate:  0.70,
		MaxCostRatePerHour: 5.00,
		MinVerifyPassRate:  0.80,
		MaxIdleTime:        1 * time.Hour,
		MinIterationRate:   0.5,

		// L2 spec (docs/AUTONOMY.md)
		MinCoverage:         80.0,
		MaxMeanCycleCostUSD: 2.00,
		MaxHITLRate:         0.10,
		MaxCriticalFindings: 3,
	}
}

// HealthMonitor evaluates repo health and returns actionable signals.
// When EvaluateFunc is set it is used directly (for testing); otherwise
// the struct-based thresholds drive evaluation.
type HealthMonitor struct {
	EvaluateFunc func(repoPath string) []HealthSignal
	Thresholds   HealthThresholds
}

// NewHealthMonitor creates a HealthMonitor with the given thresholds.
func NewHealthMonitor(t HealthThresholds) *HealthMonitor {
	return &HealthMonitor{Thresholds: t}
}

// Evaluate returns health signals for the given repo. If EvaluateFunc is set
// (typically in tests), it delegates there. Otherwise it reads observations
// and cycles to compute real metrics.
func (hm *HealthMonitor) Evaluate(repoPath string) []HealthSignal {
	if hm == nil {
		return nil
	}
	if hm.EvaluateFunc != nil {
		return hm.EvaluateFunc(repoPath)
	}
	return hm.evaluate(repoPath)
}

func (hm *HealthMonitor) evaluate(repoPath string) []HealthSignal {
	var signals []HealthSignal
	t := hm.Thresholds

	// --- Completion rate ---
	obsPath := filepath.Join(repoPath, ".ralph", "cost_observations.json")
	obs, err := LoadObservations(obsPath, time.Now().Add(-24*time.Hour))
	if err != nil {
		slog.Debug("health_monitor: load observations", "error", err)
	}
	if len(obs) > 0 {
		var completed, total int
		for _, o := range obs {
			total++
			if o.VerifyPassed {
				completed++
			}
		}
		rate := float64(completed) / float64(total)
		if rate < t.MinCompletionRate {
			signals = append(signals, HealthSignal{
				Category:        DecisionLaunch,
				Metric:          "completion_rate",
				Value:           rate,
				Threshold:       t.MinCompletionRate,
				Rationale:       "Completion rate below threshold — investigate failures",
				SuggestedAction: "launch_investigation",
			})
		}

		// --- Cost rate ---
		var totalCost float64
		for _, o := range obs {
			totalCost += o.TotalCostUSD
		}
		hours := time.Since(obs[0].Timestamp).Hours()
		if hours < 0.1 {
			hours = 0.1
		}
		costRate := totalCost / hours
		if costRate > t.MaxCostRatePerHour {
			signals = append(signals, HealthSignal{
				Category:        DecisionBudgetAdjust,
				Metric:          "cost_rate_per_hour",
				Value:           costRate,
				Threshold:       t.MaxCostRatePerHour,
				Rationale:       "Cost rate exceeds threshold — consider budget reduction",
				SuggestedAction: "reduce_budget",
			})
		}
	}

	// --- Verify pass rate (from recent cycles) ---
	cycles, err := ListCycles(repoPath)
	if err != nil {
		slog.Debug("health_monitor: list cycles", "error", err)
	}
	if len(cycles) > 0 {
		var passed, cycleTotal int
		for _, c := range cycles {
			if c.Phase != CycleComplete {
				continue
			}
			cycleTotal++
			if c.Synthesis != nil && c.Synthesis.Summary != "" {
				// A completed cycle with synthesis is considered passing.
				passed++
			}
		}
		if cycleTotal > 0 {
			passRate := float64(passed) / float64(cycleTotal)
			if passRate < t.MinVerifyPassRate {
				signals = append(signals, HealthSignal{
					Category:        DecisionLaunch,
					Metric:          "verify_pass_rate",
					Value:           passRate,
					Threshold:       t.MinVerifyPassRate,
					Rationale:       "Cycle verification rate below threshold",
					SuggestedAction: "launch_fix",
				})
			}
		}
	}

	// --- Idle time ---
	var lastActivity time.Time
	for _, c := range cycles {
		if c.UpdatedAt.After(lastActivity) {
			lastActivity = c.UpdatedAt
		}
	}
	for _, o := range obs {
		if o.Timestamp.After(lastActivity) {
			lastActivity = o.Timestamp
		}
	}
	if !lastActivity.IsZero() {
		idle := time.Since(lastActivity)
		if idle > t.MaxIdleTime {
			signals = append(signals, HealthSignal{
				Category:        DecisionLaunch,
				Metric:          "idle_time",
				Value:           idle.Seconds(),
				Threshold:       t.MaxIdleTime.Seconds(),
				Rationale:       "No recent activity — launch exploration from ROADMAP",
				SuggestedAction: "launch_exploration",
			})
		}
	} else if len(obs) == 0 && len(cycles) == 0 {
		// No history at all — treat as idle.
		signals = append(signals, HealthSignal{
			Category:        DecisionLaunch,
			Metric:          "idle_time",
			Value:           t.MaxIdleTime.Seconds() + 1,
			Threshold:       t.MaxIdleTime.Seconds(),
			Rationale:       "No activity history — launch initial exploration",
			SuggestedAction: "launch_exploration",
		})
	}

	// --- L2 spec: mean cycle cost ---
	if t.MaxMeanCycleCostUSD > 0 && len(obs) > 0 {
		var totalCostAll float64
		for _, o := range obs {
			totalCostAll += o.TotalCostUSD
		}
		meanCost := totalCostAll / float64(len(obs))
		if meanCost > t.MaxMeanCycleCostUSD {
			signals = append(signals, HealthSignal{
				Category:        DecisionBudgetAdjust,
				Metric:          "mean_cycle_cost",
				Value:           meanCost,
				Threshold:       t.MaxMeanCycleCostUSD,
				Rationale:       "Mean cycle cost exceeds L2 threshold",
				SuggestedAction: "reduce_budget",
			})
		}
	}

	// --- L2 spec: HITL intervention rate ---
	if t.MaxHITLRate > 0 {
		hitlRate := evaluateHITLRate(repoPath)
		if hitlRate >= 0 && hitlRate > t.MaxHITLRate {
			signals = append(signals, HealthSignal{
				Category:        DecisionLaunch,
				Metric:          "hitl_rate",
				Value:           hitlRate,
				Threshold:       t.MaxHITLRate,
				Rationale:       "HITL intervention rate exceeds L2 threshold — too many manual interventions",
				SuggestedAction: "launch_investigation",
			})
		}
	}

	// --- L2 spec: test coverage ---
	if t.MinCoverage > 0 {
		cov := evaluateCoverage(repoPath)
		if cov >= 0 && cov < t.MinCoverage {
			signals = append(signals, HealthSignal{
				Category:        DecisionLaunch,
				Metric:          "test_coverage",
				Value:           cov,
				Threshold:       t.MinCoverage,
				Rationale:       "Test coverage below L2 threshold",
				SuggestedAction: "launch_coverage",
			})
		}
	}

	// --- L2 spec: critical findings ---
	if t.MaxCriticalFindings > 0 {
		count := countCriticalFindings(repoPath, cycles)
		if count > t.MaxCriticalFindings {
			signals = append(signals, HealthSignal{
				Category:        DecisionLaunch,
				Metric:          "critical_findings",
				Value:           float64(count),
				Threshold:       float64(t.MaxCriticalFindings),
				Rationale:       "Open critical findings exceed L2 threshold",
				SuggestedAction: "launch_fix",
			})
		}
	}

	// --- Self-test signal: emit after every 5 completed cycles ---
	completedCycles := 0
	for _, c := range cycles {
		if c.Phase == CycleComplete {
			completedCycles++
		}
	}
	if completedCycles > 0 && completedCycles%5 == 0 {
		signals = append(signals, HealthSignal{
			Category:        DecisionSelfTest,
			Metric:          "periodic_self_test",
			Value:           float64(completedCycles),
			Threshold:       5,
			Rationale:       "Periodic self-test after 5 completed cycles",
			SuggestedAction: "run_self_test",
		})
	}

	// --- Reflexion signal: emit when patterns are accumulating ---
	patternsPath := filepath.Join(repoPath, ".ralph", "improvement_patterns.json")
	if data, err := os.ReadFile(patternsPath); err == nil && len(data) > 100 {
		signals = append(signals, HealthSignal{
			Category:        DecisionReflexion,
			Metric:          "pattern_accumulation",
			Value:           float64(len(data)),
			Threshold:       100,
			Rationale:       "Improvement patterns accumulating — consolidate learnings",
			SuggestedAction: "consolidate_patterns",
		})
	}

	return signals
}

// evaluateHITLRate computes manual/(manual+auto) ratio from hitl_events.jsonl.
// Returns -1 if no data available.
func evaluateHITLRate(repoPath string) float64 {
	path := filepath.Join(repoPath, ".ralph", "hitl_events.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()

	var manual, auto int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e HITLEvent
		if json.Unmarshal(scanner.Bytes(), &e) != nil {
			continue
		}
		switch e.Trigger {
		case TriggerManual:
			manual++
		case TriggerAutomatic, TriggerScheduled:
			auto++
		}
	}
	total := manual + auto
	if total == 0 {
		return -1
	}
	return float64(manual) / float64(total)
}

// evaluateCoverage reads the most recent coverage percentage from .ralph/coverage.txt.
// The file should contain a single line like "86.0" (percentage).
// Returns -1 if no data available.
func evaluateCoverage(repoPath string) float64 {
	path := filepath.Join(repoPath, ".ralph", "coverage.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	var pct float64
	if _, err := fmt.Sscanf(string(data), "%f", &pct); err != nil {
		return -1
	}
	return pct
}

// countCriticalFindings counts open critical-severity findings across recent cycles.
func countCriticalFindings(repoPath string, cycles []*CycleRun) int {
	count := 0
	for _, c := range cycles {
		if c.Phase == CycleComplete || c.Phase == CycleFailed {
			continue // only count open cycles
		}
		for _, f := range c.Findings {
			if f.Severity == "critical" {
				count++
			}
		}
	}
	return count
}
