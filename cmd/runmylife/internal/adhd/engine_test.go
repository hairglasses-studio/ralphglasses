package adhd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestCheckOverwhelm_NoData(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	score, err := CheckOverwhelm(ctx, db)
	if err != nil {
		t.Fatalf("CheckOverwhelm: %v", err)
	}
	if score.CompositeScore < 0 || score.CompositeScore > 1 {
		t.Errorf("CompositeScore = %v, want [0,1]", score.CompositeScore)
	}
	if score.TriageActivated {
		t.Error("should not triage with empty DB")
	}
}

func TestCheckOverwhelm_HighLoad(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// Insert 40 open tasks, 12 overdue
	for i := 0; i < 40; i++ {
		due := ""
		if i < 12 {
			due = time.Now().AddDate(0, 0, -i-1).Format("2006-01-02")
		}
		db.ExecContext(ctx,
			`INSERT INTO tasks (id, title, priority, due_date, completed) VALUES (?, ?, 2, ?, 0)`,
			fmt.Sprintf("task-%d", i), fmt.Sprintf("Task %d", i), due)
	}

	score, err := CheckOverwhelm(ctx, db)
	if err != nil {
		t.Fatalf("CheckOverwhelm: %v", err)
	}
	if score.OpenTasks != 40 {
		t.Errorf("OpenTasks = %d, want 40", score.OpenTasks)
	}
	if score.OverdueCount != 12 {
		t.Errorf("OverdueCount = %d, want 12", score.OverdueCount)
	}
	if !score.TriageActivated {
		t.Errorf("CompositeScore = %.2f, expected triage (>= 0.7)", score.CompositeScore)
	}
}

func TestBuildEnergyCurve(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()
	today := time.Now().Format("2006-01-02")

	curve := BuildEnergyCurve(ctx, db, today)
	if curve == nil {
		t.Fatal("BuildEnergyCurve returned nil")
	}
	if curve.Date != today {
		t.Errorf("Date = %q, want %q", curve.Date, today)
	}
}

func TestDetectHyperfocus_NoSession(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	alert, err := DetectHyperfocus(ctx, db)
	if err != nil {
		t.Fatalf("DetectHyperfocus: %v", err)
	}
	if alert != nil {
		t.Error("expected nil alert with no active session")
	}
}

func TestDetectHyperfocus_LongSession(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// Insert an active session started 2 hours ago
	startedAt := time.Now().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
	db.ExecContext(ctx,
		`INSERT INTO focus_sessions (id, category, started_at, planned_minutes) VALUES (1, 'coding', ?, 25)`,
		startedAt)

	alert, err := DetectHyperfocus(ctx, db)
	if err != nil {
		t.Fatalf("DetectHyperfocus: %v", err)
	}
	if alert == nil {
		t.Fatal("expected hyperfocus alert for 2h session")
	}
	if alert.Category != "coding" {
		t.Errorf("Category = %q, want coding", alert.Category)
	}
	if alert.Minutes < 110 {
		t.Errorf("Minutes = %d, want >= 110", alert.Minutes)
	}
	if !alert.ShouldBreak {
		t.Error("2h session should trigger ShouldBreak")
	}
}

func TestGetSwitchStats_Empty(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	stats, err := GetSwitchStats(ctx, db, "2020-01-01")
	if err != nil {
		t.Fatalf("GetSwitchStats: %v", err)
	}
	if stats.TotalSwitches != 0 {
		t.Errorf("TotalSwitches = %d, want 0", stats.TotalSwitches)
	}
}

func TestEstimateSwitchCost(t *testing.T) {
	tests := []struct {
		from, to string
		wantMin  int
	}{
		{"personal", "personal", 2},
		{"personal", "studio", 15},
		{"studio", "personal", 15},
		{"growth", "growth", 2},
		{"unknown", "other", 10},
	}
	for _, tt := range tests {
		est := EstimateSwitchCost(tt.from, tt.to)
		if est.CostMinutes != tt.wantMin {
			t.Errorf("EstimateSwitchCost(%s→%s) = %d, want %d", tt.from, tt.to, est.CostMinutes, tt.wantMin)
		}
		if est.Reason == "" {
			t.Errorf("EstimateSwitchCost(%s→%s) has empty reason", tt.from, tt.to)
		}
	}
}

func TestCheckStreaks_Empty(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	streaks := CheckStreaks(ctx, db)
	if len(streaks) != 0 {
		t.Errorf("expected no streaks, got %v", streaks)
	}
}

func TestCheckStreaks_WithData(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	db.ExecContext(ctx,
		`INSERT INTO habits (id, name, frequency, current_streak, created_at) VALUES (1, 'exercise', 'daily', 5, datetime('now'))`)
	db.ExecContext(ctx,
		`INSERT INTO habits (id, name, frequency, current_streak, created_at) VALUES (2, 'reading', 'daily', 0, datetime('now'))`)

	streaks := CheckStreaks(ctx, db)
	if streaks["exercise"] != 5 {
		t.Errorf("exercise streak = %d, want 5", streaks["exercise"])
	}
	if _, ok := streaks["reading"]; ok {
		t.Error("reading streak should not appear (streak = 0)")
	}
}
