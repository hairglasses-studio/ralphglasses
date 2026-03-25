package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- prompt_analyze ---

func TestHandlePromptAnalyze(t *testing.T) {
	srv, _ := setupTestServer(t)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "valid prompt",
			args: map[string]any{"prompt": "Write a Go function that sorts a slice of integers using quicksort"},
			check: func(t *testing.T, text string) {
				if text == "" {
					t.Error("expected non-empty analysis")
				}
				// Should be valid JSON
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
			},
		},
		{
			name:    "missing prompt",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			args:    map[string]any{"prompt": ""},
			wantErr: true,
		},
		{
			name: "with task_type",
			args: map[string]any{"prompt": "Implement a REST API endpoint for user creation", "task_type": "code"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
				if tt, ok := m["task_type"].(string); ok {
					if tt != "code" {
						t.Errorf("task_type = %q, want %q", tt, "code")
					}
				}
			},
		},
		{
			name: "with target_provider",
			args: map[string]any{"prompt": "Build a CLI tool in Go", "target_provider": "gemini"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
				// Should have a score_report when target_provider is set
				if _, ok := m["score_report"]; !ok {
					t.Error("expected score_report when target_provider is set")
				}
			},
		},
		{
			name: "very long prompt",
			args: map[string]any{"prompt": strings.Repeat("Write tests for all functions. ", 100)},
			check: func(t *testing.T, text string) {
				if text == "" {
					t.Error("expected non-empty analysis for long prompt")
				}
			},
		},
		{
			name: "special characters",
			args: map[string]any{"prompt": "Handle <xml> tags & \"quotes\" in 'strings' with $variables and {braces}"},
			check: func(t *testing.T, text string) {
				if text == "" {
					t.Error("expected non-empty analysis for special chars prompt")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptAnalyze(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- prompt_enhance ---

func TestHandlePromptEnhance(t *testing.T) {
	srv, _ := setupTestServer(t)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "valid prompt local mode",
			args: map[string]any{"prompt": "Write a Go function that sorts integers"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
				if enhanced, ok := m["enhanced"].(string); !ok || enhanced == "" {
					t.Error("expected non-empty enhanced prompt")
				}
			},
		},
		{
			name:    "missing prompt",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			args:    map[string]any{"prompt": ""},
			wantErr: true,
		},
		{
			name: "with task_type",
			args: map[string]any{"prompt": "Fix the login bug", "task_type": "debug"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
			},
		},
		{
			name: "with target_provider gemini",
			args: map[string]any{"prompt": "Build a REST API", "target_provider": "gemini"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
			},
		},
		{
			name: "explicit local mode",
			args: map[string]any{"prompt": "Create a database schema", "mode": "local"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
			},
		},
		{
			name: "very long prompt",
			args: map[string]any{"prompt": strings.Repeat("Implement comprehensive error handling for all edge cases. ", 50)},
			check: func(t *testing.T, text string) {
				if text == "" {
					t.Error("expected non-empty result")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptEnhance(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- prompt_lint ---

func TestHandlePromptLint(t *testing.T) {
	srv, _ := setupTestServer(t)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "clean prompt",
			args: map[string]any{"prompt": "Write a Go function that sorts a slice of integers"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
				if _, ok := m["total"]; !ok {
					t.Error("expected total field")
				}
				if _, ok := m["findings"]; !ok {
					t.Error("expected findings field")
				}
				if _, ok := m["cache_checks"]; !ok {
					t.Error("expected cache_checks field")
				}
			},
		},
		{
			name:    "missing prompt",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			args:    map[string]any{"prompt": ""},
			wantErr: true,
		},
		{
			name: "prompt with lint issues - negative framing",
			args: map[string]any{"prompt": "NEVER do this. You MUST NOT use that. DO NOT forget to ALWAYS check."},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON response: %v", err)
				}
				total, _ := m["total"].(float64)
				if total == 0 {
					t.Error("expected lint findings for prompt with negative framing")
				}
			},
		},
		{
			name: "prompt with special characters",
			args: map[string]any{"prompt": "Handle <xml> & \"quotes\" in $variables {braces} [brackets]"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptLint(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- prompt_improve ---

func TestHandlePromptImprove_NoAPIKey(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Ensure no API keys are set for this test
	for _, key := range []string{"ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(key, "")
	}
	// Reset the engine so it picks up the empty env
	srv.Engine = nil
	srv.engineOnce = sync.Once{}

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name:    "missing prompt",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			args:    map[string]any{"prompt": ""},
			wantErr: true,
		},
		{
			name: "no api key default provider",
			args: map[string]any{"prompt": "Write a function"},
			check: func(t *testing.T, text string) {
				if !strings.Contains(text, "LLM not available") {
					t.Errorf("expected LLM not available error, got: %s", text)
				}
			},
			wantErr: true,
		},
		{
			name: "no api key gemini provider",
			args: map[string]any{"prompt": "Write a function", "provider": "gemini"},
			check: func(t *testing.T, text string) {
				if !strings.Contains(text, "GOOGLE_API_KEY") {
					t.Errorf("expected GOOGLE_API_KEY hint, got: %s", text)
				}
			},
			wantErr: true,
		},
		{
			name: "no api key openai provider",
			args: map[string]any{"prompt": "Write a function", "provider": "openai"},
			check: func(t *testing.T, text string) {
				if !strings.Contains(text, "OPENAI_API_KEY") {
					t.Errorf("expected OPENAI_API_KEY hint, got: %s", text)
				}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptImprove(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				if tt.check != nil {
					tt.check(t, text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- prompt_templates ---

func TestHandlePromptTemplates(t *testing.T) {
	srv, _ := setupTestServer(t)

	result, err := srv.handlePromptTemplates(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	if text == "" {
		t.Fatal("expected non-empty template list")
	}

	var templates []any
	if err := json.Unmarshal([]byte(text), &templates); err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}
	if len(templates) == 0 {
		t.Error("expected at least one template")
	}

	// Verify each template has expected fields
	for i, tmpl := range templates {
		m, ok := tmpl.(map[string]any)
		if !ok {
			t.Errorf("template[%d] is not an object", i)
			continue
		}
		if m["name"] == nil || m["name"] == "" {
			t.Errorf("template[%d] missing name", i)
		}
		if m["description"] == nil || m["description"] == "" {
			t.Errorf("template[%d] missing description", i)
		}
	}
}

// --- prompt_classify ---

func TestHandlePromptClassify(t *testing.T) {
	srv, _ := setupTestServer(t)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "code prompt",
			args: map[string]any{"prompt": "Write a Go function that implements binary search"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				if m["task_type"] == nil || m["task_type"] == "" {
					t.Error("expected non-empty task_type")
				}
			},
		},
		{
			name:    "missing prompt",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			args:    map[string]any{"prompt": ""},
			wantErr: true,
		},
		{
			name: "debug prompt",
			args: map[string]any{"prompt": "Fix the bug where the server crashes on startup with a nil pointer dereference"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				if m["task_type"] == nil || m["task_type"] == "" {
					t.Error("expected non-empty task_type")
				}
			},
		},
		{
			name: "special characters",
			args: map[string]any{"prompt": "Handle <xml> tags & \"quotes\" in $variables"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptClassify(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- prompt_should_enhance ---

func TestHandlePromptShouldEnhance(t *testing.T) {
	srv, _ := setupTestServer(t)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "long prompt should enhance",
			args: map[string]any{"prompt": "Write a comprehensive Go function that implements a binary search tree with insert, delete, and search operations"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				if _, ok := m["should_enhance"]; !ok {
					t.Error("expected should_enhance field")
				}
				if _, ok := m["reason"]; !ok {
					t.Error("expected reason field")
				}
			},
		},
		{
			name:    "missing prompt",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty prompt",
			args:    map[string]any{"prompt": ""},
			wantErr: true,
		},
		{
			name: "short conversational should not enhance",
			args: map[string]any{"prompt": "yes"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				shouldEnhance, ok := m["should_enhance"].(bool)
				if !ok {
					t.Fatal("should_enhance is not bool")
				}
				if shouldEnhance {
					t.Error("conversational prompt should not need enhancement")
				}
				reason, _ := m["reason"].(string)
				if reason == "" {
					t.Error("expected non-empty reason when should_enhance is false")
				}
			},
		},
		{
			name: "xml structured should not enhance",
			args: map[string]any{"prompt": "<instructions>Do something useful with the code base and make sure it works properly</instructions>"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				shouldEnhance, ok := m["should_enhance"].(bool)
				if !ok {
					t.Fatal("should_enhance is not bool")
				}
				if shouldEnhance {
					t.Error("XML-structured prompt should not need enhancement")
				}
			},
		},
		{
			name: "too short should not enhance",
			args: map[string]any{"prompt": "fix it"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				shouldEnhance, ok := m["should_enhance"].(bool)
				if !ok {
					t.Fatal("should_enhance is not bool")
				}
				if shouldEnhance {
					t.Error("very short prompt should not need enhancement")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptShouldEnhance(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- claudemd_check ---

func TestHandleClaudeMDCheck(t *testing.T) {
	srv, _ := setupTestServer(t)

	// Find the repo name from the test server
	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	repos := srv.Repos
	if len(repos) == 0 {
		t.Fatal("expected at least one repo after scan")
	}
	repoName := repos[0].Name
	repoPath := repos[0].Path

	// Write a CLAUDE.md to the test repo
	claudeMD := `# Test Project

## Build & Run

` + "```bash\ngo build ./...\n```" + `

## Key Patterns

- Use Go standard library where possible
- Follow idiomatic Go patterns
`
	if err := os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "valid repo with CLAUDE.md",
			args: map[string]any{"repo": repoName},
			check: func(t *testing.T, text string) {
				// Should be valid JSON array
				var results []any
				if err := json.Unmarshal([]byte(text), &results); err != nil {
					t.Errorf("expected JSON array: %v", err)
				}
			},
		},
		{
			name:    "missing repo",
			args:    map[string]any{},
			wantErr: true,
		},
		{
			name:    "empty repo",
			args:    map[string]any{"repo": ""},
			wantErr: true,
		},
		{
			name:    "nonexistent repo",
			args:    map[string]any{"repo": "this-repo-does-not-exist-xyz"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handleClaudeMDCheck(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

func TestHandleClaudeMDCheck_WithIssues(t *testing.T) {
	srv, _ := setupTestServer(t)

	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	repos := srv.Repos
	if len(repos) == 0 {
		t.Fatal("expected at least one repo after scan")
	}
	repoPath := repos[0].Path

	// Write a CLAUDE.md with overtrigger language and aggressive caps
	claudeMD := "CRITICAL: You MUST ALWAYS follow these rules.\n" +
		"NEVER do anything without checking first.\n" +
		"IMPORTANT: ALWAYS use the EXACT format shown below.\n" +
		strings.Repeat("This is a line of content.\n", 30)

	if err := os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		t.Fatalf("write CLAUDE.md: %v", err)
	}

	result, err := srv.handleClaudeMDCheck(context.Background(), makeRequest(map[string]any{"repo": repos[0].Name}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	text := getResultText(result)
	var results []map[string]any
	if err := json.Unmarshal([]byte(text), &results); err != nil {
		t.Fatalf("expected JSON array: %v", err)
	}

	// Should find overtrigger or aggressive caps issues
	if len(results) == 0 {
		t.Error("expected findings for CLAUDE.md with overtrigger language")
	}
}

// --- prompt_template_fill ---

func TestHandlePromptTemplateFill(t *testing.T) {
	srv, _ := setupTestServer(t)

	tests := []struct {
		name    string
		args    map[string]any
		wantErr bool
		check   func(t *testing.T, text string)
	}{
		{
			name: "valid template fill",
			args: map[string]any{
				"name": "troubleshoot",
				"vars": `{"system":"nginx","symptoms":"502 errors","context":"after deployment"}`,
			},
			check: func(t *testing.T, text string) {
				if text == "" {
					t.Error("expected non-empty filled template")
				}
				if !strings.Contains(text, "nginx") {
					t.Error("expected filled template to contain variable value 'nginx'")
				}
				if !strings.Contains(text, "502 errors") {
					t.Error("expected filled template to contain variable value '502 errors'")
				}
			},
		},
		{
			name:    "missing name",
			args:    map[string]any{"vars": `{"key":"value"}`},
			wantErr: true,
		},
		{
			name:    "missing vars",
			args:    map[string]any{"name": "troubleshoot"},
			wantErr: true,
		},
		{
			name:    "empty name",
			args:    map[string]any{"name": "", "vars": `{"key":"value"}`},
			wantErr: true,
		},
		{
			name:    "empty vars",
			args:    map[string]any{"name": "troubleshoot", "vars": ""},
			wantErr: true,
		},
		{
			name:    "nonexistent template",
			args:    map[string]any{"name": "does_not_exist_template", "vars": `{"key":"value"}`},
			wantErr: true,
		},
		{
			name:    "invalid JSON vars",
			args:    map[string]any{"name": "troubleshoot", "vars": "not json"},
			wantErr: true,
		},
		{
			name: "partial variables filled",
			args: map[string]any{
				"name": "troubleshoot",
				"vars": `{"system":"redis"}`,
			},
			check: func(t *testing.T, text string) {
				if !strings.Contains(text, "redis") {
					t.Error("expected filled template to contain 'redis'")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := srv.handlePromptTemplateFill(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected error result, got: %s", text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.check != nil {
				tt.check(t, text)
			}
		})
	}
}

// --- helper unit tests ---

func TestIsConversational(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"yes", true},
		{"no", true},
		{"ok", true},
		{"lgtm", true},
		{"ship it", true},
		{"Write a Go function", false},
		{"", false},
		{"YES", true}, // case insensitive
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isConversational(tt.input); got != tt.want {
				t.Errorf("isConversational(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestHasXMLStructure(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"<instructions>do something</instructions>", true},
		{"<role>assistant</role>", true},
		{"<system>you are helpful</system>", true},
		{"<prompt>test</prompt>", true},
		{"Write a Go function", false},
		{"", false},
		{"<div>html is not xml structure</div>", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := hasXMLStructure(tt.input); got != tt.want {
				t.Errorf("hasXMLStructure(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
