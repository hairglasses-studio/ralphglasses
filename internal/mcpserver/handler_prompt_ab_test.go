package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

func TestTruncatePrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "short", s: "hello", n: 10, want: "hello"},
		{name: "exact", s: "hello", n: 5, want: "hello"},
		{name: "truncated", s: "hello world", n: 8, want: "hello wo..."},
		{name: "empty", s: "", n: 10, want: ""},
		{name: "zero_limit", s: "hello", n: 0, want: "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncatePrompt(tt.s, tt.n)
			if got != tt.want {
				t.Errorf("truncatePrompt(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
			}
		})
	}
}

func TestCountSeverity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		lints    []enhancer.LintResult
		severity string
		want     int
	}{
		{
			name:     "empty",
			lints:    nil,
			severity: "warning",
			want:     0,
		},
		{
			name: "mixed",
			lints: []enhancer.LintResult{
				{Severity: "warning", Category: "a"},
				{Severity: "error", Category: "b"},
				{Severity: "warning", Category: "c"},
				{Severity: "info", Category: "d"},
			},
			severity: "warning",
			want:     2,
		},
		{
			name: "no_match",
			lints: []enhancer.LintResult{
				{Severity: "info", Category: "a"},
			},
			severity: "error",
			want:     0,
		},
		{
			name: "all_match",
			lints: []enhancer.LintResult{
				{Severity: "error", Category: "a"},
				{Severity: "error", Category: "b"},
			},
			severity: "error",
			want:     2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := countSeverity(tt.lints, tt.severity)
			if got != tt.want {
				t.Errorf("countSeverity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHandlePromptABTest_MissingPromptA(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptABTest(context.Background(), makeRequest(map[string]any{
		"prompt_b": "do something",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt_a")
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt_a required") {
		t.Errorf("expected 'prompt_a required', got: %s", text)
	}
}

func TestHandlePromptABTest_MissingPromptB(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handlePromptABTest(context.Background(), makeRequest(map[string]any{
		"prompt_a": "do something",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing prompt_b")
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt_b required") {
		t.Errorf("expected 'prompt_b required', got: %s", text)
	}
}
