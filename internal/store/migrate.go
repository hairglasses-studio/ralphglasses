package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"
)

// Migration defines a single schema migration with up and down SQL.
type Migration struct {
	// Version is a unique, monotonically increasing migration identifier.
	Version int
	// Name is a human-readable label for this migration.
	Name string
	// Up is the SQL to apply this migration.
	Up string
	// Down is the SQL to roll back this migration (best-effort).
	Down string
}

// Migrator tracks and applies schema migrations in a SQLite database.
type Migrator struct {
	db         *sql.DB
	migrations []Migration
}

// NewMigrator creates a Migrator for the given database connection.
// Migrations are sorted by version before any operations.
func NewMigrator(db *sql.DB, migrations []Migration) *Migrator {
	sorted := make([]Migration, len(migrations))
	copy(sorted, migrations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Version < sorted[j].Version
	})
	return &Migrator{db: db, migrations: sorted}
}

// ensureTable creates the migrations tracking table if it does not exist.
func (m *Migrator) ensureTable(ctx context.Context) error {
	_, err := m.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS migrations (
			version    INTEGER PRIMARY KEY,
			name       TEXT    NOT NULL,
			applied_at TEXT    NOT NULL
		)
	`)
	return err
}

// Applied returns the set of migration versions already applied.
func (m *Migrator) Applied(ctx context.Context) (map[int]bool, error) {
	if err := m.ensureTable(ctx); err != nil {
		return nil, err
	}
	rows, err := m.db.QueryContext(ctx, `SELECT version FROM migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applied := make(map[int]bool)
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// Up applies all pending migrations in order inside a transaction per migration.
func (m *Migrator) Up(ctx context.Context) error {
	applied, err := m.Applied(ctx)
	if err != nil {
		return fmt.Errorf("migrate up: check applied: %w", err)
	}
	for _, mig := range m.migrations {
		if applied[mig.Version] {
			continue
		}
		if err := m.applyUp(ctx, mig); err != nil {
			return fmt.Errorf("migrate up v%d (%s): %w", mig.Version, mig.Name, err)
		}
	}
	return nil
}

func (m *Migrator) applyUp(ctx context.Context, mig Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, mig.Up); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		mig.Version, mig.Name, time.Now().UTC().Format(time.RFC3339),
	); err != nil {
		return err
	}
	return tx.Commit()
}

// Down rolls back the most recently applied migration.
// Returns the version that was rolled back, or 0 if nothing to roll back.
func (m *Migrator) Down(ctx context.Context) (int, error) {
	applied, err := m.Applied(ctx)
	if err != nil {
		return 0, fmt.Errorf("migrate down: check applied: %w", err)
	}
	// Find the highest applied version that we have a migration for.
	for i := len(m.migrations) - 1; i >= 0; i-- {
		mig := m.migrations[i]
		if !applied[mig.Version] {
			continue
		}
		if mig.Down == "" {
			return 0, fmt.Errorf("migrate down v%d (%s): no down SQL defined", mig.Version, mig.Name)
		}
		if err := m.applyDown(ctx, mig); err != nil {
			return 0, fmt.Errorf("migrate down v%d (%s): %w", mig.Version, mig.Name, err)
		}
		return mig.Version, nil
	}
	return 0, nil
}

func (m *Migrator) applyDown(ctx context.Context, mig Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, mig.Down); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM migrations WHERE version = ?`, mig.Version,
	); err != nil {
		return err
	}
	return tx.Commit()
}
