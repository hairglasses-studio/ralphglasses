package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps the sql.DB connection.
type DB struct {
	*sql.DB
	shared bool
}

// SqlDB returns the underlying *sql.DB connection.
func (d *DB) SqlDB() *sql.DB {
	return d.DB
}

// Close is a no-op when the DB is marked as shared (singleton).
func (d *DB) Close() error {
	if d.shared {
		return nil
	}
	return d.DB.Close()
}

// MarkShared marks this DB as a shared singleton so Close() becomes a no-op.
func (d *DB) MarkShared() {
	d.shared = true
}

// ForceClose closes the underlying connection regardless of shared status.
func (d *DB) ForceClose() error {
	d.shared = false
	return d.DB.Close()
}

// DefaultPath returns ~/.config/runmylife/runmylife.db
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "runmylife", "runmylife.db")
	}
	return filepath.Join(home, ".config", "runmylife", "runmylife.db")
}

// Open opens or creates the database at the given path, runs migrations.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	dsn := path + "?_pragma=busy_timeout%3d10000&_pragma=foreign_keys%3d1"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
	}
	for _, p := range pragmas {
		if _, err := sqlDB.Exec(p); err != nil {
			return nil, fmt.Errorf("pragma %s: %w", p, err)
		}
	}

	db := &DB{DB: sqlDB}
	if err := db.Migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}
