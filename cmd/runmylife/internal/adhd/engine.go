// Package adhd provides executive function support for ADHD:
// task initiation, overwhelm detection, time blindness countermeasures,
// and dopamine scaffolding.
//
// Design principle: "Make the hard thing easy, make the invisible visible."
package adhd

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// OverwhelmThreshold is the composite score above which triage mode activates.
const OverwhelmThreshold = 0.7

// OverwhelmScore holds the components of an overwhelm assessment.
type OverwhelmScore struct {
	Date               string
	OpenTasks          int
	CompletionVelocity float64 // tasks completed per day (7-day rolling)
	ReplyBacklog       int
	OverdueCount       int
	CompositeScore     float64 // 0.0 - 1.0
	TriageActivated    bool
}

// EnergyLevel represents current energy for task matching.
type EnergyLevel struct {
	Level  int    // 1-10
	Source string // "mood", "fitness", "inferred"
}

// TaskInitiation holds data for helping start a task.
type TaskInitiation struct {
	TaskID           string
	ActivationEnergy int    // 1-5 (1=easy to start, 5=very hard)
	FirstStep        string // pre-loaded starter step
	EstimatedMinutes int
	EnergyRequired   string // low/medium/high
}

// CheckOverwhelm calculates the current overwhelm score and optionally activates triage mode.
func CheckOverwhelm(ctx context.Context, db *sql.DB) (*OverwhelmScore, error) {
	today := time.Now().Format("2006-01-02")
	score := &OverwhelmScore{Date: today}

	// Count open (incomplete) tasks
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&score.OpenTasks)

	// Count overdue tasks
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date < date('now') AND due_date != ''").Scan(&score.OverdueCount)

	// Reply backlog
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&score.ReplyBacklog)

	// Completion velocity: tasks completed in last 7 days / 7
	var completedLast7 int
	_ = db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at > datetime('now', '-7 days')").Scan(&completedLast7)
	score.CompletionVelocity = float64(completedLast7) / 7.0

	// Composite score (0.0 - 1.0)
	// Weighted: open tasks (0.3), overdue (0.3), reply backlog (0.2), low velocity (0.2)
	openNorm := clamp(float64(score.OpenTasks)/30.0, 0, 1)       // 30+ tasks = maxed
	overdueNorm := clamp(float64(score.OverdueCount)/10.0, 0, 1) // 10+ overdue = maxed
	replyNorm := clamp(float64(score.ReplyBacklog)/15.0, 0, 1)   // 15+ replies = maxed
	velocityNorm := clamp(1.0-score.CompletionVelocity/3.0, 0, 1) // <3/day = concerning

	score.CompositeScore = 0.3*openNorm + 0.3*overdueNorm + 0.2*replyNorm + 0.2*velocityNorm

	if score.CompositeScore >= OverwhelmThreshold {
		score.TriageActivated = true
	}

	// Store in DB
	_, _ = db.ExecContext(ctx,
		`INSERT OR REPLACE INTO daily_overwhelm_metric
		 (date, open_tasks, completion_velocity, reply_backlog, overdue_count, composite_score, triage_activated)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		today, score.OpenTasks, score.CompletionVelocity, score.ReplyBacklog,
		score.OverdueCount, score.CompositeScore, boolToInt(score.TriageActivated),
	)

	return score, nil
}

// GetTopTriageTasks returns the top 3 most important tasks when in triage mode.
// Prioritizes: overdue first, then by priority, then by due date.
func GetTopTriageTasks(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT title FROM tasks WHERE completed = 0
		 ORDER BY
		   CASE WHEN due_date < date('now') AND due_date != '' THEN 0 ELSE 1 END,
		   priority DESC,
		   due_date ASC
		 LIMIT 3`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			continue
		}
		tasks = append(tasks, title)
	}
	return tasks, nil
}

// CheckTimeBlinds scans for upcoming calendar events and returns alerts
// for events happening within the next alertMinutes.
func CheckTimeBlinds(ctx context.Context, db *sql.DB, alertMinutes int) ([]string, error) {
	if alertMinutes <= 0 {
		alertMinutes = 30
	}

	cutoff := time.Now().Add(time.Duration(alertMinutes) * time.Minute).Format(time.RFC3339)
	now := time.Now().Format(time.RFC3339)

	rows, err := db.QueryContext(ctx,
		`SELECT summary, start_time FROM calendar_events
		 WHERE start_time > ? AND start_time <= ?
		 ORDER BY start_time ASC`,
		now, cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []string
	for rows.Next() {
		var summary, startTime string
		if err := rows.Scan(&summary, &startTime); err != nil {
			continue
		}
		t, err := time.Parse(time.RFC3339, startTime)
		if err != nil {
			continue
		}
		minutes := int(time.Until(t).Minutes())
		alerts = append(alerts, fmt.Sprintf("%q in %d minutes", summary, minutes))
	}
	return alerts, nil
}

// CheckFocusSession detects if the user has been focused on the same category
// for an extended period (hyperfocus detection).
func CheckFocusSession(ctx context.Context, db *sql.DB) (category string, minutes int, ok bool) {
	var cat string
	var startedAt string
	err := db.QueryRowContext(ctx,
		`SELECT category, started_at FROM focus_sessions
		 WHERE ended_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`,
	).Scan(&cat, &startedAt)
	if err != nil {
		return "", 0, false
	}

	t, err := time.Parse("2006-01-02 15:04:05", startedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, startedAt)
		if err != nil {
			return "", 0, false
		}
	}

	elapsed := int(time.Since(t).Minutes())
	return cat, elapsed, true
}

