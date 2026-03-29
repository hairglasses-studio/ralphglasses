package session

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// testSession creates a minimal session for testing.
func testSession(id, repoPath string, status SessionStatus, spend float64) *Session {
	return &Session{
		ID:           id,
		Provider:     ProviderClaude,
		RepoPath:     repoPath,
		RepoName:     filepath.Base(repoPath),
		Status:       status,
		Prompt:       "test prompt for " + id,
		Model:        "claude-sonnet-4-20250514",
		BudgetUSD:    10.0,
		SpentUSD:     spend,
		TurnCount:    3,
		LaunchedAt:   time.Now().Add(-5 * time.Minute),
		LastActivity: time.Now(),
		CostHistory:  []float64{0.01, 0.02, 0.03},
	}
}

// runStoreTests exercises all Store interface methods against any implementation.
func runStoreTests(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("SaveAndGet", func(t *testing.T) {
		s := testSession("sess-1", "/repos/alpha", StatusRunning, 1.50)
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}

		got, err := store.GetSession(ctx, "sess-1")
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.ID != "sess-1" {
			t.Errorf("ID = %q, want %q", got.ID, "sess-1")
		}
		if got.Provider != ProviderClaude {
			t.Errorf("Provider = %q, want %q", got.Provider, ProviderClaude)
		}
		if got.Status != StatusRunning {
			t.Errorf("Status = %q, want %q", got.Status, StatusRunning)
		}
		if got.SpentUSD != 1.50 {
			t.Errorf("SpentUSD = %f, want %f", got.SpentUSD, 1.50)
		}
		if got.RepoName != "alpha" {
			t.Errorf("RepoName = %q, want %q", got.RepoName, "alpha")
		}
	})

	t.Run("GetNotFound", func(t *testing.T) {
		_, err := store.GetSession(ctx, "nonexistent")
		if err != ErrSessionNotFound {
			t.Errorf("expected ErrSessionNotFound, got: %v", err)
		}
	})

	t.Run("SaveUpsert", func(t *testing.T) {
		s := testSession("sess-2", "/repos/beta", StatusLaunching, 0)
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("SaveSession: %v", err)
		}

		s.Status = StatusCompleted
		s.SpentUSD = 5.0
		now := time.Now()
		s.EndedAt = &now
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("SaveSession (upsert): %v", err)
		}

		got, err := store.GetSession(ctx, "sess-2")
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.Status != StatusCompleted {
			t.Errorf("Status after upsert = %q, want %q", got.Status, StatusCompleted)
		}
		if got.SpentUSD != 5.0 {
			t.Errorf("SpentUSD after upsert = %f, want %f", got.SpentUSD, 5.0)
		}
	})

	t.Run("ListAll", func(t *testing.T) {
		list, err := store.ListSessions(ctx, ListOpts{})
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(list) < 2 {
			t.Errorf("expected at least 2 sessions, got %d", len(list))
		}
	})

	t.Run("ListByRepo", func(t *testing.T) {
		list, err := store.ListSessions(ctx, ListOpts{RepoPath: "/repos/alpha"})
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 session for /repos/alpha, got %d", len(list))
		}
	})

	t.Run("ListByRepoName", func(t *testing.T) {
		list, err := store.ListSessions(ctx, ListOpts{RepoName: "beta"})
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 session for repo name beta, got %d", len(list))
		}
	})

	t.Run("ListByStatus", func(t *testing.T) {
		list, err := store.ListSessions(ctx, ListOpts{Status: StatusRunning})
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 running session, got %d", len(list))
		}
	})

	t.Run("ListWithLimit", func(t *testing.T) {
		list, err := store.ListSessions(ctx, ListOpts{Limit: 1})
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 session with limit=1, got %d", len(list))
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		if err := store.UpdateSessionStatus(ctx, "sess-1", StatusStopped); err != nil {
			t.Fatalf("UpdateSessionStatus: %v", err)
		}
		got, _ := store.GetSession(ctx, "sess-1")
		if got.Status != StatusStopped {
			t.Errorf("Status = %q, want %q", got.Status, StatusStopped)
		}
	})

	t.Run("UpdateStatusNotFound", func(t *testing.T) {
		err := store.UpdateSessionStatus(ctx, "nonexistent", StatusStopped)
		if err != ErrSessionNotFound {
			t.Errorf("expected ErrSessionNotFound, got: %v", err)
		}
	})

	t.Run("AggregateSpend", func(t *testing.T) {
		total, err := store.AggregateSpend(ctx, "/repos/beta")
		if err != nil {
			t.Fatalf("AggregateSpend: %v", err)
		}
		if total != 5.0 {
			t.Errorf("total spend for beta = %f, want %f", total, 5.0)
		}
	})

	t.Run("AggregateSpendAll", func(t *testing.T) {
		total, err := store.AggregateSpend(ctx, "")
		if err != nil {
			t.Fatalf("AggregateSpend: %v", err)
		}
		if total < 6.0 {
			t.Errorf("total spend across all = %f, want >= 6.0", total)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		if err := store.DeleteSession(ctx, "sess-1"); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		_, err := store.GetSession(ctx, "sess-1")
		if err != ErrSessionNotFound {
			t.Errorf("expected ErrSessionNotFound after delete, got: %v", err)
		}
	})

	t.Run("DeleteIdempotent", func(t *testing.T) {
		if err := store.DeleteSession(ctx, "sess-1"); err != nil {
			t.Errorf("second DeleteSession should not error, got: %v", err)
		}
	})

	t.Run("SaveNilSession", func(t *testing.T) {
		if err := store.SaveSession(ctx, nil); err == nil {
			t.Error("SaveSession(nil) should return error")
		}
	})

	t.Run("Close", func(t *testing.T) {
		if err := store.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
}

func TestMemoryStore(t *testing.T) {
	runStoreTests(t, NewMemoryStore())
}

func TestSQLiteStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-sessions.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	runStoreTests(t, store)
}

func TestSQLiteStoreEndedAt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ended-at.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Save session with EndedAt set
	s := testSession("ended-1", "/repos/gamma", StatusCompleted, 2.0)
	now := time.Now().Truncate(time.Second)
	s.EndedAt = &now
	if err := store.SaveSession(ctx, s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := store.GetSession(ctx, "ended-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if got.EndedAt == nil {
		t.Fatal("EndedAt should not be nil")
	}
	if !got.EndedAt.Truncate(time.Second).Equal(now) {
		t.Errorf("EndedAt = %v, want %v", got.EndedAt.Truncate(time.Second), now)
	}
}

func TestSQLiteStoreCostHistory(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cost-history.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	s := testSession("cost-1", "/repos/delta", StatusRunning, 0.06)
	s.CostHistory = []float64{0.01, 0.02, 0.03}
	if err := store.SaveSession(ctx, s); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := store.GetSession(ctx, "cost-1")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if len(got.CostHistory) != 3 {
		t.Fatalf("CostHistory len = %d, want 3", len(got.CostHistory))
	}
	if got.CostHistory[2] != 0.03 {
		t.Errorf("CostHistory[2] = %f, want 0.03", got.CostHistory[2])
	}
}

// ---------- Batch 2a pressure tests (WS-11) ----------

// saveWithRetry retries SaveSession on SQLITE_BUSY up to maxRetries times
// with exponential backoff. SQLite WAL mode serializes writes, so concurrent
// writers may transiently get SQLITE_BUSY.
func saveWithRetry(ctx context.Context, store Store, s *Session, maxRetries int) error {
	backoff := 5 * time.Millisecond
	var err error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err = store.SaveSession(ctx, s)
		if err == nil {
			return nil
		}
		// Retry on busy; fail fast on other errors.
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}
	return err
}

