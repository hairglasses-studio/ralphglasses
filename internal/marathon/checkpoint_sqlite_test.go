package marathon

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func newTestSQLiteStore(t *testing.T) *SQLiteCheckpointStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "checkpoints.db")
	store, err := NewSQLiteCheckpointStore(dbPath, "test-marathon")
	if err != nil {
		t.Fatalf("NewSQLiteCheckpointStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteStore_SaveAndLatest(t *testing.T) {
	store := newTestSQLiteStore(t)

	cp := &Checkpoint{
		Timestamp:       time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		CyclesCompleted: 10,
		SpentUSD:        5.50,
		SupervisorState: session.SupervisorState{
			Running:   true,
			RepoPath:  "/test/repo",
			TickCount: 42,
		},
	}

	if err := store.Save(cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}

	if loaded.CyclesCompleted != 10 {
		t.Fatalf("CyclesCompleted: got %d, want 10", loaded.CyclesCompleted)
	}
	if loaded.SpentUSD != 5.50 {
		t.Fatalf("SpentUSD: got %f, want 5.50", loaded.SpentUSD)
	}
	if loaded.SupervisorState.TickCount != 42 {
		t.Fatalf("TickCount: got %d, want 42", loaded.SupervisorState.TickCount)
	}
	if loaded.MarathonID != "test-marathon" {
		t.Fatalf("MarathonID: got %q, want %q", loaded.MarathonID, "test-marathon")
	}
}

func TestSQLiteStore_ZeroTimestamp(t *testing.T) {
	store := newTestSQLiteStore(t)

	cp := &Checkpoint{
		CyclesCompleted: 3,
		SpentUSD:        1.00,
	}

	if err := store.Save(cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if cp.Timestamp.IsZero() {
		t.Fatal("expected Timestamp to be set by Save")
	}

	loaded, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	if loaded.CyclesCompleted != 3 {
		t.Fatalf("CyclesCompleted: got %d, want 3", loaded.CyclesCompleted)
	}
}

func TestSQLiteStore_List_Ordering(t *testing.T) {
	store := newTestSQLiteStore(t)

	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	for i := range 5 {
		cp := &Checkpoint{
			Timestamp:       base.Add(time.Duration(i) * time.Hour),
			CyclesCompleted: i + 1,
			SpentUSD:        float64(i) * 0.5,
		}
		if err := store.Save(cp); err != nil {
			t.Fatalf("Save[%d]: %v", i, err)
		}
	}

	cps, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(cps) != 5 {
		t.Fatalf("expected 5 checkpoints, got %d", len(cps))
	}

	// Verify ascending order.
	for i := 1; i < len(cps); i++ {
		if !cps[i].Timestamp.After(cps[i-1].Timestamp) {
			t.Fatalf("checkpoint[%d] (%s) not after checkpoint[%d] (%s)",
				i, cps[i].Timestamp, i-1, cps[i-1].Timestamp)
		}
	}
}

func TestSQLiteStore_Latest_ReturnsNewest(t *testing.T) {
	store := newTestSQLiteStore(t)

	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		cp := &Checkpoint{
			Timestamp:       base.Add(time.Duration(i) * time.Hour),
			CyclesCompleted: i + 1,
		}
		if err := store.Save(cp); err != nil {
			t.Fatalf("Save[%d]: %v", i, err)
		}
	}

	latest, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}

	if latest.CyclesCompleted != 5 {
		t.Fatalf("expected latest CyclesCompleted=5, got %d", latest.CyclesCompleted)
	}
}

func TestSQLiteStore_Latest_NoCheckpoints(t *testing.T) {
	store := newTestSQLiteStore(t)

	_, err := store.Latest()
	if err == nil {
		t.Fatal("expected error for empty store")
	}
}

func TestSQLiteStore_List_Empty(t *testing.T) {
	store := newTestSQLiteStore(t)

	cps, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cps) != 0 {
		t.Fatalf("expected 0 checkpoints, got %d", len(cps))
	}
}

