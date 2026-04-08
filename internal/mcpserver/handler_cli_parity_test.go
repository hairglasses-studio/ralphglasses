package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleConfigSchema_FilterAndDefaults(t *testing.T) {
	t.Parallel()

	srv, _ := setupTestServer(t)
	result, err := srv.handleConfigSchema(context.Background(), makeRequest(map[string]any{
		"key":                 "default_provider",
		"include_defaults":    true,
		"include_constraints": true,
	}))
	if err != nil {
		t.Fatalf("handleConfigSchema: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var payload struct {
		Count int `json:"count"`
		Keys  []struct {
			Name       string `json:"name"`
			HasDefault bool   `json:"has_default"`
		} `json:"keys"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload.Count != 1 {
		t.Fatalf("count = %d, want 1", payload.Count)
	}
	if len(payload.Keys) != 1 || payload.Keys[0].Name != "default_provider" {
		t.Fatalf("unexpected keys payload: %+v", payload.Keys)
	}
}

func TestHandleFirstbootProfile_SetAndMarkDone(t *testing.T) {
	t.Parallel()

	srv, _ := setupTestServer(t)
	configDir := t.TempDir()

	setResult, err := srv.handleFirstbootProfile(context.Background(), makeRequest(map[string]any{
		"action":          "set",
		"config_dir":      configDir,
		"hostname":        "rg-parity-01",
		"autonomy_level":  float64(2),
		"coordinator_url": "http://127.0.0.1:9473",
		"openai_api_key":  "sk-test-1234567890",
	}))
	if err != nil {
		t.Fatalf("set profile: %v", err)
	}
	if setResult.IsError {
		t.Fatalf("set returned error: %s", getResultText(setResult))
	}

	var setPayload map[string]any
	if err := json.Unmarshal([]byte(getResultText(setResult)), &setPayload); err != nil {
		t.Fatalf("unmarshal set payload: %v", err)
	}
	if setPayload["done"] != false {
		t.Fatalf("done = %v, want false", setPayload["done"])
	}
	profile, _ := setPayload["profile"].(map[string]any)
	apiKeys, _ := profile["api_keys"].(map[string]any)
	if got := apiKeys["openai"]; got != "sk-t...7890" {
		t.Fatalf("redacted openai key = %v, want sk-t...7890", got)
	}

	markResult, err := srv.handleFirstbootProfile(context.Background(), makeRequest(map[string]any{
		"action":     "mark_done",
		"config_dir": configDir,
	}))
	if err != nil {
		t.Fatalf("mark_done: %v", err)
	}
	if markResult.IsError {
		t.Fatalf("mark_done returned error: %s", getResultText(markResult))
	}

	var markPayload map[string]any
	if err := json.Unmarshal([]byte(getResultText(markResult)), &markPayload); err != nil {
		t.Fatalf("unmarshal mark payload: %v", err)
	}
	if markPayload["done"] != true {
		t.Fatalf("done = %v, want true", markPayload["done"])
	}
}

func TestHandleBudgetStatus_AggregatesTenantSessions(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.TenantID = "tenant-a"
		s.BudgetUSD = 10
		s.SpentUSD = 4
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.TenantID = "tenant-a"
		s.BudgetUSD = 20
		s.SpentUSD = 10
		s.Status = session.StatusStopped
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.TenantID = "tenant-b"
		s.BudgetUSD = 99
		s.SpentUSD = 50
	})

	result, err := srv.handleBudgetStatus(context.Background(), makeRequest(map[string]any{
		"tenant_id": "tenant-a",
	}))
	if err != nil {
		t.Fatalf("handleBudgetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload["tenant_id"] != "tenant-a" {
		t.Fatalf("tenant_id = %v, want tenant-a", payload["tenant_id"])
	}
	if payload["total_spent_usd"] != float64(14) {
		t.Fatalf("total_spent_usd = %v, want 14", payload["total_spent_usd"])
	}
	if payload["total_budget_usd"] != float64(30) {
		t.Fatalf("total_budget_usd = %v, want 30", payload["total_budget_usd"])
	}
	if payload["sessions_active"] != float64(1) {
		t.Fatalf("sessions_active = %v, want 1", payload["sessions_active"])
	}
	if payload["sessions_done"] != float64(1) {
		t.Fatalf("sessions_done = %v, want 1", payload["sessions_done"])
	}
}

func TestHandleRepoSurfaceAudit(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	if err := os.WriteFile(filepath.Join(repoPath, "AGENTS.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".mcp.json"), []byte("{\"mcpServers\":{}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".codex", "config.toml"), []byte("[mcp]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleRepoSurfaceAudit(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoSurfaceAudit: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var payload struct {
		Healthy  bool           `json:"healthy"`
		Issues   []string       `json:"issues"`
		Warnings []string       `json:"warnings"`
		Surfaces map[string]any `json:"surfaces"`
		RepoPath string         `json:"repo_path"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if !payload.Healthy {
		t.Fatalf("healthy = false, issues = %v", payload.Issues)
	}
	if payload.RepoPath != repoPath {
		t.Fatalf("repo_path = %q, want %q", payload.RepoPath, repoPath)
	}
	if _, ok := payload.Surfaces["agents_md"]; !ok {
		t.Fatalf("agents_md surface missing: %+v", payload.Surfaces)
	}
	if !strings.Contains(strings.Join(payload.Warnings, " "), "CLAUDE.md") {
		t.Fatalf("expected warning about missing CLAUDE.md, warnings = %v", payload.Warnings)
	}
}
