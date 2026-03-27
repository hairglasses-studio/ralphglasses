package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- Roadmap tool tests ---

func TestHandleRoadmapParse(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

// --- Repo file tool tests ---

func TestHandleRepoScaffold(t *testing.T) {
	t.Parallel()
	_, root := setupTestServer(t)
	newRepoPath := filepath.Join(root, "new-repo")
	_ = os.MkdirAll(newRepoPath, 0755)
	_ = os.WriteFile(filepath.Join(newRepoPath, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644)

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv := NewServer("/tmp")

	result, err := srv.handleRepoOptimize(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoOptimize: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing path")
	}
}

// --- Team handler tests ---

func TestHandleTeamStatus_NotFound(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	root := t.TempDir()
	repoPath := filepath.Join(root, "test-repo")
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

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
	t.Parallel()
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
		t.Errorf("expected sonnet value, got: %s", text)
	}
}

func TestHandleConfigBulk_Set(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	if !strings.Contains(text, `"steps":2`) {
		t.Errorf("expected 2 steps, got: %s", text)
	}
}

func TestHandleWorkflowDefine_MissingArgs(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	t.Parallel()
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

func TestHandleWorkflowDelete(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// First define a workflow to delete.
	yamlStr := `name: delete-me
steps:
  - name: step1
    prompt: "do thing"
`
	result, err := srv.handleWorkflowDefine(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "delete-me",
		"yaml": yamlStr,
	}))
	if err != nil {
		t.Fatalf("handleWorkflowDefine: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleWorkflowDefine returned error: %s", getResultText(result))
	}

	// Now delete it.
	result, err = srv.handleWorkflowDelete(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "delete-me",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowDelete: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleWorkflowDelete returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "delete-me") {
		t.Errorf("expected workflow name in output, got: %s", text)
	}
	if !strings.Contains(text, `"deleted":true`) {
		t.Errorf("expected deleted:true, got: %s", text)
	}

	// Verify it's actually gone by trying to run it.
	result, err = srv.handleWorkflowRun(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "delete-me",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowRun: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for deleted workflow")
	}
}

func TestHandleWorkflowDelete_MissingArgs(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleWorkflowDelete(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowDelete: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestHandleWorkflowDelete_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleWorkflowDelete(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("handleWorkflowDelete: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent workflow")
	}
}

// --- Snapshot handler tests ---

func TestHandleSnapshot_Save(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	// Verify structured JSON fields: path, size_bytes, timestamp
	for _, key := range []string{"path", "size_bytes", "timestamp", "session_count", "team_count"} {
		if !strings.Contains(text, key) {
			t.Errorf("expected %q in structured response, got: %s", key, text)
		}
	}
}

func TestHandleSnapshot_InvalidAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleSnapshot(context.Background(), makeRequest(map[string]any{
		"action": "delete",
	}))
	if err != nil {
		t.Fatalf("handleSnapshot: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid action")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleSnapshot_List(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Save one first
	_, _ = srv.handleSnapshot(context.Background(), makeRequest(map[string]any{
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Create two agents first
	_, _ = srv.handleAgentDefine(context.Background(), makeRequest(map[string]any{
		"repo":        "test-repo",
		"name":        "agent-a",
		"prompt":      "You handle testing",
		"description": "Test runner",
		"tools":       "Bash,Read",
	}))
	_, _ = srv.handleAgentDefine(context.Background(), makeRequest(map[string]any{
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
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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

// --- Fleet Status handler tests ---

func TestHandleFleetStatus(t *testing.T) {
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
	if !strings.Contains(text, "summary") {
		t.Errorf("expected summary in output, got: %s", text)
	}
	if !strings.Contains(text, "repos") {
		t.Errorf("expected repos in output, got: %s", text)
	}
}

// --- Journal handler tests ---

func TestHandleJournalRead(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))
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
		_ = session.WriteJournalEntryManual(repoPath, entry)
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
	if !strings.Contains(text, `"count":3`) {
		t.Errorf("expected count 3, got: %s", text)
	}
}

func TestHandleJournalRead_Empty(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	if !strings.Contains(text, `"status":"empty"`) {
		t.Errorf("expected status=empty, got: %s", text)
	}
	if !strings.Contains(text, `"item_type":"journal_entries"`) {
		t.Errorf("expected item_type=journal_entries, got: %s", text)
	}
}

func TestHandleJournalWrite(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

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
	if !strings.Contains(text, `"status":"written"`) {
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
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))
	repoPath := filepath.Join(root, "test-repo")

	// Write some entries
	for i := 0; i < 5; i++ {
		_ = session.WriteJournalEntryManual(repoPath, session.JournalEntry{
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
	if !strings.Contains(text, `"dry_run":true`) {
		t.Errorf("expected dry_run true, got: %s", text)
	}
	if !strings.Contains(text, `"would_prune":2`) {
		t.Errorf("expected would_prune 2, got: %s", text)
	}

	// Verify no entries were actually removed
	entries, _ := session.ReadRecentJournal(repoPath, 100)
	if len(entries) != 5 {
		t.Errorf("dry run should not modify, got %d entries", len(entries))
	}
}

// --- Marathon monitoring tests ---

func TestHandleMarathonDashboardEmpty(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleMarathonDashboard(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleMarathonDashboard: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleMarathonDashboard returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"total":0`) {
		t.Errorf("expected 0 total sessions, got: %s", text)
	}
	if !strings.Contains(text, `"total_usd":0`) {
		t.Errorf("expected 0 total cost, got: %s", text)
	}
}

func TestHandleMarathonDashboardStale(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(text, `"stale":1`) {
		t.Errorf("expected 1 stale session, got: %s", text)
	}
	if !strings.Contains(text, "stale_session") {
		t.Errorf("expected stale_session alert, got: %s", text)
	}
}

// --- Remote Control (RC) handler tests ---

func TestRCStatus_Empty(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))
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
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleRCRead(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) {
		t.Errorf("expected empty status JSON, got: %s", text)
	}
	if !strings.Contains(text, `"item_type":"rc_messages"`) {
		t.Errorf("expected item_type rc_messages in empty result, got: %s", text)
	}
}

func TestRCRead_MostActive(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// --- Event Poll handler tests ---

func TestEventPoll_NoBus(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	if !strings.Contains(text, `"count":2`) {
		t.Errorf("expected 2 events, got: %s", text)
	}
	if !strings.Contains(text, "cursor") {
		t.Errorf("expected cursor in response, got: %s", text)
	}
}

func TestEventPoll_CursorAdvance(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv, _ := setupTestServer(t)
	srv.EventBus = bus

	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "r1"})

	result, _ := srv.handleEventPoll(context.Background(), makeRequest(nil))
	text := getResultText(result)
	// Extract cursor value
	if !strings.Contains(text, `"cursor":"1"`) {
		t.Errorf("expected cursor 1, got: %s", text)
	}

	// Publish more events
	bus.Publish(events.Event{Type: events.SessionEnded, RepoName: "r2"})

	// Poll with cursor
	result, _ = srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "1",
	}))
	text = getResultText(result)
	if !strings.Contains(text, `"count":1`) {
		t.Errorf("expected 1 new event, got: %s", text)
	}
}

// --- RC Act handler tests ---

func TestRCAct_UnknownAction(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
