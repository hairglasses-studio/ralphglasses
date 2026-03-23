package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()

	// Create a ralph-enabled repo
	repoPath := filepath.Join(root, "test-repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	logsDir := filepath.Join(ralphDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write go.mod for project type detection
	if err := os.WriteFile(filepath.Join(repoPath, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write ROADMAP.md
	roadmapContent := `# Test Repo Roadmap

## Phase 0: Setup (COMPLETE)

- [x] Initialize project
- [x] Add core module

## Phase 1: Features

### 1.1 — Parser
- [ ] 1.1.1 — Build parser
- [ ] 1.1.2 — Add tests [BLOCKED BY 1.1.1]
- **Acceptance:** parser works

### 1.2 — Export
- [ ] 1.2.1 — Export to JSON
`
	if err := os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte(roadmapContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Write .ralphrc
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("MODEL=sonnet\nBUDGET=5\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write status.json
	status := model.LoopStatus{
		LoopCount:       10,
		Status:          "running",
		CallsMadeThisHr: 5,
		MaxCallsPerHour: 100,
		LastAction:      "edit_file",
	}
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(ralphDir, "status.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write circuit breaker state
	cb := model.CircuitBreakerState{
		State:      "CLOSED",
		TotalOpens: 0,
	}
	data, _ = json.Marshal(cb)
	if err := os.WriteFile(filepath.Join(ralphDir, ".circuit_breaker_state"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write progress.json
	prog := model.Progress{
		Iteration:    3,
		Status:       "in_progress",
		CompletedIDs: []string{"task-1"},
	}
	data, _ = json.Marshal(prog)
	if err := os.WriteFile(filepath.Join(ralphDir, "progress.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	// Write log file
	if err := os.WriteFile(filepath.Join(logsDir, "ralph.log"), []byte("log line 1\nlog line 2\nlog line 3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(root)
	srv.SessMgr.SetStateDir(filepath.Join(root, ".session-state"))
	initGitRepo(t, repoPath)
	return srv, root
}

func initGitRepo(t *testing.T, repoPath string) {
	t.Helper()
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "commit.gpgsign", "false")
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial")
}

func runGit(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func makeRequest(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func TestNewServer(t *testing.T) {
	srv := NewServer("/tmp/test")
	if srv.ScanPath != "/tmp/test" {
		t.Errorf("ScanPath = %q, want %q", srv.ScanPath, "/tmp/test")
	}
	if srv.ProcMgr == nil {
		t.Fatal("ProcMgr should not be nil")
	}
}

func TestHandleScan(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleScan(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleScan: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleScan returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "1 ralph-enabled repos") {
		t.Errorf("expected to find 1 repo, got: %s", text)
	}
}

func TestHandleList(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Scan first
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleList: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleList returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected list to contain test-repo, got: %s", text)
	}
}

func TestHandleList_AutoScans(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Don't scan first -- handleList should auto-scan
	result, err := srv.handleList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleList: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleList returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected auto-scan to find test-repo, got: %s", text)
	}
}

func TestHandleStatus(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

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
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected status to mention test-repo, got: %s", text)
	}
}

func TestHandleStatus_MissingRepoArg(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStatus_UnknownRepo(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStatus(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for unknown repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", text)
	}
}

func TestHandleLogs(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLogs(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleLogs returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "log line 1") {
		t.Errorf("expected logs to contain 'log line 1', got: %s", text)
	}
}

func TestHandleLogs_MaxLines(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLogs(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"lines": float64(2),
	}))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}

	text := getResultText(result)
	lines := strings.Split(text, "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d: %q", len(lines), text)
	}
}

func TestHandleConfig_ListAll(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfig returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "MODEL") {
		t.Errorf("expected config to contain MODEL, got: %s", text)
	}
}

func TestHandleConfig_GetKey(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"key":  "MODEL",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfig returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if text != "MODEL=sonnet" {
		t.Errorf("expected MODEL=sonnet, got: %s", text)
	}
}

func TestHandleConfig_SetKey(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"key":   "MODEL",
		"value": "opus",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfig returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "Set MODEL=opus") {
		t.Errorf("expected Set confirmation, got: %s", text)
	}

	// Verify the value was persisted
	repo := srv.findRepo("test-repo")
	if repo.Config.Values["MODEL"] != "opus" {
		t.Errorf("config not updated, MODEL = %q", repo.Config.Values["MODEL"])
	}
}

func TestHandleConfig_MissingKey(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfig(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"key":  "NONEXISTENT",
	}))
	if err != nil {
		t.Fatalf("handleConfig: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent key")
	}
}

func TestHandleStart_MissingRepo(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStart(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStart: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStop_MissingRepo(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStop(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStop: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStopAll(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleStopAll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStopAll: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleStopAll returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "stopped") {
		t.Errorf("expected confirmation, got: %s", text)
	}
}

func TestHandlePause_MissingRepo(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handlePause(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handlePause: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStatus_ScanError(t *testing.T) {
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleStatus(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails")
	}
	text := getResultText(result)
	if !strings.Contains(text, "scan failed") {
		t.Errorf("expected 'scan failed' in error, got: %s", text)
	}
}

func TestHandleLogs_ScanError(t *testing.T) {
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleLogs(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleLogs: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails")
	}
	text := getResultText(result)
	if !strings.Contains(text, "scan failed") {
		t.Errorf("expected 'scan failed' in error, got: %s", text)
	}
}

func TestFindRepo(t *testing.T) {
	srv := NewServer("/tmp")
	srv.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
		{Name: "beta", Path: "/tmp/beta"},
	}

	if r := srv.findRepo("alpha"); r == nil {
		t.Error("findRepo should find alpha")
	}
	if r := srv.findRepo("beta"); r == nil {
		t.Error("findRepo should find beta")
	}
	if r := srv.findRepo("gamma"); r != nil {
		t.Error("findRepo should return nil for unknown repo")
	}
}

func TestGetStringArg(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		key  string
		want string
	}{
		{"present", map[string]any{"repo": "test"}, "repo", "test"},
		{"missing", map[string]any{}, "repo", ""},
		{"nil args", nil, "repo", ""},
		{"wrong type", map[string]any{"repo": 42}, "repo", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: tt.args,
				},
			}
			got := getStringArg(req, tt.key)
			if got != tt.want {
				t.Errorf("getStringArg(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestGetNumberArg(t *testing.T) {
	tests := []struct {
		name       string
		args       map[string]any
		key        string
		defaultVal float64
		want       float64
	}{
		{"present", map[string]any{"lines": float64(100)}, "lines", 50, 100},
		{"missing", map[string]any{}, "lines", 50, 50},
		{"nil args", nil, "lines", 50, 50},
		{"wrong type", map[string]any{"lines": "hundred"}, "lines", 50, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{
				Params: mcp.CallToolParams{
					Arguments: tt.args,
				},
			}
			got := getNumberArg(req, tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("getNumberArg(%q) = %f, want %f", tt.key, got, tt.want)
			}
		})
	}
}

func TestTextResult(t *testing.T) {
	r := textResult("hello")
	if r.IsError {
		t.Error("textResult should not be an error")
	}
	text := getResultText(r)
	if text != "hello" {
		t.Errorf("text = %q, want %q", text, "hello")
	}
}

func TestErrResult(t *testing.T) {
	r := errResult("something failed")
	if !r.IsError {
		t.Error("errResult should be an error")
	}
	text := getResultText(r)
	if text != "something failed" {
		t.Errorf("text = %q, want %q", text, "something failed")
	}
}

func TestJsonResult(t *testing.T) {
	data := map[string]string{"key": "value"}
	r := jsonResult(data)
	if r.IsError {
		t.Error("jsonResult should not be an error")
	}
	text := getResultText(r)
	if !strings.Contains(text, "key") || !strings.Contains(text, "value") {
		t.Errorf("json result missing expected content: %s", text)
	}
}

// Roadmap tool tests

func TestHandleRoadmapParse(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRoadmapParse(context.Background(), makeRequest(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("handleRoadmapParse: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRoadmapParse returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "Test Repo Roadmap") {
		t.Errorf("expected title in output, got: %s", text)
	}
	if !strings.Contains(text, "phases") {
		t.Errorf("expected phases in output, got: %s", text)
	}
}

func TestHandleRoadmapParse_MissingPath(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleRoadmapParse(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRoadmapParse: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing path")
	}
}

func TestHandleRoadmapAnalyze(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRoadmapAnalyze(context.Background(), makeRequest(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("handleRoadmapAnalyze: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRoadmapAnalyze returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "gaps") {
		t.Errorf("expected gaps in output, got: %s", text)
	}
	if !strings.Contains(text, "summary") {
		t.Errorf("expected summary in output, got: %s", text)
	}
}

func TestHandleRoadmapExport_RDCycle(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRoadmapExport(context.Background(), makeRequest(map[string]any{
		"path":   repoPath,
		"format": "rdcycle",
	}))
	if err != nil {
		t.Fatalf("handleRoadmapExport: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRoadmapExport returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "tasks") {
		t.Errorf("expected tasks in rdcycle output, got: %s", text)
	}
}

func TestHandleRoadmapExport_FixPlan(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRoadmapExport(context.Background(), makeRequest(map[string]any{
		"path":   repoPath,
		"format": "fix_plan",
	}))
	if err != nil {
		t.Fatalf("handleRoadmapExport fix_plan: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRoadmapExport returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "- [ ]") {
		t.Errorf("expected checkbox items in fix_plan output, got: %s", text)
	}
}

func TestHandleRoadmapExpand(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRoadmapExpand(context.Background(), makeRequest(map[string]any{
		"path":  repoPath,
		"style": "conservative",
	}))
	if err != nil {
		t.Fatalf("handleRoadmapExpand: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRoadmapExpand returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "proposals") {
		t.Errorf("expected proposals in output, got: %s", text)
	}
}

// Repo file tool tests

func TestHandleRepoScaffold(t *testing.T) {
	_, root := setupTestServer(t)
	newRepoPath := filepath.Join(root, "new-repo")
	os.MkdirAll(newRepoPath, 0755)
	os.WriteFile(filepath.Join(newRepoPath, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644)

	srv := NewServer(root)
	result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
		"path": newRepoPath,
	}))
	if err != nil {
		t.Fatalf("handleRepoScaffold: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoScaffold returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "created") {
		t.Errorf("expected created files in output, got: %s", text)
	}

	// Verify files were actually created
	if _, err := os.Stat(filepath.Join(newRepoPath, ".ralphrc")); err != nil {
		t.Error("expected .ralphrc to be created")
	}
	if _, err := os.Stat(filepath.Join(newRepoPath, ".ralph", "PROMPT.md")); err != nil {
		t.Error("expected .ralph/PROMPT.md to be created")
	}
}

func TestHandleRepoOptimize(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	result, err := srv.handleRepoOptimize(context.Background(), makeRequest(map[string]any{
		"path": repoPath,
	}))
	if err != nil {
		t.Fatalf("handleRepoOptimize: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoOptimize returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "issues") {
		t.Errorf("expected issues in output, got: %s", text)
	}
	_ = srv
}

func TestHandleRepoScaffold_MissingPath(t *testing.T) {
	srv := NewServer("/tmp")

	result, err := srv.handleRepoScaffold(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoScaffold: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing path")
	}
}

func TestHandleRepoOptimize_MissingPath(t *testing.T) {
	srv := NewServer("/tmp")

	result, err := srv.handleRepoOptimize(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoOptimize: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing path")
	}
}

// Helper to extract text from a CallToolResult
func getResultText(r *mcp.CallToolResult) string {
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

// --- Session & Team handler tests ---

func TestHandleSessionList_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleSessionList: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionList returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "[]") {
		t.Errorf("expected empty array, got: %s", text)
	}
}

func TestHandleSessionStatus_Missing(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionStatus(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleSessionStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleSessionStatus_MissingID(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleSessionStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing id")
	}
}

func TestHandleSessionOutput_Missing(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionOutput(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleSessionOutput: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleSessionCompare_MissingArgs(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{
		"id1": "a",
	}))
	if err != nil {
		t.Fatalf("handleSessionCompare: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing id2")
	}
}

func TestHandleSessionCompare_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{
		"id1": "a",
		"id2": "b",
	}))
	if err != nil {
		t.Fatalf("handleSessionCompare: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleSessionRetry_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionRetry(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleSessionRetry: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleSessionRetry_MissingID(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionRetry(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleSessionRetry: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing id")
	}
}

func TestHandleSessionBudget_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionBudget(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleSessionBudget: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleSessionStop_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionStop(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleSessionStop: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

// --- Team handler tests ---

func TestHandleTeamStatus_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleTeamStatus(context.Background(), makeRequest(map[string]any{
		"name": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleTeamStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent team")
	}
}

func TestHandleTeamStatus_MissingName(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleTeamStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleTeamStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestHandleTeamDelegate_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleTeamDelegate(context.Background(), makeRequest(map[string]any{
		"name": "nonexistent",
		"task": "do stuff",
	}))
	if err != nil {
		t.Fatalf("handleTeamDelegate: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent team")
	}
}

func TestHandleTeamDelegate_MissingArgs(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleTeamDelegate(context.Background(), makeRequest(map[string]any{
		"name": "team1",
	}))
	if err != nil {
		t.Fatalf("handleTeamDelegate: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing task")
	}
}

// --- Agent handler tests ---

func TestHandleAgentDefine_And_List(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	// Define an agent
	result, err := srv.handleAgentDefine(context.Background(), makeRequest(map[string]any{
		"repo":        "test-repo",
		"name":        "test-agent",
		"prompt":      "You are a test agent",
		"description": "Test agent for unit tests",
		"model":       "sonnet",
	}))
	if err != nil {
		t.Fatalf("handleAgentDefine: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleAgentDefine returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "test-agent") {
		t.Errorf("expected agent name in output, got: %s", text)
	}

	// List agents
	result, err = srv.handleAgentList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleAgentList: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleAgentList returned error: %s", getResultText(result))
	}

	text = getResultText(result)
	if !strings.Contains(text, "test-agent") {
		t.Errorf("expected agent in list, got: %s", text)
	}
}

func TestHandleAgentList_AllProviders(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleAgentList(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"provider": "all",
	}))
	if err != nil {
		t.Fatalf("handleAgentList all: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleAgentList returned error: %s", getResultText(result))
	}
}

func TestHandleAgentDefine_MissingArgs(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleAgentDefine(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleAgentDefine: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing agent name")
	}
}

// --- Event Bus handler tests ---

func TestHandleEventList_NoBus(t *testing.T) {
	srv, _ := setupTestServer(t)
	// Default setupTestServer creates srv without event bus

	result, err := srv.handleEventList(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleEventList: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when event bus not initialized")
	}
}

func TestHandleEventList_WithBus(t *testing.T) {
	root := t.TempDir()
	repoPath := filepath.Join(root, "test-repo")
	os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	bus := events.NewBus(100)
	srv := NewServerWithBus(root, bus)

	// Publish a test event
	bus.Publish(events.Event{
		Type:     events.ScanComplete,
		RepoName: "test-repo",
		Data:     map[string]any{"repo_count": 1},
	})

	result, err := srv.handleEventList(context.Background(), makeRequest(map[string]any{
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("handleEventList: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleEventList returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "scan.complete") {
		t.Errorf("expected scan.complete event, got: %s", text)
	}
}

// --- Fleet Analytics handler tests ---

func TestHandleFleetAnalytics_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetAnalytics: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetAnalytics returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected total_sessions in output, got: %s", text)
	}
}

// --- Config Bulk handler tests ---

func TestHandleConfigBulk_Query(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

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
		t.Errorf("expected sonnet value, got: %s", text)
	}
}

func TestHandleConfigBulk_Set(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(map[string]any{
		"key":   "MODEL",
		"value": "opus",
	}))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleConfigBulk returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "updated") {
		t.Errorf("expected updated confirmation, got: %s", text)
	}
}

func TestHandleConfigBulk_MissingKey(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleConfigBulk(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleConfigBulk: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing key")
	}
}

// --- Repo Health handler tests ---

func TestHandleRepoHealth(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

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
		t.Errorf("expected health_score in output, got: %s", text)
	}
}

func TestHandleRepoHealth_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent repo")
	}
}

// --- Workflow handler tests ---

func TestHandleWorkflowDefine(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	yamlStr := `name: test-workflow
steps:
  - name: step1
    prompt: "do thing 1"
  - name: step2
    prompt: "do thing 2"
    depends_on: [step1]
`
	result, err := srv.handleWorkflowDefine(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "test-workflow",
		"yaml": yamlStr,
	}))
	if err != nil {
		t.Fatalf("handleWorkflowDefine: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleWorkflowDefine returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "test-workflow") {
		t.Errorf("expected workflow name in output, got: %s", text)
	}
	if !strings.Contains(text, `"steps": 2`) {
		t.Errorf("expected 2 steps, got: %s", text)
	}
}

func TestHandleWorkflowDefine_MissingArgs(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleWorkflowDefine(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowDefine: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing yaml")
	}
}

func TestHandleWorkflowRun_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleWorkflowRun(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowRun: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent workflow")
	}
}

func TestHandleWorkflowRun_MissingArgs(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleWorkflowRun(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowRun: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

// --- Snapshot handler tests ---

func TestHandleSnapshot_Save(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleSnapshot(context.Background(), makeRequest(map[string]any{
		"name": "test-snap",
	}))
	if err != nil {
		t.Fatalf("handleSnapshot: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSnapshot returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "test-snap") {
		t.Errorf("expected snapshot name, got: %s", text)
	}
}

func TestHandleSnapshot_List(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	// Save one first
	srv.handleSnapshot(context.Background(), makeRequest(map[string]any{
		"name": "test-snap-list",
	}))

	result, err := srv.handleSnapshot(context.Background(), makeRequest(map[string]any{
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("handleSnapshot list: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSnapshot list returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "snapshots") {
		t.Errorf("expected snapshots key, got: %s", text)
	}
}

// --- Agent Compose handler tests ---

func TestHandleAgentCompose(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	// Create two agents first
	srv.handleAgentDefine(context.Background(), makeRequest(map[string]any{
		"repo":        "test-repo",
		"name":        "agent-a",
		"prompt":      "You handle testing",
		"description": "Test runner",
		"tools":       "Bash,Read",
	}))
	srv.handleAgentDefine(context.Background(), makeRequest(map[string]any{
		"repo":        "test-repo",
		"name":        "agent-b",
		"prompt":      "You handle documentation",
		"description": "Doc writer",
		"tools":       "Read,Write",
	}))

	// Compose them
	result, err := srv.handleAgentCompose(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"name":   "composite-agent",
		"agents": "agent-a,agent-b",
	}))
	if err != nil {
		t.Fatalf("handleAgentCompose: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleAgentCompose returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "composite-agent") {
		t.Errorf("expected composite-agent name, got: %s", text)
	}
	if !strings.Contains(text, "agent-a") {
		t.Errorf("expected agent-a in composed list, got: %s", text)
	}

	// Verify the composite agent can be listed
	result, err = srv.handleAgentList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleAgentList: %v", err)
	}
	text = getResultText(result)
	if !strings.Contains(text, "composite-agent") {
		t.Errorf("expected composite-agent in agent list, got: %s", text)
	}
}

func TestHandleAgentCompose_MissingArgs(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleAgentCompose(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "composite",
	}))
	if err != nil {
		t.Fatalf("handleAgentCompose: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing agents")
	}
}

func TestHandleAgentCompose_AgentNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleAgentCompose(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"name":   "composite",
		"agents": "nonexistent-agent",
	}))
	if err != nil {
		t.Fatalf("handleAgentCompose: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent agent")
	}
}

// --- Session Stop All handler tests ---

func TestHandleSessionStopAll_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionStopAll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleSessionStopAll: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionStopAll returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "Stopped 0") {
		t.Errorf("expected 0 sessions stopped, got: %s", text)
	}
}

// --- Fleet Status handler tests ---

func TestHandleFleetStatus(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetStatus returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "summary") {
		t.Errorf("expected summary in output, got: %s", text)
	}
	if !strings.Contains(text, "repos") {
		t.Errorf("expected repos in output, got: %s", text)
	}
}

