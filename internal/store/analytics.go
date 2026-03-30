// Package store — analytics.go provides an AnalyticsStore that persists and
// queries historical analytics data (cost, duration, provider usage) on top
// of the existing Store's SQLite connection.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// AnalyticsEvent represents a single analytics event to be recorded.
type AnalyticsEvent struct {
	Timestamp time.Time       `json:"timestamp"`
	EventType string          `json:"event_type"`
	SessionID string          `json:"session_id"`
	Provider  string          `json:"provider"`
	Cost      float64         `json:"cost"`
	Duration  time.Duration   `json:"duration"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// TimeRange constrains queries to a window of time.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// AggregateResult holds a grouped aggregation row.
type AggregateResult struct {
	GroupKey   string  `json:"group_key"`
	Count     int64   `json:"count"`
	TotalCost float64 `json:"total_cost"`
	AvgCost   float64 `json:"avg_cost"`
	TotalDur  int64   `json:"total_duration_ms"`
	AvgDur    int64   `json:"avg_duration_ms"`
}

// SessionSummary holds a per-session rollup for TopSessions.
type SessionSummary struct {
	SessionID  string  `json:"session_id"`
	EventCount int64   `json:"event_count"`
	TotalCost  float64 `json:"total_cost"`
	TotalDur   int64   `json:"total_duration_ms"`
}

// AnalyticsStore wraps a Store and adds analytics-specific tables and queries.
type AnalyticsStore struct {
	db       *sql.DB
	stmtRec  *sql.Stmt
}

// NewAnalyticsStore creates an AnalyticsStore using the existing Store's
// SQLite connection. It creates the analytics_events table if it does not
// exist and prepares the record statement.
func NewAnalyticsStore(s *Store) (*AnalyticsStore, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("analytics: nil store or db")
	}

	a := &AnalyticsStore{db: s.db}
	if err := a.migrate(); err != nil {
		return nil, fmt.Errorf("analytics: migrate: %w", err)
	}
	if err := a.prepare(); err != nil {
		return nil, fmt.Errorf("analytics: prepare: %w", err)
	}
	return a, nil
}

func (a *AnalyticsStore) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS analytics_events (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp   DATETIME NOT NULL,
	event_type  TEXT     NOT NULL DEFAULT '',
	session_id  TEXT     NOT NULL DEFAULT '',
	provider    TEXT     NOT NULL DEFAULT '',
	cost        REAL     NOT NULL DEFAULT 0,
	duration_ms INTEGER  NOT NULL DEFAULT 0,
	metadata    JSON
);

CREATE INDEX IF NOT EXISTS idx_analytics_event_type ON analytics_events(event_type);
CREATE INDEX IF NOT EXISTS idx_analytics_session    ON analytics_events(session_id);
CREATE INDEX IF NOT EXISTS idx_analytics_timestamp  ON analytics_events(timestamp);
`
	_, err := a.db.Exec(ddl)
	return err
}

func (a *AnalyticsStore) prepare() error {
	var err error
	a.stmtRec, err = a.db.Prepare(`
INSERT INTO analytics_events (timestamp, event_type, session_id, provider, cost, duration_ms, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?)
`)
	if err != nil {
		return fmt.Errorf("prepare Record: %w", err)
	}
	return nil
}

// Record persists a single analytics event.
func (a *AnalyticsStore) Record(ctx context.Context, event AnalyticsEvent) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	meta := normalizeJSON(event.Metadata)
	_, err := a.stmtRec.ExecContext(ctx,
		event.Timestamp.Format(time.RFC3339Nano),
		event.EventType,
		event.SessionID,
		event.Provider,
		event.Cost,
		event.Duration.Milliseconds(),
		string(meta),
	)
	if err != nil {
		return fmt.Errorf("analytics record: %w", err)
	}
	return nil
}

// Query returns events matching the given eventType within the time range.
// Pass an empty eventType to match all types.
func (a *AnalyticsStore) Query(ctx context.Context, eventType string, tr TimeRange) ([]AnalyticsEvent, error) {
	q := `SELECT timestamp, event_type, session_id, provider, cost, duration_ms, metadata
	      FROM analytics_events WHERE 1=1`
	var args []any

	if eventType != "" {
		q += " AND event_type = ?"
		args = append(args, eventType)
	}
	if !tr.Start.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, tr.Start.Format(time.RFC3339Nano))
	}
	if !tr.End.IsZero() {
		q += " AND timestamp <= ?"
		args = append(args, tr.End.Format(time.RFC3339Nano))
	}
	q += " ORDER BY timestamp DESC"

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("analytics query: %w", err)
	}
	defer rows.Close()

	var result []AnalyticsEvent
	for rows.Next() {
		ev, err := scanAnalyticsRow(rows)
		if err != nil {
			return nil, fmt.Errorf("analytics query: scan: %w", err)
		}
		result = append(result, ev)
	}
	return result, rows.Err()
}

