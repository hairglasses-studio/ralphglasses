package eval

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Direction indicates whether higher or lower values are better for a metric.
type Direction int

const (
	// HigherIsBetter means an increase is an improvement (e.g., completion rate).
	HigherIsBetter Direction = iota
	// LowerIsBetter means a decrease is an improvement (e.g., cost, latency).
	LowerIsBetter
)

// MetricThreshold configures the regression detection parameters for a single metric.
type MetricThreshold struct {
	// Extract pulls the numeric value from an observation.
	Extract MetricFunc

	// Direction indicates whether higher or lower values are better.
	Direction Direction

	// AbsoluteThreshold is the minimum absolute change to flag as a regression.
	// If zero, only relative threshold is used.
	AbsoluteThreshold float64

	// RelativeThreshold is the minimum relative change (fraction, e.g. 0.10 = 10%)
	// to flag as a regression. If zero, only absolute threshold is used.
	// At least one of AbsoluteThreshold or RelativeThreshold must be non-zero.
	RelativeThreshold float64
}

// RegressionDetector compares two sets of eval results and identifies regressions.
type RegressionDetector struct {
	// Thresholds maps metric names to their detection configuration.
	Thresholds map[string]MetricThreshold
}

// RegressionReport is the output of DetectRegressions.
type RegressionReport struct {
	// Regressions lists metrics that degraded beyond their thresholds.
	Regressions []MetricRegression `json:"regressions"`

	// NewFailures lists observations that passed in the baseline but failed
	// in the candidate set (matched by LoopID + TaskTitle).
	NewFailures []NewFailure `json:"new_failures"`

	// PerformanceRegressions lists metrics with statistically significant
	// performance degradation based on mean and standard deviation comparison.
	PerformanceRegressions []PerformanceRegression `json:"performance_regressions"`

	// Summary provides a human-readable overview.
	Summary string `json:"summary"`

	// Passed is true when no regressions, new failures, or performance
	// regressions were detected.
	Passed bool `json:"passed"`

	// Timestamp records when the report was generated.
	Timestamp time.Time `json:"timestamp"`
}

// MetricRegression describes a single metric that degraded beyond its threshold.
type MetricRegression struct {
	MetricName     string  `json:"metric_name"`
	BaselineValue  float64 `json:"baseline_value"`
	CandidateValue float64 `json:"candidate_value"`
	AbsoluteChange float64 `json:"absolute_change"` // candidate - baseline
	RelativeChange float64 `json:"relative_change"` // fraction, e.g. -0.15 = 15% worse
	Direction      string  `json:"direction"`        // "degraded"
}

// NewFailure records a task that passed in the baseline but failed in the candidate.
type NewFailure struct {
	LoopID    string `json:"loop_id"`
	TaskTitle string `json:"task_title"`
	TaskType  string `json:"task_type"`
	Error     string `json:"error,omitempty"`
}

// PerformanceRegression records a metric where the candidate is statistically
// worse, using a z-test on the difference of means.
type PerformanceRegression struct {
	MetricName    string  `json:"metric_name"`
	BaselineMean  float64 `json:"baseline_mean"`
	CandidateMean float64 `json:"candidate_mean"`
	BaselineStd   float64 `json:"baseline_std"`
	CandidateStd  float64 `json:"candidate_std"`
	ZScore        float64 `json:"z_score"`
	PValue        float64 `json:"p_value"`
	Direction     string  `json:"direction"` // "degraded"
}

// DefaultThresholds returns a set of sensible metric thresholds covering the
// standard loop observation metrics.
func DefaultThresholds() map[string]MetricThreshold {
	return map[string]MetricThreshold{
		"completion_rate": {
			Extract: func(o session.LoopObservation) float64 {
				if o.VerifyPassed {
					return 1.0
				}
				return 0.0
			},
			Direction:         HigherIsBetter,
			RelativeThreshold: 0.05, // 5% drop
		},
		"cost": {
			Extract:           func(o session.LoopObservation) float64 { return o.TotalCostUSD },
			Direction:         LowerIsBetter,
			RelativeThreshold: 0.20, // 20% increase
		},
		"latency": {
			Extract:           func(o session.LoopObservation) float64 { return float64(o.TotalLatencyMs) },
			Direction:         LowerIsBetter,
			RelativeThreshold: 0.25, // 25% increase
		},
		"confidence": {
			Extract:           func(o session.LoopObservation) float64 { return o.Confidence },
			Direction:         HigherIsBetter,
			AbsoluteThreshold: 0.05, // 0.05 absolute drop
		},
	}
}

