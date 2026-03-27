package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestHandleAnomalyDetect_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleAnomalyDetect(context.Background(), makeRequest(map[string]any{}))
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

func TestHandleAnomalyDetect_MissingMetric(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleAnomalyDetect(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing metric")
	}
	text := getResultText(result)
	if !strings.Contains(text, "metric name required") {
		t.Errorf("expected 'metric name required' in error, got: %s", text)
	}
}

func TestHandleAnomalyDetect_InvalidMetric(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleAnomalyDetect(context.Background(), makeRequest(map[string]any{
		"repo":   "test",
		"metric": "bogus",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid metric")
	}
	text := getResultText(result)
	if !strings.Contains(text, "unknown metric") {
		t.Errorf("expected 'unknown metric' in error, got: %s", text)
	}
	// Should mention at least one valid metric name.
	if !strings.Contains(text, "total_cost_usd") {
		t.Errorf("expected valid metric names in error, got: %s", text)
	}
}

func TestHandleAnomalyDetect_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleAnomalyDetect(context.Background(), makeRequest(map[string]any{
		"repo":   "nonexistent",
		"metric": "total_cost_usd",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleAnomalyDetect", result, "REPO_NOT_FOUND")
}

func TestHandleAnomalyDetect_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleAnomalyDetect(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"metric": "total_cost_usd",
		"hours":  float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) && !strings.Contains(text, "no observations") {
		t.Errorf("expected empty result or 'no observations' message, got: %s", text)
	}
}
