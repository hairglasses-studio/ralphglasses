package marathon

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver (no CGO).
)

// SQLiteCheckpointStore implements CheckpointStore backed by a SQLite database.
// It stores checkpoints in a single table with JSON-encoded data, keyed by a
// composite of timestamp and marathon ID.
type SQLiteCheckpointStore struct {
	db         *sql.DB
	marathonID string
}

// NewSQLiteCheckpointStore opens (or creates) a SQLite database at dbPath and
// initialises the checkpoints table. The marathonID is used to scope queries
// so that multiple marathon runs can share the same database.
func NewSQLiteCheckpointStore(dbPath, marathonID string) (*SQLiteCheckpointStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Enable WAL mode for better concurrent-read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if err := createCheckpointTable(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteCheckpointStore{
		db:         db,
		marathonID: marathonID,
	}, nil
}

func createCheckpointTable(db *sql.DB) error {
	const ddl = `CREATE TABLE IF NOT EXISTS checkpoints (
		id          TEXT PRIMARY KEY,
		timestamp   TEXT NOT NULL,
		data        BLOB NOT NULL,
		marathon_id TEXT NOT NULL
	)`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("create checkpoints table: %w", err)
	}

	// Index on marathon_id + timestamp for scoped queries.
	const idx = `CREATE INDEX IF NOT EXISTS idx_checkpoints_marathon_ts
		ON checkpoints (marathon_id, timestamp)`
	if _, err := db.Exec(idx); err != nil {
		return fmt.Errorf("create checkpoints index: %w", err)
	}

	return nil
}

// Save persists a checkpoint to the database. If cp.Timestamp is zero it is
// set to time.Now(). The checkpoint ID is derived from the timestamp and
// marathon ID.
func (s *SQLiteCheckpointStore) Save(cp *Checkpoint) error {
	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now()
	}
	cp.MarathonID = s.marathonID

	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	id := fmt.Sprintf("%s-%s", s.marathonID, cp.Timestamp.Format("20060102-150405.000"))
	ts := cp.Timestamp.UTC().Format(time.RFC3339Nano)

	const query = `INSERT OR REPLACE INTO checkpoints (id, timestamp, data, marathon_id)
		VALUES (?, ?, ?, ?)`
	if _, err := s.db.Exec(query, id, ts, data, s.marathonID); err != nil {
		return fmt.Errorf("insert checkpoint: %w", err)
	}

	return nil
}

// Latest returns the most recent checkpoint for this marathon, or an error if
// none exist.
func (s *SQLiteCheckpointStore) Latest() (*Checkpoint, error) {
	const query = `SELECT data FROM checkpoints
		WHERE marathon_id = ?
		ORDER BY timestamp DESC
		LIMIT 1`

	var data []byte
	err := s.db.QueryRow(query, s.marathonID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no checkpoints found for marathon %s", s.marathonID)
	}
	if err != nil {
		return nil, fmt.Errorf("query latest checkpoint: %w", err)
	}

	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

// List returns all checkpoints for this marathon, sorted by timestamp ascending.
func (s *SQLiteCheckpointStore) List() ([]*Checkpoint, error) {
	const query = `SELECT data FROM checkpoints
		WHERE marathon_id = ?
		ORDER BY timestamp ASC`

	rows, err := s.db.Query(query, s.marathonID)
	if err != nil {
		return nil, fmt.Errorf("query checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []*Checkpoint
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan checkpoint row: %w", err)
		}
		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue // skip malformed rows
		}
		checkpoints = append(checkpoints, &cp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate checkpoint rows: %w", err)
	}

	return checkpoints, nil
}

// Close closes the underlying database connection.
func (s *SQLiteCheckpointStore) Close() error {
	return s.db.Close()
}