// NewRegressionDetector creates a detector with the given per-metric thresholds.
// If thresholds is nil, DefaultThresholds() is used.
func NewRegressionDetector(thresholds map[string]MetricThreshold) *RegressionDetector {
	if thresholds == nil {
		thresholds = DefaultThresholds()
	}
	return &RegressionDetector{Thresholds: thresholds}
}

// DetectRegressions compares baseline and candidate observation sets, returning
// a report of all detected regressions.
func (d *RegressionDetector) DetectRegressions(baseline, candidate []session.LoopObservation) RegressionReport {
	report := RegressionReport{
		Timestamp: time.Now(),
	}

	// Phase 1: Metric threshold regressions.
	report.Regressions = d.detectMetricRegressions(baseline, candidate)

	// Phase 2: New failures.
	report.NewFailures = detectNewFailures(baseline, candidate)

	// Phase 3: Performance regressions (statistical).
	report.PerformanceRegressions = d.detectPerformanceRegressions(baseline, candidate)

	// Compute summary.
	report.Passed = len(report.Regressions) == 0 &&
		len(report.NewFailures) == 0 &&
		len(report.PerformanceRegressions) == 0

	report.Summary = buildSummary(report)

	return report
}

// detectMetricRegressions compares aggregate metric values between baseline
// and candidate, flagging any that degraded beyond their configured thresholds.
func (d *RegressionDetector) detectMetricRegressions(baseline, candidate []session.LoopObservation) []MetricRegression {
	if len(baseline) == 0 || len(candidate) == 0 {
		return nil
	}

	var regressions []MetricRegression

	// Collect metric names and sort for deterministic output.
	names := make([]string, 0, len(d.Thresholds))
	for name := range d.Thresholds {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		thresh := d.Thresholds[name]

		baselineMean := extractMean(baseline, thresh.Extract)
		candidateMean := extractMean(candidate, thresh.Extract)

		absChange := candidateMean - baselineMean
		var relChange float64
		if baselineMean != 0 {
			relChange = absChange / math.Abs(baselineMean)
		}

		if isRegression(absChange, relChange, thresh) {
			regressions = append(regressions, MetricRegression{
				MetricName:     name,
				BaselineValue:  baselineMean,
				CandidateValue: candidateMean,
				AbsoluteChange: absChange,
				RelativeChange: relChange,
				Direction:      "degraded",
			})
		}
	}

	return regressions
}

// isRegression determines whether the observed change constitutes a regression
// given the metric's threshold configuration and direction.
func isRegression(absChange, relChange float64, thresh MetricThreshold) bool {
	// Determine the "bad" direction of change.
	var badChange float64
	switch thresh.Direction {
	case HigherIsBetter:
		// Decrease is bad; absChange < 0 means degradation.
		badChange = -absChange
	case LowerIsBetter:
		// Increase is bad; absChange > 0 means degradation.
		badChange = absChange
	}

	// If the change is not in the bad direction, no regression.
	if badChange <= 0 {
		return false
	}

	// Check absolute threshold.
	if thresh.AbsoluteThreshold > 0 && badChange >= thresh.AbsoluteThreshold {
		return true
	}

	// Check relative threshold.
	var badRelChange float64
	switch thresh.Direction {
	case HigherIsBetter:
		badRelChange = -relChange
	case LowerIsBetter:
		badRelChange = relChange
	}
	if thresh.RelativeThreshold > 0 && badRelChange >= thresh.RelativeThreshold {
		return true
	}

	return false
}

// detectNewFailures identifies tasks that passed in the baseline but failed
// in the candidate. Tasks are matched by the composite key (LoopID, TaskTitle).
func detectNewFailures(baseline, candidate []session.LoopObservation) []NewFailure {
	if len(baseline) == 0 || len(candidate) == 0 {
		return nil
	}

	// Build a set of passing tasks from the baseline.
	type taskKey struct {
		LoopID    string
		TaskTitle string
	}
	passing := make(map[taskKey]bool)
	for _, obs := range baseline {
		if obs.VerifyPassed {
			passing[taskKey{obs.LoopID, obs.TaskTitle}] = true
		}
	}

	// Find candidate failures for tasks that previously passed.
	seen := make(map[taskKey]bool)
	var failures []NewFailure
	for _, obs := range candidate {
		key := taskKey{obs.LoopID, obs.TaskTitle}
		if !obs.VerifyPassed && passing[key] && !seen[key] {
			seen[key] = true
			failures = append(failures, NewFailure{
				LoopID:    obs.LoopID,
				TaskTitle: obs.TaskTitle,
				TaskType:  obs.TaskType,
				Error:     obs.Error,
			})
		}
	}

	return failures
}

