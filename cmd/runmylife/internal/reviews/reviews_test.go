package reviews

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestCaptureWeekly_EmptyDB(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	snap, err := CaptureWeekly(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("CaptureWeekly: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.WeekOf == "" {
		t.Error("WeekOf should not be empty")
	}
	if snap.TasksCompleted != 0 {
		t.Errorf("TasksCompleted = %d, want 0 on empty DB", snap.TasksCompleted)
	}
	if snap.GoodEnoughDays != 0 {
		t.Errorf("GoodEnoughDays = %d, want 0 on empty DB", snap.GoodEnoughDays)
	}
}

func TestCaptureWeekly_WithData(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// Seed tasks and mood data
	testutil.SeedTasks(t, db, 10)
	testutil.SeedMoodLog(t, db, 7)

	snap, err := CaptureWeekly(ctx, db, time.Now())
	if err != nil {
		t.Fatalf("CaptureWeekly: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	// MoodAvg should be non-zero since we seeded data for this week
	if snap.MoodAvg == 0 && snap.TasksCompleted == 0 && snap.TasksCreated == 0 {
		t.Error("expected some non-zero metrics with seeded data")
	}
}

func TestCaptureWeekly_Persistence(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	now := time.Now()
	_, err := CaptureWeekly(ctx, db, now)
	if err != nil {
		t.Fatalf("CaptureWeekly: %v", err)
	}

	// Verify row was persisted
	monday := mondayOf(now).Format("2006-01-02")
	var count int
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM weekly_review_snapshots WHERE week_of = ?", monday).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 snapshot row for %s, got %d", monday, count)
	}

	// Capture again — should replace, not duplicate
	_, err = CaptureWeekly(ctx, db, now)
	if err != nil {
		t.Fatalf("CaptureWeekly second call: %v", err)
	}
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM weekly_review_snapshots WHERE week_of = ?", monday).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 snapshot row after re-capture, got %d", count)
	}
}

func TestCaptureMonthly_MoodTrend(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	now := time.Now()
	snap, err := CaptureMonthly(ctx, db, now)
	if err != nil {
		t.Fatalf("CaptureMonthly: %v", err)
	}
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	// With no mood data, trend should be "no data"
	if snap.MoodTrend != "no data" {
		t.Errorf("MoodTrend = %q, want 'no data'", snap.MoodTrend)
	}
	if snap.EnergyTrend != "no data" {
		t.Errorf("EnergyTrend = %q, want 'no data'", snap.EnergyTrend)
	}
}

func TestGoodEnoughDays(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// With no tasks and no habits, should be 0
	count := countGoodEnoughDays(ctx, db, time.Now().AddDate(0, 0, -7), 7)
	if count != 0 {
		t.Errorf("GoodEnoughDays = %d, want 0 with empty DB", count)
	}
}

func TestFormatDelta(t *testing.T) {
	tests := []struct {
		current, previous int
		want              string
	}{
		{10, 5, "+5"},
		{3, 7, "-4"},
		{5, 5, "0"},
	}
	for _, tt := range tests {
		got := FormatDelta(tt.current, tt.previous)
		if got != tt.want {
			t.Errorf("FormatDelta(%d, %d) = %q, want %q", tt.current, tt.previous, got, tt.want)
		}
	}
}

func TestFormatDeltaF(t *testing.T) {
	tests := []struct {
		current, previous float64
		want              string
	}{
		{7.5, 6.0, "+1.5"},
		{4.0, 5.5, "-1.5"},
		{3.0, 3.0, "0.0"},
	}
	for _, tt := range tests {
		got := FormatDeltaF(tt.current, tt.previous)
		if got != tt.want {
			t.Errorf("FormatDeltaF(%.1f, %.1f) = %q, want %q", tt.current, tt.previous, got, tt.want)
		}
	}
}

func TestLoadPreviousWeekly_NoData(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	_, err := LoadPreviousWeekly(ctx, db, "2026-03-30")
	if err == nil {
		t.Error("expected error when no previous snapshot exists")
	}
}
