package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
		"repo":           "test-repo",
		"planner_model":  "o4-mini",
		"worker_model":   "codex-mini-latest",
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

// TestHandleLoopPrune_NoPrunableLoops verifies that when all loop runs are
// recent and running, prune returns count=0.
func TestHandleLoopPrune_NoPrunableLoops(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Write recent loop run files with status="running" into the loop state dir.
	loopDir := srv.SessMgr.LoopStateDir()
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for _, id := range []string{"run-a", "run-b"} {
		data, _ := json.Marshal(map[string]any{
			"id":         id,
			"repo_name":  "test-repo",
			"status":     "running",
			"created_at": now.Add(-1 * time.Hour),
			"updated_at": now,
		})
		if err := os.WriteFile(filepath.Join(loopDir, id+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, err := srv.handleLoopPrune(context.Background(), makeRequest(map[string]any{
		"older_than_hours": float64(72),
		"statuses":         "pending,failed",
		"dry_run":          true,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var pruneData map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &pruneData); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	pruned, _ := pruneData["pruned"].(float64)
	if pruned != 0 {
		t.Errorf("expected pruned=0 for recent running loops, got %v", pruned)
	}
}

// TestHandleLoopPrune_OlderThanZero verifies that older_than_hours=0 prunes
// ALL matching-status loops regardless of age.
func TestHandleLoopPrune_OlderThanZero(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	loopDir := srv.SessMgr.LoopStateDir()
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	// Write loop runs with status="failed" — one recent, one old.
	for i, age := range []time.Duration{-5 * time.Minute, -100 * time.Hour} {
		id := fmt.Sprintf("prune-%d", i)
		pruneFileData, _ := json.Marshal(map[string]any{
			"id":         id,
			"repo_name":  "test-repo",
			"status":     "failed",
			"created_at": now.Add(age - time.Hour),
			"updated_at": now.Add(age),
		})
		if err := os.WriteFile(filepath.Join(loopDir, id+".json"), pruneFileData, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// older_than_hours=0 means olderThan=0s, so cutoff=now; all past timestamps are before cutoff.
	result, err := srv.handleLoopPrune(context.Background(), makeRequest(map[string]any{
		"older_than_hours": float64(0),
		"statuses":         "failed",
		"dry_run":          false,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var pruneResult map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &pruneResult); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	pruned, _ := pruneResult["pruned"].(float64)
	if pruned != 2 {
		t.Errorf("expected pruned=2 with older_than_hours=0, got %v", pruned)
	}

	// Verify files were actually removed.
	entries, _ := os.ReadDir(loopDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "prune-") {
			t.Errorf("expected prune file %s to be deleted", e.Name())
		}
	}
}
