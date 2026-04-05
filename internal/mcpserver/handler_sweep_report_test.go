package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleSweepReport_MissingSweepID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSweepReport(context.Background(), makeRequest(map[string]any{}))
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

func TestHandleSweepReport_NoSessions(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSweepReport(context.Background(), makeRequest(map[string]any{
		"sweep_id": "sweep-nonexistent",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	// Should return empty result.
	text := getResultText(result)
	if !strings.Contains(text, "sweep_sessions") {
		t.Errorf("expected 'sweep_sessions' in empty result, got: %s", text)
	}
}

func TestHandleSweepPush_MissingSweepID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSweepPush(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing sweep_id")
	}
}

func TestHandleSweepRetry_MissingSweepID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager(), Tasks: NewTaskRegistry()}
	result, err := srv.handleSweepRetry(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing sweep_id")
	}
}
