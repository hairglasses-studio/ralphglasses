package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// assertErrorCode is a test helper that verifies a CallToolResult is an error
// and contains the expected error_code in its JSON payload.
func assertErrorCode(t *testing.T, handlerName string, result *mcp.CallToolResult, wantCode string) {
	t.Helper()
	if !result.IsError {
		t.Fatalf("%s: expected IsError=true, got false; text=%s", handlerName, getResultText(result))
	}
	text := getResultText(result)
	var payload map[string]string
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		// Some error helpers use plain text (errCode style); just check text contains code.
		if !strings.Contains(text, wantCode) {
			t.Fatalf("%s: expected error_code %q in text, got: %s", handlerName, wantCode, text)
		}
		return
	}
	if got := payload["error_code"]; got != wantCode {
		t.Fatalf("%s: error_code=%q, want %q; full=%s", handlerName, got, wantCode, text)
	}
}

// --- handleRepoHealth ---

func TestHandleRepoHealth_Success(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Write a CLAUDE.md so the health check can inspect it.
	claudeMD := filepath.Join(root, "test-repo", "CLAUDE.md")
	_ = os.WriteFile(claudeMD, []byte("# Test\nSome instructions.\n"), 0644)

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoHealth returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "health_score") {
		t.Errorf("expected health_score in result, got: %s", text)
	}
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected repo name in result, got: %s", text)
	}

	// Parse and verify health_score is a reasonable value.
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal health result: %v", err)
	}
	score, ok := data["health_score"].(float64)
	if !ok {
		t.Fatalf("health_score missing or wrong type: %v", data["health_score"])
	}
	if score < 0 || score > 100 {
		t.Errorf("health_score=%v, expected 0-100", score)
	}
}

func TestHandleRepoHealth_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "INVALID_PARAMS")
}

func TestHandleRepoHealth_NotFound_ErrorCode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "REPO_NOT_FOUND")
}

func TestHandleRepoHealth_InvalidName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "INVALID_PARAMS")
}

// --- handleConfig (codedError paths) ---

func TestHandleConfig_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	assertErrorCode(t, "handleConfig", result, "INVALID_PARAMS")
}

func TestHandleConfig_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "does-not-exist",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	assertErrorCode(t, "handleConfig", result, "REPO_NOT_FOUND")
}

func TestHandleConfig_MissingKeyValue(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"key":  "NONEXISTENT_KEY_XYZ",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	assertErrorCode(t, "handleConfig", result, "CONFIG_INVALID")
}

// --- handleConfigBulk ---

func TestHandleConfigBulk_GetKey(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key": "MODEL",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfigBulk returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "sonnet") {
		t.Errorf("expected MODEL=sonnet in bulk result, got: %s", text)
	}
}

func TestHandleConfigBulk_SetKey(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key":   "MODEL",
		"value": "haiku",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfigBulk returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "updated") {
		t.Errorf("expected 'updated' in bulk set result, got: %s", text)
	}

	// Verify the value was persisted to disk.
	// handleConfigBulk operates on a shallow copy, so check disk directly.
	repo := srv.findRepo("test-repo")
	reloaded, err := model.LoadConfig(filepath.Dir(repo.Config.Path))
	if err != nil {
		t.Fatalf("reload config from disk: %v", err)
	}
	if reloaded.Values["MODEL"] != "haiku" {
		t.Errorf("config not updated on disk, MODEL = %q", reloaded.Values["MODEL"])
	}
}

func TestHandleConfigBulk_MissingKey_ErrorCode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	assertErrorCode(t, "handleConfigBulk", result, "INVALID_PARAMS")
}

func TestHandleConfigBulk_FilteredRepos(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key":   "MODEL",
		"repos": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfigBulk returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected test-repo in filtered result, got: %s", text)
	}
}

func TestHandleConfigBulk_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key": "MODEL",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	assertErrorCode(t, "handleConfigBulk", result, "SCAN_FAILED")
}

// --- handleClaudeMDCheck ---

func TestHandleClaudeMDCheck_Success(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Write a CLAUDE.md file.
	claudeMD := filepath.Join(root, "test-repo", "CLAUDE.md")
	_ = os.WriteFile(claudeMD, []byte("# Project\n\nBuild instructions here.\n"), 0644)

	result, err := srv.handleClaudeMDCheck(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleClaudeMDCheck: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleClaudeMDCheck returned error: %s", getResultText(result))
	}
}

