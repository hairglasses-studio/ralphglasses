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
