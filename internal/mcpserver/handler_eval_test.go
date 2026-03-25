package mcpserver

import (
	"context"
	"strings"
	"testing"
)

// --- handleEvalCounterfactual ---

func TestHandleEvalCounterfactual_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalCounterfactual(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo name required") {
		t.Errorf("expected 'repo name required' in error, got: %s", text)
	}
}

func TestHandleEvalCounterfactual_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalCounterfactual(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo not found") {
		t.Errorf("expected 'repo not found' in error, got: %s", text)
	}
}

// --- handleEvalABTest ---

func TestHandleEvalABTest_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo name required") {
		t.Errorf("expected 'repo name required' in error, got: %s", text)
	}
}

func TestHandleEvalABTest_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

// --- handleEvalChangepoints ---

func TestHandleEvalChangepoints_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalChangepoints(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo name required") {
		t.Errorf("expected 'repo name required' in error, got: %s", text)
	}
}

func TestHandleEvalChangepoints_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalChangepoints(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

// --- handleBanditStatus ---

func TestHandleBanditStatus_NilSessionManager(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	srv.SessMgr = nil

	result, err := srv.handleBanditStatus(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nil session manager")
	}
	text := getResultText(result)
	if !strings.Contains(text, "session manager not initialized") {
		t.Errorf("expected 'session manager not initialized' in error, got: %s", text)
	}
}

func TestHandleBanditStatus_NoCascadeRouter(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleBanditStatus(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return not_configured (not an error, just status).
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
}

// --- handleConfidenceCalibration ---

func TestHandleConfidenceCalibration_NilSessionManager(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	srv.SessMgr = nil

	result, err := srv.handleConfidenceCalibration(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nil session manager")
	}
	text := getResultText(result)
	if !strings.Contains(text, "session manager not initialized") {
		t.Errorf("expected 'session manager not initialized' in error, got: %s", text)
	}
}

func TestHandleConfidenceCalibration_NoCascadeRouter(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleConfidenceCalibration(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
}
