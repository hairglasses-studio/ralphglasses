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
	t.Parallel()
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

// TestHandlePromptAnalyze_ScoreRange verifies QW-4 / FINDING-240: scores span a
// full dynamic range and do not cluster at 8-9/10.
func TestHandlePromptAnalyze_ScoreRange(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	type scoreCase struct {
		name      string
		prompt    string
		maxLegacy int // legacy score must be ≤ this
		minLegacy int // legacy score must be ≥ this
	}

	cases := []scoreCase{
		{
			name:      "poor prompt scores low",
			prompt:    "do it",
			maxLegacy: 4,
			minLegacy: 1,
		},
		{
			name:      "trivial prompt scores low",
			prompt:    "fix this",
			maxLegacy: 4,
			minLegacy: 1,
		},
		{
			name: "high quality prompt scores high",
			prompt: `<role>You are an expert Go developer with 10 years of experience.</role>

<context>
We are building a user management API in Go. The codebase uses the standard library
net/http package with chi router. This is because we want minimal dependencies.
</context>

<instructions>
Review the following function for error handling issues.
Focus on nil pointer dereferences and unchecked errors because these cause runtime panics.
Return exactly 5 issues, each in one sentence, sorted by severity.
</instructions>

<examples>
<example index="1">
Input: func getUser(id string) *User { return db.Find(id) }
Output: Missing nil check on db.Find return — will panic if user not found.
</example>
<example index="2">
Input: data, _ := json.Marshal(user)
Output: Ignoring json.Marshal error — will silently produce empty data on failure.
</example>
<example index="3">
Input: f, err := os.Open(path); defer f.Close()
Output: Defer before error check — will panic on nil file handle if Open fails.
</example>
</examples>

<output_format>
Return a numbered list of exactly 5 issues. Each issue should include:
1. The problematic code pattern
2. The risk (in 10 words or fewer)
3. The fix
</output_format>

<constraints>
- Only report real issues supported by the code, because false positives waste review time
- Distinguish severity levels (critical, warning, info) to help prioritize fixes
</constraints>`,
			minLegacy: 8,
			maxLegacy: 10,
		},
	}

	scores := make(map[string]int)
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result, err := srv.handlePromptAnalyze(context.Background(), makeRequest(map[string]any{"prompt": tc.prompt}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", getResultText(result))
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(getResultText(result)), &m); err != nil {
				t.Fatalf("expected JSON: %v", err)
			}
			score := int(m["score"].(float64))
			scores[tc.name] = score
			if score < tc.minLegacy || score > tc.maxLegacy {
				t.Errorf("score %d outside expected [%d, %d]", score, tc.minLegacy, tc.maxLegacy)
			}
		})
	}

	// Varied inputs must not produce identical scores
	t.Run("scores_are_not_all_equal", func(t *testing.T) {
		prompts := []string{
			"hello",
			"Write a Go function that parses JSON and returns a struct",
			"<role>Expert Go dev.</role>\n<instructions>Review code for bugs. Return 3 findings.</instructions>\n<context>Payment service.</context>\n<examples><example>Missing nil check.</example></examples>",
		}
		var allScores []int
		for _, p := range prompts {
			result, err := srv.handlePromptAnalyze(context.Background(), makeRequest(map[string]any{"prompt": p}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(getResultText(result)), &m); err != nil {
				t.Fatalf("expected JSON: %v", err)
			}
			allScores = append(allScores, int(m["score"].(float64)))
		}
		allSame := true
		for _, s := range allScores[1:] {
			if s != allScores[0] {
				allSame = false
				break
			}
		}
		if allSame {
			t.Errorf("all scores identical (%d) — expected varied scores for diverse prompts", allScores[0])
		}
	})
}

// --- prompt_enhance ---

