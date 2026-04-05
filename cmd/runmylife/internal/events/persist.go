package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// EventFilter constrains which persisted events to query.
type EventFilter struct {
	Type  EventType // empty = all types
	Since time.Time // zero = no lower bound
	Until time.Time // zero = no upper bound
	Limit int       // 0 = default 100
}

// PersistedEvent is an event loaded from the database.
type PersistedEvent struct {
	ID int64
	Event
}

// PersistEvent writes an event to the events table.
func PersistEvent(ctx context.Context, db *sql.DB, e Event) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO events (event_type, payload, source, created_at) VALUES (?, ?, ?, ?)`,
		string(e.Type), string(payload), e.Source, e.Timestamp.Format(time.RFC3339),
	)
	return err
}

// QueryEvents loads persisted events matching the filter.
func QueryEvents(ctx context.Context, db *sql.DB, f EventFilter) ([]PersistedEvent, error) {
	query := `SELECT id, event_type, payload, source, created_at FROM events WHERE 1=1`
	args := []any{}

	if f.Type != "" {
		query += ` AND event_type = ?`
		args = append(args, string(f.Type))
	}
	if !f.Since.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, f.Since.Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		query += ` AND created_at < ?`
		args = append(args, f.Until.Format(time.RFC3339))
	}

	query += ` ORDER BY created_at DESC`

	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var results []PersistedEvent
	for rows.Next() {
		var pe PersistedEvent
		var eventType, payload, source, createdAt string
		if err := rows.Scan(&pe.ID, &eventType, &payload, &source, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		pe.Type = EventType(eventType)
		pe.Source = source
		pe.Timestamp, _ = time.Parse(time.RFC3339, createdAt)
		pe.Payload = make(map[string]any)
		json.Unmarshal([]byte(payload), &pe.Payload)
		results = append(results, pe)
	}
	return results, rows.Err()
}

// CountEvents returns the number of events matching the filter.
func CountEvents(ctx context.Context, db *sql.DB, f EventFilter) (int, error) {
	query := `SELECT COUNT(*) FROM events WHERE 1=1`
	args := []any{}

	if f.Type != "" {
		query += ` AND event_type = ?`
		args = append(args, string(f.Type))
	}
	if !f.Since.IsZero() {
		query += ` AND created_at >= ?`
		args = append(args, f.Since.Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		query += ` AND created_at < ?`
		args = append(args, f.Until.Format(time.RFC3339))
	}

	var count int
	err := db.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}

// PruneEvents deletes events older than the given duration.
func PruneEvents(ctx context.Context, db *sql.DB, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).Format(time.RFC3339)
	result, err := db.ExecContext(ctx, `DELETE FROM events WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
