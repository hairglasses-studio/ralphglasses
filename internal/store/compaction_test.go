package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// ---------- helpers ----------

// seedSessions inserts n sessions with the given status and age offset.
// Returns the IDs created.
func seedSessions(t *testing.T, s *Store, n int, status string, ageOffset time.Duration, prefix string) []string {
	t.Helper()
	ctx := context.Background()
	ids := make([]string, n)

	for i := range n {
		id := fmt.Sprintf("%s-%04d", prefix, i)
		ids[i] = id
		ts := time.Now().UTC().Add(-ageOffset)
		row := &SessionRow{
			ID:        id,
			Repo:      "test-repo",
			Status:    status,
			Provider:  "claude",
			Data:      json.RawMessage(fmt.Sprintf(`{"seq":%d,"padding":"%s"}`, i, makePadding(200))),
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		if err := s.SaveSession(ctx, row); err != nil {
			t.Fatalf("seed session %s: %v", id, err)
		}
	}
	return ids
}

// seedObservations inserts n observations for a given sessionID with the
// given age offset. Each observation carries a ~256-byte data payload.
func seedObservations(t *testing.T, s *Store, n int, sessionID string, ageOffset time.Duration, prefix string) {
	t.Helper()
	ctx := context.Background()

	for i := range n {
		ts := time.Now().UTC().Add(-ageOffset)
		obs := &ObservationRow{
			ID:        fmt.Sprintf("%s-%04d", prefix, i),
			SessionID: sessionID,
			Type:      "metric",
			Data:      json.RawMessage(fmt.Sprintf(`{"i":%d,"payload":"%s"}`, i, makePadding(256))),
			CreatedAt: ts,
		}
		if err := s.SaveObservation(ctx, obs); err != nil {
			t.Fatalf("seed observation %s: %v", obs.ID, err)
		}
	}
}

// seedFleetState inserts session-keyed fleet state entries.
func seedFleetState(t *testing.T, s *Store, sessionIDs []string) {
	t.Helper()
	ctx := context.Background()
	for _, id := range sessionIDs {
		key := "session:" + id
		val := json.RawMessage(fmt.Sprintf(`{"sid":"%s"}`, id))
		if err := s.SetFleetState(ctx, key, val); err != nil {
			t.Fatalf("seed fleet state %s: %v", key, err)
		}
	}
}

// makePadding returns a string of n 'x' characters.
func makePadding(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'x'
	}
	return string(b)
}

// countRows returns the number of rows in a table.
func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// ---------- DefaultRetentionPolicy ----------

func TestDefaultRetentionPolicy(t *testing.T) {
	p := DefaultRetentionPolicy()

	if p.SessionRetentionDays != DefaultRetentionDays {
		t.Errorf("SessionRetentionDays = %d, want %d", p.SessionRetentionDays, DefaultRetentionDays)
	}
	if p.MaxEventLogBytes != DefaultMaxEventLogBytes {
		t.Errorf("MaxEventLogBytes = %d, want %d", p.MaxEventLogBytes, DefaultMaxEventLogBytes)
	}
	if p.ObservationRetentionDays != DefaultMaxObservationAge {
		t.Errorf("ObservationRetentionDays = %d, want %d", p.ObservationRetentionDays, DefaultMaxObservationAge)
	}
	if !p.VacuumAfter {
		t.Error("VacuumAfter should default to true")
	}
}

// ---------- Archive completed sessions ----------

func TestArchiveCompletedSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// 10 old completed sessions (60 days old) -- should be archived.
	oldIDs := seedSessions(t, s, 10, "completed", 60*24*time.Hour, "old-done")
	// 5 old failed sessions (45 days old) -- should be archived.
	seedSessions(t, s, 5, "failed", 45*24*time.Hour, "old-fail")
	// 5 recent completed sessions (5 days old) -- should be kept.
	seedSessions(t, s, 5, "completed", 5*24*time.Hour, "recent-done")
	// 5 old running sessions (60 days old) -- should be kept (still running).
	seedSessions(t, s, 5, "running", 60*24*time.Hour, "old-run")

	// Add observations for old sessions.
	for _, id := range oldIDs {
		seedObservations(t, s, 3, id, 60*24*time.Hour, "obs-"+id)
	}

	before := countRows(t, s, "sessions")
	if before != 25 {
		t.Fatalf("expected 25 sessions before compaction, got %d", before)
	}
	obsBefore := countRows(t, s, "observations")
	if obsBefore != 30 {
		t.Fatalf("expected 30 observations before compaction, got %d", obsBefore)
	}

	policy := RetentionPolicy{
		SessionRetentionDays:     30,
		ObservationRetentionDays: 0, // disable age-based obs pruning for this test
		MaxEventLogBytes:         0, // disable size-based obs pruning
		VacuumAfter:              false,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	// 10 old-done + 5 old-fail = 15 archived.
	if report.SessionsArchived != 15 {
		t.Errorf("SessionsArchived = %d, want 15", report.SessionsArchived)
	}

	after := countRows(t, s, "sessions")
	if after != 10 { // 5 recent-done + 5 old-run
		t.Errorf("sessions after = %d, want 10", after)
	}

	// Observations for old sessions should be gone too.
	obsAfter := countRows(t, s, "observations")
	if obsAfter != 0 {
		t.Errorf("observations after = %d, want 0 (all belonged to archived sessions)", obsAfter)
	}
}

// ---------- Prune observations by age ----------

