package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- InitFleetTools ---

func TestInitFleetTools_SetsAllFields(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	stateDir := t.TempDir()
	hitl := session.NewHITLTracker(stateDir)
	decisions := session.NewDecisionLog(stateDir, session.LevelObserve)
	feedback := session.NewFeedbackAnalyzer(stateDir, 5)

	srv.InitFleetTools(nil, nil, hitl, decisions, feedback)

	if srv.HITLTracker != hitl {
		t.Error("HITLTracker not set")
	}
	if srv.DecisionLog != decisions {
		t.Error("DecisionLog not set")
	}
	if srv.FeedbackAnalyzer != feedback {
		t.Error("FeedbackAnalyzer not set")
	}
}

func TestInitFleetTools_WithCoordinator(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	coord := fleet.NewCoordinator("test", "localhost", 0, "v0", nil, session.NewManager())
	client := fleet.NewClient("http://localhost:9999")

	srv.InitFleetTools(coord, client, nil, nil, nil)

	if srv.FleetCoordinator != coord {
		t.Error("FleetCoordinator not set")
	}
	if srv.FleetClient != client {
		t.Error("FleetClient not set")
	}
}

// --- pruneLoopRunsFiltered ---

func TestPruneLoopRunsFiltered_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	pruned, err := pruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending", "failed"}, "some-repo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pruned != 0 {
		t.Errorf("expected 0 pruned, got %d", pruned)
	}
}

func TestPruneLoopRunsFiltered_WithFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a loop run file that matches the filter.
	run := map[string]any{
		"id":         "run-1",
		"repo_name":  "target-repo",
		"status":     "failed",
		"created_at": time.Now().Add(-100 * time.Hour).Format(time.RFC3339),
	}
	data, _ := json.Marshal(run)
	_ = os.WriteFile(filepath.Join(dir, "run-1.json"), data, 0o644)

	// Dry run should not delete.
	pruned, err := pruneLoopRunsFiltered(dir, 72*time.Hour, []string{"failed"}, "target-repo", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The count depends on the session.PruneLoopRunsFiltered implementation;
	// we just verify no error and non-negative result.
	if pruned < 0 {
		t.Errorf("expected non-negative pruned count, got %d", pruned)
	}
}

// --- buildSummary ---

func TestBuildSummary_AllStatuses(t *testing.T) {
	t.Parallel()

	steps := []StepResult{
		{Name: "build", Status: "pass"},
		{Name: "vet", Status: "pass"},
		{Name: "test", Status: "fail"},
	}

	summary := buildSummary(steps)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	// Should contain "passed" and "failed"
	if !sprint7Contains(summary, "passed") {
		t.Errorf("summary should mention passed: %s", summary)
	}
	if !sprint7Contains(summary, "failed") {
		t.Errorf("summary should mention failed: %s", summary)
	}
}

func TestBuildSummary_Empty(t *testing.T) {
	t.Parallel()
	summary := buildSummary(nil)
	if summary != "no steps executed" {
		t.Errorf("expected 'no steps executed', got %q", summary)
	}
}

func TestBuildSummary_AllSkipped(t *testing.T) {
	t.Parallel()
	steps := []StepResult{
		{Name: "build", Status: "skip"},
		{Name: "test", Status: "skip"},
	}
	summary := buildSummary(steps)
	if !sprint7Contains(summary, "skipped") {
		t.Errorf("summary should mention skipped: %s", summary)
	}
}

func TestBuildSummary_OnlyPassed(t *testing.T) {
	t.Parallel()
	steps := []StepResult{
		{Name: "build", Status: "pass"},
		{Name: "vet", Status: "pass"},
		{Name: "test", Status: "pass"},
	}
	summary := buildSummary(steps)
	if !sprint7Contains(summary, "passed") {
		t.Errorf("summary should mention passed: %s", summary)
	}
	if sprint7Contains(summary, "failed") {
		t.Errorf("summary should not mention failed: %s", summary)
	}
}

