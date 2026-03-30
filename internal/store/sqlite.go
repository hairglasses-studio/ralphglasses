// Package store provides a SQLite WAL-mode state store for fleet-level
// persistence of sessions, observations, and fleet state key-value pairs.
// It uses modernc.org/sqlite (pure Go, no CGO required).
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Sentinel errors.
var (
	ErrNotFound = errors.New("not found")
	ErrNilValue = errors.New("nil value or empty ID")
)

// SessionRow represents a session record in the store.
type SessionRow struct {
	ID        string          `json:"id"`
	Repo      string          `json:"repo"`
	Status    string          `json:"status"`
	Provider  string          `json:"provider"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// ObservationRow represents an observation record in the store.
type ObservationRow struct {
	ID        string          `json:"id"`
	SessionID string          `json:"session_id"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// FleetStateRow represents a key-value fleet state entry.
type FleetStateRow struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Store wraps a SQLite database connection with WAL mode for fleet state.
type Store struct {
	db   *sql.DB
	path string

	// Prepared statements for writes.
	stmtSaveSession     *sql.Stmt
	stmtSaveObservation *sql.Stmt
	stmtSetFleetState   *sql.Stmt
}

// New opens (or creates) a SQLite database at path, enables WAL mode,
// creates tables and indexes, and prepares write statements.
// Use ":memory:" for an in-memory database (useful in tests).
func New(path string) (*Store, error) {
	if path != ":memory:" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("store: mkdir %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}

	// Enable WAL mode for concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: enable WAL: %w", err)
	}

	// Recommended SQLite performance pragmas.
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: set busy_timeout: %w", err)
	}

	// modernc.org/sqlite does not support true concurrent writers from
	// multiple goroutines sharing prepared statements. Serialize all
	// database access through a single connection. WAL still provides
	// the durability and crash-safety benefits even with one conn.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: migrate: %w", err)
	}
	if err := s.prepareStatements(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: prepare statements: %w", err)
	}
	return s, nil
}

