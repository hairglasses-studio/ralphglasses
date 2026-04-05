package session

import (
	"testing"
	"time"
)

func TestSpendRateMonitor_BelowThreshold(t *testing.T) {
	m := NewSpendRateMonitor(50.0)
	m.Record(10.0)
	m.Record(20.0)
	if m.Tripped() {
		t.Fatal("expected monitor not tripped below threshold")
	}
	if rate := m.HourlyRate(); rate != 30.0 {
		t.Fatalf("expected hourly rate 30.0, got %.2f", rate)
	}
}

func TestSpendRateMonitor_TripsAboveThreshold(t *testing.T) {
	m := NewSpendRateMonitor(50.0)
	m.Record(30.0)
	m.Record(25.0) // total = 55.0, exceeds $50 threshold
	if !m.Tripped() {
		t.Fatal("expected monitor to be tripped above threshold")
	}
}

func TestSpendRateMonitor_ZeroThresholdDisabled(t *testing.T) {
	m := NewSpendRateMonitor(0)
	m.Record(1000.0)
	if m.Tripped() {
		t.Fatal("expected monitor with zero threshold to never trip")
	}
}

func TestSpendRateMonitor_Reset(t *testing.T) {
	m := NewSpendRateMonitor(50.0)
	m.Record(75.0) // trip the breaker
	if !m.Tripped() {
		t.Fatal("expected tripped before reset")
	}

	m.Reset()

	if m.Tripped() {
		t.Fatal("expected not tripped after reset")
	}
	if rate := m.HourlyRate(); rate != 0 {
		t.Fatalf("expected hourly rate 0 after reset, got %.2f", rate)
	}

	// Can record spend again without retripping if below threshold.
	m.Record(10.0)
	if m.Tripped() {
		t.Fatal("expected not tripped after reset and small record")
	}
}

func TestSpendRateMonitor_BucketRotation(t *testing.T) {
	m := NewSpendRateMonitor(50.0)

	// Manually set lastAdvance to 2 minutes ago to force bucket rotation.
	m.mu.Lock()
	m.lastAdvance = time.Now().Add(-2 * time.Minute)
	// Put spend in current bucket before rotation.
	m.buckets[m.bucketIdx] = 40.0
	m.mu.Unlock()

	// Record triggers advance: 2 new buckets are zeroed, then spend is added to the new current bucket.
	m.Record(5.0)

	rate := m.HourlyRate()
	// After 2-minute advance: old bucket (40.0) was at idx 0, new buckets at idx 1 and 2 are zeroed.
	// The 40.0 from old bucket[0] still exists (not overwritten), so total = 40.0 + 5.0 = 45.0.
	if rate != 45.0 {
		t.Fatalf("expected hourly rate 45.0 after 2-minute advance, got %.2f", rate)
	}
}

func TestSpendRateMonitor_BucketRotation_ExpiresFull(t *testing.T) {
	m := NewSpendRateMonitor(200.0)

	// Fill all buckets with spend.
	m.mu.Lock()
	for i := range m.buckets {
		m.buckets[i] = 1.0
	}
	// Simulate more than 60 minutes elapsed — all buckets should be zeroed.
	m.lastAdvance = time.Now().Add(-61 * time.Minute)
	m.mu.Unlock()

	// Record a small amount — advance will clear all 60 buckets, then add to current.
	m.Record(5.0)

	rate := m.HourlyRate()
	if rate != 5.0 {
		t.Fatalf("expected hourly rate 5.0 after full rotation, got %.2f", rate)
	}
}

func TestSpendRateMonitor_TotalSpend(t *testing.T) {
	m := NewSpendRateMonitor(1000.0)
	m.Record(10.0)
	m.Record(20.0)
	m.Record(30.0)

	m.mu.Lock()
	total := m.totalSpend
	m.mu.Unlock()

	if total != 60.0 {
		t.Fatalf("expected totalSpend 60.0, got %.2f", total)
	}
}

func TestSpendRateMonitor_NegativeAmountIgnored(t *testing.T) {
	m := NewSpendRateMonitor(50.0)
	m.Record(-10.0)
	m.Record(0.0)
	if rate := m.HourlyRate(); rate != 0 {
		t.Fatalf("expected hourly rate 0 for non-positive amounts, got %.2f", rate)
	}
}
