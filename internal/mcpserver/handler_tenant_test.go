package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleTenantRoleLeaderboards_AllTenants(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	store := session.NewMemoryStore()
	srv.SessMgr.SetStore(store)

	ctx := context.Background()
	if _, err := srv.SessMgr.SaveTenant(ctx, &session.Tenant{ID: "tenant-a", DisplayName: "Tenant A"}); err != nil {
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

	result, err := srv.handleTenantRoleLeaderboards(ctx, makeRequest(map[string]any{
		"limit":         5.0,
		"include_ended": true,
	}))
	if err != nil {
		t.Fatalf("handleTenantRoleLeaderboards: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleTenantRoleLeaderboards returned error: %s", getResultText(result))
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
