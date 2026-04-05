// Package testutil provides shared test helpers for runmylife.
package testutil

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/db"
)

// TestDB opens an in-memory database with all migrations applied and registers cleanup.
func TestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("testutil.TestDB: %v", err)
	}
	t.Cleanup(func() { d.ForceClose() })
	return d.SqlDB()
}

// SeedTasks inserts n deterministic tasks with mixed completed/incomplete and priorities 1-4.
func SeedTasks(t *testing.T, sqlDB *sql.DB, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		completed := 0
		if i%3 == 0 {
			completed = 1
		}
		priority := (i % 4) + 1
		due := time.Now().AddDate(0, 0, i-n/2).Format("2006-01-02")
		_, err := sqlDB.Exec(
			`INSERT INTO tasks (id, title, priority, due_date, completed) VALUES (?, ?, ?, ?, ?)`,
			fmt.Sprintf("task-%d", i), fmt.Sprintf("Test Task %d", i), priority, due, completed,
		)
		if err != nil {
			t.Fatalf("SeedTasks: %v", err)
		}
	}
}

// SeedMoodLog inserts mood entries for the last `days` days with varied scores.
func SeedMoodLog(t *testing.T, sqlDB *sql.DB, days int) {
	t.Helper()
	for i := 0; i < days; i++ {
		score := (i % 10) + 1
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		_, err := sqlDB.Exec(
			`INSERT INTO mood_log (date, mood_score, notes) VALUES (?, ?, ?)`,
			date, score, fmt.Sprintf("day %d notes", i),
		)
		if err != nil {
			t.Fatalf("SeedMoodLog: %v", err)
		}
	}
}

// SeedHabits creates habits by name and sprinkles completions.
func SeedHabits(t *testing.T, sqlDB *sql.DB, names []string) {
	t.Helper()
	for i, name := range names {
		_, err := sqlDB.Exec(
			`INSERT INTO habits (id, name, frequency, created_at) VALUES (?, ?, 'daily', datetime('now'))`,
			i+1, name,
		)
		if err != nil {
			t.Fatalf("SeedHabits: %v", err)
		}
		// Add some completions for odd-indexed habits
		if i%2 == 1 {
			for d := 0; d < 3; d++ {
				date := time.Now().AddDate(0, 0, -d).Format("2006-01-02")
				sqlDB.Exec(`INSERT INTO habit_completions (habit_id, completed_at) VALUES (?, ?)`, i+1, date)
			}
		}
	}
}

// SeedAchievements inserts n achievement milestones with varied types.
func SeedAchievements(t *testing.T, sqlDB *sql.DB, n int) {
	t.Helper()
	types := []string{"streak_3", "inbox_zero", "focus_marathon", "habit_master", "mood_tracker"}
	for i := 0; i < n; i++ {
		earned := time.Now().AddDate(0, 0, -i).Format(time.RFC3339)
		_, err := sqlDB.Exec(
			`INSERT INTO achievement_milestones (type, title, description, earned_at) VALUES (?, ?, ?, ?)`,
			types[i%len(types)], fmt.Sprintf("Achievement %d", i), fmt.Sprintf("Earned achievement %d", i), earned,
		)
		if err != nil {
			t.Fatalf("SeedAchievements: %v", err)
		}
	}
}

// SeedFocusSessions inserts n focus sessions, alternating active (no ended_at) and completed.
func SeedFocusSessions(t *testing.T, sqlDB *sql.DB, n int) {
	t.Helper()
	categories := []string{"deep-work", "coding", "reading", "creative", "general"}
	for i := 0; i < n; i++ {
		started := time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		planned := 25 + (i%4)*10
		if i%3 == 0 {
			// Active session (no ended_at)
			_, err := sqlDB.Exec(
				`INSERT INTO focus_sessions (category, started_at, planned_minutes) VALUES (?, ?, ?)`,
				categories[i%len(categories)], started, planned,
			)
			if err != nil {
				t.Fatalf("SeedFocusSessions: %v", err)
			}
		} else {
			actual := planned - 5 + i%10
			ended := time.Now().Add(-time.Duration(i)*time.Hour + time.Duration(actual)*time.Minute).Format(time.RFC3339)
			_, err := sqlDB.Exec(
				`INSERT INTO focus_sessions (category, started_at, ended_at, planned_minutes, actual_minutes) VALUES (?, ?, ?, ?, ?)`,
				categories[i%len(categories)], started, ended, planned, actual,
			)
			if err != nil {
				t.Fatalf("SeedFocusSessions: %v", err)
			}
		}
	}
}

// SeedNotificationLog inserts n notification log entries with varied urgency.
func SeedNotificationLog(t *testing.T, sqlDB *sql.DB, n int) {
	t.Helper()
	urgencies := []string{"low", "normal", "high", "critical"}
	sources := []string{"worker", "scheduler", "api", "tui"}
	channels := []string{"log", "slack", "discord_dm", "discord_dm,homeassistant"}
	for i := 0; i < n; i++ {
		sentAt := time.Now().Add(-time.Duration(i) * time.Hour).Format(time.RFC3339)
		_, err := sqlDB.Exec(
			`INSERT INTO notification_log (title, message, urgency, source, channels, sent_at) VALUES (?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("Notification %d", i), fmt.Sprintf("Message %d", i),
			urgencies[i%len(urgencies)], sources[i%len(sources)], channels[i%len(channels)], sentAt,
		)
		if err != nil {
			t.Fatalf("SeedNotificationLog: %v", err)
		}
	}
}

// SeedTransactions inserts n transactions with varied merchants and amounts.
func SeedTransactions(t *testing.T, sqlDB *sql.DB, n int) {
	t.Helper()
	merchants := []string{"Amazon", "Costco", "Starbucks", "Uber", "Target"}
	categories := []string{"food", "transport", "shopping", "entertainment", "utilities"}
	for i := 0; i < n; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		amt := -float64((i+1)*10) + 0.99
		_, err := sqlDB.Exec(
			`INSERT INTO transactions (date, description, amount, category, type) VALUES (?, ?, ?, ?, 'expense')`,
			date, merchants[i%len(merchants)], amt, categories[i%len(categories)],
		)
		if err != nil {
			t.Fatalf("SeedTransactions: %v", err)
		}
	}
}
