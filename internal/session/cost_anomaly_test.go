package session

import "testing"

func TestCostAnomalyDetector_NoAnomalyWithStableRates(t *testing.T) {
	d := NewCostAnomalyDetector()
	for i := 0; i < 10; i++ {
		if d.Record("s1", 0.05) {
			t.Errorf("unexpected anomaly at iteration %d", i)
		}
	}
	alerts := d.Alerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestCostAnomalyDetector_DetectsSpike(t *testing.T) {
	d := NewCostAnomalyDetector()
	// Build baseline
	for i := 0; i < 5; i++ {
		d.Record("s1", 0.05)
	}
	// Spike: 3x the normal rate
	if !d.Record("s1", 0.15) {
		t.Error("expected anomaly on 3x spike")
	}
	alerts := d.Alerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	if alerts[0].Ratio < AnomalyThreshold {
		t.Errorf("expected ratio >= %f, got %f", AnomalyThreshold, alerts[0].Ratio)
	}
}

func TestCostAnomalyDetector_NeedsMinDataPoints(t *testing.T) {
	d := NewCostAnomalyDetector()
	// First two observations should never trigger
	d.Record("s1", 0.01)
	if d.Record("s1", 1.00) {
		t.Error("should not trigger anomaly with only 2 data points")
	}
}

func TestCostAnomalyDetector_ZeroCost(t *testing.T) {
	d := NewCostAnomalyDetector()
	if d.Record("s1", 0) {
		t.Error("zero cost should not trigger anomaly")
	}
}
