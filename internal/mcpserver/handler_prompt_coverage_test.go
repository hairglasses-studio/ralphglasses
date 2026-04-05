package mcpserver

import (
	"context"
	"strings"
	"testing"
)

func TestHandlePromptEnhance_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptEnhance(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt required") {
		t.Errorf("expected 'prompt required', got: %s", text)
	}
}

func TestHandlePromptClassify_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptClassify(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
}

func TestHandlePromptClassify_Valid(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptClassify(context.Background(), makeRequest(map[string]any{
		"prompt": "Write a function that reverses a string in Go",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "task_type") {
		t.Errorf("expected 'task_type' in result, got: %s", text)
	}
}

func TestHandlePromptLint_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptLint(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt")
	}
}

func TestHandlePromptLint_Valid(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptLint(context.Background(), makeRequest(map[string]any{
		"prompt": "Fix the bug in parser.go where it panics on empty input. Return the diff.",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
}

func TestHasAPIKeyForProvider(t *testing.T) {
	t.Parallel()
	// This just tests the dispatch logic without setting env vars.
	// All should return false in a clean test environment.
	tests := []struct {
		name     string
		provider string
	}{
		{name: "gemini", provider: "gemini"},
		{name: "openai", provider: "openai"},
		{name: "anthropic", provider: "anthropic"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// We just verify it doesn't panic.
			_ = hasAPIKeyForProvider(tt.provider)
		})
	}
}
