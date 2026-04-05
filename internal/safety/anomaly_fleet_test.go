package safety

import (
	"testing"
	"time"
)

func TestFleetAnomalyDetector_SpendSpike(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)

	// Record spend below threshold
	d.RecordSpend(10.0)
	anomalies := d.Check()
	if len(anomalies) != 0 {
		t.Errorf("expected 0 anomalies below threshold, got %d", len(anomalies))
	}

	// Record spend above threshold (default 50.0)
	d.RecordSpend(45.0)
	anomalies = d.Check()
	found := false
	for _, a := range anomalies {
		if a.Type == MultiRepoSpendSpike {
			found = true
			if a.Severity != SeverityCritical {
				t.Errorf("spend spike severity = %s, want critical", a.Severity)
			}
		}
	}
	if !found {
		t.Error("expected MultiRepoSpendSpike anomaly")
	}
}

func TestFleetAnomalyDetector_ProviderDegradation(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)

	// Record 10 outcomes: 2 successes, 8 failures (20% success rate)
	for range 2 {
		d.RecordProviderOutcome("claude", true)
	}
	for range 8 {
		d.RecordProviderOutcome("claude", false)
	}

	anomalies := d.Check()
	found := false
	for _, a := range anomalies {
		if a.Type == ModelDegradation {
			found = true
			if a.Severity != SeverityWarning {
				t.Errorf("degradation severity = %s, want warning", a.Severity)
			}
		}
	}
	if !found {
		t.Error("expected ModelDegradation anomaly for 20% success rate")
	}
}

func TestFleetAnomalyDetector_ProviderDegradation_Healthy(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)

	// Record 10 outcomes: 9 successes, 1 failure (90% success rate)
	for range 9 {
		d.RecordProviderOutcome("gemini", true)
	}
	d.RecordProviderOutcome("gemini", false)

	anomalies := d.Check()
	for _, a := range anomalies {
		if a.Type == ModelDegradation {
			t.Error("unexpected ModelDegradation for healthy provider")
		}
	}
}

func TestFleetAnomalyDetector_BudgetExhaustion(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)
	d.SetBudget(100.0, 99.5)

	// Already near exhaustion
	d.RecordSpend(0.5)
	anomalies := d.Check()
	found := false
	for _, a := range anomalies {
		if a.Type == FleetBudgetExhaustion {
			found = true
		}
	}
	if !found {
		t.Error("expected FleetBudgetExhaustion anomaly")
	}
}

func TestFleetAnomalyDetector_WorkerSaturation(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)

	// Below threshold
	d.SetQueueDepth(5)
	anomalies := d.Check()
	for _, a := range anomalies {
		if a.Type == WorkerSaturation {
			t.Error("unexpected WorkerSaturation below threshold")
		}
	}

	// Above threshold (default 20)
	d.SetQueueDepth(25)
	anomalies = d.Check()
	found := false
	for _, a := range anomalies {
		if a.Type == WorkerSaturation {
			found = true
		}
	}
	if !found {
		t.Error("expected WorkerSaturation anomaly")
	}
}

func TestFleetAnomalyDetector_Callback(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)

	var received []Anomaly
	d.OnAnomaly(func(a Anomaly) {
		received = append(received, a)
	})

	d.RecordSpend(60.0) // triggers spike
	d.Check()

	if len(received) == 0 {
		t.Error("expected callback to fire")
	}
}

func TestFleetAnomalyDetector_TrimOldEvents(t *testing.T) {
	d := NewFleetAnomalyDetector(nil, DefaultFleetAnomalyConfig(), nil)

	// Add old spend event
	d.mu.Lock()
	d.recentSpend = append(d.recentSpend, spendEvent{
		CostUSD:   100.0,
		Timestamp: time.Now().Add(-1 * time.Hour), // old
	})
	d.mu.Unlock()

	// Trim should remove old events
	d.RecordSpend(1.0) // triggers trim

	d.mu.Lock()
	count := len(d.recentSpend)
	d.mu.Unlock()

	if count > 2 {
		t.Errorf("expected old events trimmed, got %d", count)
	}
}
