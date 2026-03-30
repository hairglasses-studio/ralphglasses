package session

import (
	"context"
	"testing"
	"time"
)

func TestRehydrateFromStore_LoadsSessions(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()

	// Seed the store with sessions in various terminal states.
	now := time.Now()
	sessions := []*Session{
		{
			ID: "sess-completed", Provider: ProviderClaude, Status: StatusCompleted,
			RepoPath: "/tmp/repo", RepoName: "repo",
			LaunchedAt: now.Add(-1 * time.Hour), LastActivity: now.Add(-30 * time.Minute),
		},
		{
			ID: "sess-stopped", Provider: ProviderGemini, Status: StatusStopped,
			RepoPath: "/tmp/repo", RepoName: "repo",
			LaunchedAt: now.Add(-2 * time.Hour), LastActivity: now.Add(-1 * time.Hour),
		},
		{
			ID: "sess-errored", Provider: ProviderClaude, Status: StatusErrored,
			RepoPath: "/tmp/repo2", RepoName: "repo2",
			LaunchedAt: now.Add(-30 * time.Minute), LastActivity: now.Add(-10 * time.Minute),
			Error: "something failed",
		},
	}
	for _, s := range sessions {
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("seed session %s: %v", s.ID, err)
		}
	}

	// Create a manager with an empty in-memory map and the seeded store.
	m := NewManagerWithStore(store, nil)

	if err := m.RehydrateFromStore(); err != nil {
		t.Fatalf("RehydrateFromStore() error: %v", err)
	}

	// All 3 sessions should be loaded.
	loaded := m.List("")
	if len(loaded) != 3 {
		t.Errorf("expected 3 rehydrated sessions, got %d", len(loaded))
	}

	// Verify each session is present with correct status.
	for _, id := range []string{"sess-completed", "sess-stopped", "sess-errored"} {
		if _, ok := m.Get(id); !ok {
			t.Errorf("session %s not found after rehydration", id)
		}
	}
}

func TestRehydrateFromStore_RunningMarkedInterrupted(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()

	now := time.Now()

	// Seed sessions that were "running" and "launching" when the old process died.
	running := &Session{
		ID: "sess-was-running", Provider: ProviderClaude, Status: StatusRunning,
		RepoPath: "/tmp/repo", RepoName: "repo", Pid: 99999,
		LaunchedAt: now.Add(-5 * time.Minute), LastActivity: now.Add(-1 * time.Minute),
	}
	launching := &Session{
		ID: "sess-was-launching", Provider: ProviderGemini, Status: StatusLaunching,
		RepoPath: "/tmp/repo", RepoName: "repo", Pid: 88888,
		LaunchedAt: now.Add(-2 * time.Minute), LastActivity: now.Add(-1 * time.Minute),
	}
	for _, s := range []*Session{running, launching} {
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("seed session %s: %v", s.ID, err)
		}
	}

	m := NewManagerWithStore(store, nil)

	if err := m.RehydrateFromStore(); err != nil {
		t.Fatalf("RehydrateFromStore() error: %v", err)
	}

	// Both should be rehydrated with interrupted status.
	for _, id := range []string{"sess-was-running", "sess-was-launching"} {
		sess, ok := m.Get(id)
		if !ok {
			t.Fatalf("session %s not found after rehydration", id)
		}
		sess.Lock()
		status := sess.Status
		pid := sess.Pid
		endedAt := sess.EndedAt
		exitReason := sess.ExitReason
		sess.Unlock()

		if status != StatusInterrupted {
			t.Errorf("session %s: expected status %q, got %q", id, StatusInterrupted, status)
		}
		if pid != 0 {
			t.Errorf("session %s: expected PID 0 (stale cleared), got %d", id, pid)
		}
		if endedAt == nil {
			t.Errorf("session %s: expected EndedAt to be set", id)
		}
		if exitReason == "" {
			t.Errorf("session %s: expected ExitReason to be set", id)
		}
	}

	// Verify the store was updated with interrupted status.
	for _, id := range []string{"sess-was-running", "sess-was-launching"} {
		stored, err := store.GetSession(ctx, id)
		if err != nil {
			t.Fatalf("store.GetSession(%s): %v", id, err)
		}
		if stored.Status != StatusInterrupted {
			t.Errorf("store session %s: expected status %q, got %q", id, StatusInterrupted, stored.Status)
		}
	}
}

