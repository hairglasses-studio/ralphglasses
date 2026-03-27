package session

import (
	"context"
	"path/filepath"
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
