package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleTriggerWebhook_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"agent_type": "ralph",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleTriggerWebhook_MissingAgentType(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"prompt": "run tests",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing agent_type")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleTriggerWebhook_InvalidAgentType(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"prompt":     "run tests",
		"agent_type": "invalid",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid agent_type")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleTriggerWebhook_InvalidPriority(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"prompt":     "run tests",
		"agent_type": "ralph",
		"priority":   float64(15),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid priority")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleTriggerWebhook_ValidPending(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"prompt":     "run all tests and fix failures",
		"agent_type": "ralph",
		"priority":   float64(7),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleTriggerWebhook returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["trigger_id"] == nil || data["trigger_id"] == "" {
		t.Fatal("expected trigger_id in response")
	}
	if data["status"] != "pending" {
		t.Fatalf("status = %v, want pending", data["status"])
	}
	if data["agent_type"] != "ralph" {
		t.Fatalf("agent_type = %v, want ralph", data["agent_type"])
	}
	if int(data["priority"].(float64)) != 7 {
		t.Fatalf("priority = %v, want 7", data["priority"])
	}
}

func TestHandleTriggerWebhook_LaunchWithoutRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"prompt":     "run tests",
		"agent_type": "ralph",
		"launch":     true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when launch=true but no repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error, got: %s", text)
	}
}

func TestHandleTriggerWebhook_LaunchLoop(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Pre-trigger engineOnce to avoid real API calls.
	srv.engineOnce.Do(func() { srv.Engine = nil })

	srv.SessMgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			return &session.Session{
				ID:         strings.ReplaceAll(opts.SessionName, " ", "-"),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     session.StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}, nil
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

	result, err := srv.handleTriggerWebhook(context.Background(), makeRequest(map[string]any{
		"prompt":     "run CI and fix failures",
		"agent_type": "loop",
		"priority":   float64(8),
		"launch":     true,
		"repo":       "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleTriggerWebhook returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["status"] != "launched" {
		t.Fatalf("status = %v, want launched", data["status"])
	}
	if data["session_id"] == nil || data["session_id"] == "" {
		t.Fatal("expected session_id in response when launched")
	}
}


func TestHandleScheduleCreate_CreateAndList(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Override the schedules path to use a temp directory.
	tmpDir := t.TempDir()
	origPath := schedulesPath
	schedulesPath = func() string { return filepath.Join(tmpDir, "schedules.json") }
	t.Cleanup(func() { schedulesPath = origPath })

	// Create a schedule.
	result, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"prompt":          "daily CI check",
		"cron_expression": "0 9 * * *",
		"agent_type":      "ralph",
		"enabled":         true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleScheduleCreate returned error: %s", getResultText(result))
	}

	var createData map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &createData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if createData["schedule_id"] == nil || createData["schedule_id"] == "" {
		t.Fatal("expected schedule_id in response")
	}
	if createData["cron_expression"] != "0 9 * * *" {
		t.Fatalf("cron_expression = %v, want '0 9 * * *'", createData["cron_expression"])
	}

	// List schedules.
	listResult, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if listResult.IsError {
		t.Fatalf("schedule list returned error: %s", getResultText(listResult))
	}

	var listData map[string]any
	if err := json.Unmarshal([]byte(getResultText(listResult)), &listData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	count := int(listData["count"].(float64))
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestHandleScheduleCreate_DisableEnable(t *testing.T) {
	srv, _ := setupTestServer(t)

	tmpDir := t.TempDir()
	origPath := schedulesPath
	schedulesPath = func() string { return filepath.Join(tmpDir, "schedules.json") }
	t.Cleanup(func() { schedulesPath = origPath })

	// Create a schedule first.
	createResult, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"prompt":          "nightly build",
		"cron_expression": "0 2 * * *",
		"agent_type":      "cycle",
		"enabled":         true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("create returned error: %s", getResultText(createResult))
	}

	var createData map[string]any
	if err := json.Unmarshal([]byte(getResultText(createResult)), &createData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	schedID := createData["schedule_id"].(string)

	// Disable it.
	disableResult, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"action": "disable",
		"id":     schedID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if disableResult.IsError {
		t.Fatalf("disable returned error: %s", getResultText(disableResult))
	}

	// Verify disabled via list.
	listResult, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var listData map[string]any
	if err := json.Unmarshal([]byte(getResultText(listResult)), &listData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	schedules := listData["schedules"].([]any)
	if len(schedules) != 1 {
		t.Fatalf("expected 1 schedule, got %d", len(schedules))
	}
	sched := schedules[0].(map[string]any)
	if sched["enabled"].(bool) {
		t.Fatal("expected schedule to be disabled")
	}

	// Re-enable it.
	enableResult, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"action": "enable",
		"id":     schedID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enableResult.IsError {
		t.Fatalf("enable returned error: %s", getResultText(enableResult))
	}

	// Verify enabled.
	verifyData, _ := os.ReadFile(filepath.Join(tmpDir, "schedules.json"))
	var verified []ScheduleEntry
	if err := json.Unmarshal(verifyData, &verified); err != nil {
		t.Fatalf("unmarshal file: %v", err)
	}
	if !verified[0].Enabled {
		t.Fatal("expected schedule to be re-enabled")
	}
}

func TestHandleScheduleCreate_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"cron_expression": "0 9 * * *",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleScheduleCreate_DisableNotFound(t *testing.T) {
	srv, _ := setupTestServer(t)

	tmpDir := t.TempDir()
	origPath := schedulesPath
	schedulesPath = func() string { return filepath.Join(tmpDir, "schedules.json") }
	t.Cleanup(func() { schedulesPath = origPath })

	result, err := srv.handleScheduleCreate(context.Background(), makeRequest(map[string]any{
		"action": "disable",
		"id":     "nonexistent-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent schedule")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestSchedulesPath_FallsBackToStateDirWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	xdg := t.TempDir()
	t.Setenv("XDG_STATE_HOME", xdg)

	if got, want := schedulesPath(), filepath.Join(xdg, "ralph", "schedules.json"); got != want {
		t.Fatalf("schedulesPath() = %q, want %q", got, want)
	}
}