// --- Journal handler tests ---

func TestHandleJournalRead(t *testing.T) {
	srv, root := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))
	repoPath := filepath.Join(root, "test-repo")

	// Write fixture entries
	for i := 0; i < 3; i++ {
		entry := session.JournalEntry{
			SessionID: "test-sess",
			RepoName:  "test-repo",
			Worked:    []string{"Good pattern"},
			Failed:    []string{"Bad pattern"},
			Suggest:   []string{"Try this"},
		}
		session.WriteJournalEntryManual(repoPath, entry)
	}

	result, err := srv.handleJournalRead(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("handleJournalRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleJournalRead returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "synthesis") {
		t.Errorf("expected synthesis in output, got: %s", text)
	}
	if !strings.Contains(text, `"count": 3`) {
		t.Errorf("expected count 3, got: %s", text)
	}
}

func TestHandleJournalRead_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleJournalRead(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleJournalRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleJournalRead returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, `"count": 0`) {
		t.Errorf("expected count 0, got: %s", text)
	}
}

func TestHandleJournalWrite(t *testing.T) {
	srv, root := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleJournalWrite(context.Background(), makeRequest(map[string]any{
		"repo":    "test-repo",
		"worked":  "Fast builds, Clean tests",
		"failed":  "Forgot vet",
		"suggest": "Run vet first",
	}))
	if err != nil {
		t.Fatalf("handleJournalWrite: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleJournalWrite returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, `"status": "written"`) {
		t.Errorf("expected written status, got: %s", text)
	}

	// Read back
	repoPath := filepath.Join(root, "test-repo")
	entries, err := session.ReadRecentJournal(repoPath, 10)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Worked) != 2 {
		t.Errorf("expected 2 worked items, got %d", len(entries[0].Worked))
	}
}

