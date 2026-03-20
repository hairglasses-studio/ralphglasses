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
	return srv, root
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
