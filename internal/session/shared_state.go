package session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// SharedState provides a SQLite-backed key-value store for multi-session
// coordination. It supports CRUD, prefix listing, distributed locking with
// TTL, and change polling.
type SharedState struct {
	db   *sql.DB
	path string
}

// NewSharedState opens or creates a SQLite database at dbPath with WAL mode
// enabled and auto-creates the required tables.
func NewSharedState(dbPath string) (*SharedState, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("shared state: mkdir %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("shared state: open %s: %w", dbPath, err)
	}

	// Limit to one open connection so SQLite writes are serialized and
	// the busy_timeout pragma is consistently applied.
	db.SetMaxOpenConns(1)

	ss := &SharedState{db: db, path: dbPath}
	if err := ss.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("shared state: migrate: %w", err)
	}
	return ss, nil
}

// migrate creates tables if they do not already exist.
func (ss *SharedState) migrate() error {
	const ddl = `
CREATE TABLE IF NOT EXISTS shared_kv (
	key        TEXT PRIMARY KEY,
	value      TEXT NOT NULL DEFAULT '',
	updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS shared_locks (
	key        TEXT PRIMARY KEY,
	holder     TEXT NOT NULL,
	expires_at DATETIME NOT NULL
);
`
	_, err := ss.db.Exec(ddl)
	return err
}

// Put stores a key-value pair, inserting or replacing any existing entry.
func (ss *SharedState) Put(key, value string) error {
	const q = `INSERT INTO shared_kv (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`
	_, err := ss.db.Exec(q, key, value, time.Now().UTC())
	return err
}

// Get retrieves the value for key. Returns ("", sql.ErrNoRows) if not found.
func (ss *SharedState) Get(key string) (string, error) {
	var v string
	err := ss.db.QueryRow("SELECT value FROM shared_kv WHERE key = ?", key).Scan(&v)
	return v, err
}

// Delete removes a key-value pair.
func (ss *SharedState) Delete(key string) error {
	_, err := ss.db.Exec("DELETE FROM shared_kv WHERE key = ?", key)
	return err
}

// List returns all key-value pairs whose key starts with prefix.
func (ss *SharedState) List(prefix string) (map[string]string, error) {
	rows, err := ss.db.Query(
		"SELECT key, value FROM shared_kv WHERE key LIKE ? || '%'", prefix,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	m := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, rows.Err()
}

// Lock attempts to acquire a distributed lock for key on behalf of holder
// with the given TTL. Returns true if the lock was acquired or already held
// by the same holder, false if held by another holder.
func (ss *SharedState) Lock(key, holder string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	expiresAt := now.Add(ttl)

	// Delete expired locks first.
	if _, err := ss.db.Exec(
		"DELETE FROM shared_locks WHERE key = ? AND expires_at <= ?", key, now,
	); err != nil {
		return false, err
	}

	// Try to insert.
	_, err := ss.db.Exec(
		"INSERT INTO shared_locks (key, holder, expires_at) VALUES (?, ?, ?)",
		key, holder, expiresAt,
	)
	if err == nil {
		return true, nil
	}

	// Conflict — check if we already hold it and refresh.
	var existing string
	if err2 := ss.db.QueryRow(
		"SELECT holder FROM shared_locks WHERE key = ?", key,
	).Scan(&existing); err2 != nil {
		return false, err2
	}
	if existing == holder {
		_, err2 := ss.db.Exec(
			"UPDATE shared_locks SET expires_at = ? WHERE key = ? AND holder = ?",
			expiresAt, key, holder,
		)
		return err2 == nil, err2
	}
	return false, nil
}

// Unlock releases a lock held by holder. It is a no-op if the lock does not
// exist or is held by a different holder.
func (ss *SharedState) Unlock(key, holder string) error {
	_, err := ss.db.Exec(
		"DELETE FROM shared_locks WHERE key = ? AND holder = ?", key, holder,
	)
	return err
}

// Watch polls for changes to keys matching prefix and calls fn for each
// changed key-value pair. It checks every 500ms and stops when ctx is
// cancelled. The first poll reports all existing keys.
func (ss *SharedState) Watch(ctx context.Context, prefix string, fn func(key, value string)) error {
	var lastCheck time.Time

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			rows, err := ss.db.QueryContext(ctx,
				"SELECT key, value FROM shared_kv WHERE key LIKE ? || '%' AND updated_at > ?",
				prefix, lastCheck,
			)
			if err != nil {
				return err
			}
			lastCheck = time.Now().UTC()
			for rows.Next() {
				var k, v string
				if err := rows.Scan(&k, &v); err != nil {
					rows.Close()
					return err
				}
				fn(k, v)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
	}
}

// Close closes the underlying database connection.
func (ss *SharedState) Close() error {
	return ss.db.Close()
}

// DB returns the underlying *sql.DB for advanced use (e.g. checking PRAGMA).
func (ss *SharedState) DB() *sql.DB {
	return ss.db
}
