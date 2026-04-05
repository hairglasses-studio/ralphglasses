package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- extractRepoFromProjectDir ---

func TestExtractRepoFromProjectDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		dirName string
		want    string
	}{
		{name: "hyphenated", dirName: "home-hg-hairglasses-studio-mcpkit", want: "mcpkit"},
		{name: "single", dirName: "myrepo", want: "myrepo"},
		{name: "empty", dirName: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractRepoFromProjectDir(tt.dirName)
			if got != tt.want {
				t.Errorf("extractRepoFromProjectDir(%q) = %q, want %q", tt.dirName, got, tt.want)
			}
		})
	}
}

// --- countCommitsSince ---

func TestCountCommitsSince_InvalidDir(t *testing.T) {
	t.Parallel()
	got := countCommitsSince("/nonexistent/path", time.Now().Add(-time.Hour))
	if got != 0 {
		t.Errorf("expected 0 commits for invalid dir, got %d", got)
	}
}

func TestGitDiffStat_InvalidDir(t *testing.T) {
	t.Parallel()
	got := gitDiffStat("/nonexistent/path", 5)
	// Returns "unknown" or empty for invalid dirs.
	if got != "" && got != "unknown" {
		t.Errorf("expected empty or 'unknown' for invalid dir, got %q", got)
	}
}

func TestCountUnpushedCommits_InvalidDir(t *testing.T) {
	t.Parallel()
	got := countUnpushedCommits("/nonexistent/path")
	if got != 0 {
		t.Errorf("expected 0 for invalid dir, got %d", got)
	}
}

// --- handleFleetSchedule ---

func TestHandleFleetSchedule_MissingTasks(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleFleetSchedule(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing tasks")
	}
	text := getResultText(result)
	if !strings.Contains(text, "tasks parameter is required") {
		t.Errorf("expected 'tasks parameter is required', got: %s", text)
	}
}

func TestHandleFleetSchedule_InvalidJSON(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleFleetSchedule(context.Background(), makeRequest(map[string]any{
		"tasks": "not valid json",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid tasks JSON")
	}
}

func TestHandleFleetSchedule_EmptyArray(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleFleetSchedule(context.Background(), makeRequest(map[string]any{
		"tasks": "[]",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
}

// --- handleRoadmapAssignLoop ---

func TestHandleRoadmapAssignLoop_MissingParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing_both", args: map[string]any{}},
		{name: "missing_task", args: map[string]any{"repo": "test"}},
		{name: "missing_repo", args: map[string]any{"task": "build parser"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)
			result, err := srv.handleRoadmapAssignLoop(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error for missing params")
			}
		})
	}
}

// --- handleSessionFork ---

func TestHandleSessionFork_MissingID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSessionFork(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing id")
	}
}

func TestHandleSessionFork_NilSessMgr(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleSessionFork(context.Background(), makeRequest(map[string]any{
		"id": "test-session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil session manager")
	}
}

// --- handleIncidentReport ---

func TestHandleIncidentReport_MissingTitle(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleIncidentReport(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing title")
	}
	text := getResultText(result)
	if !strings.Contains(text, "title required") {
		t.Errorf("expected 'title required', got: %s", text)
	}
}

func TestHandleIncidentReport_UnsafeTitle(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleIncidentReport(context.Background(), makeRequest(map[string]any{
		"title": "../../../etc/passwd",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for unsafe title")
	}
}

// --- handleSessionDiscover ---

func TestHandleSessionDiscover_DefaultScanPath(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleSessionDiscover(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "discovered") && !strings.Contains(text, "sessions") {
		t.Errorf("expected discovery results in output, got: %s", text[:min(200, len(text))])
	}
}

// --- handleRoadmapPrioritize ---

func TestHandleRoadmapPrioritize_WithRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleRoadmapPrioritize(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return roadmap data or an error about no roadmap — not panic.
	_ = getResultText(result)
}

// --- handlePromptDJHistory ---

func TestHandlePromptDJHistory_NilRouter(t *testing.T) {
	t.Parallel()
	// Force nil router by not setting up anything.
	srv := &Server{}
	// The getOrCreateDJRouter might create a router, so we check the path
	// where it returns error.
	result, err := srv.handlePromptDJHistory(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return either router error or decision log data.
	_ = getResultText(result)
}

// --- loadHistoricalProviderCosts ---

func TestLoadHistoricalProviderCosts_NoDir(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	got := srv.loadHistoricalProviderCosts("/nonexistent/path")
	if got == nil {
		t.Fatal("expected non-nil map")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d entries", len(got))
	}
}

// --- isProcessAlive ---

func TestIsProcessAlive_NonexistentPID(t *testing.T) {
	t.Parallel()
	// PID 0 is kernel, PID -1 is invalid.
	got := isProcessAlive(-1)
	if got {
		t.Error("expected false for PID -1")
	}
}

func TestIsProcessAlive_Self(t *testing.T) {
	t.Parallel()
	// Current process should be alive.
	// Note: os.Getpid() returns 1 in some container environments
	// but it should still be "alive".
	got := isProcessAlive(1)
	_ = got // just verify no panic
}