func TestSQLiteStore_ConcurrentSaveGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrent.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	const numGoroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, numGoroutines*2)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("conc-%d", idx)
			s := testSession(id, fmt.Sprintf("/repos/conc-%d", idx), StatusRunning, float64(idx)*0.10)

			if err := saveWithRetry(ctx, store, s, 10); err != nil {
				errs <- fmt.Errorf("save %s: %w", id, err)
				return
			}

			got, err := store.GetSession(ctx, id)
			if err != nil {
				errs <- fmt.Errorf("get %s: %w", id, err)
				return
			}
			if got.ID != id {
				errs <- fmt.Errorf("get %s: ID = %q", id, got.ID)
				return
			}
			if got.SpentUSD != float64(idx)*0.10 {
				errs <- fmt.Errorf("get %s: SpentUSD = %f, want %f", id, got.SpentUSD, float64(idx)*0.10)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}

	// Verify all 20 sessions exist.
	list, err := store.ListSessions(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != numGoroutines {
		t.Errorf("total sessions = %d, want %d", len(list), numGoroutines)
	}
}

func TestSQLiteStore_LargeSessionList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "large-list.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Insert 500 sessions: 200 running, 200 completed, 100 errored.
	for i := 0; i < 500; i++ {
		var status SessionStatus
		switch {
		case i < 200:
			status = StatusRunning
		case i < 400:
			status = StatusCompleted
		default:
			status = StatusErrored
		}
		s := testSession(
			fmt.Sprintf("large-%04d", i),
			fmt.Sprintf("/repos/repo-%d", i%10), // 10 distinct repo paths
			status,
			float64(i)*0.01,
		)
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("SaveSession %d: %v", i, err)
		}
	}

	// Filter by StatusRunning — expect exactly 200.
	running, err := store.ListSessions(ctx, ListOpts{Status: StatusRunning})
	if err != nil {
		t.Fatalf("ListSessions(running): %v", err)
	}
	if len(running) != 200 {
		t.Errorf("running sessions = %d, want 200", len(running))
	}

	// Filter by a specific repo path — each repo gets 500/10 = 50 sessions.
	byRepo, err := store.ListSessions(ctx, ListOpts{RepoPath: "/repos/repo-0"})
	if err != nil {
		t.Fatalf("ListSessions(repo-0): %v", err)
	}
	if len(byRepo) != 50 {
		t.Errorf("repo-0 sessions = %d, want 50", len(byRepo))
	}

	// No filter — expect all 500.
	all, err := store.ListSessions(ctx, ListOpts{})
	if err != nil {
		t.Fatalf("ListSessions(all): %v", err)
	}
	if len(all) != 500 {
		t.Errorf("total sessions = %d, want 500", len(all))
	}
}

