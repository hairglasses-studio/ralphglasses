package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// testSessionCounter ensures unique session IDs even when time.Now().UnixNano()
// returns the same value for rapid successive calls (common under coverage).
var testSessionCounter atomic.Int64

// injectTestSession creates a fake session and inserts it directly into the manager.
func injectTestSession(t *testing.T, srv *Server, repoPath string, mods func(*session.Session)) string {
	t.Helper()
	now := time.Now()
	seq := testSessionCounter.Add(1)
	id := fmt.Sprintf("test-%d-%d", now.UnixNano(), seq)
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

func TestHandleSessionList_Empty(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestHandleSessionStopAll_Empty(t *testing.T) {
	t.Parallel()
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

func TestHandleSessionTail(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(text, `"next_cursor":"15"`) {
		t.Errorf("expected next_cursor 15, got: %s", text)
	}
}

func TestHandleSessionTailNoCursor(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(text, `"lines_returned":3`) {
		t.Errorf("expected 3 lines returned, got: %s", text)
	}
	// Should contain e, f, g but not a, b
	if strings.Contains(text, `"a"`) {
		t.Errorf("should not contain early lines, got: %s", text)
	}
}

func TestHandleSessionTailWithCursor(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(text, `"lines_returned":2`) {
		t.Errorf("expected 2 new lines, got: %s", text)
	}
	if !strings.Contains(text, "line4") || !strings.Contains(text, "line5") {
		t.Errorf("expected line4 and line5, got: %s", text)
	}
}

func TestHandleSessionTail_NotFound(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestHandleSessionErrorsClassification(t *testing.T) {
	t.Parallel()
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

func TestResolveSnapshotRepo_EmptyRepos(t *testing.T) {
	t.Parallel()
	result := resolveSnapshotRepo(nil, "", func(string) *model.Repo { return nil })
	if result != nil {
		t.Errorf("expected nil for empty repo list, got %v", result)
	}
}

func TestHandleSessionErrors_SeverityFilter(t *testing.T) {
	t.Parallel()
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
	if !strings.Contains(text, `"total_errors":1`) {
		t.Errorf("expected 1 error after filter, got: %s", text)
	}
	// The filtered errors should all be critical
	if !strings.Contains(text, `"session_error"`) {
		t.Errorf("expected session_error in filtered results, got: %s", text)
	}
}

// --- resolveSnapshotRepo pressure tests (FINDING-148/268) ---

func TestResolveSnapshotRepo_ExplicitParamThreeRepos(t *testing.T) {
	t.Parallel()

	repos := []*model.Repo{
		{Name: "alpha", Path: "/repos/alpha"},
		{Name: "beta", Path: "/repos/beta"},
		{Name: "zeta", Path: "/repos/zeta"},
	}

	findRepo := func(name string) *model.Repo {
		for _, r := range repos {
			if r.Name == name {
				return r
			}
		}
		return nil
	}

	got := resolveSnapshotRepo(repos, "beta", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}
	if got.Name != "beta" {
		t.Errorf("expected beta, got %s", got.Name)
	}
	// Must NOT return repos[0] when explicit param matches a different repo
	if got == repos[0] {
		t.Error("returned repos[0] (alpha) instead of the explicitly requested beta")
	}
}

func TestResolveSnapshotRepo_CWDFallback(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	// Create repos where one path is an ancestor of CWD
	repos := []*model.Repo{
		{Name: "unrelated", Path: "/some/unrelated/path"},
		{Name: "cwd-match", Path: cwd},
	}

	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}
	if got.Name != "cwd-match" {
		t.Errorf("expected cwd-match, got %s", got.Name)
	}
}

func TestResolveSnapshotRepo_FallbackToFirstThreeRepos(t *testing.T) {
	t.Parallel()

	repos := []*model.Repo{
		{Name: "first", Path: "/nonexistent/path/alpha"},
		{Name: "second", Path: "/nonexistent/path/beta"},
		{Name: "third", Path: "/nonexistent/path/gamma"},
	}

	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo, got nil (should not panic)")
	}
	if got.Name != "first" {
		t.Errorf("expected fallback to first repo, got %s", got.Name)
	}
	if got != repos[0] {
		t.Error("expected exact repos[0] reference")
	}
}

// TestResolveSnapshotRepo_PrefersRalphglasses verifies QW-7: when no CWD match
// or explicit param is given, the resolver prefers a repo named "ralphglasses"
// over falling back to the first repo (which could be "claudekit").
func TestResolveSnapshotRepo_PrefersRalphglasses(t *testing.T) {
	t.Parallel()

	repos := []*model.Repo{
		{Name: "claudekit", Path: "/repos/claudekit"},
		{Name: "ralphglasses", Path: "/repos/ralphglasses"},
		{Name: "other", Path: "/repos/other"},
	}

	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}
	if got.Name != "ralphglasses" {
		t.Errorf("expected ralphglasses, got %s (QW-7: snapshots should not save to claudekit)", got.Name)
	}
}

// TestResolveSnapshotRepo_PrefersRalphDir verifies QW-7: when no "ralphglasses"
// repo exists, prefer a repo with a .ralph/ directory.
func TestResolveSnapshotRepo_PrefersRalphDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	repoA := filepath.Join(dir, "repoA")
	repoB := filepath.Join(dir, "repoB")
	_ = os.MkdirAll(repoA, 0o755)
	_ = os.MkdirAll(filepath.Join(repoB, ".ralph"), 0o755)

	repos := []*model.Repo{
		{Name: "repoA", Path: repoA},
		{Name: "repoB", Path: repoB},
	}

	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}
	if got.Name != "repoB" {
		t.Errorf("expected repoB (has .ralph/), got %s", got.Name)
	}
}

