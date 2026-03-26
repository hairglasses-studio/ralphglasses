package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

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
	assertErrorCode(t, "handleRepoHealth", result, "invalid_params")
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
