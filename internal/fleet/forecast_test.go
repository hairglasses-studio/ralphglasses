package fleet

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestForecaster_ConstantBurnRate(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-3 * time.Hour) // start 3h ago

	// $0.50 every minute for 120 minutes => 120 points, $60 total.
	// Dense enough that windowed rates converge for trend stability.
	for i := 0; i < 120; i++ {
		f.RecordSpend(base.Add(time.Duration(i)*time.Minute), 0.50)
	}

	rate := f.BurnRate(0)
	// $60 over 119 minutes => ~30.25/hr
	expectedRate := 60.0 / (119.0 / 60.0)
	if math.Abs(rate-expectedRate) > 0.5 {
		t.Errorf("BurnRate() = %f, want ~%f", rate, expectedRate)
	}

	report := f.Forecast(50.0, 6*time.Hour)
	if math.Abs(report.BurnRatePerHour-expectedRate) > 0.5 {
		t.Errorf("ForecastReport.BurnRatePerHour = %f, want ~%f", report.BurnRatePerHour, expectedRate)
	}

	// Trend should be stable for constant spend.
	if report.Trend != "stable" {
		t.Errorf("Trend = %q, want stable", report.Trend)
	}
}

func TestForecaster_AcceleratingTrend(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-2 * time.Hour)

	// First hour: $0.10 every minute (60 points, $6 total).
	for i := 0; i < 60; i++ {
		f.RecordSpend(base.Add(time.Duration(i)*time.Minute), 0.10)
	}
	// Last 15 minutes: $1.00 every minute (15 points, $15 total).
	for i := 0; i < 15; i++ {
		ts := base.Add(time.Hour + time.Duration(i)*time.Minute)
		f.RecordSpend(ts, 1.0)
	}

	report := f.Forecast(100.0, 4*time.Hour)
	if report.Trend != "accelerating" {
		t.Errorf("Trend = %q, want accelerating", report.Trend)
	}
}

func TestForecaster_DeceleratingTrend(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-2 * time.Hour)

	// First hour: $1.00 every minute (60 points, $60 total).
	for i := 0; i < 60; i++ {
		f.RecordSpend(base.Add(time.Duration(i)*time.Minute), 1.0)
	}
	// Last 15 minutes: $0.05 every minute (15 points, $0.75 total).
	for i := 0; i < 15; i++ {
		ts := base.Add(time.Hour + time.Duration(i)*time.Minute)
		f.RecordSpend(ts, 0.05)
	}

	report := f.Forecast(100.0, 4*time.Hour)
	if report.Trend != "decelerating" {
		t.Errorf("Trend = %q, want decelerating", report.Trend)
	}
}

func TestForecaster_ExhaustionTime(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-time.Hour)

	// $10 every 30 minutes => 2 points over 30 min => $20/0.5h = $40/hr.
	f.RecordSpend(base, 10.0)
	f.RecordSpend(base.Add(30*time.Minute), 10.0)

	remaining := 80.0 // should exhaust in 2 hours at $40/hr
	ttx := f.TimeToExhaustion(remaining)

	expectedHours := 2.0
	gotHours := ttx.Hours()
	if math.Abs(gotHours-expectedHours) > 0.05 {
		t.Errorf("TimeToExhaustion = %v (%.2f hrs), want ~%.1f hrs", ttx, gotHours, expectedHours)
	}

	report := f.Forecast(remaining, 4*time.Hour)
	if report.ExhaustionTime == nil {
		t.Fatal("ExhaustionTime should not be nil")
	}
	hoursUntil := time.Until(*report.ExhaustionTime).Hours()
	if math.Abs(hoursUntil-expectedHours) > 0.1 {
		t.Errorf("ExhaustionTime in %f hours, want ~%f", hoursUntil, expectedHours)
	}
}

func TestForecaster_ExhaustionBeyondHorizon(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-time.Hour)

	// $1/hr burn rate.
	f.RecordSpend(base, 0.5)
	f.RecordSpend(base.Add(30*time.Minute), 0.5)

	// $100 remaining at $1/hr won't exhaust within a 2-hour horizon.
	report := f.Forecast(100.0, 2*time.Hour)
	if report.ExhaustionTime != nil {
		t.Error("ExhaustionTime should be nil when beyond horizon")
	}
}

func TestForecaster_EmptyData(t *testing.T) {
	f := NewForecaster()

	rate := f.BurnRate(time.Hour)
	if rate != 0 {
		t.Errorf("BurnRate with no data = %f, want 0", rate)
	}

	ttx := f.TimeToExhaustion(100.0)
	if ttx != 0 {
		t.Errorf("TimeToExhaustion with no data = %v, want 0", ttx)
	}

	report := f.Forecast(100.0, 4*time.Hour)
	if report.BurnRatePerHour != 0 {
		t.Errorf("BurnRatePerHour = %f, want 0", report.BurnRatePerHour)
	}
	if report.ExhaustionTime != nil {
		t.Error("ExhaustionTime should be nil with no data")
	}
	if report.Trend != "stable" {
		t.Errorf("Trend = %q, want stable", report.Trend)
	}
	if report.ProjectedSpend != 0 {
		t.Errorf("ProjectedSpend = %f, want 0", report.ProjectedSpend)
	}
	if report.ConfidenceInterval != [2]float64{0, 0} {
		t.Errorf("ConfidenceInterval = %v, want [0, 0]", report.ConfidenceInterval)
	}
}

