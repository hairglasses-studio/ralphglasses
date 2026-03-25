package eval

import (
	"math"
	"sort"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Changepoint identifies a performance shift in the observation stream.
type Changepoint struct {
	Index        int       `json:"index"`        // position in the observation slice
	Timestamp    time.Time `json:"timestamp"`
	MetricName   string    `json:"metric_name"`  // e.g. "completion_rate", "cost", "latency"
	BeforeMean   float64   `json:"before_mean"`
	AfterMean    float64   `json:"after_mean"`
	Significance float64   `json:"significance"` // 0-1, higher = more significant
	Direction    string    `json:"direction"`     // "increase" or "decrease"
}

// MetricFunc extracts a numeric metric from a LoopObservation.
type MetricFunc func(session.LoopObservation) float64

// cusumDrift is the standard allowance parameter for CUSUM.
const cusumDrift = 0.5

// cusumThreshold is the decision interval for detecting a changepoint.
const cusumThreshold = 4.0

// maxChangepoints is the maximum number of changepoints returned.
const maxChangepoints = 5

// minObservations is the minimum number of observations required for analysis.
const minObservations = 10

// DetectChangepoints runs CUSUM (Cumulative Sum Control Chart) changepoint
// detection on the given observations for a single metric.
func DetectChangepoints(observations []session.LoopObservation, metric MetricFunc, metricName string) []Changepoint {
	if len(observations) < minObservations {
		return nil
	}

	// Extract metric values.
	values := make([]float64, len(observations))
	for i, obs := range observations {
		values[i] = metric(obs)
	}

	// Compute overall mean and standard deviation.
	mean, stddev := meanStddev(values)
	if stddev == 0 {
		return nil
	}

	// Normalize values.
	z := make([]float64, len(values))
	for i, v := range values {
		z[i] = (v - mean) / stddev
	}

	// Run CUSUM — positive and negative cumulative sums.
	// Track the last reset index for each to identify the actual changepoint location.
	sPos := make([]float64, len(z))
	sNeg := make([]float64, len(z))
	lastResetPos := make([]int, len(z)) // index where sPos last reset to 0
	lastResetNeg := make([]int, len(z)) // index where sNeg last reset to 0

	for i, zi := range z {
		if i == 0 {
			sPos[i] = math.Max(0, zi-cusumDrift)
			sNeg[i] = math.Min(0, zi+cusumDrift)
			lastResetPos[i] = 0
			lastResetNeg[i] = 0
		} else {
			newPos := sPos[i-1] + zi - cusumDrift
			if newPos <= 0 {
				sPos[i] = 0
				lastResetPos[i] = i
			} else {
				sPos[i] = newPos
				lastResetPos[i] = lastResetPos[i-1]
			}
			newNeg := sNeg[i-1] + zi + cusumDrift
			if newNeg >= 0 {
				sNeg[i] = 0
				lastResetNeg[i] = i
			} else {
				sNeg[i] = newNeg
				lastResetNeg[i] = lastResetNeg[i-1]
			}
		}
	}

	// Find the maximum CUSUM magnitude for significance normalization.
	var maxCUSUM float64
	for i := range z {
		if sPos[i] > maxCUSUM {
			maxCUSUM = sPos[i]
		}
		if -sNeg[i] > maxCUSUM {
			maxCUSUM = -sNeg[i]
		}
	}
	if maxCUSUM == 0 {
		return nil
	}

	// Detect changepoints where CUSUM exceeds the threshold.
	// The actual changepoint index is the last reset point, not where the
	// threshold was crossed (which lags behind the true shift).
	var changepoints []Changepoint
	detected := make(map[int]bool) // avoid duplicates at the same changepoint index

	for i := range z {
		var magnitude float64
		var cpIdx int
		exceeded := false

		if sPos[i] >= cusumThreshold {
			magnitude = sPos[i]
			cpIdx = lastResetPos[i]
			exceeded = true
		}
		if -sNeg[i] >= cusumThreshold && -sNeg[i] > magnitude {
			magnitude = -sNeg[i]
			cpIdx = lastResetNeg[i]
			exceeded = true
		}
		if !exceeded || detected[cpIdx] {
			continue
		}

		detected[cpIdx] = true

		beforeMean, afterMean := splitMean(values, cpIdx)
		significance := magnitude / maxCUSUM
		if significance > 1 {
			significance = 1
		}

		direction := "increase"
		if afterMean < beforeMean {
			direction = "decrease"
		}

		changepoints = append(changepoints, Changepoint{
			Index:        cpIdx,
			Timestamp:    observations[cpIdx].Timestamp,
			MetricName:   metricName,
			BeforeMean:   beforeMean,
			AfterMean:    afterMean,
			Significance: significance,
			Direction:    direction,
		})
	}

	// Sort by significance, highest first.
	sort.Slice(changepoints, func(i, j int) bool {
		return changepoints[i].Significance > changepoints[j].Significance
	})

	// Limit to at most maxChangepoints.
	if len(changepoints) > maxChangepoints {
		changepoints = changepoints[:maxChangepoints]
	}

	return changepoints
}

// StandardMetrics returns metric extractors for common performance signals.
func StandardMetrics() map[string]MetricFunc {
	return map[string]MetricFunc{
		"completion_rate": func(o session.LoopObservation) float64 {
			if o.VerifyPassed {
				return 1.0
			}
			return 0.0
		},
		"cost": func(o session.LoopObservation) float64 {
			return o.TotalCostUSD
		},
		"latency": func(o session.LoopObservation) float64 {
			return float64(o.TotalLatencyMs)
		},
		"confidence": func(o session.LoopObservation) float64 {
			return o.Confidence
		},
		"difficulty": func(o session.LoopObservation) float64 {
			return o.DifficultyScore
		},
	}
}

// DetectAllChangepoints runs changepoint detection for every standard metric
// and returns a map of metric name to detected changepoints.
func DetectAllChangepoints(observations []session.LoopObservation) map[string][]Changepoint {
	metrics := StandardMetrics()
	result := make(map[string][]Changepoint, len(metrics))
	for name, fn := range metrics {
		cps := DetectChangepoints(observations, fn, name)
		if cps != nil {
			result[name] = cps
		}
	}
	return result
}

// meanStddev computes the arithmetic mean and population standard deviation.
func meanStddev(values []float64) (float64, float64) {
	n := float64(len(values))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	var sumSq float64
	for _, v := range values {
		d := v - mean
		sumSq += d * d
	}
	return mean, math.Sqrt(sumSq / n)
}

// splitMean computes the mean of values before and after (inclusive) the given index.
func splitMean(values []float64, idx int) (before, after float64) {
	if idx <= 0 {
		return values[0], mean(values)
	}
	if idx >= len(values) {
		return mean(values), values[len(values)-1]
	}
	return mean(values[:idx]), mean(values[idx:])
}

// mean computes the arithmetic mean of a slice.
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
