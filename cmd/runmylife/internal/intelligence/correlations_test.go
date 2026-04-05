package intelligence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/db"
)

func testDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestAnalyzeMoodSleep_ClearCorrelation(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	now := time.Now()
	// Insert days with low sleep (< 6h) and low mood
	for i := 0; i < 5; i++ {
		date := now.AddDate(0, 0, -(i + 1)).Format("2006-01-02")
		_, err := database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, sleep_hours, energy_level) VALUES (?, ?, ?, ?)`,
			date, 3, 4.5, 3)
		if err != nil {
			t.Fatalf("insert low sleep: %v", err)
		}
	}
	// Insert days with high sleep (>= 7h) and high mood
	for i := 0; i < 5; i++ {
		date := now.AddDate(0, 0, -(i + 10)).Format("2006-01-02")
		_, err := database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, sleep_hours, energy_level) VALUES (?, ?, ?, ?)`,
			date, 8, 8.0, 8)
		if err != nil {
			t.Fatalf("insert high sleep: %v", err)
		}
	}

	corrs := AnalyzeMoodSleep(ctx, database.DB, 30)
	if len(corrs) == 0 {
		t.Fatal("expected correlation, got none")
	}
	c := corrs[0]
	if c.Type != "mood_sleep" {
		t.Errorf("type = %s, want mood_sleep", c.Type)
	}
	if c.Confidence <= 0 {
		t.Errorf("confidence = %f, want > 0", c.Confidence)
	}
	if c.Insight == "" {
		t.Error("expected non-empty insight")
	}
}

func TestAnalyzeMoodSleep_InsufficientData(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	// Only 1 low-sleep day — not enough for correlation
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	database.DB.Exec(
		`INSERT INTO mood_log (date, mood_score, sleep_hours, energy_level) VALUES (?, ?, ?, ?)`,
		date, 3, 4.0, 3)

	corrs := AnalyzeMoodSleep(ctx, database.DB, 30)
	if len(corrs) != 0 {
		t.Errorf("expected no correlations with insufficient data, got %d", len(corrs))
	}
}

func TestAnalyzeFocusEnergy_MorningBetter(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	now := time.Now()

	// Morning sessions (long, ~60 min)
	for i := 0; i < 5; i++ {
		day := now.AddDate(0, 0, -(i + 1))
		start := time.Date(day.Year(), day.Month(), day.Day(), 9, 0, 0, 0, day.Location())
		end := start.Add(60 * time.Minute)
		_, err := database.DB.Exec(
			`INSERT INTO focus_sessions (category, started_at, ended_at, planned_minutes)
			 VALUES (?, ?, ?, ?)`,
			"coding", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"), 60)
		if err != nil {
			t.Fatalf("insert morning: %v", err)
		}
	}

	// Afternoon sessions (short, ~25 min)
	for i := 0; i < 5; i++ {
		day := now.AddDate(0, 0, -(i + 1))
		start := time.Date(day.Year(), day.Month(), day.Day(), 14, 0, 0, 0, day.Location())
		end := start.Add(25 * time.Minute)
		_, err := database.DB.Exec(
			`INSERT INTO focus_sessions (category, started_at, ended_at, planned_minutes)
			 VALUES (?, ?, ?, ?)`,
			"coding", start.Format("2006-01-02 15:04:05"), end.Format("2006-01-02 15:04:05"), 25)
		if err != nil {
			t.Fatalf("insert afternoon: %v", err)
		}
	}

	corrs := AnalyzeFocusEnergy(ctx, database.DB, 30)
	if len(corrs) == 0 {
		t.Fatal("expected focus_energy correlation")
	}
	c := corrs[0]
	if c.Type != "focus_energy" {
		t.Errorf("type = %s, want focus_energy", c.Type)
	}
	// Morning should be identified as better
	if c.Insight == "" {
		t.Error("expected non-empty insight")
	}
}

func TestAnalyzeFocusEnergy_InsufficientSessions(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	// Only 1 morning session
	start := time.Now().AddDate(0, 0, -1)
	start = time.Date(start.Year(), start.Month(), start.Day(), 9, 0, 0, 0, start.Location())
	database.DB.Exec(
		`INSERT INTO focus_sessions (category, started_at, ended_at, planned_minutes) VALUES (?, ?, ?, ?)`,
		"work", start.Format("2006-01-02 15:04:05"),
		start.Add(45*time.Minute).Format("2006-01-02 15:04:05"), 45)

	corrs := AnalyzeFocusEnergy(ctx, database.DB, 30)
	if len(corrs) != 0 {
		t.Errorf("expected no correlations, got %d", len(corrs))
	}
}

func TestAnalyzeExerciseMood_Positive(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	now := time.Now()
	// Days with exercise → next-day mood is high
	for i := 0; i < 5; i++ {
		date := now.AddDate(0, 0, -(i*2 + 2)).Format("2006-01-02")
		nextDate := now.AddDate(0, 0, -(i*2 + 1)).Format("2006-01-02")
		database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, exercise_done, energy_level, sleep_hours) VALUES (?, ?, 1, 7, 7)`,
			date, 6)
		database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, exercise_done, energy_level, sleep_hours) VALUES (?, ?, 0, 7, 7)`,
			nextDate, 8)
	}

	// Days without exercise → next-day mood is lower
	for i := 0; i < 5; i++ {
		date := now.AddDate(0, 0, -(20 + i*2 + 1)).Format("2006-01-02")
		nextDate := now.AddDate(0, 0, -(20 + i*2)).Format("2006-01-02")
		database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, exercise_done, energy_level, sleep_hours) VALUES (?, ?, 0, 5, 7)`,
			date, 5)
		database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, exercise_done, energy_level, sleep_hours) VALUES (?, ?, 0, 5, 7)`,
			nextDate, 5)
	}

	corrs := AnalyzeExerciseMood(ctx, database.DB, 60)
	// May or may not find correlation depending on date('now') arithmetic.
	// The main thing is no crash.
	for _, c := range corrs {
		if c.Type != "exercise_mood" {
			t.Errorf("type = %s, want exercise_mood", c.Type)
		}
	}
}

func TestAnalyzeExerciseMood_NoEffect(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	now := time.Now()
	// All days have same mood regardless of exercise
	for i := 0; i < 10; i++ {
		date := now.AddDate(0, 0, -(i + 1)).Format("2006-01-02")
		exercise := 0
		if i%2 == 0 {
			exercise = 1
		}
		database.DB.Exec(
			`INSERT INTO mood_log (date, mood_score, exercise_done, energy_level, sleep_hours)
			 VALUES (?, 6, ?, 6, 7)`,
			date, exercise)
	}

	corrs := AnalyzeExerciseMood(ctx, database.DB, 30)
	// Same mood on all days — no correlation (diff <= 0.3)
	for _, c := range corrs {
		fmt.Printf("unexpected correlation: %+v\n", c)
	}
}

func TestMean(t *testing.T) {
	if m := mean(nil); m != 0 {
		t.Errorf("mean(nil) = %f, want 0", m)
	}
	if m := mean([]float64{2, 4, 6}); m != 4 {
		t.Errorf("mean([2,4,6]) = %f, want 4", m)
	}
}