// RecordAchievement stores a milestone for dopamine scaffolding.
func RecordAchievement(ctx context.Context, db *sql.DB, achievementType, title, description, category string, value int) {
	_, _ = db.ExecContext(ctx,
		`INSERT INTO achievement_milestones (achievement_type, title, description, category, value)
		 VALUES (?, ?, ?, ?, ?)`,
		achievementType, title, description, category, value,
	)
}

// CheckStreaks returns current active streaks for habit tracking.
func CheckStreaks(ctx context.Context, db *sql.DB) map[string]int {
	streaks := make(map[string]int)
	rows, err := db.QueryContext(ctx,
		`SELECT h.name, h.current_streak FROM habits h WHERE h.current_streak > 0 ORDER BY h.current_streak DESC`)
	if err != nil {
		return streaks
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		var streak int
		if err := rows.Scan(&name, &streak); err != nil {
			continue
		}
		streaks[name] = streak
	}
	return streaks
}

// GetTaskInitiation returns initiation assistance data for a task.
// If no metadata exists, returns defaults based on heuristics.
func GetTaskInitiation(ctx context.Context, db *sql.DB, taskID string) *TaskInitiation {
	ti := &TaskInitiation{
		TaskID:           taskID,
		ActivationEnergy: 3,
		EnergyRequired:   "medium",
	}

	err := db.QueryRowContext(ctx,
		`SELECT activation_energy, first_step_text, estimated_minutes, energy_required
		 FROM task_metadata WHERE task_id = ?`, taskID,
	).Scan(&ti.ActivationEnergy, &ti.FirstStep, &ti.EstimatedMinutes, &ti.EnergyRequired)

	if err != nil {
		// Infer from task data
		var title string
		var priority int
		_ = db.QueryRowContext(ctx,
			"SELECT title, priority FROM tasks WHERE id = ?", taskID,
		).Scan(&title, &priority)

		// Higher priority = likely higher activation energy (counterintuitive but true for ADHD)
		if priority >= 4 {
			ti.ActivationEnergy = 4
			ti.EnergyRequired = "high"
		} else if priority <= 1 {
			ti.ActivationEnergy = 2
			ti.EnergyRequired = "low"
		}
	}

	return ti
}

// EstimateCurrentEnergy returns the estimated current energy level
// based on time of day, recent mood logs, and fitness data.
func EstimateCurrentEnergy(ctx context.Context, db *sql.DB) EnergyLevel {
	hour := time.Now().Hour()

	// Start with time-based default curve
	baseEnergy := timeBasedEnergy(hour)

	// Try to adjust based on today's mood log
	var moodScore int
	err := db.QueryRowContext(ctx,
		`SELECT score FROM mood_log WHERE date = date('now') ORDER BY created_at DESC LIMIT 1`,
	).Scan(&moodScore)
	if err == nil && moodScore > 0 {
		// Mood is 1-10, blend with base
		return EnergyLevel{Level: (baseEnergy + moodScore) / 2, Source: "mood"}
	}

	// Try fitness data
	var steps int
	err = db.QueryRowContext(ctx,
		`SELECT steps FROM fitness_daily_stats WHERE date = date('now')`,
	).Scan(&steps)
	if err == nil && steps > 0 {
		// Active day = potentially higher energy (unless over-exercised)
		if steps > 8000 {
			baseEnergy = min(baseEnergy+1, 10)
		}
		return EnergyLevel{Level: baseEnergy, Source: "fitness"}
	}

	return EnergyLevel{Level: baseEnergy, Source: "inferred"}
}

// RunOverwhelmCheck is the worker-callable function that checks overwhelm
// and logs the result.
func RunOverwhelmCheck(ctx context.Context, db *sql.DB) (*OverwhelmScore, error) {
	score, err := CheckOverwhelm(ctx, db)
	if err != nil {
		return nil, err
	}

	log.Printf("[adhd] Overwhelm score: %.2f (tasks=%d, overdue=%d, replies=%d, velocity=%.1f/day)",
		score.CompositeScore, score.OpenTasks, score.OverdueCount,
		score.ReplyBacklog, score.CompletionVelocity)

	if score.TriageActivated {
		tasks, _ := GetTopTriageTasks(ctx, db)
		log.Printf("[adhd] TRIAGE MODE activated — focus on: %v", tasks)
	}

	return score, nil
}

// timeBasedEnergy returns a default energy level based on hour of day.
func timeBasedEnergy(hour int) int {
	switch {
	case hour >= 6 && hour < 9:
		return 6 // warming up
	case hour >= 9 && hour < 12:
		return 8 // peak morning
	case hour >= 12 && hour < 14:
		return 5 // post-lunch dip
	case hour >= 14 && hour < 17:
		return 7 // afternoon recovery
	case hour >= 17 && hour < 20:
		return 6 // winding down
	case hour >= 20 && hour < 22:
		return 4 // evening low
	default:
		return 3 // night
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
