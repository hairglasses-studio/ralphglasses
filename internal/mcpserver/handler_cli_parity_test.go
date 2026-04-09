package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func reserveTCPPort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()

	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("unexpected listener addr type: %T", ln.Addr())
	}
	return addr.Port
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() (bool, string)) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		ok, detail := fn()
		if ok {
			return
		}
		last = detail
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s: %s", timeout, last)
}

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

func TestHandleFirstbootProfile_ValidateIssues(t *testing.T) {
	t.Parallel()

	srv, _ := setupTestServer(t)
	result, err := srv.handleFirstbootProfile(context.Background(), makeRequest(map[string]any{
		"action":         "validate",
		"config_dir":     t.TempDir(),
		"hostname":       "",
		"autonomy_level": float64(9),
	}))
	if err != nil {
		t.Fatalf("validate profile: %v", err)
	}
	if result.IsError {
		t.Fatalf("validate returned error: %s", getResultText(result))
	}

	var payload struct {
		Action string   `json:"action"`
		Issues []string `json:"issues"`
		Done   bool     `json:"done"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal validate payload: %v", err)
	}
	if payload.Action != "validate" {
		t.Fatalf("action = %q, want validate", payload.Action)
	}
	if payload.Done {
		t.Fatalf("done = true, want false")
	}
	if !strings.Contains(strings.Join(payload.Issues, " "), "hostname is required") {
		t.Fatalf("expected hostname issue, got %v", payload.Issues)
	}
	if !strings.Contains(strings.Join(payload.Issues, " "), "autonomy_level must be between 0 and 3") {
		t.Fatalf("expected autonomy issue, got %v", payload.Issues)
	}
}

func TestHandleFleetRuntime_CoordinatorLifecycle(t *testing.T) {
	t.Parallel()

	srv, _ := setupTestServer(t)
	port := reserveTCPPort(t)
	t.Cleanup(func() {
		_, _ = srv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{"action": "stop"}))
	})

	result, err := srv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{
		"action":     "start",
		"mode":       "coordinator",
		"port":       float64(port),
		"automation": false,
	}))
	if err != nil {
		t.Fatalf("start coordinator: %v", err)
	}
	if result.IsError {
		t.Fatalf("start coordinator returned error: %s", getResultText(result))
	}

	waitForCondition(t, 10*time.Second, func() (bool, string) {
		status := srv.fleetRuntimeStatus(context.Background())
		_, hasNodeStatus := status["node_status"]
		active, _ := status["active"].(bool)
		return active && hasNodeStatus, fmt.Sprintf("snapshot=%v", status)
	})

	stopResult, err := srv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{
		"action": "stop",
	}))
	if err != nil {
		t.Fatalf("stop coordinator: %v", err)
	}
	if stopResult.IsError {
		t.Fatalf("stop coordinator returned error: %s", getResultText(stopResult))
	}

	waitForCondition(t, 10*time.Second, func() (bool, string) {
		status := srv.fleetRuntimeStatus(context.Background())
		active, _ := status["active"].(bool)
		return !active, fmt.Sprintf("snapshot=%v", status)
	})
}

func TestHandleFleetRuntime_WorkerLifecycle(t *testing.T) {
	t.Parallel()

	coordSrv, root := setupTestServer(t)
	workerSrv := NewServer(root)
	workerSrv.SessMgr.SetStateDir(filepath.Join(root, ".worker-session-state"))

	coordPort := reserveTCPPort(t)
	workerPort := reserveTCPPort(t)
	t.Cleanup(func() {
		_, _ = workerSrv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{"action": "stop"}))
		_, _ = coordSrv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{"action": "stop"}))
	})

	startCoord, err := coordSrv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{
		"action":     "start",
		"mode":       "coordinator",
		"port":       float64(coordPort),
		"automation": false,
	}))
	if err != nil {
		t.Fatalf("start coordinator: %v", err)
	}
	if startCoord.IsError {
		t.Fatalf("start coordinator returned error: %s", getResultText(startCoord))
	}

	waitForCondition(t, 10*time.Second, func() (bool, string) {
		status := coordSrv.fleetRuntimeStatus(context.Background())
		_, hasNodeStatus := status["node_status"]
		active, _ := status["active"].(bool)
		return active && hasNodeStatus, fmt.Sprintf("coordinator snapshot=%v", status)
	})

	startWorker, err := workerSrv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{
		"action":          "start",
		"mode":            "worker",
		"port":            float64(workerPort),
		"coordinator_url": fmt.Sprintf("http://127.0.0.1:%d", coordPort),
		"automation":      false,
	}))
	if err != nil {
		t.Fatalf("start worker: %v", err)
	}
	if startWorker.IsError {
		t.Fatalf("start worker returned error: %s", getResultText(startWorker))
	}

	waitForCondition(t, 10*time.Second, func() (bool, string) {
		status := workerSrv.fleetRuntimeStatus(context.Background())
		_, hasWorker := status["worker"]
		active, _ := status["active"].(bool)
		return active && hasWorker, fmt.Sprintf("worker snapshot=%v", status)
	})

	stopWorker, err := workerSrv.handleFleetRuntime(context.Background(), makeRequest(map[string]any{
		"action": "stop",
	}))
	if err != nil {
		t.Fatalf("stop worker: %v", err)
	}
	if stopWorker.IsError {
		t.Fatalf("stop worker returned error: %s", getResultText(stopWorker))
	}

	waitForCondition(t, 10*time.Second, func() (bool, string) {
		status := workerSrv.fleetRuntimeStatus(context.Background())
		active, _ := status["active"].(bool)
		return !active, fmt.Sprintf("worker snapshot=%v", status)
	})
}

func TestHandleMarathon_Lifecycle(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")
	srv.ensureEventBus()
	srv.SessMgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			return &session.Session{
				ID:       "mock-marathon-session",
				RepoPath: opts.RepoPath,
				Provider: session.ProviderCodex,
				Status:   session.StatusCompleted,
			}, nil
		},
		func(_ context.Context, _ *session.Session) error {
			return nil
		},
	)
	t.Cleanup(func() {
		_, _ = srv.handleMarathon(context.Background(), makeRequest(map[string]any{"action": "stop"}))
	})

	result, err := srv.handleMarathon(context.Background(), makeRequest(map[string]any{
		"action":              "start",
		"repo":                "test-repo",
		"budget_usd":          5.0,
		"duration":            "350ms",
		"checkpoint_interval": "100ms",
	}))
	if err != nil {
		t.Fatalf("start marathon: %v", err)
	}
	if result.IsError {
		t.Fatalf("start marathon returned error: %s", getResultText(result))
	}

	waitForCondition(t, 10*time.Second, func() (bool, string) {
		status := srv.marathonStatus()
		active, _ := status["active"].(bool)
		checkpointCount, _ := status["checkpoint_count"].(int)
		_, hasFinalStats := status["final_stats"]
		return !active && checkpointCount > 0 && hasFinalStats, fmt.Sprintf("status=%v", status)
	})

	status := srv.marathonStatus()
	if status["repo_path"] != repoPath {
		t.Fatalf("repo_path = %v, want %s", status["repo_path"], repoPath)
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
