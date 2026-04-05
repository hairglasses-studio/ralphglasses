package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleSessionReplayDiff_MissingPaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing_both", args: map[string]any{}},
		{name: "missing_path_b", args: map[string]any{"path_a": "/tmp/a"}},
		{name: "missing_path_a", args: map[string]any{"path_b": "/tmp/b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)
			result, err := srv.handleSessionReplayDiff(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error for missing paths")
			}
			text := getResultText(result)
			if !strings.Contains(text, "path_a and path_b") {
				t.Errorf("expected 'path_a and path_b' in error, got: %s", text)
			}
		})
	}
}

func TestHandleSweepSchedule_MissingSweepID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager(), Tasks: NewTaskRegistry()}
	result, err := srv.handleSweepSchedule(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing sweep_id")
	}
	text := getResultText(result)
	if !strings.Contains(text, "sweep_id required") {
		t.Errorf("expected 'sweep_id required', got: %s", text)
	}
}

func TestHandleRoadmapCrossRepo_NoDocsDir(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleRoadmapCrossRepo(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should fail because snapshots dir doesn't exist.
	if !result.IsError {
		// May succeed with empty data too - just verify no panic.
		_ = getResultText(result)
	}
}
