package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/jobs"
	"github.com/hairglasses-studio/runmylife/internal/testutil"
	"github.com/hairglasses-studio/runmylife/internal/timecontext"
)

// --- Parse tests ---

func TestParse_Daily(t *testing.T) {
	s, err := Parse("daily:07:30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type != "daily" {
		t.Errorf("Type = %q, want daily", s.Type)
	}
	if s.Hour != 7 || s.Minute != 30 {
		t.Errorf("time = %d:%d, want 7:30", s.Hour, s.Minute)
	}
}

func TestParse_Weekly(t *testing.T) {
	s, err := Parse("weekly:Mon:09:00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type != "weekly" {
		t.Errorf("Type = %q, want weekly", s.Type)
	}
	if s.Weekday != time.Monday {
		t.Errorf("Weekday = %v, want Monday", s.Weekday)
	}
	if s.Hour != 9 || s.Minute != 0 {
		t.Errorf("time = %d:%d, want 9:00", s.Hour, s.Minute)
	}
}

func TestParse_Interval(t *testing.T) {
	s, err := Parse("interval:30m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type != "interval" {
		t.Errorf("Type = %q, want interval", s.Type)
	}
	if s.Interval != 30*time.Minute {
		t.Errorf("Interval = %v, want 30m", s.Interval)
	}
}

func TestParse_Block(t *testing.T) {
	s, err := Parse("block:work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Type != "block" {
		t.Errorf("Type = %q, want block", s.Type)
	}
	if s.Block != timecontext.Work {
		t.Errorf("Block = %q, want work", s.Block)
	}
}

func TestParse_Invalid(t *testing.T) {
	invalids := []string{
		"",
		"bad",
		"weekly:Xyz:09:00",  // bad weekday
		"interval:notadur",  // bad duration
		"unknown:foo:bar",   // unknown type
		"daily:only_one",    // missing minute
	}
	for _, s := range invalids {
		_, err := Parse(s)
		if err == nil {
			t.Errorf("Parse(%q) should have failed", s)
		}
	}
}

// --- ShouldRun tests ---

func TestShouldRun_Daily_RightTime(t *testing.T) {
	s := &Schedule{Type: "daily", Hour: 7, Minute: 0}
	now := time.Date(2026, 4, 5, 7, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 4, 4, 7, 0, 0, 0, time.UTC) // yesterday
	if !s.ShouldRun(now, lastRun) {
		t.Error("should run at 7:00 when last ran yesterday")
	}
}

func TestShouldRun_Daily_WrongTime(t *testing.T) {
	s := &Schedule{Type: "daily", Hour: 7, Minute: 0}
	now := time.Date(2026, 4, 5, 8, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 4, 4, 7, 0, 0, 0, time.UTC)
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run at 8:00 when scheduled for 7:00")
	}
}

func TestShouldRun_Daily_AlreadyRanToday(t *testing.T) {
	s := &Schedule{Type: "daily", Hour: 7, Minute: 0}
	now := time.Date(2026, 4, 5, 7, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 4, 5, 7, 0, 0, 0, time.UTC) // same day
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run twice on the same day")
	}
}

func TestShouldRun_Weekly_RightDayTime(t *testing.T) {
	s := &Schedule{Type: "weekly", Weekday: time.Sunday, Hour: 18, Minute: 0}
	// 2026-04-05 is a Sunday
	now := time.Date(2026, 4, 5, 18, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 3, 29, 18, 0, 0, 0, time.UTC) // last Sunday
	if !s.ShouldRun(now, lastRun) {
		t.Error("should run on Sunday at 18:00 when last ran previous week")
	}
}

func TestShouldRun_Weekly_WrongDay(t *testing.T) {
	s := &Schedule{Type: "weekly", Weekday: time.Sunday, Hour: 18, Minute: 0}
	// 2026-04-06 is a Monday
	now := time.Date(2026, 4, 6, 18, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 3, 29, 18, 0, 0, 0, time.UTC)
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run on Monday when scheduled for Sunday")
	}
}

func TestShouldRun_Weekly_AlreadyRanThisWeek(t *testing.T) {
	s := &Schedule{Type: "weekly", Weekday: time.Sunday, Hour: 18, Minute: 0}
	now := time.Date(2026, 4, 5, 18, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 4, 5, 18, 0, 0, 0, time.UTC) // same week
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run twice in the same week")
	}
}

func TestShouldRun_Interval_Elapsed(t *testing.T) {
	s := &Schedule{Type: "interval", Interval: 30 * time.Minute}
	now := time.Now()
	lastRun := now.Add(-35 * time.Minute)
	if !s.ShouldRun(now, lastRun) {
		t.Error("should run when interval elapsed")
	}
}

func TestShouldRun_Interval_NotElapsed(t *testing.T) {
	s := &Schedule{Type: "interval", Interval: 30 * time.Minute}
	now := time.Now()
	lastRun := now.Add(-10 * time.Minute)
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run before interval elapsed")
	}
}