func TestRehydrateFromStore_DedupsExisting(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	// Seed the store with a session.
	storeSession := &Session{
		ID: "sess-existing", Provider: ProviderClaude, Status: StatusCompleted,
		RepoPath: "/tmp/repo", RepoName: "repo",
		LaunchedAt: now.Add(-1 * time.Hour), LastActivity: now.Add(-30 * time.Minute),
		SpentUSD: 0.50,
	}
	if err := store.SaveSession(ctx, storeSession); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	// Create a manager that already has this session in memory with different data.
	m := NewManagerWithStore(store, nil)
	inMemory := &Session{
		ID: "sess-existing", Provider: ProviderClaude, Status: StatusRunning,
		RepoPath: "/tmp/repo", RepoName: "repo",
		LaunchedAt: now, LastActivity: now,
		SpentUSD: 1.25,
	}
	m.sessionsMu.Lock()
	m.sessions["sess-existing"] = inMemory
	m.sessionsMu.Unlock()

	if err := m.RehydrateFromStore(); err != nil {
		t.Fatalf("RehydrateFromStore() error: %v", err)
	}

	// The in-memory session should NOT be overwritten.
	sess, ok := m.Get("sess-existing")
	if !ok {
		t.Fatal("session sess-existing not found")
	}
	sess.Lock()
	status := sess.Status
	spent := sess.SpentUSD
	sess.Unlock()

	if status != StatusRunning {
		t.Errorf("expected in-memory status %q preserved, got %q", StatusRunning, status)
	}
	if spent != 1.25 {
		t.Errorf("expected in-memory SpentUSD 1.25 preserved, got %.2f", spent)
	}
}

func TestRehydrateFromStore_NilStoreNoop(t *testing.T) {
	t.Parallel()

	// Manager with no store configured.
	m := NewManager()

	err := m.RehydrateFromStore()
	if err != nil {
		t.Errorf("RehydrateFromStore() with nil store should return nil, got: %v", err)
	}

	// No sessions should exist.
	if count := len(m.List("")); count != 0 {
		t.Errorf("expected 0 sessions with nil store, got %d", count)
	}
}

func TestRehydrateFromStore_TerminalSessionsPreserved(t *testing.T) {
	t.Parallel()

	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	// Seed with a completed session and an errored session.
	completed := &Session{
		ID: "sess-done", Provider: ProviderClaude, Status: StatusCompleted,
		RepoPath: "/tmp/repo", RepoName: "repo",
		LaunchedAt: now.Add(-1 * time.Hour), LastActivity: now,
		ExitReason: "completed normally",
	}
	errored := &Session{
		ID: "sess-err", Provider: ProviderClaude, Status: StatusErrored,
		RepoPath: "/tmp/repo", RepoName: "repo",
		LaunchedAt: now.Add(-30 * time.Minute), LastActivity: now,
		Error: "out of budget",
	}
	for _, s := range []*Session{completed, errored} {
		if err := store.SaveSession(ctx, s); err != nil {
			t.Fatalf("seed %s: %v", s.ID, err)
		}
	}

	m := NewManagerWithStore(store, nil)
	if err := m.RehydrateFromStore(); err != nil {
		t.Fatalf("RehydrateFromStore() error: %v", err)
	}

	// Completed session should keep its original status.
	if sess, ok := m.Get("sess-done"); ok {
		sess.Lock()
		if sess.Status != StatusCompleted {
			t.Errorf("completed session status = %q, want %q", sess.Status, StatusCompleted)
		}
		sess.Unlock()
	} else {
		t.Error("sess-done not found")
	}

	// Errored session should keep its original status and error.
	if sess, ok := m.Get("sess-err"); ok {
		sess.Lock()
		if sess.Status != StatusErrored {
			t.Errorf("errored session status = %q, want %q", sess.Status, StatusErrored)
		}
		if sess.Error != "out of budget" {
			t.Errorf("errored session error = %q, want %q", sess.Error, "out of budget")
		}
		sess.Unlock()
	} else {
		t.Error("sess-err not found")
	}
}
