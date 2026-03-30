package session

import (
	"sync"
	"time"
)

// Regression represents a detected metric regression.
type Regression struct {
	Metric     string
	Baseline   float64
	Current    float64
	DropPercent float64
	DetectedAt time.Time
}

// RegressionDetector tracks metric baselines and detects regressions
// when new measurements drop below baseline by a configurable threshold.
type RegressionDetector struct {
	mu        sync.RWMutex
	baselines map[string]float64
	threshold float64 // e.g. 0.05 = 5% drop triggers regression
}

// NewRegressionDetector creates a detector with the given threshold.
// Threshold is the fractional drop to trigger (e.g. 0.05 = 5%).
func NewRegressionDetector(threshold float64) *RegressionDetector {
	return &RegressionDetector{
		baselines: make(map[string]float64),
		threshold: threshold,
	}
}

// SetBaseline records the baseline value for a metric.
func (rd *RegressionDetector) SetBaseline(metric string, value float64) {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	rd.baselines[metric] = value
}

// Check tests whether the given value represents a regression for the metric.
// Returns nil if no baseline is set or no regression is detected.
func (rd *RegressionDetector) Check(metric string, value float64) *Regression {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	baseline, ok := rd.baselines[metric]
	if !ok {
		return nil
	}

	if baseline == 0 {
		return nil
	}

	drop := (baseline - value) / baseline
	if drop > rd.threshold {
		return &Regression{
			Metric:      metric,
			Baseline:    baseline,
			Current:     value,
			DropPercent:  drop,
			DetectedAt:  time.Now(),
		}
	}
	return nil
}

// CheckAll tests multiple metrics at once and returns all detected regressions.
func (rd *RegressionDetector) CheckAll(metrics map[string]float64) []Regression {
	rd.mu.RLock()
	defer rd.mu.RUnlock()

	var regressions []Regression
	for metric, value := range metrics {
		baseline, ok := rd.baselines[metric]
		if !ok {
			continue
		}
		if baseline == 0 {
			continue
		}
		drop := (baseline - value) / baseline
		if drop > rd.threshold {
			regressions = append(regressions, Regression{
				Metric:      metric,
				Baseline:    baseline,
				Current:     value,
				DropPercent:  drop,
				DetectedAt:  time.Now(),
			})
		}
	}
	return regressions
}
