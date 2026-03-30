package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Default retention thresholds.
const (
	DefaultRetentionDays     = 30
	DefaultMaxEventLogBytes  = 50 * 1024 * 1024 // 50 MiB
	DefaultMaxObservationAge = 90                // days
)

// Terminal session statuses considered "completed" for archival purposes.
var terminalStatuses = []string{"completed", "failed", "cancelled", "stopped"}

// RetentionPolicy configures how aggressively the Compactor reclaims space.
type RetentionPolicy struct {
	// SessionRetentionDays is the minimum age (in days) for a completed
	// session to be eligible for archival/deletion. Sessions that are still
	// running or pending are never touched regardless of age.
	SessionRetentionDays int

	// MaxEventLogBytes is the target ceiling for the observations table.
	// When the estimated size exceeds this value the oldest observations
	// are pruned until the table fits. Set to 0 to disable size-based pruning.
	MaxEventLogBytes int64

	// ObservationRetentionDays is the maximum age (in days) for observations.
	// Observations older than this are unconditionally pruned.
	ObservationRetentionDays int

	// VacuumAfter controls whether VACUUM is run after deletions to reclaim
	// disk space. Vacuuming rewrites the entire database, so it is expensive
	// on large files but produces the greatest space savings.
	VacuumAfter bool
}

// DefaultRetentionPolicy returns a policy with sensible defaults.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		SessionRetentionDays:     DefaultRetentionDays,
		MaxEventLogBytes:         DefaultMaxEventLogBytes,
		ObservationRetentionDays: DefaultMaxObservationAge,
		VacuumAfter:              true,
	}
}

// CompactionReport summarizes the work done and space reclaimed by a
// compaction run.
type CompactionReport struct {
	SessionsArchived    int   `json:"sessions_archived"`
	ObservationsPruned  int   `json:"observations_pruned"`
	FleetKeysRemoved    int   `json:"fleet_keys_removed"`
	VacuumRan           bool  `json:"vacuum_ran"`
	BytesBefore         int64 `json:"bytes_before"`
	BytesAfter          int64 `json:"bytes_after"`
	BytesReclaimed      int64 `json:"bytes_reclaimed"`
	ElapsedMilliseconds int64 `json:"elapsed_ms"`
}

// Compactor performs data compaction operations on a Store.
type Compactor struct {
	store  *Store
	policy RetentionPolicy
	// nowFunc can be overridden in tests to control time.
	nowFunc func() time.Time
}

// NewCompactor creates a Compactor bound to the given Store and policy.
func NewCompactor(s *Store, policy RetentionPolicy) *Compactor {
	return &Compactor{
		store:   s,
		policy:  policy,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

// Run executes the full compaction pipeline: archive old sessions, prune
// observations by age and size, and optionally vacuum. It returns a report
// describing the work performed.
func (c *Compactor) Run(ctx context.Context) (*CompactionReport, error) {
	start := time.Now()
	report := &CompactionReport{}

	sizeBefore, err := c.dbPageSize(ctx)
	if err != nil {
		return nil, fmt.Errorf("compaction: measure before size: %w", err)
	}
	report.BytesBefore = sizeBefore

	// Step 1: archive completed sessions older than the retention window.
	archived, err := c.archiveSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("compaction: archive sessions: %w", err)
	}
	report.SessionsArchived = archived

	// Step 2: prune observations by age.
	prunedAge, err := c.pruneObservationsByAge(ctx)
	if err != nil {
		return nil, fmt.Errorf("compaction: prune observations by age: %w", err)
	}
	report.ObservationsPruned += prunedAge

	// Step 3: prune observations by total size.
	prunedSize, err := c.pruneObservationsBySize(ctx)
	if err != nil {
		return nil, fmt.Errorf("compaction: prune observations by size: %w", err)
	}
	report.ObservationsPruned += prunedSize

	// Step 4: remove orphaned fleet_state keys whose sessions no longer exist.
	fleetRemoved, err := c.pruneOrphanedFleetState(ctx)
	if err != nil {
		return nil, fmt.Errorf("compaction: prune fleet state: %w", err)
	}
	report.FleetKeysRemoved = fleetRemoved

	// Step 5: vacuum if configured.
	if c.policy.VacuumAfter {
		if err := c.vacuum(ctx); err != nil {
			return nil, fmt.Errorf("compaction: vacuum: %w", err)
		}
		report.VacuumRan = true
	}

	sizeAfter, err := c.dbPageSize(ctx)
	if err != nil {
		return nil, fmt.Errorf("compaction: measure after size: %w", err)
	}
	report.BytesAfter = sizeAfter
	report.BytesReclaimed = sizeBefore - sizeAfter
	if report.BytesReclaimed < 0 {
		report.BytesReclaimed = 0
	}

	report.ElapsedMilliseconds = time.Since(start).Milliseconds()
	return report, nil
}

// archiveSessions deletes completed/failed/cancelled sessions older than
// the retention window along with their associated observations.
func (c *Compactor) archiveSessions(ctx context.Context) (int, error) {
	cutoff := c.nowFunc().AddDate(0, 0, -c.policy.SessionRetentionDays)
	cutoffStr := cutoff.Format(time.RFC3339Nano)

	// Build the status placeholders.
	args := make([]any, 0, len(terminalStatuses)+1)
	placeholders := ""
	for i, s := range terminalStatuses {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, s)
	}
	args = append(args, cutoffStr)

	// Delete observations belonging to archived sessions first.
	deleteObs := fmt.Sprintf(`
		DELETE FROM observations WHERE session_id IN (
			SELECT id FROM sessions
			WHERE status IN (%s) AND updated_at < ?
		)`, placeholders)
	if _, err := c.store.db.ExecContext(ctx, deleteObs, args...); err != nil {
		return 0, fmt.Errorf("delete orphan observations: %w", err)
	}

	// Delete the sessions themselves.
	deleteSess := fmt.Sprintf(`
		DELETE FROM sessions
		WHERE status IN (%s) AND updated_at < ?`, placeholders)
	res, err := c.store.db.ExecContext(ctx, deleteSess, args...)
	if err != nil {
		return 0, fmt.Errorf("delete sessions: %w", err)
	}

	n, _ := res.RowsAffected()
	return int(n), nil
}

