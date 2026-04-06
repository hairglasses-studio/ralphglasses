package session

import (
	"math"
	"testing"
	"time"
)

func TestBudgetForecast_ExhaustedBudget(t *testing.T) {
	t.Parallel()
	f := NewBudgetForecaster(time.Hour)
	fc := f.Forecast(10.0, 10.0)
	if fc.RemainingUSD != 0 {
		t.Errorf("expected 0 remaining, got %f", fc.RemainingUSD)
	}
	if len(fc.Warnings) == 0 || fc.Warnings[0] != "budget exhausted" {
		t.Error("expected budget exhausted warning")
	}
}

func TestBudgetForecast_InsufficientData(t *testing.T) {
	t.Parallel()
	f := NewBudgetForecaster(time.Hour)
	f.AddSample(SpendSample{Timestamp: time.Now(), TotalUSD: 1.0})
	fc := f.Forecast(10.0, 1.0)
	if fc.ConfidencePct > 10 {
		t.Errorf("expected low confidence with 1 sample, got %.0f%%", fc.ConfidencePct)
	}
}

func TestBudgetForecast_SteadySpend(t *testing.T) {
	t.Parallel()
	f := NewBudgetForecaster(time.Hour)

	now := time.Now()
	// $1/hour spend rate: 5 samples over 4 hours.
	for i := 0; i < 5; i++ {
		f.AddSample(SpendSample{
			Timestamp: now.Add(time.Duration(i) * time.Hour),
			TotalUSD:  float64(i),
		})
	}

	fc := f.Forecast(10.0, 4.0)
	if fc.RemainingUSD != 6.0 {
		t.Errorf("expected 6.0 remaining, got %f", fc.RemainingUSD)
	}
	// Velocity should be ~$1/hr.
	if math.Abs(fc.VelocityPerHr-1.0) > 0.1 {
		t.Errorf("expected ~$1/hr velocity, got $%.2f/hr", fc.VelocityPerHr)
	}
	// 6 hours left at $1/hr.
	if math.Abs(fc.EstHoursLeft-6.0) > 0.5 {
		t.Errorf("expected ~6 hours left, got %.1f", fc.EstHoursLeft)
	}
	if fc.ConfidencePct < 50 {
		t.Errorf("expected decent confidence for steady spend, got %.0f%%", fc.ConfidencePct)
	}
}

func TestBudgetForecast_HighSpendWarning(t *testing.T) {
	t.Parallel()
	f := NewBudgetForecaster(2 * time.Hour)

	now := time.Now()
	// $5/hr spend rate.
	for i := 0; i < 5; i++ {
		f.AddSample(SpendSample{
			Timestamp: now.Add(time.Duration(i) * 30 * time.Minute),
			TotalUSD:  float64(i) * 2.5,
		})
	}

	fc := f.Forecast(12.0, 10.0)
	hasWarning := false
	for _, w := range fc.Warnings {
		if w == "budget exhaustion estimated within 1 hour" || w == "over 90% of budget consumed" {
			hasWarning = true
		}
	}
	if !hasWarning {
		t.Error("expected urgency warning for high spend rate")
	}
}

func TestBudgetForecast_ZeroVelocity(t *testing.T) {
	t.Parallel()
	f := NewBudgetForecaster(time.Hour)

	now := time.Now()
	// No spend change.
	for i := 0; i < 5; i++ {
		f.AddSample(SpendSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			TotalUSD:  5.0,
		})
	}

	fc := f.Forecast(10.0, 5.0)
	if fc.VelocityPerHr != 0 {
		t.Errorf("expected 0 velocity, got %f", fc.VelocityPerHr)
	}
	if !math.IsInf(fc.EstHoursLeft, 1) {
		t.Errorf("expected infinite hours left, got %f", fc.EstHoursLeft)
	}
}

func TestBudgetForecast_WindowFilter(t *testing.T) {
	t.Parallel()
	f := NewBudgetForecaster(30 * time.Minute) // 30-min window

	now := time.Now()
	// Old samples (outside window).
	f.AddSample(SpendSample{Timestamp: now.Add(-2 * time.Hour), TotalUSD: 0})
	f.AddSample(SpendSample{Timestamp: now.Add(-1 * time.Hour), TotalUSD: 5})
	// Recent samples (inside window).
	f.AddSample(SpendSample{Timestamp: now.Add(-20 * time.Minute), TotalUSD: 6})
	f.AddSample(SpendSample{Timestamp: now.Add(-10 * time.Minute), TotalUSD: 7})
	f.AddSample(SpendSample{Timestamp: now, TotalUSD: 8})

	fc := f.Forecast(20.0, 8.0)
	// Only recent samples should be used. $6->$8 in 20 min = $6/hr.
	if fc.VelocityPerHr < 3 || fc.VelocityPerHr > 9 {
		t.Errorf("expected velocity ~$6/hr from recent window, got $%.2f/hr", fc.VelocityPerHr)
	}
}

func TestComputeConfidence_HighVariance(t *testing.T) {
	t.Parallel()
	// Erratic samples should yield lower confidence.
	now := time.Now()
	samples := []SpendSample{
		{Timestamp: now, TotalUSD: 0},
		{Timestamp: now.Add(time.Minute), TotalUSD: 10},  // spike
		{Timestamp: now.Add(2 * time.Minute), TotalUSD: 10.1}, // crawl
		{Timestamp: now.Add(3 * time.Minute), TotalUSD: 25}, // spike
		{Timestamp: now.Add(4 * time.Minute), TotalUSD: 25.1}, // crawl
	}
	velocity := forecastVelocity(samples)
	conf := forecastConfidence(samples, velocity)

	// High variance should reduce confidence.
	if conf > 70 {
		t.Errorf("expected lower confidence for erratic spend, got %.0f%%", conf)
	}
}
