package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleLoopStart_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStart(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopStart_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStart(context.Background(), makeRequest(map[string]any{
		"repo": "../escape-attempt",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid repo name")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrRepoNameInvalid)) {
		t.Fatalf("expected REPO_NAME_INVALID error code, got: %s", text)
	}
}

func TestHandleLoopStart_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	// Trigger scan so repos are populated but won't contain "nonexistent"
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLoopStart(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for repo not found")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrRepoNotFound)) {
		t.Fatalf("expected REPO_NOT_FOUND error code, got: %s", text)
	}
}

func TestHandleLoopStart_ValidParams(t *testing.T) {
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

	result, err := srv.handleLoopStart(context.Background(), makeRequest(map[string]any{
		"repo":         "test-repo",
		"planner_model": "o1-pro",
		"worker_model":  "sonnet",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleLoopStart returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["id"] == nil || data["id"] == "" {
		t.Fatal("expected loop id in response")
	}
	if data["repo"] != "test-repo" {
		t.Fatalf("repo = %v, want test-repo", data["repo"])
	}
}

func TestHandleLoopStatus_MissingID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopStatus_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStatus(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent-loop-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for loop not found")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrLoopNotFound)) {
		t.Fatalf("expected LOOP_NOT_FOUND error code, got: %s", text)
	}
}

func TestHandleLoopStep_MissingID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStep(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopStep_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStep(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent-loop-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for step on nonexistent loop")
	}
}

func TestHandleLoopStop_MissingID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStop(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopStop_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopStop(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent-loop-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for stop on nonexistent loop")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrLoopNotFound)) {
		t.Fatalf("expected LOOP_NOT_FOUND error code, got: %s", text)
	}
}

func TestHandleLoopStatus_JournalEntryCount(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	repoPath := filepath.Join(root, "test-repo")

	// Start a loop to get a valid loop ID.
	run, err := srv.SessMgr.StartLoop(context.Background(), repoPath, session.DefaultLoopProfile())
	if err != nil {
		t.Fatalf("start loop: %v", err)
	}

	// Case 1: No journal file — count should be 0.
	result, err := srv.handleLoopStatus(context.Background(), makeRequest(map[string]any{
		"id": run.ID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleLoopStatus returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	countVal, ok := data["journal_entry_count"]
	if !ok {
		t.Fatal("expected journal_entry_count key in response")
	}
	if int(countVal.(float64)) != 0 {
		t.Fatalf("journal_entry_count = %v, want 0 (no journal file)", countVal)
	}

	// Case 2: Write journal entries and verify count increases.
	for i := 0; i < 3; i++ {
		entry := session.JournalEntry{
			Timestamp: time.Now(),
			SessionID: fmt.Sprintf("test-session-%d", i),
			RepoName:  "test-repo",
			Worked:    []string{"item"},
		}
		if err := session.WriteJournalEntryManual(repoPath, entry); err != nil {
			t.Fatalf("write journal entry: %v", err)
		}
	}

	result2, err := srv.handleLoopStatus(context.Background(), makeRequest(map[string]any{
		"id": run.ID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.IsError {
		t.Fatalf("handleLoopStatus returned error: %s", getResultText(result2))
	}

	var data2 map[string]any
	if err := json.Unmarshal([]byte(getResultText(result2)), &data2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	countVal2, ok := data2["journal_entry_count"]
	if !ok {
		t.Fatal("expected journal_entry_count key in response after writing entries")
	}
	if int(countVal2.(float64)) != 3 {
		t.Fatalf("journal_entry_count = %v, want 3", countVal2)
	}
}
