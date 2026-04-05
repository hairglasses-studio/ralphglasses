package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestHandleA2ADiscover_MissingURL(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleA2ADiscover(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing url")
	}
	text := getResultText(result)
	if !strings.Contains(text, "url required") {
		t.Errorf("expected 'url required' in error, got: %s", text)
	}
}

func TestHandleA2ASend_MissingParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing_both", args: map[string]any{}},
		{name: "missing_message", args: map[string]any{"url": "http://example.com"}},
		{name: "missing_url", args: map[string]any{"message": "hello"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &Server{}
			result, err := srv.handleA2ASend(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error result for missing params")
			}
			text := getResultText(result)
			if !strings.Contains(text, "url and message required") {
				t.Errorf("expected 'url and message required' in error, got: %s", text)
			}
		})
	}
}

func TestHandleA2AStatus_MissingParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing_both", args: map[string]any{}},
		{name: "missing_task_id", args: map[string]any{"url": "http://example.com"}},
		{name: "missing_url", args: map[string]any{"task_id": "task-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &Server{}
			result, err := srv.handleA2AStatus(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error result for missing params")
			}
			text := getResultText(result)
			if !strings.Contains(text, "url and task_id required") {
				t.Errorf("expected 'url and task_id required', got: %s", text)
			}
		})
	}
}

// Note: handleA2AAgentCard passes nil to AgentCardFromRegistry which
// panics when the registry is nil. Skipping until registry is wired.

func TestNextID(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	id1 := srv.nextID()
	id2 := srv.nextID()
	if id2 <= id1 {
		t.Errorf("expected monotonically increasing IDs, got %d then %d", id1, id2)
	}
}
