package timecontext

import (
	"context"
	"database/sql"
	"time"
)

// Checkpoint represents a time awareness checkpoint from the time_checkpoints table.
type Checkpoint struct {
	ID                 int64
	EventType          string
	ReferenceID        string
	CheckpointTime     string
	AlertMinutesBefore int
	Acknowledged       bool
	CreatedAt          string
}

// SaveCheckpoint inserts a time checkpoint for proactive time awareness.
func SaveCheckpoint(ctx context.Context, db *sql.DB, eventType, referenceID string, checkpointTime time.Time, alertMinutes int) error {
	if alertMinutes <= 0 {
		alertMinutes = 15
	}
	_, err := db.ExecContext(ctx,
		`INSERT INTO time_checkpoints (event_type, reference_id, checkpoint_time, alert_minutes_before)
		 VALUES (?, ?, ?, ?)`,
		eventType, referenceID, checkpointTime.Format(time.RFC3339), alertMinutes)
	return err
}

// LoadCheckpoints returns unacknowledged checkpoints since the given time.
func LoadCheckpoints(ctx context.Context, db *sql.DB, since time.Time) ([]Checkpoint, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, event_type, COALESCE(reference_id,''), checkpoint_time, alert_minutes_before, acknowledged, created_at
		 FROM time_checkpoints
		 WHERE checkpoint_time >= ? AND acknowledged = 0
		 ORDER BY checkpoint_time ASC`, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checkpoints []Checkpoint
	for rows.Next() {
		var cp Checkpoint
		var ack int
		if err := rows.Scan(&cp.ID, &cp.EventType, &cp.ReferenceID, &cp.CheckpointTime, &cp.AlertMinutesBefore, &ack, &cp.CreatedAt); err != nil {
			continue
		}
		cp.Acknowledged = ack == 1
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints, nil
}

// AcknowledgeCheckpoint marks a checkpoint as acknowledged.
func AcknowledgeCheckpoint(ctx context.Context, db *sql.DB, id int64) error {
	_, err := db.ExecContext(ctx, "UPDATE time_checkpoints SET acknowledged = 1 WHERE id = ?", id)
	return err
}