func TestHandleClaudeMDCheck_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleClaudeMDCheck(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleClaudeMDCheck: %v", err)
	}
	assertErrorCode(t, "handleClaudeMDCheck", result, "INVALID_PARAMS")
}

func TestHandleClaudeMDCheck_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleClaudeMDCheck(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleClaudeMDCheck: %v", err)
	}
	assertErrorCode(t, "handleClaudeMDCheck", result, "REPO_NOT_FOUND")
}

// --- handleFleetStatus ---

func TestHandleFleetStatus_Success(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetStatus returned error: %s", getResultText(result))
	}

	text := getResultText(result)

	// Should contain top-level summary keys.
	for _, key := range []string{"summary", "repos", "sessions", "alerts"} {
		if !strings.Contains(text, key) {
			t.Errorf("expected %q in fleet status result, got: %s", key, text)
		}
	}

	// Parse and verify summary fields.
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal fleet status: %v", err)
	}
	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatal("summary field missing or wrong type")
	}
	totalRepos, ok := summary["total_repos"].(float64)
	if !ok || totalRepos < 1 {
		t.Errorf("total_repos=%v, expected >= 1", summary["total_repos"])
	}
}

func TestHandleFleetStatus_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	assertErrorCode(t, "handleFleetStatus", result, "SCAN_FAILED")
}

// --- handleRepoScaffold ---

func TestHandleRepoScaffold_Success(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	newRepo := filepath.Join(root, "new-project")
	if err := os.MkdirAll(newRepo, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
		"path":         newRepo,
		"project_type": "go",
		"project_name": "new-project",
	}))
	if err != nil {
		t.Fatalf("handleRepoScaffold: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoScaffold returned error: %s", getResultText(result))
	}

	// Verify .ralph directory was created.
	if _, err := os.Stat(filepath.Join(newRepo, ".ralph")); os.IsNotExist(err) {
		t.Error("expected .ralph directory to be created")
	}
}

func TestHandleRepoScaffold_MissingPath_ErrorCode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRepoScaffold(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoScaffold: %v", err)
	}
	assertErrorCode(t, "handleRepoScaffold", result, "INVALID_PARAMS")
}

// --- handleRepoOptimize ---

func TestHandleRepoOptimize_Success(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRepoOptimize(context.Background(), makeRequest(map[string]any{
		"path":    repoPath,
		"dry_run": "true",
	}))
	if err != nil {
		t.Fatalf("handleRepoOptimize: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoOptimize returned error: %s", getResultText(result))
	}
}

func TestHandleRepoOptimize_MissingPath_ErrorCode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRepoOptimize(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoOptimize: %v", err)
	}
	assertErrorCode(t, "handleRepoOptimize", result, "INVALID_PARAMS")
}

// --- handlePause (codedError path) ---

func TestHandlePause_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handlePause(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handlePause: %v", err)
	}
	assertErrorCode(t, "handlePause", result, "REPO_NOT_FOUND")
}

// --- handleLogs (codedError paths) ---

func TestHandleLogs_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLogs(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}
	assertErrorCode(t, "handleLogs", result, "INVALID_PARAMS")
}

func TestHandleLogs_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLogs(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}
	assertErrorCode(t, "handleLogs", result, "REPO_NOT_FOUND")
}

func TestHandleLogs_NoLogFile(t *testing.T) {
	t.Parallel()
	// Create a repo without a log file to exercise the ErrNotExist path.
	root := t.TempDir()
	repoPath := filepath.Join(root, "no-log-repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("MODEL=sonnet\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write minimal status.json so the repo is recognized.
	statusData, _ := json.Marshal(model.LoopStatus{Status: "idle"})
	if err := os.WriteFile(filepath.Join(ralphDir, "status.json"), statusData, 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(root)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLogs(context.Background(), makeRequest(map[string]any{
		"repo": "no-log-repo",
	}))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success (empty logs), got error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) {
		t.Errorf("expected status=empty, got: %s", text)
	}
	if !strings.Contains(text, `"item_type":"log_lines"`) {
		t.Errorf("expected item_type=log_lines, got: %s", text)
	}
}

// --- handleStart (codedError paths) ---

func TestHandleStart_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleStart(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handleStart: %v", err)
	}
	assertErrorCode(t, "handleStart", result, "INVALID_PARAMS")
}

