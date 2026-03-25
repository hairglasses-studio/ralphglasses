package fleet

import (
	"math"
	"testing"
	"time"
)

func TestEmptyPredictor(t *testing.T) {
	p := NewCostPredictor(0) // should default to 2.5
	f := p.Forecast(100.0)

	if f.BurnRatePerHour != 0 {
		t.Errorf("expected zero burn rate, got %f", f.BurnRatePerHour)
	}
	if f.ExhaustionETA != nil {
		t.Error("expected nil exhaustion ETA for empty predictor")
	}
	if f.TrendDirection != "stable" {
		t.Errorf("expected stable trend, got %q", f.TrendDirection)
	}
	if f.SampleCount != 0 {
		t.Errorf("expected 0 samples, got %d", f.SampleCount)
	}
}

func TestConstantRate(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	// 10 samples, $1 each, 1 hour apart.
	for i := 0; i < 10; i++ {
		p.Record(CostSample{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			CostUSD:   1.0,
			Provider:  "claude",
			TaskType:  "session",
		})
	}

	f := p.Forecast(0)

	// Total $10 over 9 hours => ~1.111/hr.
	expectedRate := 10.0 / 9.0
	if math.Abs(f.BurnRatePerHour-expectedRate) > 0.01 {
		t.Errorf("burn rate: got %f, want ~%f", f.BurnRatePerHour, expectedRate)
	}
	if f.TrendDirection != "stable" {
		t.Errorf("trend: got %q, want stable", f.TrendDirection)
	}
}

func TestIncreasingTrend(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	// First 5 at $0.50, next 5 at $2.00.
	for i := 0; i < 10; i++ {
		cost := 0.50
		if i >= 5 {
			cost = 2.00
		}
		p.Record(CostSample{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			CostUSD:   cost,
			Provider:  "gemini",
			TaskType:  "loop_task",
		})
	}

	f := p.Forecast(50.0)
	if f.TrendDirection != "increasing" {
		t.Errorf("trend: got %q, want increasing", f.TrendDirection)
	}
}

func TestExhaustionETA(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	// 2 samples, $10 each, 1 hour apart => $20/hr burn rate.
	p.Record(CostSample{Timestamp: base, CostUSD: 10.0, Provider: "claude"})
	p.Record(CostSample{Timestamp: base.Add(time.Hour), CostUSD: 10.0, Provider: "claude"})

	f := p.Forecast(100.0)

	if f.ExhaustionETA == nil {
		t.Fatal("expected non-nil exhaustion ETA")
	}

	// Burn rate = 20/1 = 20 $/hr. 100/20 = 5 hours from now.
	hoursUntil := time.Until(*f.ExhaustionETA).Hours()
	if math.Abs(hoursUntil-5.0) > 0.1 {
		t.Errorf("ETA hours: got %f, want ~5.0", hoursUntil)
	}

	// No budget => nil ETA.
	f2 := p.Forecast(0)
	if f2.ExhaustionETA != nil {
		t.Error("expected nil ETA when budget <= 0")
	}
}

func TestAnomalyDetection(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	// 25 uniform samples at $1, then one outlier at $100.
	for i := 0; i < 25; i++ {
		p.Record(CostSample{
			Timestamp: base.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.0,
			Provider:  "claude",
		})
	}
	p.Record(CostSample{
		Timestamp: base.Add(25 * time.Minute),
		CostUSD:   100.0,
		Provider:  "claude",
	})

	anomalies := p.DetectAnomalies()
	if len(anomalies) == 0 {
		t.Fatal("expected at least one anomaly")
	}

	found := false
	for _, a := range anomalies {
		if a.ActualUSD == 100.0 {
			found = true
			if a.ZScore < 2.5 {
				t.Errorf("outlier z-score %f should be >= 2.5", a.ZScore)
			}
		}
	}
	if !found {
		t.Error("did not find the $100 outlier in anomalies")
	}
}

func TestWindowEviction(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	// Insert more than maxSamples.
	for i := 0; i < defaultMaxSamples+50; i++ {
		p.Record(CostSample{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			CostUSD:   0.01,
			Provider:  "openai",
		})
	}

	if p.Len() != defaultMaxSamples {
		t.Errorf("sample count: got %d, want %d", p.Len(), defaultMaxSamples)
	}
}

func TestCostPredictorZeroBurnRate(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	// All zero-cost samples, 1 hour apart.
	for i := 0; i < 10; i++ {
		p.Record(CostSample{
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			CostUSD:   0.0,
			Provider:  "claude",
			TaskType:  "session",
		})
	}

	f := p.Forecast(100.0)
	if f.BurnRatePerHour != 0 {
		t.Errorf("expected zero burn rate for all-zero costs, got %f", f.BurnRatePerHour)
	}
	if f.ExhaustionETA != nil {
		t.Error("expected nil exhaustion ETA when burn rate is 0")
	}
	if f.TrendDirection != "stable" {
		t.Errorf("expected stable trend for zero costs, got %q", f.TrendDirection)
	}
}

func TestCostPredictorSingleSample(t *testing.T) {
	p := NewCostPredictor(2.5)
	base := time.Now()

	p.Record(CostSample{
		Timestamp: base,
		CostUSD:   5.0,
		Provider:  "claude",
		TaskType:  "session",
	})

	f := p.Forecast(100.0)

	// With only 1 sample, Forecast returns early with defaults.
	if f.SampleCount != 1 {
		t.Errorf("expected sample count 1, got %d", f.SampleCount)
	}
	if f.BurnRatePerHour != 0 {
		t.Errorf("expected zero burn rate for single sample, got %f", f.BurnRatePerHour)
	}
	if f.TrendDirection != "stable" {
		t.Errorf("expected stable trend for single sample, got %q", f.TrendDirection)
	}

	// BurnRate() should also handle single sample.
	br := p.BurnRate()
	if br != 0 {
		t.Errorf("BurnRate with 1 sample should be 0, got %f", br)
	}
}