func TestForecaster_ConfidenceIntervalWidensWithVariableSpend(t *testing.T) {
	// Constant spend: narrow CI.
	fConst := NewForecaster()
	base := time.Now().Add(-time.Hour)
	for i := 0; i < 60; i++ {
		fConst.RecordSpend(base.Add(time.Duration(i)*time.Minute), 1.0)
	}
	rConst := fConst.Forecast(100.0, 4*time.Hour)

	// Variable spend: wider CI.
	fVar := NewForecaster()
	for i := 0; i < 60; i++ {
		amount := 0.5
		if i%2 == 0 {
			amount = 5.0
		}
		fVar.RecordSpend(base.Add(time.Duration(i)*time.Minute), amount)
	}
	rVar := fVar.Forecast(100.0, 4*time.Hour)

	constWidth := rConst.ConfidenceInterval[1] - rConst.ConfidenceInterval[0]
	varWidth := rVar.ConfidenceInterval[1] - rVar.ConfidenceInterval[0]

	if varWidth <= constWidth {
		t.Errorf("variable CI width (%f) should exceed constant CI width (%f)", varWidth, constWidth)
	}
}

func TestForecaster_ProjectedSpend(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-time.Hour)

	// $5 every 10 minutes for 1 hour => 6 points, $30 total over ~50min.
	for i := 0; i < 6; i++ {
		f.RecordSpend(base.Add(time.Duration(i)*10*time.Minute), 5.0)
	}

	report := f.Forecast(1000.0, 2*time.Hour)

	// BurnRate * 2 hours should equal ProjectedSpend.
	expected := report.BurnRatePerHour * 2.0
	if math.Abs(report.ProjectedSpend-expected) > 0.01 {
		t.Errorf("ProjectedSpend = %f, want %f (rate*horizon)", report.ProjectedSpend, expected)
	}
}

func TestForecaster_WindowedBurnRate(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-2 * time.Hour)

	// First hour: $1 every 10 minutes (6 points, $6).
	for i := 0; i < 6; i++ {
		f.RecordSpend(base.Add(time.Duration(i)*10*time.Minute), 1.0)
	}
	// Second hour: $10 every 10 minutes (6 points, $60).
	for i := 0; i < 6; i++ {
		f.RecordSpend(base.Add(time.Hour+time.Duration(i)*10*time.Minute), 10.0)
	}

	// Full window: ($66) over ~110 min.
	rateAll := f.BurnRate(0)

	// Last-hour window should show higher burn rate than overall.
	rate1h := f.BurnRate(time.Hour)

	if rate1h <= rateAll {
		t.Errorf("1h rate (%f) should be > all-data rate (%f) with accelerating spend", rate1h, rateAll)
	}
}

func TestForecaster_ConcurrentAccess(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-time.Hour)

	var wg sync.WaitGroup

	// Concurrent writers.
	n := 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			f.RecordSpend(base.Add(time.Duration(idx)*time.Second), 0.01)
		}(i)
	}

	// Concurrent readers.
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			f.BurnRate(time.Hour)
			f.Forecast(100.0, 4*time.Hour)
			f.TimeToExhaustion(50.0)
		}()
	}

	wg.Wait()

	if f.Len() != n {
		t.Errorf("Len() = %d, want %d after concurrent writes", f.Len(), n)
	}
}

func TestForecaster_ZeroRemaining(t *testing.T) {
	f := NewForecaster()
	base := time.Now().Add(-time.Hour)
	f.RecordSpend(base, 10.0)
	f.RecordSpend(base.Add(30*time.Minute), 10.0)

	// Zero remaining budget.
	ttx := f.TimeToExhaustion(0)
	if ttx != 0 {
		t.Errorf("TimeToExhaustion(0) = %v, want 0", ttx)
	}

	report := f.Forecast(0, 2*time.Hour)
	if report.ExhaustionTime != nil {
		t.Error("ExhaustionTime should be nil with zero remaining budget")
	}
}

func TestForecaster_SingleDataPoint(t *testing.T) {
	f := NewForecaster()
	f.RecordSpend(time.Now(), 5.0)

	rate := f.BurnRate(0)
	if rate != 0 {
		t.Errorf("BurnRate with 1 point = %f, want 0", rate)
	}

	report := f.Forecast(100.0, 4*time.Hour)
	if report.BurnRatePerHour != 0 {
		t.Errorf("BurnRatePerHour = %f, want 0", report.BurnRatePerHour)
	}
	if report.Trend != "stable" {
		t.Errorf("Trend = %q, want stable", report.Trend)
	}
}