func TestHandleJournalPrune_DryRun(t *testing.T) {
	srv, root := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))
	repoPath := filepath.Join(root, "test-repo")

	// Write some entries
	for i := 0; i < 5; i++ {
		session.WriteJournalEntryManual(repoPath, session.JournalEntry{
			SessionID: "s",
			RepoName:  "test-repo",
		})
	}

	result, err := srv.handleJournalPrune(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"keep": float64(3),
	}))
	if err != nil {
		t.Fatalf("handleJournalPrune: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleJournalPrune returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, `"dry_run": true`) {
		t.Errorf("expected dry_run true, got: %s", text)
	}
	if !strings.Contains(text, `"would_prune": 2`) {
		t.Errorf("expected would_prune 2, got: %s", text)
	}

	// Verify no entries were actually removed
	entries, _ := session.ReadRecentJournal(repoPath, 100)
	if len(entries) != 5 {
		t.Errorf("dry run should not modify, got %d entries", len(entries))
	}
}

func TestHandleLoopLifecycle(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))

	srv.SessMgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			sess := &session.Session{
				ID:         strings.ReplaceAll(opts.SessionName, " ", "-"),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     session.StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			if opts.Model == "o1-pro" {
				sess.LastOutput = `{"title":"Tighten provider docs","prompt":"Update provider docs and tests to match actual codex resume behavior."}`
				sess.OutputHistory = []string{sess.LastOutput}
			} else {
				sess.LastOutput = "worker done"
				sess.OutputHistory = []string{"worker done"}
			}
			return sess, nil
		},
		func(_ context.Context, sess *session.Session) error {
			sess.Lock()
			sess.Status = session.StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	startResult, err := srv.handleLoopStart(context.Background(), makeRequest(map[string]any{
		"repo":            "test-repo",
		"verify_commands": "test -f go.mod",
	}))
	if err != nil {
		t.Fatalf("handleLoopStart: %v", err)
	}
	if startResult.IsError {
		t.Fatalf("handleLoopStart returned error: %s", getResultText(startResult))
	}

	var started map[string]any
	if err := json.Unmarshal([]byte(getResultText(startResult)), &started); err != nil {
		t.Fatalf("unmarshal loop start: %v", err)
	}
	id, _ := started["id"].(string)
	if id == "" {
		t.Fatal("expected loop id")
	}

	stepResult, err := srv.handleLoopStep(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleLoopStep: %v", err)
	}
	if stepResult.IsError {
		t.Fatalf("handleLoopStep returned error: %s", getResultText(stepResult))
	}

	var stepped map[string]any
	if err := json.Unmarshal([]byte(getResultText(stepResult)), &stepped); err != nil {
		t.Fatalf("unmarshal loop step: %v", err)
	}
	if stepped["status"] != "idle" {
		t.Fatalf("loop status = %v, want idle", stepped["status"])
	}

	iterations, _ := stepped["iterations"].([]any)
	if len(iterations) != 1 {
		t.Fatalf("iterations = %d, want 1", len(iterations))
	}

	iter, _ := iterations[0].(map[string]any)
	if iter["status"] != "idle" {
		t.Fatalf("iteration status = %v, want idle", iter["status"])
	}
	if iter["worktree_path"] == "" {
		t.Fatal("expected worktree path")
	}
	task, _ := iter["task"].(map[string]any)
	if task["title"] != "Tighten provider docs" {
		t.Fatalf("task title = %v", task["title"])
	}

	statusResult, err := srv.handleLoopStatus(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleLoopStatus: %v", err)
	}
	if statusResult.IsError {
		t.Fatalf("handleLoopStatus returned error: %s", getResultText(statusResult))
	}

	stopResult, err := srv.handleLoopStop(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleLoopStop: %v", err)
	}
	if stopResult.IsError {
		t.Fatalf("handleLoopStop returned error: %s", getResultText(stopResult))
	}
}