// migrate creates tables and indexes if they don't exist.
func (s *Store) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS sessions (
	id         TEXT PRIMARY KEY,
	repo       TEXT NOT NULL DEFAULT '',
	status     TEXT NOT NULL DEFAULT '',
	provider   TEXT NOT NULL DEFAULT '',
	data       JSON,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS observations (
	id         TEXT PRIMARY KEY,
	session_id TEXT NOT NULL DEFAULT '',
	type       TEXT NOT NULL DEFAULT '',
	data       JSON,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS fleet_state (
	key        TEXT PRIMARY KEY,
	value      JSON,
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sessions_repo ON sessions(repo);
CREATE INDEX IF NOT EXISTS idx_observations_session ON observations(session_id);
`
	_, err := s.db.Exec(ddl)
	return err
}

// prepareStatements creates prepared statements for frequent write operations.
func (s *Store) prepareStatements() error {
	var err error

	s.stmtSaveSession, err = s.db.Prepare(`
INSERT INTO sessions (id, repo, status, provider, data, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	repo=excluded.repo, status=excluded.status, provider=excluded.provider,
	data=excluded.data, updated_at=excluded.updated_at
`)
	if err != nil {
		return fmt.Errorf("prepare SaveSession: %w", err)
	}

	s.stmtSaveObservation, err = s.db.Prepare(`
INSERT INTO observations (id, session_id, type, data, created_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	session_id=excluded.session_id, type=excluded.type,
	data=excluded.data, created_at=excluded.created_at
`)
	if err != nil {
		return fmt.Errorf("prepare SaveObservation: %w", err)
	}

	s.stmtSetFleetState, err = s.db.Prepare(`
INSERT INTO fleet_state (key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
	value=excluded.value, updated_at=excluded.updated_at
`)
	if err != nil {
		return fmt.Errorf("prepare SetFleetState: %w", err)
	}

	return nil
}

// DB returns the underlying *sql.DB for advanced use cases.
func (s *Store) DB() *sql.DB { return s.db }

// ---------- Session CRUD ----------

// SaveSession inserts or updates a session row.
func (s *Store) SaveSession(ctx context.Context, row *SessionRow) error {
	if row == nil || row.ID == "" {
		return ErrNilValue
	}
	now := time.Now().UTC()
	if row.CreatedAt.IsZero() {
		row.CreatedAt = now
	}
	row.UpdatedAt = now

	dataBytes := normalizeJSON(row.Data)

	_, err := s.stmtSaveSession.ExecContext(ctx,
		row.ID, row.Repo, row.Status, row.Provider,
		string(dataBytes),
		row.CreatedAt.Format(time.RFC3339Nano),
		row.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save session %s: %w", row.ID, err)
	}
	return nil
}

// GetSession retrieves a session by ID. Returns ErrNotFound if absent.
func (s *Store) GetSession(ctx context.Context, id string) (*SessionRow, error) {
	const query = `SELECT id, repo, status, provider, data, created_at, updated_at FROM sessions WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	sess, err := scanSessionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get session %s: %w", id, err)
	}
	return sess, nil
}

// ListSessions returns sessions optionally filtered by repo and/or status.
// Pass empty strings to skip filters. Results ordered by created_at DESC.
func (s *Store) ListSessions(ctx context.Context, repo, status string) ([]*SessionRow, error) {
	query := `SELECT id, repo, status, provider, data, created_at, updated_at FROM sessions WHERE 1=1`
	var args []any

	if repo != "" {
		query += " AND repo = ?"
		args = append(args, repo)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var result []*SessionRow
	for rows.Next() {
		sess, err := scanSessionRows(rows)
		if err != nil {
			return nil, fmt.Errorf("list sessions: scan: %w", err)
		}
		result = append(result, sess)
	}
	return result, rows.Err()
}

// ---------- Observation CRUD ----------

// SaveObservation inserts or updates an observation row.
func (s *Store) SaveObservation(ctx context.Context, row *ObservationRow) error {
	if row == nil || row.ID == "" {
		return ErrNilValue
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now().UTC()
	}

	dataBytes := normalizeJSON(row.Data)

	_, err := s.stmtSaveObservation.ExecContext(ctx,
		row.ID, row.SessionID, row.Type,
		string(dataBytes),
		row.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("save observation %s: %w", row.ID, err)
	}
	return nil
}

// QueryObservations returns observations filtered by session_id and/or type.
// Pass empty strings to skip filters. Results ordered by created_at DESC.
func (s *Store) QueryObservations(ctx context.Context, sessionID, obsType string) ([]*ObservationRow, error) {
	query := `SELECT id, session_id, type, data, created_at FROM observations WHERE 1=1`
	var args []any

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}
	if obsType != "" {
		query += " AND type = ?"
		args = append(args, obsType)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()

	var result []*ObservationRow
	for rows.Next() {
		obs, err := scanObservationRows(rows)
		if err != nil {
			return nil, fmt.Errorf("query observations: scan: %w", err)
		}
		result = append(result, obs)
	}
	return result, rows.Err()
}

// ---------- Fleet State CRUD ----------

// SetFleetState sets a fleet state key-value pair (upsert).
func (s *Store) SetFleetState(ctx context.Context, key string, value json.RawMessage) error {
	if key == "" {
		return ErrNilValue
	}
	now := time.Now().UTC()
	dataBytes := normalizeJSON(value)

	_, err := s.stmtSetFleetState.ExecContext(ctx,
		key, string(dataBytes), now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("set fleet state %s: %w", key, err)
	}
	return nil
}

// GetFleetState retrieves a fleet state value by key. Returns ErrNotFound if absent.
func (s *Store) GetFleetState(ctx context.Context, key string) (*FleetStateRow, error) {
	const query = `SELECT key, value, updated_at FROM fleet_state WHERE key = ?`
	row := s.db.QueryRowContext(ctx, query, key)

	var (
		fs        FleetStateRow
		updatedAt string
		dataStr   sql.NullString
	)
	err := row.Scan(&fs.Key, &dataStr, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("fleet state %s: %w", key, ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("get fleet state %s: %w", key, err)
	}

	if dataStr.Valid {
		fs.Value = json.RawMessage(dataStr.String)
	}
	fs.UpdatedAt = parseTime(updatedAt)
	return &fs, nil
}

// ---------- Close ----------

// Close closes prepared statements and the database connection.
func (s *Store) Close() error {
	if s.stmtSaveSession != nil {
		s.stmtSaveSession.Close()
	}
	if s.stmtSaveObservation != nil {
		s.stmtSaveObservation.Close()
	}
	if s.stmtSetFleetState != nil {
		s.stmtSetFleetState.Close()
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

// ---------- Scan helpers ----------

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSessionFromScanner(sc rowScanner) (*SessionRow, error) {
	var (
		sess      SessionRow
		createdAt string
		updatedAt string
		dataStr   sql.NullString
	)
	err := sc.Scan(&sess.ID, &sess.Repo, &sess.Status, &sess.Provider, &dataStr, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}
	if dataStr.Valid {
		sess.Data = json.RawMessage(dataStr.String)
	}
	sess.CreatedAt = parseTime(createdAt)
	sess.UpdatedAt = parseTime(updatedAt)
	return &sess, nil
}

func scanSessionRow(row *sql.Row) (*SessionRow, error) {
	return scanSessionFromScanner(row)
}

func scanSessionRows(rows *sql.Rows) (*SessionRow, error) {
	return scanSessionFromScanner(rows)
}

func scanObservationFromScanner(sc rowScanner) (*ObservationRow, error) {
	var (
		obs       ObservationRow
		createdAt string
		dataStr   sql.NullString
	)
	err := sc.Scan(&obs.ID, &obs.SessionID, &obs.Type, &dataStr, &createdAt)
	if err != nil {
		return nil, err
	}
	if dataStr.Valid {
		obs.Data = json.RawMessage(dataStr.String)
	}
	obs.CreatedAt = parseTime(createdAt)
	return &obs, nil
}

func scanObservationRows(rows *sql.Rows) (*ObservationRow, error) {
	return scanObservationFromScanner(rows)
}

// normalizeJSON returns the raw bytes or "null" if nil/empty.
func normalizeJSON(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	return []byte(raw)
}

// parseTime tries RFC3339Nano first (what we write), then RFC3339 (fallback).
func parseTime(s string) time.Time {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}