// Aggregate groups events by the specified column (one of "event_type",
// "session_id", "provider") within the time range, returning counts and
// cost/duration aggregates.
func (a *AnalyticsStore) Aggregate(ctx context.Context, eventType string, tr TimeRange, groupBy string) ([]AggregateResult, error) {
	col, err := validGroupColumn(groupBy)
	if err != nil {
		return nil, err
	}

	q := fmt.Sprintf(`SELECT %s, COUNT(*), SUM(cost), AVG(cost), SUM(duration_ms), AVG(duration_ms)
	      FROM analytics_events WHERE 1=1`, col)
	var args []any

	if eventType != "" {
		q += " AND event_type = ?"
		args = append(args, eventType)
	}
	if !tr.Start.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, tr.Start.Format(time.RFC3339Nano))
	}
	if !tr.End.IsZero() {
		q += " AND timestamp <= ?"
		args = append(args, tr.End.Format(time.RFC3339Nano))
	}
	q += fmt.Sprintf(" GROUP BY %s ORDER BY SUM(cost) DESC", col)

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("analytics aggregate: %w", err)
	}
	defer rows.Close()

	var result []AggregateResult
	for rows.Next() {
		var ar AggregateResult
		if err := rows.Scan(&ar.GroupKey, &ar.Count, &ar.TotalCost, &ar.AvgCost, &ar.TotalDur, &ar.AvgDur); err != nil {
			return nil, fmt.Errorf("analytics aggregate: scan: %w", err)
		}
		result = append(result, ar)
	}
	return result, rows.Err()
}

// TopSessions returns the top sessions by total cost within the time range.
func (a *AnalyticsStore) TopSessions(ctx context.Context, tr TimeRange, limit int) ([]SessionSummary, error) {
	if limit <= 0 {
		limit = 10
	}

	q := `SELECT session_id, COUNT(*), SUM(cost), SUM(duration_ms)
	      FROM analytics_events WHERE session_id != ''`
	var args []any

	if !tr.Start.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, tr.Start.Format(time.RFC3339Nano))
	}
	if !tr.End.IsZero() {
		q += " AND timestamp <= ?"
		args = append(args, tr.End.Format(time.RFC3339Nano))
	}
	q += " GROUP BY session_id ORDER BY SUM(cost) DESC LIMIT ?"
	args = append(args, limit)

	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("analytics top sessions: %w", err)
	}
	defer rows.Close()

	var result []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		if err := rows.Scan(&ss.SessionID, &ss.EventCount, &ss.TotalCost, &ss.TotalDur); err != nil {
			return nil, fmt.Errorf("analytics top sessions: scan: %w", err)
		}
		result = append(result, ss)
	}
	return result, rows.Err()
}

// Close closes the prepared statement. The underlying DB is owned by Store
// and should NOT be closed here.
func (a *AnalyticsStore) Close() error {
	if a.stmtRec != nil {
		return a.stmtRec.Close()
	}
	return nil
}

// ---------- helpers ----------

func validGroupColumn(col string) (string, error) {
	switch col {
	case "event_type", "session_id", "provider":
		return col, nil
	default:
		return "", fmt.Errorf("analytics: invalid groupBy column %q (allowed: event_type, session_id, provider)", col)
	}
}

func scanAnalyticsRow(sc rowScanner) (AnalyticsEvent, error) {
	var (
		ev        AnalyticsEvent
		ts        string
		durMS     int64
		metaStr   sql.NullString
	)
	err := sc.Scan(&ts, &ev.EventType, &ev.SessionID, &ev.Provider, &ev.Cost, &durMS, &metaStr)
	if err != nil {
		return ev, err
	}
	ev.Timestamp = parseTime(ts)
	ev.Duration = time.Duration(durMS) * time.Millisecond
	if metaStr.Valid && metaStr.String != "null" {
		ev.Metadata = json.RawMessage(metaStr.String)
	}
	return ev, nil
}
