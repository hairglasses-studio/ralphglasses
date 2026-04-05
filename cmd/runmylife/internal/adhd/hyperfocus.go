package adhd

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HyperfocusAlert describes a detected hyperfocus state.
type HyperfocusAlert struct {
	Category     string
	Minutes      int
	SessionID    int
	TaskID       string
	ShouldBreak  bool   // true if >=180min without break
	GentleNudge  string // human-readable nudge message
}

// HyperfocusThresholdMinutes is the minimum duration to flag as hyperfocus.
const HyperfocusThresholdMinutes = 90

// DetectHyperfocus checks for an active focus session exceeding the threshold
// and returns an alert if found.
func DetectHyperfocus(ctx context.Context, db *sql.DB) (*HyperfocusAlert, error) {
	var id int
	var cat, startedAt string
	var taskID sql.NullString
	err := db.QueryRowContext(ctx,
		`SELECT id, category, started_at, task_id FROM focus_sessions
		 WHERE ended_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`,
	).Scan(&id, &cat, &startedAt, &taskID)
	if err != nil {
		return nil, nil // no active session
	}

	t, err := time.Parse("2006-01-02 15:04:05", startedAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, startedAt)
		if err != nil {
			return nil, nil
		}
	}

	elapsed := int(time.Since(t).Minutes())
	if elapsed < HyperfocusThresholdMinutes {
		return nil, nil // not yet hyperfocused
	}

	alert := &HyperfocusAlert{
		Category:  cat,
		Minutes:   elapsed,
		SessionID: id,
	}
	if taskID.Valid {
		alert.TaskID = taskID.String
	}

	if elapsed >= 180 {
		alert.ShouldBreak = true
		alert.GentleNudge = fmt.Sprintf("You've been deep in %s for %d minutes. Time to stretch, hydrate, and breathe. The work will still be here.", cat, elapsed)
	} else {
		alert.GentleNudge = fmt.Sprintf("Heads up: you've been in %s for %d minutes. Consider a quick break soon.", cat, elapsed)
	}

	return alert, nil
}

// StartFocusSession begins a new focus session tracking entry.
func StartFocusSession(ctx context.Context, db *sql.DB, category, taskID string, plannedMinutes int) (int64, error) {
	if plannedMinutes <= 0 {
		plannedMinutes = 25 // default pomodoro
	}

	// End any currently active session first
	EndActiveFocusSessions(ctx, db)

	result, err := db.ExecContext(ctx,
		`INSERT INTO focus_sessions (category, task_id, planned_minutes)
		 VALUES (?, ?, ?)`,
		category, taskID, plannedMinutes,
	)
	if err != nil {
		return 0, fmt.Errorf("start focus session: %w", err)
	}
	return result.LastInsertId()
}

// EndFocusSession ends an active focus session and records actual duration.
func EndFocusSession(ctx context.Context, db *sql.DB, sessionID int, interrupted bool) error {
	interruptedInt := 0
	if interrupted {
		interruptedInt = 1
	}
	_, err := db.ExecContext(ctx,
		`UPDATE focus_sessions SET
		   ended_at = datetime('now'),
		   actual_minutes = CAST((julianday('now') - julianday(started_at)) * 1440 AS INTEGER),
		   interrupted = ?
		 WHERE id = ? AND ended_at IS NULL`,
		interruptedInt, sessionID,
	)
	return err
}

// EndActiveFocusSessions ends all currently active sessions.
func EndActiveFocusSessions(ctx context.Context, db *sql.DB) {
	db.ExecContext(ctx,
		`UPDATE focus_sessions SET
		   ended_at = datetime('now'),
		   actual_minutes = CAST((julianday('now') - julianday(started_at)) * 1440 AS INTEGER)
		 WHERE ended_at IS NULL`,
	)
}

// FocusStats returns focus session statistics for a date range.
type FocusStats struct {
	TotalSessions   int
	TotalMinutes    int
	AvgMinutes      int
	Interrupted     int
	TopCategory     string
	CategoryMinutes map[string]int
}

// GetFocusStats returns focus session statistics for the given date range.
func GetFocusStats(ctx context.Context, db *sql.DB, since, until string) (*FocusStats, error) {
	stats := &FocusStats{CategoryMinutes: make(map[string]int)}

	db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(actual_minutes), 0), COALESCE(AVG(actual_minutes), 0),
		  SUM(CASE WHEN interrupted = 1 THEN 1 ELSE 0 END)
		 FROM focus_sessions WHERE started_at >= ? AND started_at < ? AND ended_at IS NOT NULL`,
		since, until).Scan(&stats.TotalSessions, &stats.TotalMinutes, &stats.AvgMinutes, &stats.Interrupted)

	rows, err := db.QueryContext(ctx,
		`SELECT category, SUM(actual_minutes) as total
		 FROM focus_sessions WHERE started_at >= ? AND started_at < ? AND ended_at IS NOT NULL
		 GROUP BY category ORDER BY total DESC`,
		since, until)
	if err == nil {
		defer rows.Close()
		first := true
		for rows.Next() {
			var cat string
			var mins int
			if rows.Scan(&cat, &mins) == nil {
				stats.CategoryMinutes[cat] = mins
				if first {
					stats.TopCategory = cat
					first = false
				}
			}
		}
	}

	return stats, nil
}