func TestPruneObservationsByAge(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedSessions(t, s, 1, "running", 0, "active")
	// 20 old observations (100 days) and 10 recent (5 days).
	seedObservations(t, s, 20, "active-0000", 100*24*time.Hour, "old-obs")
	seedObservations(t, s, 10, "active-0000", 5*24*time.Hour, "new-obs")

	before := countRows(t, s, "observations")
	if before != 30 {
		t.Fatalf("expected 30 observations before, got %d", before)
	}

	policy := RetentionPolicy{
		SessionRetentionDays:     365, // don't archive sessions
		ObservationRetentionDays: 90,
		MaxEventLogBytes:         0, // disable size pruning
		VacuumAfter:              false,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if report.ObservationsPruned != 20 {
		t.Errorf("ObservationsPruned = %d, want 20", report.ObservationsPruned)
	}

	after := countRows(t, s, "observations")
	if after != 10 {
		t.Errorf("observations after = %d, want 10", after)
	}
}

// ---------- Prune observations by size ----------

func TestPruneObservationsBySize(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	seedSessions(t, s, 1, "running", 0, "active")
	// Insert 100 observations with ~300-byte payloads each (~30 KiB total).
	seedObservations(t, s, 100, "active-0000", time.Hour, "sized-obs")

	before := countRows(t, s, "observations")
	if before != 100 {
		t.Fatalf("expected 100 observations before, got %d", before)
	}

	// Set a very small size limit to force pruning.
	policy := RetentionPolicy{
		SessionRetentionDays:     365,
		ObservationRetentionDays: 0,     // disable age pruning
		MaxEventLogBytes:         5_000, // ~5 KiB — should force major pruning
		VacuumAfter:              false,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if report.ObservationsPruned == 0 {
		t.Error("expected some observations to be pruned by size limit")
	}

	after := countRows(t, s, "observations")
	if after >= before {
		t.Errorf("observations after (%d) should be less than before (%d)", after, before)
	}
	t.Logf("size-based pruning: %d -> %d observations (pruned %d)",
		before, after, report.ObservationsPruned)
}

// ---------- Vacuum and space reclaimed (on-disk) ----------

func TestVacuumReclaimsSpace(t *testing.T) {
	path := t.TempDir() + "/compact.db"
	s, err := New(path)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	// Insert a meaningful amount of data.
	seedSessions(t, s, 50, "completed", 60*24*time.Hour, "bulk")
	for i := range 50 {
		seedObservations(t, s, 20, fmt.Sprintf("bulk-%04d", i), 60*24*time.Hour, fmt.Sprintf("bulk-obs-%04d", i))
	}

	policy := RetentionPolicy{
		SessionRetentionDays:     30,
		ObservationRetentionDays: 30,
		MaxEventLogBytes:         0,
		VacuumAfter:              true,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if !report.VacuumRan {
		t.Error("VacuumRan should be true")
	}
	if report.SessionsArchived != 50 {
		t.Errorf("SessionsArchived = %d, want 50", report.SessionsArchived)
	}

	t.Logf("before: %d bytes, after: %d bytes, reclaimed: %d bytes",
		report.BytesBefore, report.BytesAfter, report.BytesReclaimed)

	// After vacuuming a database that had bulk data removed, the file
	// should be smaller than before.
	if report.BytesAfter >= report.BytesBefore {
		t.Errorf("expected bytes_after (%d) < bytes_before (%d)", report.BytesAfter, report.BytesBefore)
	}
	if report.BytesReclaimed <= 0 {
		t.Errorf("expected positive bytes_reclaimed, got %d", report.BytesReclaimed)
	}
}

// ---------- Orphaned fleet state pruning ----------

func TestPruneOrphanedFleetState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create sessions and matching fleet state.
	ids := seedSessions(t, s, 5, "completed", 60*24*time.Hour, "fleet-sess")
	seedFleetState(t, s, ids)

	// Add some non-session fleet state that should survive.
	if err := s.SetFleetState(ctx, "global_budget", json.RawMessage(`{"usd":100}`)); err != nil {
		t.Fatalf("set global fleet state: %v", err)
	}

	fsBefore := countRows(t, s, "fleet_state")
	if fsBefore != 6 { // 5 session keys + 1 global
		t.Fatalf("expected 6 fleet_state rows before, got %d", fsBefore)
	}

	// Archive the sessions (they're old + completed).
	policy := RetentionPolicy{
		SessionRetentionDays:     30,
		ObservationRetentionDays: 0,
		MaxEventLogBytes:         0,
		VacuumAfter:              false,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if report.SessionsArchived != 5 {
		t.Errorf("SessionsArchived = %d, want 5", report.SessionsArchived)
	}
	if report.FleetKeysRemoved != 5 {
		t.Errorf("FleetKeysRemoved = %d, want 5", report.FleetKeysRemoved)
	}

	fsAfter := countRows(t, s, "fleet_state")
	if fsAfter != 1 { // only global_budget remains
		t.Errorf("fleet_state after = %d, want 1", fsAfter)
	}
}

// ---------- No-op when nothing to compact ----------

func TestCompactionNoOp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Only recent running sessions — nothing to compact.
	seedSessions(t, s, 3, "running", time.Hour, "recent")
	seedObservations(t, s, 5, "recent-0000", time.Hour, "recent-obs")

	policy := RetentionPolicy{
		SessionRetentionDays:     30,
		ObservationRetentionDays: 90,
		MaxEventLogBytes:         50 * 1024 * 1024,
		VacuumAfter:              false,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if report.SessionsArchived != 0 {
		t.Errorf("SessionsArchived = %d, want 0", report.SessionsArchived)
	}
	if report.ObservationsPruned != 0 {
		t.Errorf("ObservationsPruned = %d, want 0", report.ObservationsPruned)
	}
	if report.FleetKeysRemoved != 0 {
		t.Errorf("FleetKeysRemoved = %d, want 0", report.FleetKeysRemoved)
	}
}

// ---------- Custom now function (time control) ----------

func TestCompactorWithCustomTime(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Insert a session that is "recent" by wall clock but old by custom time.
	seedSessions(t, s, 1, "completed", 5*24*time.Hour, "time-test")

	policy := RetentionPolicy{
		SessionRetentionDays: 3,
		VacuumAfter:          false,
	}
	c := NewCompactor(s, policy)
	// Override nowFunc so "now" is 0 days in the future — the session at
	// 5 days ago is beyond the 3-day retention.
	c.nowFunc = func() time.Time { return time.Now().UTC() }

	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if report.SessionsArchived != 1 {
		t.Errorf("SessionsArchived = %d, want 1", report.SessionsArchived)
	}
}

// ---------- Before/after size comparison ----------

func TestBeforeAfterSizeReport(t *testing.T) {
	path := t.TempDir() + "/size-test.db"
	s, err := New(path)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()
	ctx := context.Background()

	// Seed bulk data: 30 completed sessions, each with 50 observations.
	for i := range 30 {
		id := fmt.Sprintf("size-sess-%04d", i)
		ts := time.Now().UTC().Add(-90 * 24 * time.Hour)
		row := &SessionRow{
			ID:        id,
			Repo:      "size-test",
			Status:    "completed",
			Provider:  "claude",
			Data:      json.RawMessage(fmt.Sprintf(`{"i":%d,"big":"%s"}`, i, makePadding(512))),
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		if err := s.SaveSession(ctx, row); err != nil {
			t.Fatalf("seed session: %v", err)
		}
		for j := range 50 {
			obs := &ObservationRow{
				ID:        fmt.Sprintf("size-obs-%04d-%04d", i, j),
				SessionID: id,
				Type:      "metric",
				Data:      json.RawMessage(fmt.Sprintf(`{"j":%d,"blob":"%s"}`, j, makePadding(1024))),
				CreatedAt: ts,
			}
			if err := s.SaveObservation(ctx, obs); err != nil {
				t.Fatalf("seed observation: %v", err)
			}
		}
	}

	sessBefore := countRows(t, s, "sessions")
	obsBefore := countRows(t, s, "observations")
	t.Logf("before compaction: %d sessions, %d observations", sessBefore, obsBefore)

	policy := RetentionPolicy{
		SessionRetentionDays:     30,
		ObservationRetentionDays: 60,
		MaxEventLogBytes:         0,
		VacuumAfter:              true,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	sessAfter := countRows(t, s, "sessions")
	obsAfter := countRows(t, s, "observations")
	t.Logf("after compaction:  %d sessions, %d observations", sessAfter, obsAfter)
	t.Logf("report: archived=%d pruned=%d vacuum=%v",
		report.SessionsArchived, report.ObservationsPruned, report.VacuumRan)
	t.Logf("size: before=%d bytes, after=%d bytes, reclaimed=%d bytes (%.1f%%)",
		report.BytesBefore, report.BytesAfter, report.BytesReclaimed,
		float64(report.BytesReclaimed)/float64(report.BytesBefore)*100)

	if sessBefore == sessAfter {
		t.Error("sessions count did not change")
	}
	if report.BytesBefore <= 0 {
		t.Errorf("BytesBefore should be positive, got %d", report.BytesBefore)
	}
	if report.BytesAfter <= 0 {
		t.Errorf("BytesAfter should be positive, got %d", report.BytesAfter)
	}
	if report.BytesReclaimed <= 0 {
		t.Errorf("BytesReclaimed should be positive, got %d", report.BytesReclaimed)
	}
	if report.ElapsedMilliseconds < 0 {
		t.Errorf("ElapsedMilliseconds should be non-negative, got %d", report.ElapsedMilliseconds)
	}
}

// ---------- All terminal statuses are handled ----------

func TestAllTerminalStatusesArchived(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	statuses := []string{"completed", "failed", "cancelled", "stopped"}
	for i, status := range statuses {
		id := fmt.Sprintf("term-%d", i)
		ts := time.Now().UTC().Add(-60 * 24 * time.Hour)
		row := &SessionRow{
			ID:        id,
			Repo:      "test",
			Status:    status,
			Provider:  "claude",
			CreatedAt: ts,
			UpdatedAt: ts,
		}
		if err := s.SaveSession(ctx, row); err != nil {
			t.Fatalf("seed %s: %v", status, err)
		}
	}

	policy := RetentionPolicy{
		SessionRetentionDays: 30,
		VacuumAfter:          false,
	}
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}

	if report.SessionsArchived != 4 {
		t.Errorf("SessionsArchived = %d, want 4 (all terminal statuses)", report.SessionsArchived)
	}

	remaining := countRows(t, s, "sessions")
	if remaining != 0 {
		t.Errorf("remaining sessions = %d, want 0", remaining)
	}
}

// ---------- Empty database ----------

func TestCompactionEmptyDB(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	policy := DefaultRetentionPolicy()
	policy.VacuumAfter = false
	c := NewCompactor(s, policy)
	report, err := c.Run(ctx)
	if err != nil {
		t.Fatalf("compaction on empty db: %v", err)
	}

	if report.SessionsArchived != 0 || report.ObservationsPruned != 0 || report.FleetKeysRemoved != 0 {
		t.Errorf("expected all zeros on empty db, got %+v", report)
	}
}