// pruneObservationsByAge deletes observations older than the configured
// retention window.
func (c *Compactor) pruneObservationsByAge(ctx context.Context) (int, error) {
	if c.policy.ObservationRetentionDays <= 0 {
		return 0, nil
	}
	cutoff := c.nowFunc().AddDate(0, 0, -c.policy.ObservationRetentionDays)
	cutoffStr := cutoff.Format(time.RFC3339Nano)

	res, err := c.store.db.ExecContext(ctx,
		`DELETE FROM observations WHERE created_at < ?`, cutoffStr)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// pruneObservationsBySize estimates total observation data size and, if it
// exceeds MaxEventLogBytes, deletes the oldest rows until it fits.
func (c *Compactor) pruneObservationsBySize(ctx context.Context) (int, error) {
	if c.policy.MaxEventLogBytes <= 0 {
		return 0, nil
	}

	totalSize, err := c.observationDataSize(ctx)
	if err != nil {
		return 0, err
	}
	if totalSize <= c.policy.MaxEventLogBytes {
		return 0, nil
	}

	excess := totalSize - c.policy.MaxEventLogBytes
	// Estimate average row size to determine how many rows to delete.
	var rowCount int64
	if err := c.store.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM observations`).Scan(&rowCount); err != nil {
		return 0, err
	}
	if rowCount == 0 {
		return 0, nil
	}

	avgSize := totalSize / rowCount
	if avgSize == 0 {
		avgSize = 1
	}
	deleteCount := excess / avgSize
	if deleteCount <= 0 {
		deleteCount = 1
	}

	// Delete the oldest N observations.
	res, err := c.store.db.ExecContext(ctx, `
		DELETE FROM observations WHERE id IN (
			SELECT id FROM observations ORDER BY created_at ASC LIMIT ?
		)`, deleteCount)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// pruneOrphanedFleetState removes fleet_state entries whose keys follow
// the pattern "session:<id>" but whose session no longer exists.
func (c *Compactor) pruneOrphanedFleetState(ctx context.Context) (int, error) {
	res, err := c.store.db.ExecContext(ctx, `
		DELETE FROM fleet_state
		WHERE key LIKE 'session:%'
		AND SUBSTR(key, 9) NOT IN (SELECT id FROM sessions)
	`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// vacuum runs SQLite VACUUM to reclaim free pages.
func (c *Compactor) vacuum(ctx context.Context) error {
	_, err := c.store.db.ExecContext(ctx, "VACUUM")
	return err
}

// dbPageSize returns the total database size in bytes, computed from
// page_count * page_size. This works for both on-disk and in-memory
// databases.
func (c *Compactor) dbPageSize(ctx context.Context) (int64, error) {
	var pageSize, pageCount int64
	row := c.store.db.QueryRowContext(ctx, "PRAGMA page_size")
	if err := row.Scan(&pageSize); err != nil {
		return 0, err
	}
	row = c.store.db.QueryRowContext(ctx, "PRAGMA page_count")
	if err := row.Scan(&pageCount); err != nil {
		return 0, err
	}
	return pageSize * pageCount, nil
}

// observationDataSize returns the approximate total size in bytes of all
// observation data payloads.
func (c *Compactor) observationDataSize(ctx context.Context) (int64, error) {
	var size sql.NullInt64
	err := c.store.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(LENGTH(data)), 0) FROM observations`).Scan(&size)
	if err != nil {
		return 0, err
	}
	if !size.Valid {
		return 0, nil
	}
	return size.Int64, nil
}
