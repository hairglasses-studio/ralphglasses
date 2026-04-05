// Package jobs provides a background job queue with scheduling and retry support.
package jobs

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Job represents a queued job.
type Job struct {
	ID           int64
	Type         string
	Payload      string
	Status       string // pending, running, completed, failed, dead
	Priority     int    // higher = more important
	Attempts     int
	MaxAttempts  int
	NextRunAt    string
	LockedAt     string
	ErrorMessage string
	CreatedAt    string
	CompletedAt  string
}

// EnsureTable creates the job_queue table if it doesn't exist.
func EnsureTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS job_queue (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			payload TEXT,
			status TEXT NOT NULL DEFAULT 'pending',
			priority INTEGER DEFAULT 0,
			attempts INTEGER DEFAULT 0,
			max_attempts INTEGER DEFAULT 3,
			next_run_at TEXT DEFAULT (datetime('now')),
			locked_at TEXT,
			error_message TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			completed_at TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_job_queue_status ON job_queue(status, next_run_at);
		CREATE INDEX IF NOT EXISTS idx_job_queue_type ON job_queue(type);
	`)
	return err
}

// Enqueue adds a new job to the queue.
func Enqueue(ctx context.Context, db *sql.DB, jobType, payload string, priority int) (int64, error) {
	result, err := db.ExecContext(ctx,
		"INSERT INTO job_queue (type, payload, priority) VALUES (?, ?, ?)",
		jobType, payload, priority,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue job: %w", err)
	}
	return result.LastInsertId()
}

// EnqueueDelayed adds a job that will run after a delay.
func EnqueueDelayed(ctx context.Context, db *sql.DB, jobType, payload string, priority int, delay time.Duration) (int64, error) {
	runAt := time.Now().Add(delay).Format("2006-01-02T15:04:05")
	result, err := db.ExecContext(ctx,
		"INSERT INTO job_queue (type, payload, priority, next_run_at) VALUES (?, ?, ?, ?)",
		jobType, payload, priority, runAt,
	)
	if err != nil {
		return 0, fmt.Errorf("enqueue delayed job: %w", err)
	}
	return result.LastInsertId()
}

// ListPending returns pending jobs ordered by priority and scheduled time.
func ListPending(ctx context.Context, db *sql.DB, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, type, payload, status, priority, attempts, max_attempts, next_run_at, COALESCE(locked_at,''), error_message, created_at, COALESCE(completed_at,'')
		 FROM job_queue WHERE status IN ('pending', 'failed')
		 ORDER BY priority DESC, next_run_at ASC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

// ListAll returns all jobs, optionally filtered by status.
func ListAll(ctx context.Context, db *sql.DB, status string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows *sql.Rows
	var err error
	if status != "" {
		rows, err = db.QueryContext(ctx,
			`SELECT id, type, payload, status, priority, attempts, max_attempts, next_run_at, COALESCE(locked_at,''), error_message, created_at, COALESCE(completed_at,'')
			 FROM job_queue WHERE status = ? ORDER BY created_at DESC LIMIT ?`, status, limit)
	} else {
		rows, err = db.QueryContext(ctx,
			`SELECT id, type, payload, status, priority, attempts, max_attempts, next_run_at, COALESCE(locked_at,''), error_message, created_at, COALESCE(completed_at,'')
			 FROM job_queue ORDER BY created_at DESC LIMIT ?`, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

// RetryJob resets a failed job back to pending.
func RetryJob(ctx context.Context, db *sql.DB, jobID int64) error {
	_, err := db.ExecContext(ctx,
		"UPDATE job_queue SET status = 'pending', error_message = '', next_run_at = datetime('now') WHERE id = ? AND status IN ('failed', 'dead')",
		jobID,
	)
	return err
}

// ClearCompleted removes completed jobs older than the given age.
func ClearCompleted(ctx context.Context, db *sql.DB, age time.Duration) (int64, error) {
	cutoff := time.Now().Add(-age).Format("2006-01-02T15:04:05")
	result, err := db.ExecContext(ctx,
		"DELETE FROM job_queue WHERE status = 'completed' AND completed_at < ?", cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// GetStats returns job queue statistics.
func GetStats(ctx context.Context, db *sql.DB) (pending, running, completed, failed, dead int, err error) {
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM job_queue WHERE status = 'pending'").Scan(&pending)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM job_queue WHERE status = 'running'").Scan(&running)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM job_queue WHERE status = 'completed'").Scan(&completed)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM job_queue WHERE status = 'failed'").Scan(&failed)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM job_queue WHERE status = 'dead'").Scan(&dead)
	return
}

func scanJobs(rows *sql.Rows) ([]Job, error) {
	var jobs []Job
	for rows.Next() {
		var j Job
		var errMsg sql.NullString
		if err := rows.Scan(&j.ID, &j.Type, &j.Payload, &j.Status, &j.Priority, &j.Attempts, &j.MaxAttempts, &j.NextRunAt, &j.LockedAt, &errMsg, &j.CreatedAt, &j.CompletedAt); err != nil {
			continue
		}
		j.ErrorMessage = errMsg.String
		jobs = append(jobs, j)
	}
	return jobs, nil
}