// --- Marathon monitoring tests ---

// injectTestSession creates a fake session and inserts it directly into the manager.
func injectTestSession(t *testing.T, srv *Server, repoPath string, mods func(*session.Session)) string {
	t.Helper()
	now := time.Now()
	id := fmt.Sprintf("test-%d", now.UnixNano())
	sess := &session.Session{
		ID:           id,
		Provider:     session.ProviderClaude,
		RepoPath:     repoPath,
		RepoName:     filepath.Base(repoPath),
		Prompt:       "test prompt",
		Model:        "sonnet",
		Status:       session.StatusRunning,
		LaunchedAt:   now,
		LastActivity: now,
		OutputCh:     make(chan string, 1),
	}
	if mods != nil {
		mods(sess)
	}
	srv.SessMgr.AddSessionForTesting(sess)
	return sess.ID
}

func TestHandleSessionTail(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"line1", "line2", "line3", "line4", "line5"}
		s.TotalOutputCount = 15 // 15 total ever, but only last 5 in history
	})

	// Test: no cursor, default lines
	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionTail returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "line1") || !strings.Contains(text, "line5") {
		t.Errorf("expected all lines, got: %s", text)
	}
	if !strings.Contains(text, `"next_cursor": "15"`) {
		t.Errorf("expected next_cursor 15, got: %s", text)
	}
}

