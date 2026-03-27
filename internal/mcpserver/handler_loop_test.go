package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
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

// TestConcurrentLoopStartAndStatus exercises handleLoopStart and handleLoopStatus
// from multiple goroutines to detect data races on shared state such as the
// Enhancer field. Run with -race to verify.
func TestConcurrentLoopStartAndStatus(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	ctx := context.Background()

	// Scan so repos are populated.
	_, _ = srv.handleScan(ctx, makeRequest(nil))

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

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n * 2)

	// Half goroutines call handleLoopStart, half call handleLoopStatus.
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = srv.handleLoopStart(ctx, makeRequest(map[string]any{
				"repo": "test-repo",
			}))
		}()
		go func() {
			defer wg.Done()
			_, _ = srv.handleLoopStatus(ctx, makeRequest(map[string]any{
				"id": "nonexistent",
			}))
		}()
	}
	wg.Wait()
}
