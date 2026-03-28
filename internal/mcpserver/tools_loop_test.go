package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleLoopLifecycle(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Pre-trigger engineOnce with nil engine so handleLoopStart won't create
	// an LLM-enabled enhancer that makes real API calls (30s timeout each).
	srv.engineOnce.Do(func() { srv.Engine = nil })

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
			if opts.Model == "gpt-4o" || opts.Model == "o1-pro" {
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
