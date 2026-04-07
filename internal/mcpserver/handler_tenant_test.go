package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestTenantRoleLeaderboardsTool_AllTenants(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	store := session.NewMemoryStore()
	h.Server.SessMgr.SetStore(store)

	ctx := context.Background()
	if _, err := h.Server.SessMgr.SaveTenant(ctx, &session.Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}
	now := time.Now()
	for _, sess := range []*session.Session{
		{ID: "a1", TenantID: "tenant-a", AgentName: "reviewer", Status: session.StatusCompleted, RepoPath: "/tmp/a", RepoName: "a", LaunchedAt: now},
		{ID: "a2", TenantID: "tenant-a", AgentName: "reviewer", Status: session.StatusRunning, RepoPath: "/tmp/a", RepoName: "a", LaunchedAt: now},
	} {
		if err := store.SaveSession(ctx, sess); err != nil {
			t.Fatalf("SaveSession %s: %v", sess.ID, err)
		}
	}

	result, err := h.CallTool("ralphglasses_tenant_role_leaderboards", map[string]any{
		"limit":         5.0,
		"include_ended": true,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tenant_role_leaderboards returned error: %s", getResultText(result))
	}

	var payload struct {
		Count   int `json:"count"`
		Tenants []struct {
			TenantID string `json:"tenant_id"`
			Roles    []struct {
				Role     string `json:"role"`
				Sessions int    `json:"sessions"`
			} `json:"roles"`
		} `json:"tenants"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Count != 2 {
		t.Fatalf("count = %d, want 2 (_default + tenant-a)", payload.Count)
	}
	var found bool
	for _, tenant := range payload.Tenants {
		if tenant.TenantID != "tenant-a" {
			continue
		}
		found = true
		if len(tenant.Roles) != 1 {
			t.Fatalf("tenant-a roles = %d, want 1", len(tenant.Roles))
		}
		if tenant.Roles[0].Role != "reviewer" || tenant.Roles[0].Sessions != 2 {
			t.Fatalf("tenant-a top role = %#v, want reviewer/2", tenant.Roles[0])
		}
	}
	if !found {
		t.Fatal("tenant-a leaderboard missing")
	}
}

func TestTenantRoleLeaderboardsTool_SingleTenantLimitExcludeEnded(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)
	store := session.NewMemoryStore()
	h.Server.SessMgr.SetStore(store)

	ctx := context.Background()
	if _, err := h.Server.SessMgr.SaveTenant(ctx, &session.Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
		t.Fatalf("SaveTenant: %v", err)
	}

	now := time.Now()
	h.Server.SessMgr.AddSessionForTesting(&session.Session{
		ID:           "live-reviewer",
		TenantID:     "tenant-a",
		AgentName:    "reviewer",
		Status:       session.StatusRunning,
		RepoPath:     "/tmp/a",
		RepoName:     "a",
		LaunchedAt:   now,
		LastActivity: now,
	})
	if err := store.SaveSession(ctx, &session.Session{
		ID:           "ended-planner",
		TenantID:     "tenant-a",
		AgentName:    "planner",
		Status:       session.StatusCompleted,
		RepoPath:     "/tmp/a",
		RepoName:     "a",
		LaunchedAt:   now.Add(-time.Hour),
		LastActivity: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}

	result, err := h.CallTool("ralphglasses_tenant_role_leaderboards", map[string]any{
		"tenant_id":     "tenant-a",
		"limit":         1.0,
		"include_ended": false,
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if result.IsError {
		t.Fatalf("tenant_role_leaderboards returned error: %s", getResultText(result))
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
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Count != 1 || len(payload.Tenants) != 1 {
		t.Fatalf("payload = %+v, want one tenant", payload)
	}
	if payload.Tenants[0].TenantID != "tenant-a" || payload.Tenants[0].TotalSessions != 1 {
		t.Fatalf("tenant payload = %+v, want tenant-a with one live session", payload.Tenants[0])
	}
	if len(payload.Tenants[0].Roles) != 1 || payload.Tenants[0].Roles[0].Role != "reviewer" {
		t.Fatalf("roles = %+v, want reviewer only", payload.Tenants[0].Roles)
	}
}
