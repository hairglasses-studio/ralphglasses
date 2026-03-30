package session

import (
	"math"
	"testing"
	"time"
)

func TestRateCalculator_EmptyLedger(t *testing.T) {
	cl := NewCostLedger()
	rc := NewRateCalculator(cl, time.Hour)

	if got := rc.CurrentRate(); got != 0 {
		t.Fatalf("CurrentRate() = %f, want 0", got)
	}
	if got := rc.SessionRate("s1"); got != 0 {
		t.Fatalf("SessionRate(s1) = %f, want 0", got)
	}
	if got := rc.ProjectedCost(time.Hour); got != 0 {
		t.Fatalf("ProjectedCost(1h) = %f, want 0", got)
	}
}

func TestRateCalculator_SingleEntry(t *testing.T) {
	cl := NewCostLedger()
	cl.Record("s1", 5.00, "claude")
	rc := NewRateCalculator(cl, time.Hour)

	// With only one entry there is no time span, so rate should be 0.
	if got := rc.CurrentRate(); got != 0 {
		t.Fatalf("CurrentRate() = %f, want 0 (single entry)", got)
	}
}

func TestRateCalculator_CurrentRate(t *testing.T) {
	cl := NewCostLedger()
	now := time.Now()

	// $10 over 30 minutes = $20/hr
	cl.RecordAt("s1", 4.00, "claude", now.Add(-30*time.Minute))
	cl.RecordAt("s1", 6.00, "claude", now)

	rc := NewRateCalculator(cl, time.Hour)
	got := rc.rateFor("", now)

	if math.Abs(got-20.0) > 0.01 {
		t.Fatalf("CurrentRate() = %f, want ~20.0", got)
	}
}

func TestRateCalculator_SessionRate(t *testing.T) {
	cl := NewCostLedger()
	now := time.Now()

	// s1: $6 over 1 hour = $6/hr
	cl.RecordAt("s1", 2.00, "claude", now.Add(-time.Hour))
	cl.RecordAt("s1", 4.00, "claude", now)

	// s2: noise
	cl.RecordAt("s2", 100.00, "gemini", now.Add(-30*time.Minute))
	cl.RecordAt("s2", 100.00, "gemini", now)

	rc := NewRateCalculator(cl, 2*time.Hour)
	got := rc.rateFor("s1", now)

	if math.Abs(got-6.0) > 0.01 {
		t.Fatalf("SessionRate(s1) = %f, want ~6.0", got)
	}
}

func TestRateCalculator_WindowExcludesOld(t *testing.T) {
	cl := NewCostLedger()
	now := time.Now()

	// Old entry outside window
	cl.RecordAt("s1", 100.00, "claude", now.Add(-3*time.Hour))
	// Recent entries: $3 over 15 min = $12/hr
	cl.RecordAt("s1", 1.00, "claude", now.Add(-15*time.Minute))
	cl.RecordAt("s1", 2.00, "claude", now)

	rc := NewRateCalculator(cl, time.Hour)
	got := rc.rateFor("", now)

	if math.Abs(got-12.0) > 0.01 {
		t.Fatalf("rate = %f, want ~12.0 (old entry excluded)", got)
	}
}

func TestRateCalculator_ProjectedCost(t *testing.T) {
	cl := NewCostLedger()
	now := time.Now()

	// $10 over 30 min = $20/hr
	cl.RecordAt("s1", 4.00, "claude", now.Add(-30*time.Minute))
	cl.RecordAt("s1", 6.00, "claude", now)

	rc := NewRateCalculator(cl, time.Hour)

	// Override time for deterministic test
	got := rc.rateFor("", now) * 2.0 // 2 hours
	want := 40.0

	if math.Abs(got-want) > 0.1 {
		t.Fatalf("ProjectedCost(2h) = %f, want ~%f", got, want)
	}
}

func TestRateCalculator_AllSameTimestamp(t *testing.T) {
	cl := NewCostLedger()
	now := time.Now()

	cl.RecordAt("s1", 1.00, "claude", now)
	cl.RecordAt("s1", 2.00, "claude", now)

	rc := NewRateCalculator(cl, time.Hour)
	got := rc.rateFor("", now)

	// Zero elapsed time means rate should be 0.
	if got != 0 {
		t.Fatalf("rate = %f, want 0 (all same timestamp)", got)
	}
}