func TestHandleSessionTailNoCursor(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"a", "b", "c", "d", "e", "f", "g"}
		s.TotalOutputCount = 7
	})

	// Request only last 3 lines
	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id":    id,
		"lines": float64(3),
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"lines_returned": 3`) {
		t.Errorf("expected 3 lines returned, got: %s", text)
	}
	// Should contain e, f, g but not a, b
	if strings.Contains(text, `"a"`) {
		t.Errorf("should not contain early lines, got: %s", text)
	}
}

func TestHandleSessionTailWithCursor(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"line1", "line2", "line3", "line4", "line5"}
		s.TotalOutputCount = 5
	})

	// Cursor at 3 means "give me everything since output #3"
	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id":     id,
		"cursor": "3",
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"lines_returned": 2`) {
		t.Errorf("expected 2 new lines, got: %s", text)
	}
	if !strings.Contains(text, "line4") || !strings.Contains(text, "line5") {
		t.Errorf("expected line4 and line5, got: %s", text)
	}
}

func TestHandleSessionTail_NotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestHandleSessionDiffNoRepo(t *testing.T) {
	srv, _ := setupTestServer(t)

	id := injectTestSession(t, srv, "/nonexistent/path", func(s *session.Session) {
		s.RepoPath = "/nonexistent/path"
	})

	result, err := srv.handleSessionDiff(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleSessionDiff: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent repo path")
	}
}