// detectPerformanceRegressions uses a two-sample z-test on the difference
// of means for each configured metric. A regression is flagged when the
// p-value is below 0.05 and the direction indicates degradation.
func (d *RegressionDetector) detectPerformanceRegressions(baseline, candidate []session.LoopObservation) []PerformanceRegression {
	// Need sufficient data for statistical comparison.
	if len(baseline) < 2 || len(candidate) < 2 {
		return nil
	}

	var regressions []PerformanceRegression

	names := make([]string, 0, len(d.Thresholds))
	for name := range d.Thresholds {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		thresh := d.Thresholds[name]

		bValues := extractValues(baseline, thresh.Extract)
		cValues := extractValues(candidate, thresh.Extract)

		bMean, bStd := meanStddev(bValues)
		cMean, cStd := meanStddev(cValues)

		// Two-sample z-test for difference of means.
		z, pValue := twoSampleZTest(bMean, bStd, len(bValues), cMean, cStd, len(cValues))

		// Determine if the change is a regression.
		isDegraded := false
		switch thresh.Direction {
		case HigherIsBetter:
			isDegraded = cMean < bMean
		case LowerIsBetter:
			isDegraded = cMean > bMean
		}

		if pValue < 0.05 && isDegraded {
			regressions = append(regressions, PerformanceRegression{
				MetricName:    name,
				BaselineMean:  bMean,
				CandidateMean: cMean,
				BaselineStd:   bStd,
				CandidateStd:  cStd,
				ZScore:        z,
				PValue:        pValue,
				Direction:     "degraded",
			})
		}
	}

	return regressions
}

// extractMean computes the arithmetic mean of a metric across observations.
func extractMean(observations []session.LoopObservation, extract MetricFunc) float64 {
	if len(observations) == 0 {
		return 0
	}
	var sum float64
	for _, obs := range observations {
		sum += extract(obs)
	}
	return sum / float64(len(observations))
}

// extractValues pulls the metric value from each observation into a slice.
func extractValues(observations []session.LoopObservation, extract MetricFunc) []float64 {
	values := make([]float64, len(observations))
	for i, obs := range observations {
		values[i] = extract(obs)
	}
	return values
}

// twoSampleZTest computes a two-sample z-test for the difference of means.
// Returns the z-score and two-tailed p-value. If the pooled standard error
// is zero, returns z=0 and p=1 (no evidence of difference).
func twoSampleZTest(mean1, std1 float64, n1 int, mean2, std2 float64, n2 int) (float64, float64) {
	if n1 == 0 || n2 == 0 {
		return 0, 1
	}

	se := math.Sqrt((std1*std1)/float64(n1) + (std2*std2)/float64(n2))
	if se == 0 {
		return 0, 1
	}

	z := (mean1 - mean2) / se
	// Two-tailed p-value using the standard normal CDF approximation.
	p := 2 * normalCDFComplement(math.Abs(z))

	return z, p
}

// normalCDFComplement returns P(Z > z) for the standard normal distribution
// using the Abramowitz and Stegun rational approximation (formula 26.2.17).
func normalCDFComplement(z float64) float64 {
	if z < 0 {
		return 1 - normalCDFComplement(-z)
	}
	// Constants for the approximation.
	const (
		b1 = 0.319381530
		b2 = -0.356563782
		b3 = 1.781477937
		b4 = -1.821255978
		b5 = 1.330274429
		p  = 0.2316419
	)
	t := 1.0 / (1.0 + p*z)
	t2 := t * t
	t3 := t2 * t
	t4 := t3 * t
	t5 := t4 * t

	phi := math.Exp(-z*z/2) / math.Sqrt(2*math.Pi)
	return phi * (b1*t + b2*t2 + b3*t3 + b4*t4 + b5*t5)
}

// buildSummary generates a human-readable summary for the regression report.
func buildSummary(report RegressionReport) string {
	if report.Passed {
		return "No regressions detected."
	}

	parts := make([]string, 0, 3)

	if n := len(report.Regressions); n > 0 {
		names := make([]string, n)
		for i, r := range report.Regressions {
			names[i] = r.MetricName
		}
		parts = append(parts, fmt.Sprintf("%d metric regression(s): %v", n, names))
	}

	if n := len(report.NewFailures); n > 0 {
		parts = append(parts, fmt.Sprintf("%d new failure(s)", n))
	}

	if n := len(report.PerformanceRegressions); n > 0 {
		names := make([]string, n)
		for i, r := range report.PerformanceRegressions {
			names[i] = r.MetricName
		}
		parts = append(parts, fmt.Sprintf("%d performance regression(s): %v", n, names))
	}

	summary := "Regressions detected: "
	for i, p := range parts {
		if i > 0 {
			summary += "; "
		}
		summary += p
	}
	return summary
}