func TestMemoryStore_ConcurrentOps(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Pre-seed some sessions so Get/Delete/List have data to work with.
	for i := 0; i < 20; i++ {
		s := testSession(fmt.Sprintf("pre-%d", i), "/repos/mem", StatusRunning, 0.01)
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("seed SaveSession: %v", err)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	// 3 savers
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				id := fmt.Sprintf("save-%d-%d", idx, j)
				s := testSession(id, "/repos/mem", StatusRunning, 0.01)
				if err := store.SaveSession(ctx, s); err != nil {
					errs <- fmt.Errorf("save %s: %w", id, err)
				}
			}
		}(i)
	}

	// 3 getters
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				id := fmt.Sprintf("pre-%d", j%20)
				_, _ = store.GetSession(ctx, id) // may or may not find (deletes racing)
			}
		}(i)
	}

	// 2 deleters
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				id := fmt.Sprintf("pre-%d", j%20)
				_ = store.DeleteSession(ctx, id) // idempotent
			}
		}(i)
	}

	// 2 listers
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, err := store.ListSessions(ctx, ListOpts{})
				if err != nil {
					errs <- fmt.Errorf("list: %w", err)
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}

func TestSQLiteStore_AggregateSpend_EmptyDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty-agg.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	total, err := store.AggregateSpend(ctx, "")
	if err != nil {
		t.Fatalf("AggregateSpend: %v", err)
	}
	if total != 0.0 {
		t.Errorf("AggregateSpend on empty DB = %f, want 0.0", total)
	}

	// Also test with a specific repo filter on empty DB.
	total, err = store.AggregateSpend(ctx, "/repos/nonexistent")
	if err != nil {
		t.Fatalf("AggregateSpend(repo): %v", err)
	}
	if total != 0.0 {
		t.Errorf("AggregateSpend(repo) on empty DB = %f, want 0.0", total)
	}
}