func TestHandlePromptEnhance(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	// Cannot use t.Parallel() because t.Setenv is used below.
	srv, _ := setupTestServer(t)

	// Ensure no API keys are set for this test
	for _, key := range []string{"ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY", "OPENAI_API_KEY", "OLLAMA_API_KEY"} {
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
		{
			name: "ollama provider considered locally available",
			args: map[string]any{"prompt": "Write a function", "provider": "ollama"},
			check: func(t *testing.T, text string) {
				if strings.Contains(text, "OLLAMA_API_KEY") {
					t.Errorf("expected runtime improve failure, not missing-key hint: %s", text)
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

func TestHasAPIKeyForProvider_Ollama(t *testing.T) {
	t.Parallel()
	if !hasAPIKeyForProvider("ollama") {
		t.Fatal("expected ollama provider to be treated as locally available")
	}
}

// --- prompt_templates ---

func TestHandlePromptTemplates(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
				// Verify confidence is present and in range
				conf, ok := m["confidence"].(float64)
				if !ok {
					t.Fatal("expected confidence to be a float")
				}
				if conf < 0.0 || conf > 1.0 {
					t.Errorf("confidence %f not in [0,1]", conf)
				}
				// Verify alternatives is present as array
				if _, ok := m["alternatives"].([]any); !ok {
					t.Fatal("expected alternatives to be an array")
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
			name: "mixed signal prompt with alternatives",
			args: map[string]any{"prompt": "Review and fix the broken build pipeline code that crashes with an error"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				conf, ok := m["confidence"].(float64)
				if !ok {
					t.Fatal("expected confidence float")
				}
				if conf < 0.0 || conf > 1.0 {
					t.Errorf("confidence %f not in [0,1]", conf)
				}
				alts, ok := m["alternatives"].([]any)
				if !ok {
					t.Fatal("expected alternatives array")
				}
				if len(alts) == 0 {
					t.Error("expected non-empty alternatives for prompt with mixed signals")
				}
				for i, alt := range alts {
					am, ok := alt.(map[string]any)
					if !ok {
						t.Errorf("alternative[%d] is not an object", i)
						continue
					}
					if am["task_type"] == nil || am["task_type"] == "" {
						t.Errorf("alternative[%d] missing task_type", i)
					}
					ac, ok := am["confidence"].(float64)
					if !ok {
						t.Errorf("alternative[%d] missing confidence float", i)
					} else if ac < 0.0 || ac > 1.0 {
						t.Errorf("alternative[%d] confidence %f not in [0,1]", i, ac)
					}
				}
			},
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
				// Confidence should still be present
				if _, ok := m["confidence"].(float64); !ok {
					t.Error("expected confidence float")
				}
				// Alternatives should be present (may be empty array)
				if _, ok := m["alternatives"].([]any); !ok {
					t.Error("expected alternatives array")
				}
			},
		},
		{
			name: "general prompt with no keywords",
			args: map[string]any{"prompt": "Tell me about the weather today"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				if m["task_type"] != "general" {
					t.Errorf("expected task_type=general for ambiguous prompt, got %v", m["task_type"])
				}
				conf, ok := m["confidence"].(float64)
				if !ok {
					t.Fatal("expected confidence float")
				}
				if conf < 0.0 || conf > 1.0 {
					t.Errorf("confidence %f not in [0,1]", conf)
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
	t.Parallel()
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
				shouldEnhance, ok := m["should_enhance"].(bool)
				if !ok {
					t.Fatal("expected should_enhance bool")
				}
				if !shouldEnhance {
					t.Error("expected should_enhance to be true for this prompt")
				}
				reason, _ := m["reason"].(string)
				if reason == "" {
					t.Error("expected non-empty reason when should_enhance is true")
				}
				// Reason should contain score information
				if !strings.Contains(reason, "score") {
					t.Errorf("expected reason to contain score info, got: %s", reason)
				}
			},
		},
		{
			name: "low quality prompt should enhance with detailed reason",
			args: map[string]any{"prompt": "Make the code better and fix all the things that are wrong with it please"},
			check: func(t *testing.T, text string) {
				var m map[string]any
				if err := json.Unmarshal([]byte(text), &m); err != nil {
					t.Errorf("expected JSON: %v", err)
				}
				shouldEnhance, ok := m["should_enhance"].(bool)
				if !ok {
					t.Fatal("expected should_enhance bool")
				}
				if !shouldEnhance {
					t.Error("expected should_enhance to be true for low-quality prompt")
				}
				reason, _ := m["reason"].(string)
				if reason == "" {
					t.Error("expected non-empty reason for low-quality prompt")
				}
				// Reason should mention the score
				if !strings.Contains(reason, "/100") {
					t.Errorf("expected reason to contain score denominator, got: %s", reason)
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
			name: "xml structured with weak dimensions should enhance",
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
				if !shouldEnhance {
					t.Error("XML-structured prompt with weak dimensions should still need enhancement")
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
	t.Parallel()
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
				// When no findings, returns standardized empty-result format.
				var result map[string]any
				if err := json.Unmarshal([]byte(text), &result); err != nil {
					t.Errorf("expected JSON object: %v", err)
					return
				}
				if result["status"] != "empty" {
					t.Errorf("expected status=empty, got %v", result["status"])
				}
				if result["item_type"] != "claudemd_issues" {
					t.Errorf("expected item_type=claudemd_issues, got %v", result["item_type"])
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