func TestHandleSessionDiff(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.LaunchedAt = time.Now().Add(-1 * time.Hour)
	})

	result, err := srv.handleSessionDiff(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleSessionDiff: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionDiff returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "window") {
		t.Errorf("expected window in response, got: %s", text)
	}
	if !strings.Contains(text, "stat") {
		t.Errorf("expected stat in response, got: %s", text)
	}
}

func TestHandleMarathonDashboardEmpty(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handleMarathonDashboard(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleMarathonDashboard: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleMarathonDashboard returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"total": 0`) {
		t.Errorf("expected 0 total sessions, got: %s", text)
	}
	if !strings.Contains(text, `"total_usd": 0`) {
		t.Errorf("expected 0 total cost, got: %s", text)
	}
}

func TestHandleMarathonDashboardStale(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.LastActivity = time.Now().Add(-10 * time.Minute) // 10 min idle
		s.Status = session.StatusRunning
		s.SpentUSD = 1.50
	})

	result, err := srv.handleMarathonDashboard(context.Background(), makeRequest(map[string]any{
		"stale_threshold_min": float64(5),
	}))
	if err != nil {
		t.Fatalf("handleMarathonDashboard: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"stale": 1`) {
		t.Errorf("expected 1 stale session, got: %s", text)
	}
	if !strings.Contains(text, "stale_session") {
		t.Errorf("expected stale_session alert, got: %s", text)
	}
}

