package session

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func currentTestProviderRate(t *testing.T, provider Provider) CostRate {
	t.Helper()
	rate, ok := getProviderCostRate(provider)
	if !ok {
		t.Fatalf("provider %q missing from ProviderCostRates", provider)
	}
	return rate
}

func TestValidateProviderUnknown(t *testing.T) {
	err := ValidateProvider(Provider("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unknown provider") {
		t.Errorf("error = %q, want 'unknown provider'", err)
	}
}

func TestProviderDefaults(t *testing.T) {
	tests := []struct {
		provider  Provider
		wantModel string
	}{
		{ProviderClaude, "sonnet"},
		{ProviderGemini, "gemini-2.5-pro"},
		{ProviderCodex, "gpt-5.4"},
	}
	for _, tt := range tests {
		got := ProviderDefaults(tt.provider)
		if got != tt.wantModel {
			t.Errorf("ProviderDefaults(%q) = %q, want %q", tt.provider, got, tt.wantModel)
		}
	}
}

func TestBuildGeminiCmd(t *testing.T) {
	ctx := context.Background()
	cmd := buildGeminiCmd(ctx, LaunchOptions{
		RepoPath: "/tmp/repo",
		Prompt:   "do something",
		Model:    "gemini-2.5-pro",
		Resume:   "sess-123",
	})

	cmdStr := strings.Join(cmd.Args, " ")
	for _, want := range []string{"--output-format", "stream-json", "--model", "gemini-2.5-pro", "--resume", "sess-123", "--approval-mode", "yolo", "-p", "do something"} {
		if !strings.Contains(cmdStr, want) {
			t.Errorf("gemini cmd %q missing %q", cmdStr, want)
		}
	}
	if cmd.Dir != "/tmp/repo" {
		t.Errorf("cmd.Dir = %q, want /tmp/repo", cmd.Dir)
	}
}

func TestBuildCodexCmd(t *testing.T) {
	ctx := context.Background()
	cmd := buildCodexCmd(ctx, LaunchOptions{
		RepoPath: "/tmp/repo",
		Model:    "o4-mini",
		Prompt:   "Fix the bug",
	})

	cmdStr := strings.Join(cmd.Args, " ")
	for _, want := range []string{"exec", "--model", "o4-mini", "--json", "--full-auto", "Fix the bug"} {
		if !strings.Contains(cmdStr, want) {
			t.Errorf("codex cmd %q missing %q", cmdStr, want)
		}
	}
	if strings.Contains(cmdStr, "--quiet") {
		t.Errorf("codex cmd %q should not contain --quiet", cmdStr)
	}
	if cmd.Dir != "/tmp/repo" {
		t.Errorf("cmd.Dir = %q, want /tmp/repo", cmd.Dir)
	}
}

func TestBuildClaudeCmdNewFlags(t *testing.T) {
	ctx := context.Background()

	t.Run("bare flag", func(t *testing.T) {
		cmd := buildClaudeCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Bare:     true,
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--bare") {
			t.Errorf("cmd %q missing --bare", cmdStr)
		}
	})

	t.Run("effort flag", func(t *testing.T) {
		cmd := buildClaudeCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Effort:   "high",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--effort high") {
			t.Errorf("cmd %q missing --effort high", cmdStr)
		}
	})

	t.Run("betas flag", func(t *testing.T) {
		cmd := buildClaudeCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Betas:    []string{"interleaved-thinking", "prompt-caching"},
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--betas interleaved-thinking") {
			t.Errorf("cmd %q missing --betas interleaved-thinking", cmdStr)
		}
		if !strings.Contains(cmdStr, "--betas prompt-caching") {
			t.Errorf("cmd %q missing --betas prompt-caching", cmdStr)
		}
	})

	t.Run("fallback-model flag", func(t *testing.T) {
		cmd := buildClaudeCmd(ctx, LaunchOptions{
			RepoPath:      "/tmp/repo",
			FallbackModel: "haiku",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--fallback-model haiku") {
			t.Errorf("cmd %q missing --fallback-model haiku", cmdStr)
		}
	})

	t.Run("no new flags when unset", func(t *testing.T) {
		cmd := buildClaudeCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		for _, flag := range []string{"--bare", "--effort", "--betas", "--fallback-model"} {
			if strings.Contains(cmdStr, flag) {
				t.Errorf("cmd %q should not contain %s when unset", cmdStr, flag)
			}
		}
	})
}

func TestNormalizeClaudeEvent(t *testing.T) {
	line := []byte(`{"type":"result","result":"Done","cost_usd":0.05,"num_turns":3,"session_id":"abc"}`)
	event, err := normalizeEvent(ProviderClaude, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "result" {
		t.Errorf("Type = %q, want result", event.Type)
	}
	if event.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f, want 0.05", event.CostUSD)
	}
	if event.NumTurns != 3 {
		t.Errorf("NumTurns = %d, want 3", event.NumTurns)
	}
	if event.SessionID != "abc" {
		t.Errorf("SessionID = %q, want abc", event.SessionID)
	}
}

