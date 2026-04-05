package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestHandlePromptDJRoute_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handlePromptDJRoute(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt is required") {
		t.Errorf("expected 'prompt is required', got: %s", text)
	}
}

func TestHandlePromptDJDispatch_MissingParams(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "missing_both", args: map[string]any{}},
		{name: "missing_repo", args: map[string]any{"prompt": "do something"}},
		{name: "missing_prompt", args: map[string]any{"repo": "my-repo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &Server{}
			result, err := srv.handlePromptDJDispatch(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error result for missing params")
			}
			text := getResultText(result)
			if !strings.Contains(text, "prompt and repo are required") {
				t.Errorf("expected 'prompt and repo are required', got: %s", text)
			}
		})
	}
}

func TestHandlePromptDJFeedback_MissingParams(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handlePromptDJFeedback(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing params")
	}
}

func TestHandlePromptDJSimilar_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handlePromptDJSimilar(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
}

func TestHandlePromptDJSuggest_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handlePromptDJSuggest(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
}
