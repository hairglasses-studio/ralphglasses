package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// testSessionCounter is defined in tools_session_test.go.

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

	// Stop any background RunLoop goroutines before temp dir cleanup.
	t.Cleanup(func() {
		for _, run := range srv.SessMgr.ListLoops() {
			_ = srv.SessMgr.StopLoop(run.ID)
		}
	})

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

// Helper to extract text from a CallToolResult
func getResultText(r *mcp.CallToolResult) string {
	for _, c := range r.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestNewServer(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	if srv.ScanPath != "/tmp/test" {
		t.Errorf("ScanPath = %q, want %q", srv.ScanPath, "/tmp/test")
	}
	if srv.ProcMgr == nil {
		t.Fatal("ProcMgr should not be nil")
	}
}

func TestHandleScan(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Scan first
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
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
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected status to mention test-repo, got: %s", text)
	}
}

func TestHandleStatus_MissingRepoArg(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStatus: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStatus_UnknownRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStart(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStart: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStop_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleStop(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleStop: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStopAll(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handlePause(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handlePause: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo arg")
	}
}

func TestHandleStatus_ScanError(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	r := codedError(ErrInternal, "something failed")
	if !r.IsError {
		t.Error("codedError should be an error")
	}
	text := getResultText(r)
	if !strings.Contains(text, `"error_code":"INTERNAL_ERROR"`) {
		t.Errorf("expected error_code in text, got %q", text)
	}
	if !strings.Contains(text, "[INTERNAL_ERROR] something failed") {
		t.Errorf("expected prefixed message in text, got %q", text)
	}
}

func TestJsonResult(t *testing.T) {
	t.Parallel()
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

func TestHandleScan_ConcurrentRace(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, _ = srv.handleScan(context.Background(), makeRequest(nil))
		}()
	}
	close(ready)
	wg.Wait()

	if srv.Repos == nil {
		t.Fatal("expected Repos to be non-nil after concurrent scans")
	}
}

func TestHandleScan_ConcurrentReadWrite(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	ready := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, _ = srv.handleScan(context.Background(), makeRequest(nil))
		}()
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			_, _ = srv.handleList(context.Background(), makeRequest(nil))
		}()
	}
	close(ready)
	wg.Wait()
}
