package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleLoopAwaitMissingID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopAwait(context.Background(), makeRequest(map[string]any{
		"type": "session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing id")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopAwaitMissingType(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopAwait(context.Background(), makeRequest(map[string]any{
		"id": "some-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing type")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopAwaitTimeout(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	// Register a session that stays in running state so await must time out.
	sess := &session.Session{
		ID:           "running-sess",
		Status:       session.StatusRunning,
		RepoName:     "test-repo",
		Provider:     session.ProviderClaude,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	start := time.Now()
	result, err := srv.handleLoopAwait(context.Background(), makeRequest(map[string]any{
		"id":                    "running-sess",
		"type":                  "session",
		"timeout_seconds":       float64(1),
		"poll_interval_seconds": float64(5), // min is 5, but timeout fires first
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	if data["status"] != "timeout" {
		t.Fatalf("expected status=timeout, got %v", data["status"])
	}
	if elapsed > 5*time.Second {
		t.Fatalf("await took too long (%v), expected ~1s timeout", elapsed)
	}
}

func TestHandleLoopPollUnknownSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopPoll(context.Background(), makeRequest(map[string]any{
		"id":   "nonexistent-id",
		"type": "session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected graceful response, got error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	if data["status"] != "not_found" {
		t.Fatalf("expected status=not_found, got %v", data["status"])
	}
}

func TestHandleLoopPollInvalidType(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopPoll(context.Background(), makeRequest(map[string]any{
		"id":   "some-id",
		"type": "invalid",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid type")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

// --- isTerminalSessionStatus unit tests ---

func TestIsTerminalSessionStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status   session.SessionStatus
		terminal bool
	}{
		{session.StatusCompleted, true},
		{session.StatusStopped, true},
		{session.StatusErrored, true},
		{session.StatusRunning, false},
		{session.StatusLaunching, false},
		{session.SessionStatus("unknown"), false},
	}
	for _, tc := range cases {
		if got := isTerminalSessionStatus(tc.status); got != tc.terminal {
			t.Errorf("isTerminalSessionStatus(%q) = %v, want %v", tc.status, got, tc.terminal)
		}
	}
}

// --- isTerminalLoopStatus unit tests ---

func TestIsTerminalLoopStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status   string
		terminal bool
	}{
		{"completed", true},
		{"stopped", true},
		{"failed", true},
		{"idle", true},
		{"converged", true},
		{"running", false},
		{"pending", false},
		{"paused", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		if got := isTerminalLoopStatus(tc.status); got != tc.terminal {
			t.Errorf("isTerminalLoopStatus(%q) = %v, want %v", tc.status, got, tc.terminal)
		}
	}
}

// --- checkAwaitStatus with completed session ---

func TestCheckAwaitStatus_CompletedSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	sess := &session.Session{
		ID:           "done-sess",
		Status:       session.StatusCompleted,
		RepoName:     "test-repo",
		Provider:     session.ProviderClaude,
		SpentUSD:     1.5,
		TurnCount:    5,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	result, done := srv.checkAwaitStatus("done-sess", "session", time.Now().Add(-time.Second))
	if !done {
		t.Fatal("expected done=true for completed session")
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data["status"] != "completed" {
		t.Errorf("expected status=completed, got %v", data["status"])
	}
}

func TestCheckAwaitStatus_ErroredSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	sess := &session.Session{
		ID:           "err-sess",
		Status:       session.StatusErrored,
		RepoName:     "test-repo",
		Provider:     session.ProviderClaude,
		Error:        "something went wrong",
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	result, done := srv.checkAwaitStatus("err-sess", "session", time.Now().Add(-time.Second))
	if !done {
		t.Fatal("expected done=true for errored session")
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data["status"] != "failed" {
		t.Errorf("expected status=failed for errored session, got %v", data["status"])
	}
}

func TestCheckAwaitStatus_NotFoundSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, done := srv.checkAwaitStatus("no-such-id", "session", time.Now())
	if !done {
		t.Fatal("expected done=true for missing session")
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data["status"] != "completed" {
		t.Errorf("expected status=completed for not-found, got %v", data["status"])
	}
}

func TestCheckAwaitStatus_RunningSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	sess := &session.Session{
		ID:           "run-sess",
		Status:       session.StatusRunning,
		RepoName:     "test-repo",
		Provider:     session.ProviderClaude,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	_, done := srv.checkAwaitStatus("run-sess", "session", time.Now())
	if done {
		t.Fatal("expected done=false for running session")
	}
}

// --- checkAwaitStatus with loop type (not found) ---

func TestCheckAwaitStatus_NotFoundLoop(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, done := srv.checkAwaitStatus("no-such-loop", "loop", time.Now())
	if !done {
		t.Fatal("expected done=true for missing loop")
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if data["status"] != "completed" {
		t.Errorf("expected status=completed for not-found loop, got %v", data["status"])
	}
}

// --- collectStatus ---

func TestCollectStatus_SessionFound(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	sess := &session.Session{
		ID:           "cs-sess",
		Status:       session.StatusRunning,
		RepoName:     "my-repo",
		Provider:     session.ProviderGemini,
		SpentUSD:     0.5,
		TurnCount:    3,
		Error:        "some error",
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	state := srv.collectStatus("cs-sess", "session")
	if state["status"] != string(session.StatusRunning) {
		t.Errorf("expected running, got %v", state["status"])
	}
	if state["type"] != "session" {
		t.Errorf("expected type=session, got %v", state["type"])
	}
	if state["error"] != "some error" {
		t.Errorf("expected error field, got %v", state["error"])
	}
	if state["provider"] != string(session.ProviderGemini) {
		t.Errorf("expected gemini provider, got %v", state["provider"])
	}
}

func TestCollectStatus_SessionNotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	state := srv.collectStatus("missing", "session")
	if state["status"] != "not_found" {
		t.Errorf("expected not_found, got %v", state["status"])
	}
}

func TestCollectStatus_LoopNotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	state := srv.collectStatus("missing-loop", "loop")
	if state["status"] != "not_found" {
		t.Errorf("expected not_found, got %v", state["status"])
	}
}

// --- handleLoopPoll with existing session ---

func TestHandleLoopPoll_ExistingSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	sess := &session.Session{
		ID:           "poll-sess",
		Status:       session.StatusRunning,
		RepoName:     "test-repo",
		Provider:     session.ProviderClaude,
		SpentUSD:     2.0,
		TurnCount:    7,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	result, err := srv.handleLoopPoll(context.Background(), makeRequest(map[string]any{
		"id":   "poll-sess",
		"type": "session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if data["status"] != "running" {
		t.Errorf("expected running, got %v", data["status"])
	}
	if data["type"] != "session" {
		t.Errorf("expected type=session, got %v", data["type"])
	}
}

func TestHandleLoopPoll_MissingID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopPoll(context.Background(), makeRequest(map[string]any{
		"type": "session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing id")
	}
}

func TestHandleLoopPoll_UnknownLoop(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopPoll(context.Background(), makeRequest(map[string]any{
		"id":   "no-loop",
		"type": "loop",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if data["status"] != "not_found" {
		t.Errorf("expected not_found, got %v", data["status"])
	}
}

// --- handleLoopAwait with invalid type ---

func TestHandleLoopAwaitInvalidType(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	result, err := srv.handleLoopAwait(context.Background(), makeRequest(map[string]any{
		"id":   "some-id",
		"type": "bogus",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid type")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

// --- handleLoopAwait immediately completed ---

func TestHandleLoopAwaitCompletedSession(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}

	sess := &session.Session{
		ID:           "await-done",
		Status:       session.StatusCompleted,
		RepoName:     "test-repo",
		Provider:     session.ProviderClaude,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}
	srv.SessMgr.AddSessionForTesting(sess)

	start := time.Now()
	result, err := srv.handleLoopAwait(context.Background(), makeRequest(map[string]any{
		"id":              "await-done",
		"type":            "session",
		"timeout_seconds": float64(30),
	}))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if data["status"] != "completed" {
		t.Errorf("expected completed, got %v", data["status"])
	}
	// Should return immediately, not wait for timeout
	if elapsed > 2*time.Second {
		t.Errorf("should return immediately for completed session, took %v", elapsed)
	}
}