// TestResolveSnapshotRepo_NestedPathBoundary verifies that path-separator
// boundary checking prevents /repos/foo from matching CWD /repos/foobar.
// This is the fix for FINDING-268.
func TestResolveSnapshotRepo_NestedPathBoundary(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	// Create repos where one is a prefix-substring of CWD but NOT an ancestor.
	// For example, if CWD is /a/b/cdef, then /a/b/cd should NOT match,
	// but /a/b should match.
	parent := filepath.Dir(cwd)

	repos := []*model.Repo{
		{Name: "prefix-only", Path: cwd + "extra"},  // longer than CWD, won't match
		{Name: "parent", Path: parent},               // proper ancestor of CWD
		{Name: "exact", Path: cwd},                   // exact match
	}

	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}
	// Should pick "exact" (longest match) not "prefix-only"
	if got.Name != "exact" {
		t.Errorf("expected exact match (longest path), got %s", got.Name)
	}
}

// TestResolveSnapshotRepo_LongestPathWins verifies that nested repos resolve
// to the longest matching path (deepest ancestor wins).
func TestResolveSnapshotRepo_LongestPathWins(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}

	parent := filepath.Dir(cwd)
	grandparent := filepath.Dir(parent)

	repos := []*model.Repo{
		{Name: "grandparent", Path: grandparent},
		{Name: "parent", Path: parent},
		{Name: "exact", Path: cwd},
	}

	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}
	if got.Name != "exact" {
		t.Errorf("expected deepest match 'exact', got %s", got.Name)
	}
}

// TestResolveSnapshotRepo_PathNotInClaudekitBase verifies QW-7 (FINDING-148/268):
// the resolved snapshot base path must be inside the ralphglasses repo root,
// not inside an unrelated repo such as claudekit.
func TestResolveSnapshotRepo_PathNotInClaudekitBase(t *testing.T) {
	t.Parallel()

	repos := []*model.Repo{
		{Name: "claudekit", Path: "/repos/claudekit"},
		{Name: "ralphglasses", Path: "/repos/ralphglasses"},
	}
	findRepo := func(_ string) *model.Repo { return nil }

	got := resolveSnapshotRepo(repos, "", findRepo)
	if got == nil {
		t.Fatal("expected non-nil repo")
	}

	snapshotBase := filepath.Join(got.Path, ".ralph", "snapshots")

	// The resolved base must be inside ralphglasses, not claudekit.
	if !strings.HasPrefix(snapshotBase, "/repos/ralphglasses") {
		t.Errorf("QW-7: snapshot base %q must be under ralphglasses, not claudekit", snapshotBase)
	}
	if strings.HasPrefix(snapshotBase, "/repos/claudekit") {
		t.Errorf("QW-7: snapshot base %q must NOT be under claudekit", snapshotBase)
	}
}

// TestResolveSnapshotRepo_Idempotent verifies that calling resolveSnapshotRepo
// twice with identical inputs returns the same repo (deterministic resolution).
func TestResolveSnapshotRepo_Idempotent(t *testing.T) {
	t.Parallel()

	repos := []*model.Repo{
		{Name: "claudekit", Path: "/repos/claudekit"},
		{Name: "ralphglasses", Path: "/repos/ralphglasses"},
		{Name: "other", Path: "/repos/other"},
	}
	findRepo := func(_ string) *model.Repo { return nil }

	first := resolveSnapshotRepo(repos, "", findRepo)
	second := resolveSnapshotRepo(repos, "", findRepo)

	if first == nil || second == nil {
		t.Fatal("expected non-nil results from both calls")
	}
	if first != second {
		t.Errorf("idempotency violated: first call returned %q, second returned %q", first.Name, second.Name)
	}
}
