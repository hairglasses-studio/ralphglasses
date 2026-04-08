package cmd

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/testutil/tenanttest"
)

func TestTenantCmd_Registered(t *testing.T) {
	for _, c := range rootCmd.Commands() {
		if c.Name() == "tenant" {
			return
		}
	}
	t.Fatal("tenant command not registered on rootCmd")
}

func TestTenantLeaderboardCmd_JSON_AllTenants(t *testing.T) {
	fx := tenanttest.NewFixture(t)
	fx.ApplyHome(t)

	store := openTenantTestStore(t, fx.StorePath)
	defer store.Close()

	ctx := context.Background()
	for _, tenant := range []*session.Tenant{
		{ID: "tenant-a", DisplayName: "Tenant A"},
		{ID: "tenant-b", DisplayName: "Tenant B"},
		{ID: "tenant-empty", DisplayName: "Tenant Empty"},
	} {
		if err := store.SaveTenant(ctx, tenant); err != nil {
			t.Fatalf("SaveTenant(%s): %v", tenant.ID, err)
		}
	}

	now := time.Now().UTC().Add(-time.Hour)
	fx.WriteExternalSession(t, "tenant-a", "reviewer-live", &session.Session{
		ID:           "reviewer-live",
		TenantID:     "tenant-a",
		Provider:     session.ProviderCodex,
		RepoPath:     fx.ScanRoot,
		RepoName:     "scan-root",
		Status:       session.StatusRunning,
		Prompt:       "Review the tenant tests",
		Model:        "gpt-5.4",
		AgentName:    "reviewer",
		SpentUSD:     2.50,
		TurnCount:    7,
		LaunchedAt:   now,
		LastActivity: now,
	})
	if err := store.SaveSession(ctx, &session.Session{
		ID:           "reviewer-ended",
		TenantID:     "tenant-a",
		Provider:     session.ProviderCodex,
		RepoPath:     fx.ScanRoot,
		RepoName:     "scan-root",
		Status:       session.StatusCompleted,
		Prompt:       "Review the persisted tenant state",
		Model:        "gpt-5.4",
		AgentName:    "reviewer",
		SpentUSD:     1.25,
		TurnCount:    4,
		LaunchedAt:   now.Add(-time.Hour),
		LastActivity: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveSession tenant-a ended: %v", err)
	}
	if err := store.SaveSession(ctx, &session.Session{
		ID:           "planner-ended",
		TenantID:     "tenant-b",
		Provider:     session.ProviderCodex,
		RepoPath:     fx.ScanRoot,
		RepoName:     "scan-root",
		Status:       session.StatusCompleted,
		Prompt:       "Plan the next tenant rollout",
		Model:        "gpt-5.4",
		AgentName:    "planner",
		SpentUSD:     4.20,
		TurnCount:    9,
		LaunchedAt:   now.Add(-2 * time.Hour),
		LastActivity: now.Add(-90 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveSession tenant-b ended: %v", err)
	}

	restore := setTenantCommandState(fx.ScanRoot, true, 10, true)
	defer restore()

	out, err := captureStdout(t, func() error {
		return tenantLeaderboardCmd.RunE(tenantLeaderboardCmd, nil)
	})
	if err != nil {
		t.Fatalf("tenant leaderboard all tenants: %v", err)
	}

	var payload struct {
		Count   int `json:"count"`
		Tenants []struct {
			TenantID      string `json:"tenant_id"`
			DisplayName   string `json:"display_name"`
			TotalSessions int    `json:"total_sessions"`
			Roles         []struct {
				Role     string  `json:"role"`
				Sessions int     `json:"sessions"`
				SpendUSD float64 `json:"spend_usd"`
				Turns    int     `json:"turns"`
			} `json:"roles"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal tenant leaderboard output: %v\n%s", err, out)
	}
	if payload.Count != 4 {
		t.Fatalf("count = %d, want 4 (_default + tenant-a + tenant-b + tenant-empty)", payload.Count)
	}

	byTenant := make(map[string]struct {
		displayName   string
		totalSessions int
		roles         []struct {
			Role     string  `json:"role"`
			Sessions int     `json:"sessions"`
			SpendUSD float64 `json:"spend_usd"`
			Turns    int     `json:"turns"`
		}
	})
	for _, tenant := range payload.Tenants {
		byTenant[tenant.TenantID] = struct {
			displayName   string
			totalSessions int
			roles         []struct {
				Role     string  `json:"role"`
				Sessions int     `json:"sessions"`
				SpendUSD float64 `json:"spend_usd"`
				Turns    int     `json:"turns"`
			}
		}{
			displayName:   tenant.DisplayName,
			totalSessions: tenant.TotalSessions,
			roles:         tenant.Roles,
		}
	}

	if got, ok := byTenant[session.DefaultTenantID]; !ok {
		t.Fatal("missing _default tenant")
	} else if got.totalSessions != 0 || len(got.roles) != 0 {
		t.Fatalf("_default tenant = %+v, want empty leaderboard", got)
	}
	if got, ok := byTenant["tenant-empty"]; !ok {
		t.Fatal("missing tenant-empty tenant")
	} else if got.totalSessions != 0 || len(got.roles) != 0 {
		t.Fatalf("tenant-empty = %+v, want empty leaderboard", got)
	}
	if got, ok := byTenant["tenant-a"]; !ok {
		t.Fatal("missing tenant-a tenant")
	} else {
		if got.totalSessions != 2 {
			t.Fatalf("tenant-a total sessions = %d, want 2", got.totalSessions)
		}
		if len(got.roles) != 1 || got.roles[0].Role != "reviewer" {
			t.Fatalf("tenant-a roles = %+v, want reviewer only", got.roles)
		}
		if got.roles[0].Sessions != 2 || got.roles[0].SpendUSD != 3.75 || got.roles[0].Turns != 11 {
			t.Fatalf("tenant-a reviewer = %+v, want 2 sessions / 3.75 / 11 turns", got.roles[0])
		}
	}
	if got, ok := byTenant["tenant-b"]; !ok {
		t.Fatal("missing tenant-b tenant")
	} else {
		if got.totalSessions != 1 {
			t.Fatalf("tenant-b total sessions = %d, want 1", got.totalSessions)
		}
		if len(got.roles) != 1 || got.roles[0].Role != "planner" || got.roles[0].Sessions != 1 {
			t.Fatalf("tenant-b roles = %+v, want planner/1", got.roles)
		}
	}
}

func TestTenantLeaderboardCmd_JSON_SingleTenantLimitExcludeEnded(t *testing.T) {
	fx := tenanttest.NewFixture(t)
	fx.ApplyHome(t)

	store := openTenantTestStore(t, fx.StorePath)
	defer store.Close()

	ctx := context.Background()
	if err := store.SaveTenant(ctx, &session.Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}

	now := time.Now().UTC().Add(-time.Hour)
	for _, sess := range []*session.Session{
		{
			ID:           "reviewer-1",
			TenantID:     "tenant-a",
			Provider:     session.ProviderCodex,
			RepoPath:     fx.ScanRoot,
			RepoName:     "scan-root",
			Status:       session.StatusRunning,
			Prompt:       "Review 1",
			Model:        "gpt-5.4",
			AgentName:    "reviewer",
			SpentUSD:     2.00,
			TurnCount:    5,
			LaunchedAt:   now,
			LastActivity: now,
		},
		{
			ID:           "reviewer-2",
			TenantID:     "tenant-a",
			Provider:     session.ProviderCodex,
			RepoPath:     fx.ScanRoot,
			RepoName:     "scan-root",
			Status:       session.StatusRunning,
			Prompt:       "Review 2",
			Model:        "gpt-5.4",
			AgentName:    "reviewer",
			SpentUSD:     1.00,
			TurnCount:    3,
			LaunchedAt:   now,
			LastActivity: now,
		},
		{
			ID:           "planner-1",
			TenantID:     "tenant-a",
			Provider:     session.ProviderCodex,
			RepoPath:     fx.ScanRoot,
			RepoName:     "scan-root",
			Status:       session.StatusRunning,
			Prompt:       "Plan 1",
			Model:        "gpt-5.4",
			AgentName:    "planner",
			SpentUSD:     0.50,
			TurnCount:    2,
			LaunchedAt:   now,
			LastActivity: now,
		},
	} {
		fx.WriteExternalSession(t, sess.TenantID, sess.ID, sess)
	}
	if err := store.SaveSession(ctx, &session.Session{
		ID:           "ended-only",
		TenantID:     "tenant-a",
		Provider:     session.ProviderCodex,
		RepoPath:     fx.ScanRoot,
		RepoName:     "scan-root",
		Status:       session.StatusCompleted,
		Prompt:       "Ended session",
		Model:        "gpt-5.4",
		AgentName:    "implementer",
		SpentUSD:     9.99,
		TurnCount:    40,
		LaunchedAt:   now.Add(-time.Hour),
		LastActivity: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveSession ended-only: %v", err)
	}

	restore := setTenantCommandState(fx.ScanRoot, true, 1, false)
	defer restore()

	out, err := captureStdout(t, func() error {
		return tenantLeaderboardCmd.RunE(tenantLeaderboardCmd, []string{"tenant-a"})
	})
	if err != nil {
		t.Fatalf("tenant leaderboard single tenant: %v", err)
	}

	var payload struct {
		Count   int `json:"count"`
		Tenants []struct {
			TenantID      string `json:"tenant_id"`
			TotalSessions int    `json:"total_sessions"`
			Roles         []struct {
				Role     string `json:"role"`
				Sessions int    `json:"sessions"`
			} `json:"roles"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal tenant leaderboard output: %v\n%s", err, out)
	}
	if payload.Count != 1 || len(payload.Tenants) != 1 {
		t.Fatalf("payload = %+v, want exactly one tenant", payload)
	}
	tenant := payload.Tenants[0]
	if tenant.TenantID != "tenant-a" {
		t.Fatalf("tenant_id = %q, want tenant-a", tenant.TenantID)
	}
	if tenant.TotalSessions != 3 {
		t.Fatalf("total_sessions = %d, want 3 (ended store session excluded)", tenant.TotalSessions)
	}
	if len(tenant.Roles) != 1 {
		t.Fatalf("roles = %+v, want limit=1 to keep one role", tenant.Roles)
	}
	if tenant.Roles[0].Role != "reviewer" || tenant.Roles[0].Sessions != 2 {
		t.Fatalf("top role = %+v, want reviewer/2", tenant.Roles[0])
	}
}

func TestTenantLeaderboardCmd_Text_EmptyTenant(t *testing.T) {
	fx := tenanttest.NewFixture(t)
	fx.ApplyHome(t)

	store := openTenantTestStore(t, fx.StorePath)
	defer store.Close()

	if err := store.SaveTenant(context.Background(), &session.Tenant{ID: "tenant-empty", DisplayName: "Tenant Empty"}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}

	restore := setTenantCommandState(fx.ScanRoot, false, 10, true)
	defer restore()

	out, err := captureStdout(t, func() error {
		return tenantLeaderboardCmd.RunE(tenantLeaderboardCmd, []string{"tenant-empty"})
	})
	if err != nil {
		t.Fatalf("tenant leaderboard text: %v", err)
	}
	if !strings.Contains(out, "Tenant: tenant-empty (Tenant Empty)") {
		t.Fatalf("output missing tenant header:\n%s", out)
	}
	if !strings.Contains(out, "Total sessions: 0") {
		t.Fatalf("output missing total sessions:\n%s", out)
	}
	if !strings.Contains(out, "No role activity.") {
		t.Fatalf("output missing empty-role message:\n%s", out)
	}
}

func openTenantTestStore(t *testing.T, dbPath string) *session.SQLiteStore {
	t.Helper()
	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore(%s): %v", dbPath, err)
	}
	return store
}

func setTenantCommandState(scan string, jsonOut bool, limit int, includeEnded bool) func() {
	origScanPath := scanPath
	origTenantJSON := tenantJSON
	origLimit := tenantLeaderboardLimit
	origIncludeEnded := tenantLeaderboardIncludeEnded

	scanPath = scan
	tenantJSON = jsonOut
	tenantLeaderboardLimit = limit
	tenantLeaderboardIncludeEnded = includeEnded

	return func() {
		scanPath = origScanPath
		tenantJSON = origTenantJSON
		tenantLeaderboardLimit = origLimit
		tenantLeaderboardIncludeEnded = origIncludeEnded
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	runErr := fn()
	_ = w.Close()

	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	return string(data), runErr
}
