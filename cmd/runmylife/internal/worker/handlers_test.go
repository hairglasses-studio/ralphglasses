package worker

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/runmylife/internal/events"
	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func testJC(t *testing.T) *JobContext {
	t.Helper()
	db := testutil.TestDB(t)
	bus := events.NewBus(nil)
	emitter := events.NewEmitter(bus)
	return &JobContext{DB: db, Emitter: emitter, Notify: nil}
}

func TestHandleJob_CheckOverwhelm(t *testing.T) {
	jc := testJC(t)
	ctx := context.Background()

	// Seed some tasks to exercise the overwhelm check
	testutil.SeedTasks(t, jc.DB, 10)

	err := HandleJob(ctx, jc, "check_overwhelm", "")
	if err != nil {
		t.Fatalf("check_overwhelm: %v", err)
	}

	// Verify overwhelm metric was stored
	var count int
	jc.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM daily_overwhelm_metric").Scan(&count)
	if count == 0 {
		t.Error("expected overwhelm metric row to be stored")
	}
}

func TestHandleJob_MorningBriefing(t *testing.T) {
	jc := testJC(t)
	ctx := context.Background()

	// Should not error even with empty DB
	err := HandleJob(ctx, jc, "morning_briefing", "")
	if err != nil {
		t.Fatalf("morning_briefing: %v", err)
	}
}

func TestHandleJob_CheckHabits(t *testing.T) {
	jc := testJC(t)
	ctx := context.Background()

	testutil.SeedHabits(t, jc.DB, []string{"exercise", "reading", "meditation"})

	err := HandleJob(ctx, jc, "check_habits", "")
	if err != nil {
		t.Fatalf("check_habits: %v", err)
	}
}

func TestHandleJob_WeeklyStats(t *testing.T) {
	jc := testJC(t)
	ctx := context.Background()

	testutil.SeedTasks(t, jc.DB, 5)
	testutil.SeedMoodLog(t, jc.DB, 7)

	err := HandleJob(ctx, jc, "weekly_stats", "")
	if err != nil {
		t.Fatalf("weekly_stats: %v", err)
	}

	// Verify snapshot was created
	var count int
	jc.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM weekly_review_snapshots").Scan(&count)
	if count == 0 {
		t.Error("expected weekly review snapshot to be stored")
	}
}

func TestHandleJob_FinancialSummary(t *testing.T) {
	jc := testJC(t)
	ctx := context.Background()

	testutil.SeedTransactions(t, jc.DB, 5)

	err := HandleJob(ctx, jc, "financial_summary", "")
	if err != nil {
		t.Fatalf("financial_summary: %v", err)
	}
}

func TestHandleJob_HabitStreakReview(t *testing.T) {
	jc := testJC(t)
	ctx := context.Background()

	// Insert a habit with a streak
	jc.DB.ExecContext(ctx,
		`INSERT INTO habits (id, name, frequency, current_streak, created_at) VALUES (1, 'exercise', 'daily', 10, datetime('now'))`)

	err := HandleJob(ctx, jc, "habit_streak_review", "")
	if err != nil {
		t.Fatalf("habit_streak_review: %v", err)
	}
}

func TestHandleJob_UnknownType(t *testing.T) {
	jc := testJC(t)
	err := HandleJob(context.Background(), jc, "nonexistent_job", "")
	if err == nil {
		t.Error("expected error for unknown job type")
	}
}

func TestHandleJob_NilEmitter(t *testing.T) {
	db := testutil.TestDB(t)
	jc := &JobContext{DB: db, Emitter: nil, Notify: nil}

	// Should not panic
	err := HandleJob(context.Background(), jc, "check_overwhelm", "")
	if err != nil {
		t.Fatalf("check_overwhelm with nil emitter: %v", err)
	}
}

func TestHandleJob_ScanReplies(t *testing.T) {
	jc := testJC(t)
	// Should not error even with empty message tables
	err := HandleJob(context.Background(), jc, "scan_replies", "")
	if err != nil {
		t.Fatalf("scan_replies: %v", err)
	}
}

func TestHandleJob_LogMoodPrompt(t *testing.T) {
	jc := testJC(t)
	// No-op with nil notifier, should not error
	err := HandleJob(context.Background(), jc, "log_mood_prompt", "")
	if err != nil {
		t.Fatalf("log_mood_prompt: %v", err)
	}
}

func TestHandleJob_TomorrowPrep(t *testing.T) {
	jc := testJC(t)
	err := HandleJob(context.Background(), jc, "tomorrow_prep", "")
	if err != nil {
		t.Fatalf("tomorrow_prep: %v", err)
	}
}

func TestHandleJob_BuildKnowledgeGraph(t *testing.T) {
	jc := testJC(t)
	err := HandleJob(context.Background(), jc, "build_knowledge_graph", "")
	if err != nil {
		t.Fatalf("build_knowledge_graph: %v", err)
	}
}
