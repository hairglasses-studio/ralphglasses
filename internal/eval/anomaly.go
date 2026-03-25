package eval

import (
	"fmt"
	"math"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// SlidingWindowAnomaly detects anomalies using a sliding-window z-score method.
type SlidingWindowAnomaly struct {
	WindowSize int     // lookback window (default 20)
	Threshold  float64 // z-score threshold (default 2.5)
}

// Anomaly represents a detected anomaly in a metric stream.
type Anomaly struct {
	Index     int       `json:"index"`
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Expected  float64   `json:"expected"`
	ZScore    float64   `json:"z_score"`
	Direction string    `json:"direction"` // "high" or "low"
}

// NewSlidingWindowAnomaly creates an anomaly detector with given params.
// Uses defaults if windowSize <= 0 or threshold <= 0.
func NewSlidingWindowAnomaly(windowSize int, threshold float64) *SlidingWindowAnomaly {
	if windowSize <= 0 {
		windowSize = 20
	}
	if threshold <= 0 {
		threshold = 2.5
	}
	return &SlidingWindowAnomaly{
		WindowSize: windowSize,
		Threshold:  threshold,
	}
}

// Detect scans a time series for anomalies using sliding z-scores.
// values and timestamps must be the same length.
// For each point i (where i >= WindowSize), compute mean and stddev of
// the preceding WindowSize points. If |value[i] - mean| / stddev > Threshold,
// mark as anomaly. When stddev is 0, any non-equal value is an anomaly.
func (s *SlidingWindowAnomaly) Detect(values []float64, timestamps []time.Time) []Anomaly {
	if len(values) != len(timestamps) || len(values) <= s.WindowSize {
		return nil
	}

	var anomalies []Anomaly
	for i := s.WindowSize; i < len(values); i++ {
		window := values[i-s.WindowSize : i]
		mean, stddev := meanStddev(window)

		if stddev == 0 {
			if values[i] != mean {
				dir := "high"
				if values[i] < mean {
					dir = "low"
				}
				anomalies = append(anomalies, Anomaly{
					Index:     i,
					Timestamp: timestamps[i],
					Value:     values[i],
					Expected:  mean,
					ZScore:    math.Inf(1),
					Direction: dir,
				})
			}
			continue
		}

		z := (values[i] - mean) / stddev
		if math.Abs(z) > s.Threshold {
			dir := "high"
			if z < 0 {
				dir = "low"
			}
			anomalies = append(anomalies, Anomaly{
				Index:     i,
				Timestamp: timestamps[i],
				Value:     values[i],
				Expected:  mean,
				ZScore:    z,
				Direction: dir,
			})
		}
	}
	return anomalies
}

// AnomalyMetrics maps metric names to extractor functions for LoopObservation fields.
// This is a superset of the StandardMetrics used by changepoint detection.
func AnomalyMetrics() map[string]func(session.LoopObservation) float64 {
	return map[string]func(session.LoopObservation) float64{
		"total_cost_usd":     func(o session.LoopObservation) float64 { return o.TotalCostUSD },
		"planner_cost_usd":   func(o session.LoopObservation) float64 { return o.PlannerCostUSD },
		"worker_cost_usd":    func(o session.LoopObservation) float64 { return o.WorkerCostUSD },
		"total_latency_ms":   func(o session.LoopObservation) float64 { return float64(o.TotalLatencyMs) },
		"planner_latency_ms": func(o session.LoopObservation) float64 { return float64(o.PlannerLatencyMs) },
		"worker_latency_ms":  func(o session.LoopObservation) float64 { return float64(o.WorkerLatencyMs) },
		"verify_latency_ms":  func(o session.LoopObservation) float64 { return float64(o.VerifyLatencyMs) },
		"files_changed":      func(o session.LoopObservation) float64 { return float64(o.FilesChanged) },
		"lines_added":        func(o session.LoopObservation) float64 { return float64(o.LinesAdded) },
		"lines_removed":      func(o session.LoopObservation) float64 { return float64(o.LinesRemoved) },
		"confidence":         func(o session.LoopObservation) float64 { return o.Confidence },
		"difficulty_score":   func(o session.LoopObservation) float64 { return o.DifficultyScore },
	}
}

// DetectFromObservations runs anomaly detection on loop observations using
// a named metric from StandardMetrics().
func DetectFromObservations(observations []session.LoopObservation, metricName string) ([]Anomaly, error) {
	metrics := AnomalyMetrics()
	extract, ok := metrics[metricName]
	if !ok {
		return nil, fmt.Errorf("unknown metric %q; valid metrics: %v", metricName, metricKeys())
	}

	values := make([]float64, len(observations))
	timestamps := make([]time.Time, len(observations))
	for i, obs := range observations {
		values[i] = extract(obs)
		timestamps[i] = obs.Timestamp
	}

	detector := NewSlidingWindowAnomaly(0, 0) // use defaults
	return detector.Detect(values, timestamps), nil
}

// meanStddev is defined in changepoint.go (same package).

// metricKeys returns sorted metric names for error messages.
func metricKeys() []string {
	m := AnomalyMetrics()
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
