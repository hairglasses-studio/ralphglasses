package session

import (
	"context"
	"testing"
	"time"
)

func TestBuildRoleLeaderboard_DedupesLiveAndStoredSessions(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	mgr := NewManagerWithStore(store, nil)

	if _, err := mgr.SaveTenant(ctx, &Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}

	now := time.Now()
	live := &Session{
		ID:         "sess-live-reviewer",
		TenantID:   "tenant-a",
		AgentName:  "reviewer",
		Status:     StatusRunning,
		SpentUSD:   1.25,
		TurnCount:  4,
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		LaunchedAt: now,
	}
	mgr.AddSessionForTesting(live)

	// Duplicate of the live session in the store should not double-count.
	if err := store.SaveSession(ctx, &Session{
		ID:         "sess-live-reviewer",
		TenantID:   "tenant-a",
		AgentName:  "reviewer",
		Status:     StatusCompleted,
		SpentUSD:   9.99,
		TurnCount:  99,
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		LaunchedAt: now,
	}); err != nil {
		t.Fatalf("SaveSession duplicate: %v", err)
	}

	if err := store.SaveSession(ctx, &Session{
		ID:         "sess-ended-reviewer",
		TenantID:   "tenant-a",
		AgentName:  "reviewer",
		Status:     StatusCompleted,
		SpentUSD:   2.00,
		TurnCount:  3,
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		LaunchedAt: now,
	}); err != nil {
		t.Fatalf("SaveSession ended reviewer: %v", err)
	}
	if err := store.SaveSession(ctx, &Session{
		ID:         "sess-ended-unassigned",
		TenantID:   "tenant-a",
		Status:     StatusErrored,
		SpentUSD:   0.75,
		TurnCount:  2,
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		LaunchedAt: now,
	}); err != nil {
		t.Fatalf("SaveSession unassigned: %v", err)
	}

	board, err := mgr.BuildRoleLeaderboard(ctx, "tenant-a", RoleLeaderboardOptions{
		IncludeEnded: true,
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("BuildRoleLeaderboard: %v", err)
	}
	if board.TenantID != "tenant-a" {
		t.Fatalf("TenantID = %q, want tenant-a", board.TenantID)
	}
	if board.TotalSessions != 3 {
		t.Fatalf("TotalSessions = %d, want 3", board.TotalSessions)
	}
	if len(board.Roles) != 2 {
		t.Fatalf("len(Roles) = %d, want 2", len(board.Roles))
	}
	if board.Roles[0].Role != "reviewer" {
		t.Fatalf("top role = %q, want reviewer", board.Roles[0].Role)
	}
	if board.Roles[0].Sessions != 2 {
		t.Fatalf("reviewer sessions = %d, want 2", board.Roles[0].Sessions)
	}
	if board.Roles[0].SpendUSD != 3.25 {
		t.Fatalf("reviewer spend = %.2f, want 3.25", board.Roles[0].SpendUSD)
	}
	if board.Roles[0].Active != 1 || board.Roles[0].Completed != 1 {
		t.Fatalf("reviewer active/completed = %d/%d, want 1/1", board.Roles[0].Active, board.Roles[0].Completed)
	}
	if board.Roles[1].Role != UnassignedRoleName {
		t.Fatalf("second role = %q, want %q", board.Roles[1].Role, UnassignedRoleName)
	}
	if board.Roles[1].Errored != 1 {
		t.Fatalf("unassigned errored = %d, want 1", board.Roles[1].Errored)
	}
}

func TestBuildRoleLeaderboards_AllTenants(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	mgr := NewManagerWithStore(store, nil)

	if _, err := mgr.SaveTenant(ctx, &Tenant{ID: "tenant-b", DisplayName: "Tenant B"}); err != nil {
		t.Fatalf("SaveTenant tenant-b: %v", err)
	}
	if _, err := mgr.SaveTenant(ctx, &Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
		t.Fatalf("SaveTenant tenant-a: %v", err)
	}

	now := time.Now()
	for _, sess := range []*Session{
		{ID: "a1", TenantID: "tenant-a", AgentName: "reviewer", Status: StatusCompleted, RepoPath: "/tmp/a", RepoName: "a", LaunchedAt: now},
		{ID: "b1", TenantID: "tenant-b", AgentName: "implementer", Status: StatusCompleted, RepoPath: "/tmp/b", RepoName: "b", LaunchedAt: now},
	} {
		if err := store.SaveSession(ctx, sess); err != nil {
			t.Fatalf("SaveSession %s: %v", sess.ID, err)
		}
	}

	boards, err := mgr.BuildRoleLeaderboards(ctx, RoleLeaderboardOptions{
		IncludeEnded: true,
		Limit:        5,
	})
	if err != nil {
		t.Fatalf("BuildRoleLeaderboards: %v", err)
	}
	if len(boards) != 3 {
		t.Fatalf("len(boards) = %d, want 3 (_default + tenant-a + tenant-b)", len(boards))
	}
	if boards[0].TenantID != DefaultTenantID {
		t.Fatalf("boards[0].TenantID = %q, want %q", boards[0].TenantID, DefaultTenantID)
	}
	if boards[1].TenantID != "tenant-a" {
		t.Fatalf("boards[1].TenantID = %q, want tenant-a", boards[1].TenantID)
	}
	if boards[2].TenantID != "tenant-b" {
		t.Fatalf("boards[2].TenantID = %q, want tenant-b", boards[2].TenantID)
	}
}
