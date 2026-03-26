package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHITLTracker_RecordAndScore(t *testing.T) {
	dir := t.TempDir()
	tracker := NewHITLTracker(dir)

	// Record some events
	tracker.RecordManual(MetricManualSessionStop, "s1", "repo1", "user stopped")
	tracker.RecordManual(MetricManualSessionLaunch, "s2", "repo1", "user launched")
	tracker.RecordAuto(MetricAutoRecovery, "s3", "repo1", "auto-restarted")
	tracker.RecordAuto(MetricSessionCompleted, "s4", "repo1", "completed")
	tracker.RecordAuto(MetricSessionCompleted, "s5", "repo2", "completed")

	snap := tracker.CurrentScore(24 * time.Hour)

	if snap.TotalActions != 5 {
		t.Errorf("total: got %d, want 5", snap.TotalActions)
	}
	if snap.ManualInterventions != 2 {
		t.Errorf("manual: got %d, want 2", snap.ManualInterventions)
	}
	if snap.AutoActions != 3 {
		t.Errorf("auto: got %d, want 3", snap.AutoActions)
	}

	// HITL score should be 40% (2/5 * 100)
	expectedScore := 40.0
	if snap.HITLScore != expectedScore {
		t.Errorf("HITL score: got %.1f, want %.1f", snap.HITLScore, expectedScore)
	}
}

func TestHITLTracker_History(t *testing.T) {
	dir := t.TempDir()
	tracker := NewHITLTracker(dir)

	tracker.RecordManual(MetricManualSessionStop, "s1", "repo1", "stop1")
	tracker.RecordAuto(MetricAutoRecovery, "s2", "repo1", "recover")
	tracker.RecordManual(MetricManualConfigChange, "s3", "repo2", "config")

	events := tracker.History(24*time.Hour, 10)
	if len(events) != 3 {
		t.Errorf("got %d events, want 3", len(events))
	}

	// Should be in chronological order
	if events[0].MetricType != MetricManualSessionStop {
		t.Error("first event should be stop")
	}
}

func TestHITLTracker_WindowFiltering(t *testing.T) {
	dir := t.TempDir()
	tracker := NewHITLTracker(dir)

	// Record an old event
	oldEvent := HITLEvent{
		Timestamp:  time.Now().Add(-48 * time.Hour),
		MetricType: MetricManualSessionStop,
		Trigger:    TriggerManual,
	}
	tracker.Record(oldEvent)

	// Record a recent event
	tracker.RecordAuto(MetricSessionCompleted, "s1", "repo1", "done")

	snap := tracker.CurrentScore(24 * time.Hour)
	if snap.TotalActions != 1 {
		t.Errorf("total in window: got %d, want 1 (old event should be excluded)", snap.TotalActions)
	}
}

func TestHITLTracker_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create tracker and record events
	t1 := NewHITLTracker(dir)
	t1.RecordManual(MetricManualSessionStop, "s1", "repo1", "stop")
	t1.RecordAuto(MetricAutoRecovery, "s2", "repo1", "recover")

	// Verify file exists
	path := filepath.Join(dir, "hitl_events.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("events file not created: %v", err)
	}

	// Create new tracker from same dir
	t2 := NewHITLTracker(dir)
	snap := t2.CurrentScore(24 * time.Hour)
	if snap.TotalActions != 2 {
		t.Errorf("after reload: got %d events, want 2", snap.TotalActions)
	}
}

func TestHITLTracker_AutoRecoveryRate(t *testing.T) {
	dir := t.TempDir()
	tracker := NewHITLTracker(dir)

	// 2 errors, 1 auto-recovered
	tracker.RecordAuto(MetricAutoRecovery, "s1", "repo1", "auto")
	tracker.RecordAuto(MetricSessionErrored, "s2", "repo1", "error")
	tracker.RecordManual(MetricManualRestart, "s3", "repo1", "manual restart")

	snap := tracker.CurrentScore(24 * time.Hour)
	// Auto-recovery rate: 1 auto-recovered / 2 total errors = 50%
	if snap.AutoRecoveryRate != 50 {
		t.Errorf("auto recovery rate: got %.1f%%, want 50%%", snap.AutoRecoveryRate)
	}
}

func TestHITLTracker_ComputeTrend(t *testing.T) {
	dir := t.TempDir()
	tracker := NewHITLTracker(dir)

	now := time.Now()
	window := 24 * time.Hour

	// Insufficient data — no events
	if trend := tracker.computeTrend(now.Add(-window), window); trend != "insufficient_data" {
		t.Errorf("empty: got %q, want insufficient_data", trend)
	}

	// Add events in current window (high manual rate)
	tracker.Record(HITLEvent{Timestamp: now.Add(-1 * time.Hour), Trigger: TriggerManual, MetricType: MetricManualSessionStop})
	tracker.Record(HITLEvent{Timestamp: now.Add(-2 * time.Hour), Trigger: TriggerManual, MetricType: MetricManualSessionLaunch})
	tracker.Record(HITLEvent{Timestamp: now.Add(-3 * time.Hour), Trigger: TriggerAutomatic, MetricType: MetricSessionCompleted})

	// Add events in previous window (low manual rate)
	tracker.Record(HITLEvent{Timestamp: now.Add(-25 * time.Hour), Trigger: TriggerAutomatic, MetricType: MetricSessionCompleted})
	tracker.Record(HITLEvent{Timestamp: now.Add(-26 * time.Hour), Trigger: TriggerAutomatic, MetricType: MetricSessionCompleted})
	tracker.Record(HITLEvent{Timestamp: now.Add(-27 * time.Hour), Trigger: TriggerAutomatic, MetricType: MetricAutoRecovery})

	// Current: 2/3 manual = 66%, Previous: 0/3 = 0% => degrading
	trend := tracker.computeTrend(now.Add(-window), window)
	if trend != "degrading" {
		t.Errorf("got %q, want degrading", trend)
	}
}

func TestHITLTracker_SessionCompletionRate(t *testing.T) {
	dir := t.TempDir()
	tracker := NewHITLTracker(dir)

	tracker.RecordAuto(MetricSessionCompleted, "s1", "repo1", "done")
	tracker.RecordAuto(MetricSessionCompleted, "s2", "repo1", "done")
	tracker.RecordAuto(MetricSessionErrored, "s3", "repo1", "error")

	snap := tracker.CurrentScore(24 * time.Hour)
	// 2 completed / 3 total = 66.67%
	expected := 200.0 / 3.0
	if snap.SessionCompletionRate < expected-0.1 || snap.SessionCompletionRate > expected+0.1 {
		t.Errorf("completion rate: got %.1f%%, want ~%.1f%%", snap.SessionCompletionRate, expected)
	}
}