func TestSQLiteStore_MarathonIDScoping(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	store1, err := NewSQLiteCheckpointStore(dbPath, "marathon-1")
	if err != nil {
		t.Fatalf("NewSQLiteCheckpointStore(1): %v", err)
	}
	defer store1.Close()

	store2, err := NewSQLiteCheckpointStore(dbPath, "marathon-2")
	if err != nil {
		t.Fatalf("NewSQLiteCheckpointStore(2): %v", err)
	}
	defer store2.Close()

	// Save checkpoints to both stores.
	cp1 := &Checkpoint{
		Timestamp:       time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		CyclesCompleted: 10,
	}
	cp2 := &Checkpoint{
		Timestamp:       time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
		CyclesCompleted: 20,
	}

	if err := store1.Save(cp1); err != nil {
		t.Fatalf("Save(1): %v", err)
	}
	if err := store2.Save(cp2); err != nil {
		t.Fatalf("Save(2): %v", err)
	}

	// Each store should only see its own checkpoints.
	cps1, err := store1.List()
	if err != nil {
		t.Fatalf("List(1): %v", err)
	}
	if len(cps1) != 1 {
		t.Fatalf("store1: expected 1 checkpoint, got %d", len(cps1))
	}
	if cps1[0].CyclesCompleted != 10 {
		t.Fatalf("store1: expected CyclesCompleted=10, got %d", cps1[0].CyclesCompleted)
	}

	cps2, err := store2.List()
	if err != nil {
		t.Fatalf("List(2): %v", err)
	}
	if len(cps2) != 1 {
		t.Fatalf("store2: expected 1 checkpoint, got %d", len(cps2))
	}
	if cps2[0].CyclesCompleted != 20 {
		t.Fatalf("store2: expected CyclesCompleted=20, got %d", cps2[0].CyclesCompleted)
	}
}

func TestSQLiteStore_FullRoundtrip(t *testing.T) {
	store := newTestSQLiteStore(t)

	cp := &Checkpoint{
		Timestamp:       time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC),
		CyclesCompleted: 42,
		SpentUSD:        15.75,
		SupervisorState: session.SupervisorState{
			Running:        true,
			RepoPath:       "/test/repo",
			TickCount:      100,
			BudgetSpentUSD: 12.50,
		},
	}

	if err := store.Save(cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}

	if loaded.CyclesCompleted != 42 {
		t.Fatalf("CyclesCompleted: got %d, want 42", loaded.CyclesCompleted)
	}
	if loaded.SpentUSD != 15.75 {
		t.Fatalf("SpentUSD: got %f, want 15.75", loaded.SpentUSD)
	}
	if loaded.SupervisorState.TickCount != 100 {
		t.Fatalf("TickCount: got %d, want 100", loaded.SupervisorState.TickCount)
	}
	if loaded.SupervisorState.BudgetSpentUSD != 12.50 {
		t.Fatalf("BudgetSpentUSD: got %f, want 12.50", loaded.SupervisorState.BudgetSpentUSD)
	}
}

func TestSQLiteStore_Upsert(t *testing.T) {
	store := newTestSQLiteStore(t)

	ts := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	// Save a checkpoint with a specific timestamp.
	cp := &Checkpoint{
		Timestamp:       ts,
		CyclesCompleted: 5,
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Save again with the same timestamp — should upsert.
	cp2 := &Checkpoint{
		Timestamp:       ts,
		CyclesCompleted: 10,
	}
	if err := store.Save(cp2); err != nil {
		t.Fatalf("Save(upsert): %v", err)
	}

	cps, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cps) != 1 {
		t.Fatalf("expected 1 checkpoint after upsert, got %d", len(cps))
	}
	if cps[0].CyclesCompleted != 10 {
		t.Fatalf("expected CyclesCompleted=10 after upsert, got %d", cps[0].CyclesCompleted)
	}
}

func TestSQLiteStore_NestedDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "a", "b", "c", "checkpoints.db")
	store, err := NewSQLiteCheckpointStore(dbPath, "test")
	if err != nil {
		t.Fatalf("NewSQLiteCheckpointStore with nested dir: %v", err)
	}
	defer store.Close()

	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: 1,
	}
	if err := store.Save(cp); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// TestSQLiteStore_ImplementsInterface verifies the interface at compile time.
var _ CheckpointStore = (*SQLiteCheckpointStore)(nil)
var _ CheckpointStore = (*FileCheckpointStore)(nil)