func TestShouldRun_Block_Entered(t *testing.T) {
	s := &Schedule{Type: "block", Block: timecontext.Work}
	now := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)       // work block
	lastRun := time.Date(2026, 4, 5, 7, 0, 0, 0, time.UTC)    // morning block, same day
	if !s.ShouldRun(now, lastRun) {
		t.Error("should run when entering work block from morning")
	}
}

func TestShouldRun_Block_AlreadyRanToday(t *testing.T) {
	s := &Schedule{Type: "block", Block: timecontext.Work}
	now := time.Date(2026, 4, 5, 14, 0, 0, 0, time.UTC)       // still work block
	lastRun := time.Date(2026, 4, 5, 10, 0, 0, 0, time.UTC)   // ran earlier in work block today
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run again in the same block on the same day")
	}
}

func TestShouldRun_Block_WrongBlock(t *testing.T) {
	s := &Schedule{Type: "block", Block: timecontext.Work}
	now := time.Date(2026, 4, 5, 20, 0, 0, 0, time.UTC)       // evening
	lastRun := time.Date(2026, 4, 4, 10, 0, 0, 0, time.UTC)   // yesterday work
	if s.ShouldRun(now, lastRun) {
		t.Error("should not run in evening when scheduled for work block")
	}
}

// --- Builtin workflow tests ---

func TestBuiltinWorkflows_Count(t *testing.T) {
	wfs := BuiltinWorkflows()
	if len(wfs) != 4 {
		t.Errorf("builtin workflows = %d, want 4", len(wfs))
	}
}

func TestBuiltinWorkflows_ValidSchedules(t *testing.T) {
	for _, wf := range BuiltinWorkflows() {
		s, err := Parse(wf.Schedule)
		if err != nil {
			t.Errorf("workflow %q has invalid schedule %q: %v", wf.Name, wf.Schedule, err)
		}
		if s == nil {
			t.Errorf("workflow %q parsed schedule is nil", wf.Name)
		}
	}
}

func TestBuiltinWorkflows_HaveSteps(t *testing.T) {
	for _, wf := range BuiltinWorkflows() {
		if len(wf.Steps) == 0 {
			t.Errorf("workflow %q has no steps", wf.Name)
		}
		for _, step := range wf.Steps {
			if step.Action == "" {
				t.Errorf("workflow %q step %d has empty action", wf.Name, step.Order)
			}
		}
	}
}

// --- Executor tests ---

func TestExecutor_CheckAndRun(t *testing.T) {
	db := testutil.TestDB(t)
	if err := jobs.EnsureTable(db); err != nil {
		t.Fatalf("ensure job_queue: %v", err)
	}

	// Create workflows table
	db.Exec(`CREATE TABLE IF NOT EXISTS workflows (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT DEFAULT '',
		steps TEXT DEFAULT '[]',
		schedule TEXT DEFAULT '',
		enabled INTEGER DEFAULT 1,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	)`)

	// Insert a user-defined workflow that fires every 1m
	db.Exec(`INSERT INTO workflows (name, steps, schedule, enabled) VALUES (?, ?, ?, 1)`,
		"test_wf",
		`[{"order":1,"action":"test_action","description":"do test"}]`,
		"interval:1m",
	)

	e := NewExecutor(db)
	// Set last run to far in the past so interval workflows fire
	e.lastRuns["test_wf"] = time.Time{}

	e.CheckAndRun(context.Background())

	// Check that user workflow enqueued a job
	var count int
	db.QueryRow("SELECT COUNT(*) FROM job_queue WHERE type = 'test_action'").Scan(&count)
	if count != 1 {
		t.Errorf("job_queue test_action rows = %d, want 1", count)
	}
}

// --- parseWeekday tests ---

func TestParseWeekday_AllDays(t *testing.T) {
	tests := []struct {
		input string
		want  time.Weekday
	}{
		{"Sun", time.Sunday},
		{"sunday", time.Sunday},
		{"Mon", time.Monday},
		{"monday", time.Monday},
		{"Tue", time.Tuesday},
		{"tuesday", time.Tuesday},
		{"Wed", time.Wednesday},
		{"wednesday", time.Wednesday},
		{"Thu", time.Thursday},
		{"thursday", time.Thursday},
		{"Fri", time.Friday},
		{"friday", time.Friday},
		{"Sat", time.Saturday},
		{"saturday", time.Saturday},
	}
	for _, tt := range tests {
		got, err := parseWeekday(tt.input)
		if err != nil {
			t.Errorf("parseWeekday(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseWeekday(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestParseWeekday_Invalid(t *testing.T) {
	_, err := parseWeekday("Xyz")
	if err == nil {
		t.Error("parseWeekday(Xyz) should fail")
	}
}

// --- SuppressNightBlock ---

func TestSuppressNightBlock(t *testing.T) {
	// Just verify it returns a bool without panicking.
	// The result depends on current time, so we can't assert a value.
	_ = SuppressNightBlock()
}