func TestHandleStart_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStart(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleStart: %v", err)
	}
	assertErrorCode(t, "handleStart", result, "REPO_NOT_FOUND")
}

func TestHandleStart_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleStart(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleStart: %v", err)
	}
	assertErrorCode(t, "handleStart", result, "SCAN_FAILED")
}

// --- handleStop (codedError paths) ---

func TestHandleStop_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleStop(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handleStop: %v", err)
	}
	assertErrorCode(t, "handleStop", result, "INVALID_PARAMS")
}

func TestHandleStop_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStop(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleStop: %v", err)
	}
	assertErrorCode(t, "handleStop", result, "REPO_NOT_FOUND")
}

func TestHandleStop_NotRunning(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStop(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleStop: %v", err)
	}
	assertErrorCode(t, "handleStop", result, "NOT_RUNNING")
}

func TestHandleStop_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleStop(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleStop: %v", err)
	}
	assertErrorCode(t, "handleStop", result, "SCAN_FAILED")
}

// --- handlePause (additional codedError paths) ---

func TestHandlePause_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handlePause(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handlePause: %v", err)
	}
	assertErrorCode(t, "handlePause", result, "INVALID_PARAMS")
}

func TestHandlePause_NotRunning(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handlePause(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handlePause: %v", err)
	}
	assertErrorCode(t, "handlePause", result, "NOT_RUNNING")
}

func TestHandlePause_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handlePause(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handlePause: %v", err)
	}
	assertErrorCode(t, "handlePause", result, "SCAN_FAILED")
}

// --- handleStatus (additional codedError paths) ---

func TestHandleStatus_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleStatus(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	assertErrorCode(t, "handleStatus", result, "INVALID_PARAMS")
}

// --- handleLogs (additional codedError paths) ---

func TestHandleLogs_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLogs(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}
	assertErrorCode(t, "handleLogs", result, "INVALID_PARAMS")
}

// --- handleConfig (additional codedError paths) ---

func TestHandleConfig_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "../escape",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	assertErrorCode(t, "handleConfig", result, "INVALID_PARAMS")
}

func TestHandleConfig_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	assertErrorCode(t, "handleConfig", result, "SCAN_FAILED")
}

// --- handleScan with EventBus ---

func TestHandleScan_WithEventBus(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	bus := events.NewBus(100)
	srv.EventBus = bus

	result, err := srv.handleScan(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleScan: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleScan returned error: %s", getResultText(result))
	}

	// Verify event was published
	history := bus.History("", 10)
	found := false
	for _, e := range history {
		if e.Type == events.ScanComplete {
			found = true
			if count, ok := e.Data["repo_count"]; ok {
				if c, ok := count.(int); !ok || c < 1 {
					t.Errorf("expected repo_count >= 1, got: %v", count)
				}
			}
		}
	}
	if !found {
		t.Error("expected ScanComplete event in bus history")
	}
}

// --- handleConfigBulk with EventBus ---

func TestHandleConfigBulk_SetWithEventBus(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	bus := events.NewBus(100)
	srv.EventBus = bus
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key":   "MODEL",
		"value": "opus",
		"repos": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfigBulk returned error: %s", getResultText(result))
	}

	// Verify event was published
	history := bus.History("", 10)
	found := false
	for _, e := range history {
		if e.Type == events.ConfigChanged {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ConfigChanged event in bus history")
	}
}

// --- handleConfigBulk: repo with no config ---

func TestHandleConfigBulk_RepoWithoutConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repoPath := filepath.Join(root, "no-config-repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Write minimal status.json but no .ralphrc
	statusData, _ := json.Marshal(model.LoopStatus{Status: "idle"})
	if err := os.WriteFile(filepath.Join(ralphDir, "status.json"), statusData, 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(root)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key": "MODEL",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfigBulk returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "no .ralphrc") {
		t.Errorf("expected 'no .ralphrc' in result, got: %s", text)
	}
}

// --- handleList_ScanError ---

func TestHandleList_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleList: %v", err)
	}
	assertErrorCode(t, "handleList", result, "SCAN_FAILED")
}

// --- handleStatus with full detail ---

func TestHandleStatus_FullDetail(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStatus(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleStatus returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	// Verify detailed fields are present
	for _, key := range []string{"name", "path", "managed", "status", "circuit_breaker", "progress", "config"} {
		if !strings.Contains(text, key) {
			t.Errorf("expected %q in status detail, got: %s", key, text)
		}
	}
}