func TestNormalizeClaudeEventNestedCost(t *testing.T) {
	// Claude CLI may emit cost nested under usage object
	line := []byte(`{"type":"result","result":"Done","session_id":"abc","usage":{"cost_usd":0.12,"input_tokens":1500,"output_tokens":300}}`)
	event, err := normalizeEvent(ProviderClaude, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.CostUSD != 0.12 {
		t.Errorf("CostUSD = %f, want 0.12", event.CostUSD)
	}
}

func TestNormalizeClaudeEventTopLevelCostPreferred(t *testing.T) {
	// When top-level cost_usd is present, it should be used (not overwritten)
	line := []byte(`{"type":"result","cost_usd":0.05,"usage":{"cost_usd":0.12}}`)
	event, err := normalizeEvent(ProviderClaude, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.CostUSD != 0.05 {
		t.Errorf("CostUSD = %f, want 0.05 (top-level should take precedence)", event.CostUSD)
	}
}

func TestNormalizeGeminiEvent(t *testing.T) {
	line := []byte(`{"type":"result","result":"Generated code","cost_usd":0.03,"num_turns":2,"model":"gemini-2.5-pro","session_id":"gem-123"}`)
	event, err := normalizeEvent(ProviderGemini, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "result" {
		t.Errorf("Type = %q, want result", event.Type)
	}
	if event.CostUSD != 0.03 {
		t.Errorf("CostUSD = %f, want 0.03", event.CostUSD)
	}
	if event.Model != "gemini-2.5-pro" {
		t.Errorf("Model = %q, want gemini-2.5-pro", event.Model)
	}
	if event.SessionID != "gem-123" {
		t.Errorf("SessionID = %q, want gem-123", event.SessionID)
	}
}

func TestNormalizeGeminiEventNested(t *testing.T) {
	line := []byte(`{"event":"message","message":{"parts":[{"text":"Working tree ready"}]},"usage":{"total_cost_usd":0.4,"turns":3},"session":{"id":"gem-456"},"metadata":{"model":"gemini-2.5-pro"}}`)
	event, err := normalizeEvent(ProviderGemini, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "assistant" {
		t.Errorf("Type = %q, want assistant", event.Type)
	}
	if event.Content != "Working tree ready" {
		t.Errorf("Content = %q", event.Content)
	}
	if event.CostUSD != 0.4 {
		t.Errorf("CostUSD = %f, want 0.4", event.CostUSD)
	}
	if event.NumTurns != 3 {
		t.Errorf("NumTurns = %d, want 3", event.NumTurns)
	}
	if event.SessionID != "gem-456" {
		t.Errorf("SessionID = %q, want gem-456", event.SessionID)
	}
}

func TestNormalizeCodexEvent(t *testing.T) {
	line := []byte(`{"type":"result","result":"Refactored module","cost_usd":0.01,"num_turns":1,"is_error":false}`)
	event, err := normalizeEvent(ProviderCodex, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "result" {
		t.Errorf("Type = %q, want result", event.Type)
	}
	if event.CostUSD != 0.01 {
		t.Errorf("CostUSD = %f, want 0.01", event.CostUSD)
	}
	if event.IsError {
		t.Error("IsError should be false")
	}
}

func TestNormalizeCodexEventNested(t *testing.T) {
	line := []byte(`{"event":"message","message":{"content":"Refactor complete"},"usage":{"total_cost_usd":0.12,"turns":2},"session":{"id":"cx-123"},"metadata":{"model":"gpt-5.4"}}`)
	event, err := normalizeEvent(ProviderCodex, line)
	if err != nil {
		t.Fatal(err)
	}
	if event.Type != "assistant" {
		t.Errorf("Type = %q, want assistant", event.Type)
	}
	if event.Content != "Refactor complete" {
		t.Errorf("Content = %q", event.Content)
	}
	if event.CostUSD != 0.12 {
		t.Errorf("CostUSD = %f, want 0.12", event.CostUSD)
	}
	if event.NumTurns != 2 {
		t.Errorf("NumTurns = %d, want 2", event.NumTurns)
	}
}

func TestNormalizeCodexTextFallback(t *testing.T) {
	event, err := normalizeEvent(ProviderCodex, []byte("\x1b[32mRefactored 3 files successfully\x1b[0m"))
	if err != nil {
		t.Fatal(err)
	}
	if event.Text != "Refactored 3 files successfully" {
		t.Errorf("Text = %q", event.Text)
	}
}

func TestNormalizeEventEmptyLine(t *testing.T) {
	_, err := normalizeEvent(ProviderClaude, []byte{})
	if err == nil {
		t.Error("expected error for empty line")
	}
}

func TestNormalizeEventInvalidJSON(t *testing.T) {
	_, err := normalizeEvent(ProviderGemini, []byte("not json"))
	if err != nil {
		t.Errorf("expected fallback event, got error: %v", err)
	}
}

func TestValidateLaunchOptionsCodexResume(t *testing.T) {
	orig := codexExecResumeSupported
	t.Cleanup(func() { codexExecResumeSupported = orig })
	codexExecResumeSupported = func() bool { return true }

	err := validateLaunchOptions(LaunchOptions{
		Provider: ProviderCodex,
		Resume:   "sess-123",
	})
	if err != nil {
		t.Fatalf("expected codex resume to validate when CLI support is present, got %v", err)
	}
}

func TestValidateLaunchOptionsCodexResumeUnsupportedInstall(t *testing.T) {
	orig := codexExecResumeSupported
	t.Cleanup(func() { codexExecResumeSupported = orig })
	codexExecResumeSupported = func() bool { return false }

	err := validateLaunchOptions(LaunchOptions{
		Provider: ProviderCodex,
		Resume:   "sess-123",
	})
	if err == nil {
		t.Fatal("expected error when local codex install lacks exec resume")
	}
	if !strings.Contains(err.Error(), "does not support exec resume") {
		t.Errorf("error = %q", err)
	}
}

func TestRunSessionOutputWithProvider(t *testing.T) {
	streamData := `{"type":"system","session_id":"gem-abc"}
{"type":"assistant","content":"Working..."}
{"type":"result","result":"Done!","cost_usd":0.02,"num_turns":2,"session_id":"gem-abc"}
`
	s := &Session{
		ID:       "test",
		Provider: ProviderGemini,
		Status:   StatusRunning,
	}

	stdout := strings.NewReader(streamData)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runSessionOutput(context.Background(), s, stdout, nil)
	}()
	<-done

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ProviderSessionID != "gem-abc" {
		t.Errorf("ProviderSessionID = %q, want gem-abc", s.ProviderSessionID)
	}
	if s.SpentUSD != 0.02 {
		t.Errorf("SpentUSD = %f, want 0.02", s.SpentUSD)
	}
	if s.TurnCount != 2 {
		t.Errorf("TurnCount = %d, want 2", s.TurnCount)
	}
}

func TestValidateProviderEnvMissing(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	err := ValidateProviderEnv(ProviderGemini)
	if err == nil {
		t.Fatal("expected error when GOOGLE_API_KEY is unset")
	}
	if !strings.Contains(err.Error(), "GOOGLE_API_KEY not set") {
		t.Errorf("error = %q, want mention of GOOGLE_API_KEY", err)
	}
}

func TestValidateProviderEnvPresent(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test-key")
	err := ValidateProviderEnv(ProviderCodex)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSanitizeStderr(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		input    string
		want     string
	}{
		{
			name:     "gemini strips stack traces",
			provider: ProviderGemini,
			input: `Error when talking to Gemini API: got status INTERNAL
    at Object.handleError (/usr/lib/node_modules/@google/gemini-cli/dist/index.js:123:45)
    at async Session.run (/usr/lib/node_modules/@google/gemini-cli/dist/index.js:456:78)
ApiError: got status: INTERNAL`,
			want: "Error when talking to Gemini API: got status INTERNAL\nApiError: got status: INTERNAL",
		},
		{
			name:     "gemini empty input",
			provider: ProviderGemini,
			input:    "",
			want:     "",
		},
		{
			name:     "claude passes through unchanged",
			provider: ProviderClaude,
			input:    "some error\n    at something",
			want:     "some error\n    at something",
		},
		{
			name:     "gemini only stack frames returns raw",
			provider: ProviderGemini,
			input:    "    at foo()\n    at bar()",
			want:     "    at foo()\n    at bar()",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeStderr(tt.provider, tt.input)
			if got != tt.want {
				t.Errorf("sanitizeStderr(%q, ...) =\n%q\nwant\n%q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestCleanProviderOutput(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		input    string
		want     string
	}{
		{
			name:     "codex strips ansi and returns last line",
			provider: ProviderCodex,
			input:    "\x1b[32mProcessing...\x1b[0m\n\x1b[1mRefactored 3 files successfully\x1b[0m\n",
			want:     "Refactored 3 files successfully",
		},
		{
			name:     "codex empty input",
			provider: ProviderCodex,
			input:    "",
			want:     "",
		},
		{
			name:     "claude returns empty",
			provider: ProviderClaude,
			input:    "some output",
			want:     "",
		},
		{
			name:     "codex all blank lines",
			provider: ProviderCodex,
			input:    "\n\n  \n",
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanProviderOutput(tt.provider, tt.input)
			if got != tt.want {
				t.Errorf("cleanProviderOutput(%q, ...) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestUnsupportedOptionsWarnings(t *testing.T) {
	opts := LaunchOptions{
		SystemPrompt: "be helpful",
		MaxBudgetUSD: 5.0,
		Agent:        "reviewer",
	}

	// Claude supports everything — no warnings
	if w := UnsupportedOptionsWarnings(ProviderClaude, opts); len(w) != 0 {
		t.Errorf("claude warnings = %v, want none", w)
	}

	// Gemini ignores system_prompt, max_budget_usd, agent
	gw := UnsupportedOptionsWarnings(ProviderGemini, opts)
	if len(gw) != 3 {
		t.Errorf("gemini warnings count = %d, want 3: %v", len(gw), gw)
	}

	// Codex ignores system_prompt, max_budget_usd, agent
	cw := UnsupportedOptionsWarnings(ProviderCodex, opts)
	if len(cw) != 3 {
		t.Errorf("codex warnings count = %d, want 3: %v", len(cw), cw)
	}
}

func TestEstimateCostFromTokens(t *testing.T) {
	codexRate := currentTestProviderRate(t, ProviderCodex)

	tests := []struct {
		name     string
		provider Provider
		raw      map[string]any
		wantCost float64
	}{
		{
			name:     "gemini with usage_metadata tokens",
			provider: ProviderGemini,
			raw: map[string]any{
				"usage_metadata": map[string]any{
					"prompt_token_count":     float64(1000),
					"candidates_token_count": float64(500),
				},
			},
			// (1000/1M)*0.30 + (500/1M)*2.50 = 0.0003 + 0.00125 = 0.00155
			wantCost: 0.00155,
		},
		{
			name:     "codex with usage tokens",
			provider: ProviderCodex,
			raw: map[string]any{
				"usage": map[string]any{
					"prompt_tokens":     float64(2000),
					"completion_tokens": float64(1000),
				},
			},
			wantCost: (2000.0/1_000_000)*codexRate.InputPer1M + (1000.0/1_000_000)*codexRate.OutputPer1M,
		},
		{
			name:     "claude with usage input/output tokens",
			provider: ProviderClaude,
			raw: map[string]any{
				"usage": map[string]any{
					"input_tokens":  float64(5000),
					"output_tokens": float64(2000),
				},
			},
			// (5000/1M)*3.00 + (2000/1M)*15.00 = 0.015 + 0.03 = 0.045
			wantCost: 0.045,
		},
		{
			name:     "no token data returns zero",
			provider: ProviderGemini,
			raw: map[string]any{
				"type":    "assistant",
				"content": "hello",
			},
			wantCost: 0,
		},
		{
			name:     "only input tokens",
			provider: ProviderCodex,
			raw: map[string]any{
				"usage": map[string]any{
					"prompt_tokens": float64(1000),
				},
			},
			wantCost: (1000.0 / 1_000_000) * codexRate.InputPer1M,
		},
		{
			name:     "only output tokens",
			provider: ProviderGemini,
			raw: map[string]any{
				"usage_metadata": map[string]any{
					"candidates_token_count": float64(1000),
				},
			},
			wantCost: 0.0025, // (1000/1M)*2.50
		},
		{
			name:     "unknown provider returns zero",
			provider: Provider("unknown"),
			raw: map[string]any{
				"usage": map[string]any{
					"input_tokens":  float64(1000),
					"output_tokens": float64(1000),
				},
			},
			wantCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateCostFromTokens(tt.provider, tt.raw)
			if math.Abs(got-tt.wantCost) > 1e-9 {
				t.Errorf("estimateCostFromTokens(%s) = %v, want %v", tt.provider, got, tt.wantCost)
			}
		})
	}
}

func TestNormalizeGeminiEventWithTokenCost(t *testing.T) {
	raw := map[string]any{
		"type":    "result",
		"content": "done",
		"usage_metadata": map[string]any{
			"prompt_token_count":     float64(1000),
			"candidates_token_count": float64(500),
		},
	}
	line, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	event, err := normalizeGeminiEvent(line)
	if err != nil {
		t.Fatalf("normalizeGeminiEvent() error: %v", err)
	}

	// (1000/1M)*0.30 + (500/1M)*2.50 = 0.0003 + 0.00125 = 0.00155
	want := 0.00155
	if math.Abs(event.CostUSD-want) > 1e-9 {
		t.Errorf("CostUSD = %v, want %v", event.CostUSD, want)
	}
}

func TestNormalizeGeminiEventExplicitCostTakesPrecedence(t *testing.T) {
	raw := map[string]any{
		"type":     "result",
		"content":  "done",
		"cost_usd": float64(0.05),
		"usage_metadata": map[string]any{
			"prompt_token_count":     float64(1000),
			"candidates_token_count": float64(500),
		},
	}
	line, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	event, err := normalizeGeminiEvent(line)
	if err != nil {
		t.Fatalf("normalizeGeminiEvent() error: %v", err)
	}

	if math.Abs(event.CostUSD-0.05) > 1e-9 {
		t.Errorf("CostUSD = %v, want 0.05 (explicit cost should take precedence)", event.CostUSD)
	}
}

func TestNormalizeCodexEventWithTokenCost(t *testing.T) {
	codexRate := currentTestProviderRate(t, ProviderCodex)
	raw := map[string]any{
		"type":    "result",
		"content": "done",
		"usage": map[string]any{
			"prompt_tokens":     float64(2000),
			"completion_tokens": float64(1000),
		},
	}
	line, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	event, err := normalizeCodexEvent(line)
	if err != nil {
		t.Fatalf("normalizeCodexEvent() error: %v", err)
	}

	want := (2000.0/1_000_000)*codexRate.InputPer1M + (1000.0/1_000_000)*codexRate.OutputPer1M
	if math.Abs(event.CostUSD-want) > 1e-9 {
		t.Errorf("CostUSD = %v, want %v", event.CostUSD, want)
	}
}

func TestNormalizeClaudeEventTokenFallback(t *testing.T) {
	raw := map[string]any{
		"type": "result",
		"usage": map[string]any{
			"input_tokens":  float64(5000),
			"output_tokens": float64(2000),
		},
		"result": "all done",
	}
	line, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}

	event, err := normalizeClaudeEvent(line)
	if err != nil {
		t.Fatalf("normalizeClaudeEvent() error: %v", err)
	}

	// (5000/1M)*3.00 + (2000/1M)*15.00 = 0.015 + 0.03 = 0.045
	want := 0.045
	if math.Abs(event.CostUSD-want) > 1e-9 {
		t.Errorf("CostUSD = %v, want %v", event.CostUSD, want)
	}
}

func TestParseCostFromStderr(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		wantCost float64
	}{
		{
			name:     "cost with dollar sign",
			stderr:   "Processing complete.\nCost: $0.0023\nDone.",
			wantCost: 0.0023,
		},
		{
			name:     "total cost without dollar sign",
			stderr:   "Total cost: 0.0450",
			wantCost: 0.0450,
		},
		{
			name:     "cost_usd field",
			stderr:   "cost_usd: $1.23",
			wantCost: 1.23,
		},
		{
			name:     "session cost",
			stderr:   "Session cost: $0.05",
			wantCost: 0.05,
		},
		{
			name:     "multiple costs uses last",
			stderr:   "Cost: $0.01\nMore output\nTotal cost: $0.05",
			wantCost: 0.05,
		},
		{
			name:     "with ANSI codes",
			stderr:   "\x1b[32mCost: $0.0023\x1b[0m",
			wantCost: 0.0023,
		},
		{
			name:     "no cost returns zero",
			stderr:   "Some random output\nNo cost info here",
			wantCost: 0,
		},
		{
			name:     "empty string returns zero",
			stderr:   "",
			wantCost: 0,
		},
		{
			name:     "case insensitive",
			stderr:   "COST: $0.12",
			wantCost: 0.12,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCostFromStderr(tt.stderr)
			if math.Abs(got-tt.wantCost) > 1e-9 {
				t.Errorf("ParseCostFromStderr() = %v, want %v", got, tt.wantCost)
			}
		})
	}
}

func TestCostAccumulation(t *testing.T) {
	// Simulate per-event cost accumulation for Gemini (non-Claude provider)
	s := &Session{Provider: ProviderGemini}

	costs := []float64{0.001, 0.002, 0.003}
	var totalExpected float64
	for _, cost := range costs {
		totalExpected += cost
		s.SpentUSD += cost // Gemini: accumulate
	}

	if math.Abs(s.SpentUSD-totalExpected) > 1e-9 {
		t.Errorf("accumulated SpentUSD = %v, want %v", s.SpentUSD, totalExpected)
	}
	if math.Abs(s.SpentUSD-0.006) > 1e-9 {
		t.Errorf("accumulated SpentUSD = %v, want 0.006", s.SpentUSD)
	}

	// Simulate cumulative cost for Claude (replacement, not accumulation)
	sc := &Session{Provider: ProviderClaude}
	cumulativeCosts := []float64{0.01, 0.03, 0.06}
	for _, cost := range cumulativeCosts {
		sc.SpentUSD = cost // Claude: replace
	}

	if math.Abs(sc.SpentUSD-0.06) > 1e-9 {
		t.Errorf("Claude cumulative SpentUSD = %v, want 0.06", sc.SpentUSD)
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"string", "hello", "hello"},
		{"string_with_spaces", "  hello  ", "hello"},
		{"float64", float64(3.14), "3.14"},
		{"int", int(42), "42"},
		{"json_number", json.Number("99"), "99"},
		{"nil", nil, ""},
		{"bool", true, ""},
		{"stringer", testStringer{"world"}, "world"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asString(tt.val)
			if got != tt.want {
				t.Errorf("asString(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

type testStringer struct{ s string }

func (ts testStringer) String() string { return ts.s }

func TestAsFloat(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   float64
		wantOK bool
	}{
		{"float64", float64(3.14), 3.14, true},
		{"float32", float32(2.5), 2.5, true},
		{"int", int(42), 42.0, true},
		{"int64", int64(100), 100.0, true},
		{"json_number", json.Number("1.5"), 1.5, true},
		{"string_number", "3.14", 3.14, true},
		{"string_invalid", "not-a-number", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asFloat(tt.val)
			if ok != tt.wantOK {
				t.Errorf("asFloat(%v) ok = %v, want %v", tt.val, ok, tt.wantOK)
			}
			if ok && math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("asFloat(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestAsInt(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   int
		wantOK bool
	}{
		{"int", int(42), 42, true},
		{"int64", int64(100), 100, true},
		{"float64", float64(3.0), 3, true},
		{"json_number", json.Number("99"), 99, true},
		{"string_number", "123", 123, true},
		{"string_invalid", "abc", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asInt(tt.val)
			if ok != tt.wantOK {
				t.Errorf("asInt(%v) ok = %v, want %v", tt.val, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("asInt(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestAsBool(t *testing.T) {
	tests := []struct {
		name   string
		val    any
		want   bool
		wantOK bool
	}{
		{"true_bool", true, true, true},
		{"false_bool", false, false, true},
		{"string_true", "true", true, true},
		{"string_false", "false", false, true},
		{"string_1", "1", true, true},
		{"non_empty_text", "hello", false, false},
		{"nil", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := asBool(tt.val)
			if ok != tt.wantOK {
				t.Errorf("asBool(%v) ok = %v, want %v", tt.val, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("asBool(%v) = %v, want %v", tt.val, got, tt.want)
			}
		})
	}
}

func TestTextValue(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"array", []any{"line1", "line2"}, "line1\nline2"},
		{"map_with_text", map[string]any{"text": "foo"}, "foo"},
		{"map_with_content", map[string]any{"content": "bar"}, "bar"},
		{"map_with_parts", map[string]any{"parts": []any{"p1", "p2"}}, "p1\np2"},
		{"map_with_error", map[string]any{"error": "oops"}, "oops"},
		{"map_empty", map[string]any{}, ""},
		{"number", float64(42), "42"},
		{"nested_array", []any{map[string]any{"text": "nested"}}, "nested"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textValue(tt.val)
			if got != tt.want {
				t.Errorf("textValue(%v) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestApplyEventDefaults(t *testing.T) {
	tests := []struct {
		name      string
		event     StreamEvent
		wantType  string
		wantError bool
	}{
		{
			name:     "message_becomes_assistant",
			event:    StreamEvent{Type: "message", Content: "hello"},
			wantType: "assistant",
		},
		{
			name:     "delta_becomes_assistant",
			event:    StreamEvent{Type: "delta", Content: "chunk"},
			wantType: "assistant",
		},
		{
			name:      "error_becomes_result",
			event:     StreamEvent{Type: "error", Error: "something broke"},
			wantType:  "result",
			wantError: true,
		},
		{
			name:     "empty_type_with_result",
			event:    StreamEvent{Result: "done"},
			wantType: "result",
		},
		{
			name:     "empty_type_with_content",
			event:    StreamEvent{Content: "working"},
			wantType: "assistant",
		},
		{
			name:     "empty_type_with_session_id",
			event:    StreamEvent{SessionID: "abc"},
			wantType: "system",
		},
		{
			name:      "empty_type_with_error_flag",
			event:     StreamEvent{IsError: true, Text: "err"},
			wantType:  "result",
			wantError: true,
		},
		{
			name:      "error_string_sets_is_error",
			event:     StreamEvent{Type: "result", Error: "fail"},
			wantType:  "result",
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.event
			applyEventDefaults(&e)
			if e.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", e.Type, tt.wantType)
			}
			if e.IsError != tt.wantError {
				t.Errorf("IsError = %v, want %v", e.IsError, tt.wantError)
			}
		})
	}
}

func TestFallbackTextEvent(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		input    string
		wantType string
		wantErr  bool
	}{
		{
			name:     "gemini_plain_text",
			provider: ProviderGemini,
			input:    "Working on the task...",
			wantType: "assistant",
		},
		{
			name:     "codex_ansi_text",
			provider: ProviderCodex,
			input:    "\x1b[32mDone with refactoring\x1b[0m",
			wantType: "assistant",
		},
		{
			name:     "error_text",
			provider: ProviderGemini,
			input:    "Error: connection refused",
			wantType: "result",
		},
		{
			name:     "failed_text",
			provider: ProviderCodex,
			input:    "\x1b[31mBuild failed with 3 errors\x1b[0m",
			wantType: "result",
		},
		{
			name:     "empty_after_clean",
			provider: ProviderCodex,
			input:    "\n\n  \n",
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := fallbackTextEvent(tt.provider, []byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if event.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", event.Type, tt.wantType)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	m := map[string]any{
		"name":   "test",
		"count":  42,
		"nested": map[string]any{"key": "val"},
	}

	if got := getString(m, "name"); got != "test" {
		t.Errorf("getString(name) = %q, want test", got)
	}
	if got := getString(m, "count"); got != "" {
		t.Errorf("getString(count) = %q, want empty (not a string)", got)
	}
	if got := getString(m, "missing"); got != "" {
		t.Errorf("getString(missing) = %q, want empty", got)
	}
}

func TestFilterEnv(t *testing.T) {
	env := []string{"HOME=/home/user", "CLAUDECODE=1", "PATH=/usr/bin", "CLAUDECODED=yes"}
	filtered := filterEnv(env, "CLAUDECODE")

	if len(filtered) != 3 {
		t.Errorf("expected 3 remaining env vars, got %d: %v", len(filtered), filtered)
	}
	for _, e := range filtered {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			t.Errorf("CLAUDECODE should have been filtered: %s", e)
		}
	}
}

func TestStripNestingEnv_RemovesCodexAndClaudeMarkers(t *testing.T) {
	env := []string{
		"HOME=/home/user",
		"PATH=/usr/bin",
		"CLAUDECODE=1",
		"CLAUDE_CODE_ENTRYPOINT=cli",
		"CODEX_THREAD_ID=thread-123",
		"CODEX_CI=1",
		"CODEX_SANDBOX_NETWORK_DISABLED=1",
		"CODEX_HOME=/tmp/codex-home",
	}

	filtered := stripNestingEnv(env)
	got := strings.Join(filtered, "\n")

	for _, unwanted := range []string{
		"CLAUDECODE=",
		"CLAUDE_CODE_ENTRYPOINT=",
		"CODEX_THREAD_ID=",
		"CODEX_CI=",
		"CODEX_SANDBOX_NETWORK_DISABLED=",
	} {
		if strings.Contains(got, unwanted) {
			t.Errorf("stripNestingEnv() should remove %s, got %v", unwanted, filtered)
		}
	}
	if !strings.Contains(got, "CODEX_HOME=/tmp/codex-home") {
		t.Errorf("stripNestingEnv() should preserve CODEX_HOME, got %v", filtered)
	}
}

func TestValidateLaunchOptions(t *testing.T) {
	tests := []struct {
		name    string
		opts    LaunchOptions
		wantErr bool
	}{
		{
			name:    "codex_with_resume",
			opts:    LaunchOptions{Provider: ProviderCodex, Resume: "sess-1"},
			wantErr: true,
		},
		{
			name:    "claude_with_resume",
			opts:    LaunchOptions{Provider: ProviderClaude, Resume: "sess-1"},
			wantErr: false,
		},
		{
			name:    "gemini_with_resume",
			opts:    LaunchOptions{Provider: ProviderGemini, Resume: "sess-1"},
			wantErr: false,
		},
		{
			name:    "codex_no_resume",
			opts:    LaunchOptions{Provider: ProviderCodex},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := codexExecResumeSupported
			t.Cleanup(func() { codexExecResumeSupported = orig })
			codexExecResumeSupported = func() bool { return false }
			if tt.name == "codex_with_resume" {
				codexExecResumeSupported = func() bool { return true }
				tt.wantErr = false
			}
			err := validateLaunchOptions(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLaunchOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnsupportedOptionsWarnings_AllFields(t *testing.T) {
	opts := LaunchOptions{
		SystemPrompt: "prompt",
		MaxBudgetUSD: 5.0,
		Agent:        "reviewer",
		MaxTurns:     10,
		AllowedTools: []string{"tool1"},
		Worktree:     "feature-branch",
		Resume:       "sess-123",
	}

	// Gemini: system_prompt, max_budget, agent, max_turns, allowed_tools, worktree
	gw := UnsupportedOptionsWarnings(ProviderGemini, opts)
	if len(gw) != 6 {
		t.Errorf("gemini warnings count = %d, want 6: %v", len(gw), gw)
	}

	// Codex: system_prompt, max_budget, agent, max_turns, allowed_tools, worktree
	cw := UnsupportedOptionsWarnings(ProviderCodex, opts)
	if len(cw) != 6 {
		t.Errorf("codex warnings count = %d, want 6: %v", len(cw), cw)
	}

	// Empty provider is resolved to the primary provider (currently Codex).
	ew := UnsupportedOptionsWarnings("", opts)
	if len(ew) != 6 {
		t.Errorf("empty provider warnings count = %d, want 6: %v", len(ew), ew)
	}
}

func TestNormalizeClaudeEvent_SubAgent(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantType string
		wantText string
	}{
		{
			name:     "agent_event_with_description",
			line:     `{"type":"agent","description":"Working on subtask"}`,
			wantType: "agent",
			wantText: "Working on subtask",
		},
		{
			name:     "subagent_normalized_to_agent",
			line:     `{"type":"subagent","message":"Delegating to sub-agent"}`,
			wantType: "agent",
			wantText: "Delegating to sub-agent",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeClaudeEvent([]byte(tt.line))
			if err != nil {
				t.Fatal(err)
			}
			if event.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", event.Type, tt.wantType)
			}
			if event.Content != tt.wantText {
				t.Errorf("Content = %q, want %q", event.Content, tt.wantText)
			}
		})
	}
}

func TestNormalizeClaudeEvent_DurationAndTurns(t *testing.T) {
	line := []byte(`{"type":"result","result":"done","duration_seconds":12.5,"num_turns":7}`)
	event, err := normalizeClaudeEvent(line)
	if err != nil {
		t.Fatal(err)
	}
	if event.Duration != 12.5 {
		t.Errorf("Duration = %f, want 12.5", event.Duration)
	}
	if event.NumTurns != 7 {
		t.Errorf("NumTurns = %d, want 7", event.NumTurns)
	}
}

func TestNormalizeGeminiEvent_ErrorField(t *testing.T) {
	line := []byte(`{"type":"result","error":"API rate limit exceeded","is_error":true}`)
	event, err := normalizeGeminiEvent(line)
	if err != nil {
		t.Fatal(err)
	}
	if !event.IsError {
		t.Error("expected IsError=true")
	}
	if event.Error != "API rate limit exceeded" {
		t.Errorf("Error = %q, want 'API rate limit exceeded'", event.Error)
	}
}

func TestNormalizeCodexEvent_OutputSchema(t *testing.T) {
	ctx := context.Background()
	cmd := buildCodexCmd(ctx, LaunchOptions{
		RepoPath:     "/tmp/repo",
		Prompt:       "analyze",
		OutputSchema: json.RawMessage(`{"type":"object"}`),
	})
	cmdStr := strings.Join(cmd.Args, " ")
	if !strings.Contains(cmdStr, "--output-schema") {
		t.Errorf("codex cmd %q missing --output-schema", cmdStr)
	}
}

func TestBuildClaudeCmd_AllOptions(t *testing.T) {
	ctx := context.Background()
	cmd := buildClaudeCmd(ctx, LaunchOptions{
		RepoPath:     "/tmp/repo",
		Model:        "opus",
		MaxBudgetUSD: 5.0,
		MaxTurns:     20,
		Agent:        "reviewer",
		AllowedTools: []string{"bash", "read"},
		SystemPrompt: "be helpful",
		Continue:     true,
		Worktree:     "true",
		SessionName:  "test-sess",
		OutputSchema: json.RawMessage(`{"type":"object"}`),
	})
	cmdStr := strings.Join(cmd.Args, " ")

	for _, want := range []string{
		"--model opus",
		"--max-budget-usd 5.00",
		"--max-turns 20",
		"--agent reviewer",
		"--allowedTools bash,read",
		"--append-system-prompt",
		"--continue",
		"-w",
		"--json-schema",
	} {
		if !strings.Contains(cmdStr, want) {
			t.Errorf("cmd %q missing %q", cmdStr, want)
		}
	}

	// Continue should not appear when Resume is set
	cmd2 := buildClaudeCmd(ctx, LaunchOptions{
		RepoPath: "/tmp/repo",
		Resume:   "sess-123",
		Continue: true,
	})
	cmdStr2 := strings.Join(cmd2.Args, " ")
	if strings.Contains(cmdStr2, "--continue") {
		t.Error("--continue should not be set when --resume is used")
	}
	if !strings.Contains(cmdStr2, "--resume sess-123") {
		t.Error("missing --resume flag")
	}
}

func TestBuildClaudeCmd_WorktreeBranch(t *testing.T) {
	ctx := context.Background()
	cmd := buildClaudeCmd(ctx, LaunchOptions{
		RepoPath: "/tmp/repo",
		Worktree: "feature-branch",
	})
	cmdStr := strings.Join(cmd.Args, " ")
	if !strings.Contains(cmdStr, "-w feature-branch") {
		t.Errorf("cmd %q missing '-w feature-branch'", cmdStr)
	}
}

func TestBuildClaudeCmd_NoSessionPersistence(t *testing.T) {
	ctx := context.Background()
	cmd := buildClaudeCmd(ctx, LaunchOptions{
		RepoPath:             "/tmp/repo",
		NoSessionPersistence: true,
		SessionID:            "sweep-abc-myrepo",
	})
	cmdStr := strings.Join(cmd.Args, " ")
	if !strings.Contains(cmdStr, "--no-session-persistence") {
		t.Errorf("cmd %q missing --no-session-persistence", cmdStr)
	}
	if !strings.Contains(cmdStr, "--session-id sweep-abc-myrepo") {
		t.Errorf("cmd %q missing --session-id", cmdStr)
	}

	// Verify flags are absent when not set.
	cmd2 := buildClaudeCmd(ctx, LaunchOptions{RepoPath: "/tmp/repo"})
	cmdStr2 := strings.Join(cmd2.Args, " ")
	if strings.Contains(cmdStr2, "--no-session-persistence") {
		t.Error("--no-session-persistence should not be set when NoSessionPersistence=false")
	}
	if strings.Contains(cmdStr2, "--session-id") {
		t.Error("--session-id should not be set when SessionID is empty")
	}
}

func TestValidateProviderEnv_GeminiAlternateKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "test-key")
	err := ValidateProviderEnv(ProviderGemini)
	if err != nil {
		t.Fatalf("expected no error with GEMINI_API_KEY set: %v", err)
	}
}

func TestValueAtPath(t *testing.T) {
	raw := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"value": "deep",
			},
		},
		"flat": "top",
	}

	if got := asString(valueAtPath(raw, "flat")); got != "top" {
		t.Errorf("flat path = %q, want top", got)
	}
	if got := asString(valueAtPath(raw, "level1.level2.value")); got != "deep" {
		t.Errorf("nested path = %q, want deep", got)
	}
	if got := valueAtPath(raw, "missing.path"); got != nil {
		t.Errorf("missing path = %v, want nil", got)
	}
	if got := valueAtPath(raw, "flat.invalid"); got != nil {
		t.Errorf("invalid nested on string = %v, want nil", got)
	}
}

func TestParseProviderCostFromStderr(t *testing.T) {
	codexRate := currentTestProviderRate(t, ProviderCodex)

	tests := []struct {
		name     string
		provider Provider
		stderr   string
		wantCost float64
		wantOK   bool
	}{
		// Claude: universal cost patterns
		{
			name:     "claude_cost_with_dollar",
			provider: ProviderClaude,
			stderr:   "Processing complete.\nCost: $0.0023\nDone.",
			wantCost: 0.0023,
			wantOK:   true,
		},
		{
			name:     "claude_total_cost",
			provider: ProviderClaude,
			stderr:   "Total cost: 0.0450",
			wantCost: 0.0450,
			wantOK:   true,
		},
		{
			name:     "claude_session_cost",
			provider: ProviderClaude,
			stderr:   "Session cost: $1.05",
			wantCost: 1.05,
			wantOK:   true,
		},
		{
			name:     "claude_no_cost",
			provider: ProviderClaude,
			stderr:   "Some random output without cost",
			wantCost: 0,
			wantOK:   false,
		},
		{
			name:     "claude_empty",
			provider: ProviderClaude,
			stderr:   "",
			wantCost: 0,
			wantOK:   false,
		},
		// Gemini: token count patterns
		{
			name:     "gemini_token_counts",
			provider: ProviderGemini,
			stderr:   "prompt_token_count: 1000, candidates_token_count: 500",
			wantCost: (1000.0/1_000_000)*CostGeminiFlashInput + (500.0/1_000_000)*CostGeminiFlashOutput,
			wantOK:   true,
		},
		{
			name:     "gemini_tokens_used",
			provider: ProviderGemini,
			stderr:   "Completed. 2000 tokens used.",
			wantCost: (2000.0 / 1_000_000) * (CostGeminiFlashInput + CostGeminiFlashOutput) / 2,
			wantOK:   true,
		},
		{
			name:     "gemini_cost_pattern",
			provider: ProviderGemini,
			stderr:   "Total cost: $0.05",
			wantCost: 0.05,
			wantOK:   true,
		},
		{
			name:     "gemini_no_cost",
			provider: ProviderGemini,
			stderr:   "Task completed successfully",
			wantCost: 0,
			wantOK:   false,
		},
		// Codex: token count patterns
		{
			name:     "codex_input_output_tokens",
			provider: ProviderCodex,
			stderr:   "Used 5000 input tokens, 1000 output tokens",
			wantCost: (5000.0/1_000_000)*codexRate.InputPer1M + (1000.0/1_000_000)*codexRate.OutputPer1M,
			wantOK:   true,
		},
		{
			name:     "codex_tokens_format2",
			provider: ProviderCodex,
			stderr:   "Tokens: 3000 input / 800 output",
			wantCost: (3000.0/1_000_000)*codexRate.InputPer1M + (800.0/1_000_000)*codexRate.OutputPer1M,
			wantOK:   true,
		},
		{
			name:     "codex_cost_pattern",
			provider: ProviderCodex,
			stderr:   "Cost: $0.12",
			wantCost: 0.12,
			wantOK:   true,
		},
		{
			name:     "codex_no_cost",
			provider: ProviderCodex,
			stderr:   "Done.",
			wantCost: 0,
			wantOK:   false,
		},
		// ANSI codes should be stripped
		{
			name:     "ansi_codes_stripped",
			provider: ProviderClaude,
			stderr:   "\x1b[32mCost: $0.50\x1b[0m",
			wantCost: 0.50,
			wantOK:   true,
		},
		// Multiple cost lines: last wins
		{
			name:     "multiple_costs_last_wins",
			provider: ProviderClaude,
			stderr:   "Cost: $0.01\nMore output\nTotal cost: $0.05",
			wantCost: 0.05,
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseProviderCostFromStderr(tt.provider, tt.stderr)
			if ok != tt.wantOK {
				t.Errorf("ParseProviderCostFromStderr() ok = %v, want %v", ok, tt.wantOK)
			}
			if math.Abs(got-tt.wantCost) > 1e-9 {
				t.Errorf("ParseProviderCostFromStderr() = %v, want %v", got, tt.wantCost)
			}
		})
	}
}

func TestBatchOptionsInLaunchOptions(t *testing.T) {
	// Verify nil Batch means non-batch mode (backward compatibility)
	opts := LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "test",
	}
	if opts.Batch != nil {
		t.Error("default LaunchOptions.Batch should be nil")
	}

	// Verify BatchOptions can be set
	opts.Batch = &BatchOptions{
		Enabled:     true,
		CallbackURL: "https://example.com/webhook",
		BatchID:     "batch-123",
		Priority:    5,
	}
	if !opts.Batch.Enabled {
		t.Error("Batch.Enabled should be true")
	}
	if opts.Batch.CallbackURL != "https://example.com/webhook" {
		t.Errorf("Batch.CallbackURL = %q", opts.Batch.CallbackURL)
	}
	if opts.Batch.BatchID != "batch-123" {
		t.Errorf("Batch.BatchID = %q", opts.Batch.BatchID)
	}
	if opts.Batch.Priority != 5 {
		t.Errorf("Batch.Priority = %d", opts.Batch.Priority)
	}
}

func TestBatchOptionsJSON(t *testing.T) {
	bo := BatchOptions{
		Enabled:     true,
		CallbackURL: "https://example.com/callback",
		BatchID:     "b-1",
		Priority:    3,
	}
	data, err := json.Marshal(bo)
	if err != nil {
		t.Fatal(err)
	}

	var decoded BatchOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != bo {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", decoded, bo)
	}

	// Verify omitempty for optional fields
	bo2 := BatchOptions{Enabled: true}
	data2, _ := json.Marshal(bo2)
	s := string(data2)
	if strings.Contains(s, "callback_url") {
		t.Error("empty callback_url should be omitted")
	}
	if strings.Contains(s, "batch_id") {
		t.Error("empty batch_id should be omitted")
	}
	if strings.Contains(s, "priority") {
		t.Error("zero priority should be omitted")
	}
}

func TestBuildCodexCmdSandbox(t *testing.T) {
	ctx := context.Background()

	t.Run("sandbox flag", func(t *testing.T) {
		cmd := buildCodexCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Prompt:   "Fix it",
			Sandbox:  true,
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--sandbox workspace-write") {
			t.Errorf("codex cmd %q missing --sandbox workspace-write", cmdStr)
		}
	})

	t.Run("permission mode maps to sandbox", func(t *testing.T) {
		cmd := buildCodexCmd(ctx, LaunchOptions{
			RepoPath:       "/tmp/repo",
			Prompt:         "Fix it",
			PermissionMode: "workspace-write",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--sandbox workspace-write") {
			t.Errorf("codex cmd %q missing --sandbox workspace-write", cmdStr)
		}
	})

	t.Run("no sandbox without flags", func(t *testing.T) {
		cmd := buildCodexCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Prompt:   "Fix it",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if strings.Contains(cmdStr, "--sandbox") {
			t.Errorf("codex cmd %q should not contain --sandbox when not requested", cmdStr)
		}
	})
}

func TestNormalizeCodexEventDuration(t *testing.T) {
	raw := map[string]any{
		"type":             "result",
		"content":          "done",
		"duration_seconds": 12.5,
	}
	line, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	event, err := normalizeCodexEvent(line)
	if err != nil {
		t.Fatalf("normalizeCodexEvent() error: %v", err)
	}
	if event.Duration != 12.5 {
		t.Errorf("Duration = %v, want 12.5", event.Duration)
	}
}

func TestSanitizeCodexStderr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips debug lines",
			input: "[debug] loading config\nActual error message\n[trace] exiting",
			want:  "Actual error message",
		},
		{
			name:  "strips pip warnings",
			input: "WARNING: pip is configured with locations\nRate limit exceeded",
			want:  "Rate limit exceeded",
		},
		{
			name:  "strips npm warnings",
			input: "npm WARN deprecated some-pkg\nTask completed",
			want:  "Task completed",
		},
		{
			name:  "strips stack frames",
			input: "Error: something\n    at Object.run (/usr/lib/index.js:1:2)\nat Session.exec",
			want:  "Error: something",
		},
		{
			name:  "all noise returns raw",
			input: "[debug] a\n[trace] b",
			want:  "[debug] a\n[trace] b",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeCodexStderr(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeCodexStderr() =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestCleanProviderOutputSkipsNoise(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		input    string
		want     string
	}{
		{
			name:     "codex skips exit code line",
			provider: ProviderCodex,
			input:    "Refactored 3 files\nExit code: 0\n",
			want:     "Refactored 3 files",
		},
		{
			name:     "codex skips pip warning at end",
			provider: ProviderCodex,
			input:    "All tests pass\nWARNING: pip is configured\n",
			want:     "All tests pass",
		},
		{
			name:     "codex skips debug lines at end",
			provider: ProviderCodex,
			input:    "Done\n[debug] cleanup\n[trace] exit\n",
			want:     "Done",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanProviderOutput(tt.provider, tt.input)
			if got != tt.want {
				t.Errorf("cleanProviderOutput(%q, ...) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestUnsupportedOptionsWarningsCodexSandboxImage(t *testing.T) {
	opts := LaunchOptions{
		SandboxImage: "custom:latest",
	}
	warnings := UnsupportedOptionsWarnings(ProviderCodex, opts)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "sandbox_image") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected sandbox_image warning for codex provider")
	}
}