func TestHandleSessionErrorsClassification(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	// Errored session (critical)
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "API rate limit exceeded"
	})

	// Session with parse errors (warning)
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusRunning
		s.StreamParseErrors = 3
	})

	// Stopped session (info)
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusStopped
		s.ExitReason = "stopped by user"
	})

	result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleSessionErrors: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionErrors returned error: %s", getResultText(result))
	}
	text := getResultText(result)

	if !strings.Contains(text, "session_error") {
		t.Errorf("expected session_error type, got: %s", text)
	}
	if !strings.Contains(text, "stream_parse") {
		t.Errorf("expected stream_parse type, got: %s", text)
	}
	if !strings.Contains(text, "session_stopped") {
		t.Errorf("expected session_stopped type, got: %s", text)
	}
	if !strings.Contains(text, `"critical"`) {
		t.Errorf("expected critical severity, got: %s", text)
	}
}

func TestHandleSessionErrors_SeverityFilter(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "critical error"
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusStopped
		s.ExitReason = "stopped"
	})

	result, err := srv.handleSessionErrors(context.Background(), makeRequest(map[string]any{
		"severity": "critical",
	}))
	if err != nil {
		t.Fatalf("handleSessionErrors: %v", err)
	}
	text := getResultText(result)
	// The errors array should only contain critical entries
	if !strings.Contains(text, `"total_errors": 1`) {
		t.Errorf("expected 1 error after filter, got: %s", text)
	}
	// The filtered errors should all be critical
	if !strings.Contains(text, `"session_error"`) {
		t.Errorf("expected session_error in filtered results, got: %s", text)
	}
}

