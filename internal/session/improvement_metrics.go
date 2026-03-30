package session

import (
	"sync"
	"time"
)

// MetricsSummary is a JSON-serializable aggregate snapshot of improvement metrics.
type MetricsSummary struct {
	// LearningVelocity is the rate of new insights gained per session (0-1).
	LearningVelocity float64 `json:"learning_velocity"`
	// TotalInsights is the total number of active insights across all types.
	TotalInsights int `json:"total_insights"`
	// InsightsByType breaks down insight counts by type.
	InsightsByType map[string]int `json:"insights_by_type"`
	// RegressionFrequency is the fraction of recent checks that found regressions (0-1).
	RegressionFrequency float64 `json:"regression_frequency"`
	// RecentRegressions is the count of regressions detected in the last analysis window.
	RecentRegressions int `json:"recent_regressions"`
	// OptimizationEffectiveness is the fraction of sessions improved vs baseline (0-1).
	OptimizationEffectiveness float64 `json:"optimization_effectiveness"`
	// TotalSessionsTracked is the number of sessions contributing to these metrics.
	TotalSessionsTracked int `json:"total_sessions_tracked"`
	// SuccessRateTrend is the slope of the success rate trend over tracked sessions (-1 to +1).
	SuccessRateTrend float64 `json:"success_rate_trend"`
	// AvgCostUSD is the average cost per session across all tracked sessions.
	AvgCostUSD float64 `json:"avg_cost_usd"`
	// ComputedAt records when this summary was generated.
	ComputedAt time.Time `json:"computed_at"`
}

// regressionCheckRecord stores the outcome of a single regression check call.
type regressionCheckRecord struct {
	Timestamp       time.Time
	RegressionsFound int
}

// ImprovementMetrics aggregates metrics from LearningTransfer and
// SessionRegressionDetector to produce a unified improvement health summary.
type ImprovementMetrics struct {
	mu               sync.Mutex
	learningTransfer *LearningTransfer
	regressionDet    *SessionRegressionDetector
	checkHistory     []regressionCheckRecord
	// baseline tracks per-session success rate for trend computation.
	successHistory []float64
	costHistory    []float64
}

// NewImprovementMetrics creates an ImprovementMetrics instance wiring the
// two underlying subsystems.
func NewImprovementMetrics(lt *LearningTransfer, rd *SessionRegressionDetector) *ImprovementMetrics {
	return &ImprovementMetrics{
		learningTransfer: lt,
		regressionDet:    rd,
	}
}

// RecordCheck runs a regression check and records its outcome for
// frequency tracking. Call this after each session batch.
func (im *ImprovementMetrics) RecordCheck() []SessionRegression {
	im.mu.Lock()
	defer im.mu.Unlock()

	var regressions []SessionRegression
	if im.regressionDet != nil {
		regressions = im.regressionDet.Check()
	}

	im.checkHistory = append(im.checkHistory, regressionCheckRecord{
		Timestamp:        time.Now(),
		RegressionsFound: len(regressions),
	})
	return regressions
}

// RecordSessionOutcome records a session outcome for trend and effectiveness tracking.
// success indicates whether the session completed successfully.
// costUSD is the session cost.
func (im *ImprovementMetrics) RecordSessionOutcome(success bool, costUSD float64) {
	im.mu.Lock()
	defer im.mu.Unlock()

	rate := 0.0
	if success {
		rate = 1.0
	}
	im.successHistory = append(im.successHistory, rate)
	if costUSD > 0 {
		im.costHistory = append(im.costHistory, costUSD)
	}
}

// Summary computes and returns an aggregate metrics snapshot.
func (im *ImprovementMetrics) Summary() MetricsSummary {
	im.mu.Lock()
	defer im.mu.Unlock()

	summary := MetricsSummary{
		ComputedAt:     time.Now(),
		InsightsByType: make(map[string]int),
	}

	// --- Learning velocity and insight counts ---
	if im.learningTransfer != nil {
		insights := im.learningTransfer.AllInsights()
		sessions := im.learningTransfer.AllSessions()

		summary.TotalInsights = len(insights)
		for _, ins := range insights {
			summary.InsightsByType[ins.Type]++
		}

		// Learning velocity = insights per session, capped at 1.
		if len(sessions) > 0 {
			vel := float64(len(insights)) / float64(len(sessions))
			if vel > 1.0 {
				vel = 1.0
			}
			summary.LearningVelocity = vel
		}

		summary.TotalSessionsTracked = len(sessions)

		// Optimization effectiveness: fraction of sessions with success that
		// also have provider_hint or budget_hint insights active.
		hintCount := summary.InsightsByType["provider_hint"] + summary.InsightsByType["budget_hint"]
		if hintCount > 0 && summary.TotalSessionsTracked > 0 {
			eff := float64(hintCount) / float64(summary.TotalSessionsTracked)
			if eff > 1.0 {
				eff = 1.0
			}
			summary.OptimizationEffectiveness = eff
		}
	}

	// --- Regression frequency ---
	if len(im.checkHistory) > 0 {
		checksWithRegressions := 0
		totalRegressions := 0
		for _, c := range im.checkHistory {
			totalRegressions += c.RegressionsFound
			if c.RegressionsFound > 0 {
				checksWithRegressions++
			}
		}
		summary.RegressionFrequency = float64(checksWithRegressions) / float64(len(im.checkHistory))
		// Most recent check's regressions.
		summary.RecentRegressions = im.checkHistory[len(im.checkHistory)-1].RegressionsFound
	}

	// --- Success rate trend ---
	if len(im.successHistory) >= 2 {
		summary.SuccessRateTrend = linearTrend(im.successHistory)
	}

	// --- Average cost ---
	if len(im.costHistory) > 0 {
		var sum float64
		for _, c := range im.costHistory {
			sum += c
		}
		summary.AvgCostUSD = sum / float64(len(im.costHistory))
	}

	return summary
}

// linearTrend computes the normalized linear regression slope of a series.
// Returns a value in [-1, +1] where positive = improving, negative = degrading.
func linearTrend(values []float64) float64 {
	n := float64(len(values))
	if n < 2 {
		return 0
	}

	// Compute means.
	var sumX, sumY float64
	for i, v := range values {
		sumX += float64(i)
		sumY += v
	}
	meanX := sumX / n
	meanY := sumY / n

	// Compute slope via least squares.
	var num, denom float64
	for i, v := range values {
		dx := float64(i) - meanX
		dy := v - meanY
		num += dx * dy
		denom += dx * dx
	}
	if denom == 0 {
		return 0
	}
	slope := num / denom

	// Normalize by the range of the series to get a [-1, +1] value.
	// The maximum absolute slope for a 0-1 series over n points is 1/(n-1).
	maxSlope := 1.0 / (n - 1)
	if maxSlope == 0 {
		return 0
	}
	normalized := slope / maxSlope
	if normalized > 1.0 {
		return 1.0
	}
	if normalized < -1.0 {
		return -1.0
	}
	return normalized
}