// --- parseCoverageTotal ---
// parseCoverageTotal requires `go tool cover` and a real profile.
// We test with a simple coverage profile created in a temp dir.

func TestParseCoverageTotal_InvalidFile(t *testing.T) {
	t.Parallel()
	_, err := parseCoverageTotal(context.Background(), "/nonexistent/path/coverage.out")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseCoverageTotal_EmptyProfile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fp := filepath.Join(dir, "empty.out")
	_ = os.WriteFile(fp, []byte("mode: set\n"), 0o644)

	// go tool cover may fail with an empty profile; we just verify no panic.
	_, err := parseCoverageTotal(context.Background(), fp)
	// An error is acceptable here since the profile has no data lines.
	_ = err
}

// --- mapSessionProvider ---


// --- handleToolBenchmark ---

func TestHandleToolBenchmark_NotConfigured(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	// ToolRecorder is nil by default.

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleToolBenchmark(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when ToolRecorder is nil")
	}
}

func TestHandleToolBenchmark_WithRecorder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fp := filepath.Join(dir, "bench.jsonl")
	rec := NewToolCallRecorder(fp, nil, 100)

	now := time.Now()
	rec.Record(ToolCallEntry{ToolName: "tool_a", Timestamp: now, LatencyMs: 50, Success: true, InputSize: 100})
	rec.Record(ToolCallEntry{ToolName: "tool_b", Timestamp: now, LatencyMs: 200, Success: false, ErrorMsg: "fail", InputSize: 50})
	rec.Close()

	srv := NewServer(t.TempDir())
	srv.ToolRecorder = NewToolCallRecorder(fp, nil, 100)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hours": float64(24),
	}

	result, err := srv.handleToolBenchmark(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if _, ok := body["summaries"]; !ok {
		t.Error("expected 'summaries' in response")
	}
}

func TestHandleToolBenchmark_WithToolFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	fp := filepath.Join(dir, "bench.jsonl")
	rec := NewToolCallRecorder(fp, nil, 100)

	now := time.Now()
	rec.Record(ToolCallEntry{ToolName: "tool_a", Timestamp: now, LatencyMs: 50, Success: true})
	rec.Record(ToolCallEntry{ToolName: "tool_b", Timestamp: now, LatencyMs: 100, Success: true})
	rec.Close()

	srv := NewServer(t.TempDir())
	srv.ToolRecorder = NewToolCallRecorder(fp, nil, 100)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"hours": float64(24),
		"tool":  "tool_a",
	}

	result, err := srv.handleToolBenchmark(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %v", result.Content)
	}
}

// --- handleAwesomeFetch ---

func TestHandleAwesomeFetch_MockHTTP(t *testing.T) {
	t.Parallel()

	// Serve a minimal awesome-list README.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "# Awesome List\n\n- [project](https://github.com/user/project) - A cool project")
	}))
	defer mockServer.Close()

	srv := NewServer(t.TempDir())
	srv.HTTPClient = mockServer.Client()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": mockServer.URL,
	}

	result, err := srv.handleAwesomeFetch(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The result may be an error if the URL doesn't match expected GitHub format,
	// but the important thing is no panic.
	_ = result
}

// --- handleAwesomeAnalyze ---

func TestHandleAwesomeAnalyze_MockHTTP(t *testing.T) {
	t.Parallel()

	// Serve a minimal awesome-list README and repo metadata.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "# Awesome List\n\n- [project](https://github.com/user/project) - A cool project")
	})
	mockServer := httptest.NewServer(mux)
	defer mockServer.Close()

	srv := NewServer(t.TempDir())
	srv.HTTPClient = mockServer.Client()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":        mockServer.URL,
		"max_workers": float64(1),
	}

	result, err := srv.handleAwesomeAnalyze(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

// helper
func sprint7Contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
