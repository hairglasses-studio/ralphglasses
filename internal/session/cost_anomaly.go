package session

import (
	"log/slog"
	"sync"
)

// AnomalyThreshold is the multiplier above which a cost rate is flagged.
const AnomalyThreshold = 2.0

// CostAnomalyDetector tracks per-session cost rates and flags anomalies
// when the current rate exceeds 2x the historical average.
type CostAnomalyDetector struct {
	mu       sync.Mutex
	history  map[string][]float64 // session ID -> cost-per-turn history
	alerts   []CostAnomaly
}

// CostAnomaly records a detected cost anomaly.
type CostAnomaly struct {
	SessionID   string  `json:"session_id"`
	CurrentRate float64 `json:"current_rate"`
	AvgRate     float64 `json:"avg_rate"`
	Ratio       float64 `json:"ratio"`
}

// NewCostAnomalyDetector creates a new detector.
func NewCostAnomalyDetector() *CostAnomalyDetector {
	return &CostAnomalyDetector{
		history: make(map[string][]float64),
	}
}

// Record adds a cost observation for a session and checks for anomalies.
// Returns true if an anomaly was detected.
func (d *CostAnomalyDetector) Record(sessionID string, costPerTurn float64) bool {
	if costPerTurn <= 0 {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	hist := d.history[sessionID]
	hist = append(hist, costPerTurn)
	// Keep last 50 observations
	if len(hist) > 50 {
		hist = hist[len(hist)-50:]
	}
	d.history[sessionID] = hist

	// Need at least 3 data points to detect anomalies
	if len(hist) < 3 {
		return false
	}

	// Compare current rate to average of previous observations
	var sum float64
	for _, v := range hist[:len(hist)-1] {
		sum += v
	}
	avg := sum / float64(len(hist)-1)
	if avg <= 0 {
		return false
	}

	ratio := costPerTurn / avg
	if ratio >= AnomalyThreshold {
		anomaly := CostAnomaly{
			SessionID:   sessionID,
			CurrentRate: costPerTurn,
			AvgRate:     avg,
			Ratio:       ratio,
		}
		d.alerts = append(d.alerts, anomaly)
		slog.Warn("cost anomaly detected",
			"session_id", sessionID,
			"current_rate", costPerTurn,
			"avg_rate", avg,
			"ratio", ratio,
		)
		return true
	}
	return false
}

// Alerts returns all detected anomalies and clears the list.
func (d *CostAnomalyDetector) Alerts() []CostAnomaly {
	d.mu.Lock()
	defer d.mu.Unlock()
	alerts := d.alerts
	d.alerts = nil
	return alerts
}
