package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// TestIntegration_RepoLifecycle exercises the multi-step workflow:
// scan → list → status → repo_health
// Each step verifies state changes from the previous step.
func TestIntegration_RepoLifecycle(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	ctx := context.Background()

	// Write a CLAUDE.md so repo_health has something to inspect.
	claudeMD := filepath.Join(root, "test-repo", "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte("# Test Project\n\nBuild with go build ./...\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Step 1: Scan — should discover the test repo.
	scanResult, err := srv.handleScan(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if scanResult.IsError {
		t.Fatalf("scan returned error: %s", getResultText(scanResult))
	}
	scanText := getResultText(scanResult)
	if !strings.Contains(scanText, "repos_found") {
		t.Fatalf("scan: expected JSON with repos_found, got: %s", scanText)
	}

	// Step 2: List — repo discovered by scan should appear.
	listResult, err := srv.handleList(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list returned error: %s", getResultText(listResult))
	}
	listText := getResultText(listResult)
	if !strings.Contains(listText, "test-repo") {
		t.Fatalf("list: expected test-repo after scan, got: %s", listText)
	}

	// Step 3: Status — should return detailed info for the scanned repo.
	statusResult, err := srv.handleStatus(ctx, makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if statusResult.IsError {
		t.Fatalf("status returned error: %s", getResultText(statusResult))
	}
	statusText := getResultText(statusResult)
	var statusData map[string]any
	if err := json.Unmarshal([]byte(statusText), &statusData); err != nil {
		t.Fatalf("status: invalid JSON: %v", err)
	}
	if statusData["name"] != "test-repo" {
		t.Fatalf("status: expected name=test-repo, got: %v", statusData["name"])
	}

	// Step 4: Repo Health — should return a health score for the scanned repo.
	healthResult, err := srv.handleRepoHealth(ctx, makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("repo_health: %v", err)
	}
	if healthResult.IsError {
		t.Fatalf("repo_health returned error: %s", getResultText(healthResult))
	}
	healthText := getResultText(healthResult)
	var healthData map[string]any
	if err := json.Unmarshal([]byte(healthText), &healthData); err != nil {
		t.Fatalf("repo_health: invalid JSON: %v", err)
	}
	score, ok := healthData["health_score"].(float64)
	if !ok {
		t.Fatalf("repo_health: missing health_score field")
	}
	if score < 0 || score > 100 {
		t.Fatalf("repo_health: score %v out of range [0,100]", score)
	}
}

// TestIntegration_ScratchpadLifecycle exercises:
// append → list → read → resolve → read (verify resolved)
func TestIntegration_ScratchpadLifecycle(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	srv := &Server{ScanPath: root}
	ctx := context.Background()

	// Step 1: Append content to a new scratchpad.
	appendResult, err := srv.handleScratchpadAppend(ctx, makeRequest(map[string]any{
		"name":    "integration_test",
		"content": "1. First task\n2. Second task\n3. Third task",
	}))
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if appendResult.IsError {
		t.Fatalf("append returned error: %s", getResultText(appendResult))
	}
	appendText := getResultText(appendResult)
	if !strings.Contains(appendText, "Appended to integration_test scratchpad") {
		t.Fatalf("append: unexpected result: %s", appendText)
	}

	// Step 2: List — the new scratchpad should appear.
	listResult, err := srv.handleScratchpadList(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list returned error: %s", getResultText(listResult))
	}
	listText := getResultText(listResult)
	var names []string
	if err := json.Unmarshal([]byte(listText), &names); err != nil {
		t.Fatalf("list: invalid JSON: %v", err)
	}
	found := false
	for _, n := range names {
		if n == "integration_test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("list: expected integration_test in %v", names)
	}

	// Step 3: Read — should return content we appended.
	readResult, err := srv.handleScratchpadRead(ctx, makeRequest(map[string]any{
		"name": "integration_test",
	}))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if readResult.IsError {
		t.Fatalf("read returned error: %s", getResultText(readResult))
	}
	readText := getResultText(readResult)
	if !strings.Contains(readText, "1. First task") {
		t.Fatalf("read: expected First task, got: %s", readText)
	}
	if !strings.Contains(readText, "3. Third task") {
		t.Fatalf("read: expected Third task, got: %s", readText)
	}

	// Step 4: Resolve item 2.
	resolveResult, err := srv.handleScratchpadResolve(ctx, makeRequest(map[string]any{
		"name":        "integration_test",
		"item_number": float64(2),
		"resolution":  "completed in PR #42",
	}))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolveResult.IsError {
		t.Fatalf("resolve returned error: %s", getResultText(resolveResult))
	}
	resolveText := getResultText(resolveResult)
	if !strings.Contains(resolveText, "Resolved item 2") {
		t.Fatalf("resolve: unexpected result: %s", resolveText)
	}

	// Step 5: Read again — item 2 should be marked RESOLVED.
	readResult2, err := srv.handleScratchpadRead(ctx, makeRequest(map[string]any{
		"name": "integration_test",
	}))
	if err != nil {
		t.Fatalf("read after resolve: %v", err)
	}
	readText2 := getResultText(readResult2)
	if !strings.Contains(readText2, "RESOLVED: completed in PR #42") {
		t.Fatalf("read after resolve: expected RESOLVED marker, got: %s", readText2)
	}
	// Other items should NOT be resolved.
	if strings.Contains(readText2, "1. First task -- RESOLVED") {
		t.Fatalf("read after resolve: item 1 should not be resolved")
	}
}

// TestIntegration_SessionLifecycle exercises:
// session_list (empty) → inject session → session_list → session_status → session_budget → session_stop_all
func TestIntegration_SessionLifecycle(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	ctx := context.Background()
	repoPath := filepath.Join(root, "test-repo")

	// Step 1: List sessions — should be empty.
	listResult, err := srv.handleSessionList(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("list (empty): %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list (empty) returned error: %s", getResultText(listResult))
	}
	listText := getResultText(listResult)
	if !strings.Contains(listText, "[]") {
		t.Fatalf("list (empty): expected empty array, got: %s", listText)
	}

	// Step 2: Inject a test session (simulates session_launch without requiring real CLI).
	sessionID := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Model = "opus"
		s.BudgetUSD = 10.0
		s.SpentUSD = 2.0
		s.TurnCount = 8
	})

	// Step 3: List sessions — should now contain our session.
	listResult2, err := srv.handleSessionList(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("list (with session): %v", err)
	}
	if listResult2.IsError {
		t.Fatalf("list (with session) returned error: %s", getResultText(listResult2))
	}
	listText2 := getResultText(listResult2)
	if !strings.Contains(listText2, sessionID) {
		t.Fatalf("list: expected session ID %s, got: %s", sessionID, listText2)
	}

	// Step 4: Get session status — verify details match what we injected.
	statusResult, err := srv.handleSessionStatus(ctx, makeRequest(map[string]any{
		"id": sessionID,
	}))
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if statusResult.IsError {
		t.Fatalf("status returned error: %s", getResultText(statusResult))
	}
	statusText := getResultText(statusResult)
	if !strings.Contains(statusText, `"model":"opus"`) {
		t.Fatalf("status: expected model opus, got: %s", statusText)
	}
	if !strings.Contains(statusText, `"turns":8`) {
		t.Fatalf("status: expected turns 8, got: %s", statusText)
	}

	// Step 5: Update budget — verify new budget and remaining.
	budgetResult, err := srv.handleSessionBudget(ctx, makeRequest(map[string]any{
		"id":     sessionID,
		"budget_usd": float64(25),
	}))
	if err != nil {
		t.Fatalf("budget: %v", err)
	}
	if budgetResult.IsError {
		t.Fatalf("budget returned error: %s", getResultText(budgetResult))
	}
	budgetText := getResultText(budgetResult)
	if !strings.Contains(budgetText, `"budget_usd":25`) {
		t.Fatalf("budget: expected budget 25, got: %s", budgetText)
	}
	if !strings.Contains(budgetText, `"remaining":23`) {
		t.Fatalf("budget: expected remaining 23, got: %s", budgetText)
	}

	// Step 6: Stop all sessions — verify count matches.
	stopResult, err := srv.handleSessionStopAll(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("stop_all: %v", err)
	}
	if stopResult.IsError {
		t.Fatalf("stop_all returned error: %s", getResultText(stopResult))
	}
	stopText := getResultText(stopResult)
	if !strings.Contains(stopText, "Stopped 1") {
		t.Fatalf("stop_all: expected Stopped 1, got: %s", stopText)
	}

	// Step 7: Verify session is now stopped via status.
	statusResult2, err := srv.handleSessionStatus(ctx, makeRequest(map[string]any{
		"id": sessionID,
	}))
	if err != nil {
		t.Fatalf("status after stop: %v", err)
	}
	statusText2 := getResultText(statusResult2)
	if !strings.Contains(statusText2, `"status":"stopped"`) {
		t.Fatalf("status after stop: expected stopped, got: %s", statusText2)
	}
}

// TestIntegration_ConfigLifecycle exercises:
// config (list all) → config (get key) → config (set key) → config (read back)
func TestIntegration_ConfigLifecycle(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Pre-scan.
	if _, err := srv.handleScan(ctx, makeRequest(nil)); err != nil {
		t.Fatal(err)
	}

	// Step 1: List all config values.
	listResult, err := srv.handleConfig(ctx, makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("config list: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("config list returned error: %s", getResultText(listResult))
	}
	listText := getResultText(listResult)
	if !strings.Contains(listText, "MODEL") {
		t.Fatalf("config list: expected MODEL key, got: %s", listText)
	}
	if !strings.Contains(listText, "BUDGET") {
		t.Fatalf("config list: expected BUDGET key, got: %s", listText)
	}

	// Step 2: Get a specific key.
	getResult, err := srv.handleConfig(ctx, makeRequest(map[string]any{
		"repo": "test-repo",
		"key":  "MODEL",
	}))
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("config get returned error: %s", getResultText(getResult))
	}
	getText := getResultText(getResult)
	if getText != "MODEL=sonnet" {
		t.Fatalf("config get: expected MODEL=sonnet, got: %s", getText)
	}

	// Step 3: Set the key to a new value.
	setResult, err := srv.handleConfig(ctx, makeRequest(map[string]any{
		"repo":  "test-repo",
		"key":   "MODEL",
		"value": "haiku",
	}))
	if err != nil {
		t.Fatalf("config set: %v", err)
	}
	if setResult.IsError {
		t.Fatalf("config set returned error: %s", getResultText(setResult))
	}
	setText := getResultText(setResult)
	if !strings.Contains(setText, "Set MODEL=haiku") {
		t.Fatalf("config set: expected confirmation, got: %s", setText)
	}

	// Step 4: Read back — should reflect the updated value.
	getResult2, err := srv.handleConfig(ctx, makeRequest(map[string]any{
		"repo": "test-repo",
		"key":  "MODEL",
	}))
	if err != nil {
		t.Fatalf("config get after set: %v", err)
	}
	if getResult2.IsError {
		t.Fatalf("config get after set returned error: %s", getResultText(getResult2))
	}
	getText2 := getResultText(getResult2)
	if getText2 != "MODEL=haiku" {
		t.Fatalf("config get after set: expected MODEL=haiku, got: %s", getText2)
	}
}

// TestIntegration_EventFlow exercises:
// event_list → publish events → event_list (verify) → event_poll
func TestIntegration_EventFlow(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	srv := NewServerWithBus(t.TempDir(), bus)
	ctx := context.Background()

	// Step 1: Event list — should be empty initially.
	listResult, err := srv.handleEventList(ctx, makeRequest(map[string]any{
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("event_list (empty): %v", err)
	}
	if listResult.IsError {
		t.Fatalf("event_list returned error: %s", getResultText(listResult))
	}
	var listData map[string]any
	if err := json.Unmarshal([]byte(getResultText(listResult)), &listData); err != nil {
		t.Fatalf("event_list: invalid JSON: %v", err)
	}
	eventsArr, ok := listData["events"].([]any)
	if !ok {
		t.Fatalf("event_list: events field missing or wrong type")
	}
	if len(eventsArr) != 0 {
		t.Fatalf("event_list: expected 0 events initially, got %d", len(eventsArr))
	}

	// Step 2: Publish some events.
	bus.Publish(events.Event{
		Type:     events.SessionStarted,
		RepoName: "test-repo",
	})
	bus.Publish(events.Event{
		Type:     events.LoopIterated,
		RepoName: "test-repo",
	})
	// Give the bus a moment to process.
	time.Sleep(10 * time.Millisecond)

	// Step 3: Event list — should now contain the published events.
	listResult2, err := srv.handleEventList(ctx, makeRequest(map[string]any{
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("event_list (after publish): %v", err)
	}
	if listResult2.IsError {
		t.Fatalf("event_list returned error: %s", getResultText(listResult2))
	}
	var listData2 map[string]any
	if err := json.Unmarshal([]byte(getResultText(listResult2)), &listData2); err != nil {
		t.Fatalf("event_list: invalid JSON: %v", err)
	}
	eventsArr2, ok := listData2["events"].([]any)
	if !ok {
		t.Fatalf("event_list: events field missing")
	}
	if len(eventsArr2) < 2 {
		t.Fatalf("event_list: expected >= 2 events, got %d", len(eventsArr2))
	}

	// Step 4: Event poll — verify structure and cursor.
	pollResult, err := srv.handleEventPoll(ctx, makeRequest(map[string]any{
		"cursor": "0",
		"limit":  float64(10),
	}))
	if err != nil {
		t.Fatalf("event_poll: %v", err)
	}
	if pollResult.IsError {
		t.Fatalf("event_poll returned error: %s", getResultText(pollResult))
	}
	pollText := getResultText(pollResult)
	var pollData map[string]any
	if err := json.Unmarshal([]byte(pollText), &pollData); err != nil {
		t.Fatalf("event_poll: invalid JSON: %v", err)
	}
	if _, ok := pollData["cursor"]; !ok {
		t.Fatalf("event_poll: expected cursor field")
	}
	pollEvents, ok := pollData["events"].([]any)
	if !ok {
		t.Fatalf("event_poll: events field missing")
	}
	if len(pollEvents) < 2 {
		t.Fatalf("event_poll: expected >= 2 events, got %d", len(pollEvents))
	}
}

// TestIntegration_ToolGroups exercises:
// tool_groups → verify all 13 groups → load_tool_group → verify loaded → load again (already_loaded)
func TestIntegration_ToolGroups(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	srv.DeferredLoading = true
	mcpSrv := server.NewMCPServer("test-integration", "1.0")
	srv.RegisterCoreTools(mcpSrv)
	ctx := context.Background()

	// Step 1: List tool groups — should show all 13 groups.
	groupsResult, err := srv.handleToolGroups(ctx, mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("tool_groups: %v", err)
	}
	if groupsResult.IsError {
		t.Fatalf("tool_groups returned error: %s", getResultText(groupsResult))
	}
	groupsText := getResultText(groupsResult)
	for _, name := range ToolGroupNames {
		if !strings.Contains(groupsText, name) {
			t.Errorf("tool_groups: missing group %q in output", name)
		}
	}

	// Verify group count matches ToolGroupNames.
	if len(ToolGroupNames) != 29 {
		t.Fatalf("tool_groups: expected 29 group names, got %d", len(ToolGroupNames))
	}

	// Step 2: Only core should be loaded initially.
	if !srv.loadedGroups["core"] {
		t.Fatalf("core group should be loaded after RegisterCoreTools")
	}
	if srv.loadedGroups["session"] {
		t.Fatalf("session group should NOT be loaded yet")
	}

	// Step 3: Load the "session" group.
	loadResult, err := srv.handleLoadToolGroup(ctx, makeRequest(map[string]any{
		"group": "session",
	}))
	if err != nil {
		t.Fatalf("load_tool_group: %v", err)
	}
	if loadResult.IsError {
		t.Fatalf("load_tool_group returned error: %s", getResultText(loadResult))
	}
	loadText := getResultText(loadResult)
	if !strings.Contains(loadText, "loaded") {
		t.Fatalf("load_tool_group: expected 'loaded', got: %s", loadText)
	}

	// Step 4: Verify session group is now loaded.
	if !srv.loadedGroups["session"] {
		t.Fatalf("session group should be loaded after handleLoadToolGroup")
	}

	// Step 5: Load again — should say already_loaded.
	loadResult2, err := srv.handleLoadToolGroup(ctx, makeRequest(map[string]any{
		"group": "session",
	}))
	if err != nil {
		t.Fatalf("load_tool_group (again): %v", err)
	}
	loadText2 := getResultText(loadResult2)
	if !strings.Contains(loadText2, "already_loaded") {
		t.Fatalf("load_tool_group (again): expected 'already_loaded', got: %s", loadText2)
	}

	// Step 6: Load an invalid group — should return error.
	loadResult3, err := srv.handleLoadToolGroup(ctx, makeRequest(map[string]any{
		"group": "nonexistent_group",
	}))
	if err != nil {
		t.Fatalf("load_tool_group (invalid): %v", err)
	}
	if !loadResult3.IsError {
		t.Fatalf("load_tool_group (invalid): expected error")
	}
}

// TestIntegration_JournalLifecycle exercises:
// journal_write → journal_read → journal_write (more) → journal_read (verify both)
func TestIntegration_JournalLifecycle(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Pre-scan.
	if _, err := srv.handleScan(ctx, makeRequest(nil)); err != nil {
		t.Fatal(err)
	}

	// Step 1: Write a journal entry.
	writeResult, err := srv.handleJournalWrite(ctx, makeRequest(map[string]any{
		"repo":       "test-repo",
		"session_id": "sess-001",
		"worked":     "implemented parser, added tests",
		"failed":     "export module broken",
		"suggest":    "try different approach for export",
	}))
	if err != nil {
		t.Fatalf("journal_write: %v", err)
	}
	if writeResult.IsError {
		t.Fatalf("journal_write returned error: %s", getResultText(writeResult))
	}
	writeText := getResultText(writeResult)
	if !strings.Contains(writeText, `"status":"written"`) {
		t.Fatalf("journal_write: expected written status, got: %s", writeText)
	}

	// Step 2: Read journal — should contain the entry.
	readResult, err := srv.handleJournalRead(ctx, makeRequest(map[string]any{
		"repo":  "test-repo",
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("journal_read: %v", err)
	}
	if readResult.IsError {
		t.Fatalf("journal_read returned error: %s", getResultText(readResult))
	}
	readText := getResultText(readResult)
	var readData map[string]any
	if err := json.Unmarshal([]byte(readText), &readData); err != nil {
		t.Fatalf("journal_read: invalid JSON: %v", err)
	}
	count, ok := readData["count"].(float64)
	if !ok || count < 1 {
		t.Fatalf("journal_read: expected count >= 1, got: %v", readData["count"])
	}

	// Step 3: Write another journal entry.
	writeResult2, err := srv.handleJournalWrite(ctx, makeRequest(map[string]any{
		"repo":       "test-repo",
		"session_id": "sess-002",
		"worked":     "fixed export module",
	}))
	if err != nil {
		t.Fatalf("journal_write 2: %v", err)
	}
	if writeResult2.IsError {
		t.Fatalf("journal_write 2 returned error: %s", getResultText(writeResult2))
	}

	// Step 4: Read again — should now have at least 2 entries.
	readResult2, err := srv.handleJournalRead(ctx, makeRequest(map[string]any{
		"repo":  "test-repo",
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("journal_read 2: %v", err)
	}
	readText2 := getResultText(readResult2)
	var readData2 map[string]any
	if err := json.Unmarshal([]byte(readText2), &readData2); err != nil {
		t.Fatalf("journal_read 2: invalid JSON: %v", err)
	}
	count2, ok := readData2["count"].(float64)
	if !ok || count2 < 2 {
		t.Fatalf("journal_read 2: expected count >= 2, got: %v", readData2["count"])
	}

	// Verify synthesis is present (aggregated context from journal entries).
	if _, ok := readData2["synthesis"]; !ok {
		t.Fatalf("journal_read: expected synthesis field in response")
	}
}

// TestIntegration_ScaffoldThenScan exercises:
// scaffold a new repo → re-scan → verify new repo appears in list
func TestIntegration_ScaffoldThenScan(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	ctx := context.Background()

	// Step 1: Initial scan — only test-repo.
	if _, err := srv.handleScan(ctx, makeRequest(nil)); err != nil {
		t.Fatal(err)
	}
	listResult, err := srv.handleList(ctx, makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	listText := getResultText(listResult)
	if strings.Contains(listText, "new-project") {
		t.Fatalf("new-project should not exist before scaffold")
	}

	// Step 2: Scaffold a new repo.
	newRepoPath := filepath.Join(root, "new-project")
	if err := os.MkdirAll(newRepoPath, 0755); err != nil {
		t.Fatal(err)
	}
	scaffoldResult, err := srv.handleRepoScaffold(ctx, makeRequest(map[string]any{
		"path":         newRepoPath,
		"project_type": "go",
		"project_name": "new-project",
	}))
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if scaffoldResult.IsError {
		t.Fatalf("scaffold returned error: %s", getResultText(scaffoldResult))
	}

	// Verify .ralph directory was created.
	if _, err := os.Stat(filepath.Join(newRepoPath, ".ralph")); os.IsNotExist(err) {
		t.Fatalf("scaffold: .ralph directory not created")
	}

	// Step 3: Re-scan — should find both repos now.
	scanResult, err := srv.handleScan(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("re-scan: %v", err)
	}
	scanText := getResultText(scanResult)
	if !strings.Contains(scanText, "repos_found") {
		t.Fatalf("re-scan: expected JSON with repos_found, got: %s", scanText)
	}

	// Step 4: List — both repos should appear.
	listResult2, err := srv.handleList(ctx, makeRequest(nil))
	if err != nil {
		t.Fatal(err)
	}
	listText2 := getResultText(listResult2)
	if !strings.Contains(listText2, "test-repo") {
		t.Fatalf("list after scaffold: expected test-repo, got: %s", listText2)
	}
	if !strings.Contains(listText2, "new-project") {
		t.Fatalf("list after scaffold: expected new-project, got: %s", listText2)
	}
}

// TestIntegration_SessionErrorsFlow exercises:
// inject sessions with various states → session_errors → verify budget warnings and error counts
func TestIntegration_SessionErrorsFlow(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	ctx := context.Background()
	repoPath := filepath.Join(root, "test-repo")

	// Step 1: No sessions — errors should report 0.
	errResult, err := srv.handleSessionErrors(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("errors (empty): %v", err)
	}
	errText := getResultText(errResult)
	if !strings.Contains(errText, `"total_errors":0`) {
		t.Fatalf("errors: expected 0 total_errors, got: %s", errText)
	}

	// Step 2: Inject a session at 90% budget (should trigger budget warning).
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.BudgetUSD = 10.0
		s.SpentUSD = 9.0
	})

	// Step 3: Inject an errored session.
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "out of memory"
	})

	// Step 4: Check errors — should show budget warning and errored session.
	errResult2, err := srv.handleSessionErrors(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("errors (with sessions): %v", err)
	}
	errText2 := getResultText(errResult2)
	if !strings.Contains(errText2, "budget_warning") {
		t.Fatalf("errors: expected budget_warning, got: %s", errText2)
	}
	if !strings.Contains(errText2, `"total_errors":2`) {
		t.Fatalf("errors: expected 2 total_errors (budget_warning + session_error), got: %s", errText2)
	}
}
