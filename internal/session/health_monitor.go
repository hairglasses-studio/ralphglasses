package session

import (
	"log/slog"
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
}

// DefaultHealthThresholds returns production defaults.
func DefaultHealthThresholds() HealthThresholds {
	return HealthThresholds{
		MinCompletionRate:  0.70,
		MaxCostRatePerHour: 5.00,
		MinVerifyPassRate:  0.80,
		MaxIdleTime:        1 * time.Hour,
		MinIterationRate:   0.5,
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

	return signals
}