// --- Remote Control (RC) handler tests ---

func TestRCStatus_Empty(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "0 running") {
		t.Errorf("expected 0 running, got: %s", text)
	}
	if !strings.Contains(text, "No active or recent sessions") {
		t.Errorf("expected no sessions message, got: %s", text)
	}
}

func TestRCStatus_WithSessions(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.SpentUSD = 1.23
		s.TurnCount = 15
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderGemini
		s.SpentUSD = 2.22
		s.TurnCount = 8
	})

	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "2 running") {
		t.Errorf("expected 2 running, got: %s", text)
	}
	if !strings.Contains(text, "$3.45") {
		t.Errorf("expected total $3.45, got: %s", text)
	}
}

func TestRCSend_MissingRepo(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"prompt": "test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing repo")
	}
}

func TestRCSend_MissingPrompt(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing prompt")
	}
}

func TestRCSend_RepoNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)
	srv.handleScan(context.Background(), makeRequest(nil))
	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "nonexistent",
		"prompt": "test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for nonexistent repo")
	}
	if !strings.Contains(getResultText(result), "not found") {
		t.Errorf("expected not found message, got: %s", getResultText(result))
	}
}

func TestRCRead_NoSessions(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCRead(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "No sessions") {
		t.Errorf("expected no sessions, got: %s", text)
	}
}

func TestRCRead_MostActive(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"line1", "line2", "line3"}
		s.TotalOutputCount = 3
		s.SpentUSD = 1.50
		s.TurnCount = 10
	})

	result, err := srv.handleRCRead(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "line1") {
		t.Errorf("expected output lines, got: %s", text)
	}
	if !strings.Contains(text, "$1.50") {
		t.Errorf("expected cost, got: %s", text)
	}
	if !strings.Contains(text, "cursor:3") {
		t.Errorf("expected cursor, got: %s", text)
	}
}

func TestRCRead_WithCursor(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"old1", "old2", "new1", "new2"}
		s.TotalOutputCount = 10
	})

	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"cursor": "8", // 10 - 8 = 2 new lines
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "new1") {
		t.Errorf("expected new1, got: %s", text)
	}
	if !strings.Contains(text, "new2") {
		t.Errorf("expected new2, got: %s", text)
	}
	if strings.Contains(text, "old1") {
		t.Errorf("should not contain old lines, got: %s", text)
	}
}

func TestEventPoll_NoBus(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleEventPoll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error when bus is nil")
	}
}

func TestEventPoll_WithEvents(t *testing.T) {
	bus := events.NewBus(100)
	srv, _ := setupTestServer(t)
	srv.EventBus = bus

	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "repo1", SessionID: "abc123"})
	bus.Publish(events.Event{Type: events.CostUpdate, RepoName: "repo1", Data: map[string]any{"cost_usd": 1.5}})

	result, err := srv.handleEventPoll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"count": 2`) {
		t.Errorf("expected 2 events, got: %s", text)
	}
	if !strings.Contains(text, "cursor") {
		t.Errorf("expected cursor in response, got: %s", text)
	}
}

func TestEventPoll_CursorAdvance(t *testing.T) {
	bus := events.NewBus(100)
	srv, _ := setupTestServer(t)
	srv.EventBus = bus

	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "r1"})

	result, _ := srv.handleEventPoll(context.Background(), makeRequest(nil))
	text := getResultText(result)
	// Extract cursor value
	if !strings.Contains(text, `"cursor": "1"`) {
		t.Errorf("expected cursor 1, got: %s", text)
	}

	// Publish more events
	bus.Publish(events.Event{Type: events.SessionEnded, RepoName: "r2"})

	// Poll with cursor
	result, _ = srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "1",
	}))
	text = getResultText(result)
	if !strings.Contains(text, `"count": 1`) {
		t.Errorf("expected 1 new event, got: %s", text)
	}
}

func TestRCAct_UnknownAction(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "explode",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for unknown action")
	}
}

func TestRCAct_StopAll(t *testing.T) {
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, nil)
	injectTestSession(t, srv, repoPath, nil)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop_all",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "Stopped 2 session(s)") {
		t.Errorf("expected stopped 2, got: %s", text)
	}
}

func TestRCAct_StopMissing(t *testing.T) {
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop",
		"target": "nonexistent",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing target")
	}
}
