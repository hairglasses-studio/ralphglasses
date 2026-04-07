package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/testutil/tenanttest"
)

func TestMemoryStore_ListSessionsFiltersByTenant(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()

	for _, sess := range []*Session{
		{ID: "sess-default", TenantID: DefaultTenantID, RepoPath: "/tmp/a", RepoName: "a", Status: StatusRunning, LaunchedAt: now, LastActivity: now},
		{ID: "sess-tenant-a", TenantID: "tenant-a", RepoPath: "/tmp/a", RepoName: "a", Status: StatusRunning, LaunchedAt: now, LastActivity: now},
		{ID: "sess-tenant-b", TenantID: "tenant-b", RepoPath: "/tmp/b", RepoName: "b", Status: StatusRunning, LaunchedAt: now, LastActivity: now},
	} {
		if err := store.SaveSession(ctx, sess); err != nil {
			t.Fatalf("SaveSession(%s): %v", sess.ID, err)
		}
	}

	got, err := store.ListSessions(ctx, ListOpts{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(got) != 1 || got[0].ID != "sess-tenant-a" {
		t.Fatalf("tenant-a sessions = %#v, want only sess-tenant-a", got)
	}
}

func TestMemoryStore_GetSessionReturnsDetachedCopy(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()
	original := &Session{
		ID:            "sess-copy",
		TenantID:      "tenant-a",
		RepoPath:      "/tmp/a",
		RepoName:      "a",
		Status:        StatusRunning,
		OutputHistory: []string{"first"},
		LaunchedAt:    now,
		LastActivity:  now,
	}
	if err := store.SaveSession(ctx, original); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	got, err := store.GetSession(ctx, original.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	got.Status = StatusStopped
	got.OutputHistory[0] = "mutated"

	again, err := store.GetSession(ctx, original.ID)
	if err != nil {
		t.Fatalf("GetSession(second): %v", err)
	}
	if again.Status != StatusRunning {
		t.Fatalf("stored status = %q, want %q", again.Status, StatusRunning)
	}
	if again.OutputHistory[0] != "first" {
		t.Fatalf("stored output history = %#v, want first entry preserved", again.OutputHistory)
	}
}

func TestSQLiteStore_TenantRoundTrip(t *testing.T) {
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	tenant := &Tenant{
		ID:               "tenant-a",
		DisplayName:      "Tenant A",
		AllowedRepoRoots: []string{"/repos/a"},
		BudgetCapUSD:     25,
	}
	if err := store.SaveTenant(ctx, tenant); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}
	gotTenant, err := store.GetTenant(ctx, "tenant-a")
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if gotTenant.DisplayName != "Tenant A" {
		t.Fatalf("DisplayName = %q, want Tenant A", gotTenant.DisplayName)
	}

	now := time.Now()
	sess := &Session{
		ID:           "sess-a",
		TenantID:     "tenant-a",
		Provider:     ProviderCodex,
		RepoPath:     "/repos/a/project",
		RepoName:     "project",
		Status:       StatusRunning,
		Prompt:       "ship it",
		SpentUSD:     3.25,
		LaunchedAt:   now,
		LastActivity: now,
	}
	if err := store.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	total, err := store.AggregateSpend(ctx, "tenant-a", "")
	if err != nil {
		t.Fatalf("AggregateSpend: %v", err)
	}
	if total != 3.25 {
		t.Fatalf("AggregateSpend = %.2f, want 3.25", total)
	}
}

func TestSQLiteStore_MigratesLegacyTenantColumnsBeforeIndexes(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-state.db")
	tenanttest.SeedLegacySQLiteSchema(t, dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore(legacy): %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveTenant(ctx, &Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}

	now := time.Now().UTC()
	if err := store.SaveSession(ctx, &Session{
		ID:           "legacy-session",
		TenantID:     "tenant-a",
		Provider:     ProviderCodex,
		RepoPath:     "/repos/a",
		RepoName:     "a",
		Status:       StatusRunning,
		AgentName:    "reviewer",
		LaunchedAt:   now,
		LastActivity: now,
	}); err != nil {
		t.Fatalf("SaveSession after migration: %v", err)
	}

	list, err := store.ListSessions(ctx, ListOpts{TenantID: "tenant-a"})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 1 || list[0].TenantID != "tenant-a" {
		t.Fatalf("migrated tenant sessions = %#v, want tenant-a session", list)
	}
}

func TestStructuredTeamPathsIncludeTenant(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), "repo")
	task := &TeamTask{ID: "task-1", Attempt: 1}

	teamA := &TeamStatus{Name: "platform", TenantID: "tenant-a", RepoPath: repoPath}
	teamB := &TeamStatus{Name: "platform", TenantID: "tenant-b", RepoPath: repoPath}

	intA := structuredTeamIntegrationPath(teamA)
	intB := structuredTeamIntegrationPath(teamB)
	if intA == intB {
		t.Fatalf("integration paths should differ by tenant: %q", intA)
	}
	if !strings.Contains(intA, "tenant-a") || !strings.Contains(intB, "tenant-b") {
		t.Fatalf("integration paths should contain tenant IDs: %q %q", intA, intB)
	}

	wtA := structuredTeamTaskWorktreePath(teamA, task)
	wtB := structuredTeamTaskWorktreePath(teamB, task)
	if wtA == wtB {
		t.Fatalf("worktree paths should differ by tenant: %q", wtA)
	}
	if !strings.Contains(wtA, "tenant-a") || !strings.Contains(wtB, "tenant-b") {
		t.Fatalf("worktree paths should contain tenant IDs: %q %q", wtA, wtB)
	}
}

func TestManagerLaunch_EnforcesTenantRepoRoots(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	if err := store.SaveTenant(ctx, &Tenant{
		ID:               "tenant-a",
		DisplayName:      "Tenant A",
		AllowedRepoRoots: []string{filepath.Join(t.TempDir(), "allowed-root")},
	}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}

	mgr := NewManager()
	mgr.SetStore(store)
	mgr.SetHooksForTesting(func(_ context.Context, opts LaunchOptions) (*Session, error) {
		now := time.Now()
		return &Session{
			ID:           "sess-a",
			TenantID:     opts.TenantID,
			Provider:     opts.Provider,
			RepoPath:     opts.RepoPath,
			RepoName:     filepath.Base(opts.RepoPath),
			Status:       StatusRunning,
			Prompt:       opts.Prompt,
			LaunchedAt:   now,
			LastActivity: now,
		}, nil
	}, nil)

	_, err := mgr.Launch(ctx, LaunchOptions{
		TenantID: "tenant-a",
		Provider: ProviderCodex,
		RepoPath: filepath.Join(t.TempDir(), "forbidden-root", "repo"),
		Prompt:   "test",
	})
	if err == nil {
		t.Fatal("expected launch to fail for repo outside allowed roots")
	}
	if !strings.Contains(err.Error(), "cannot access repo path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
