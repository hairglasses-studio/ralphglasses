package session

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

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
		{ProviderGemini, "gemini-3-pro"},
		{ProviderCodex, "gpt-5.4-xhigh"},
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
	line := []byte(`{"event":"message","message":{"parts":[{"text":"Working tree ready"}]},"usage":{"total_cost_usd":0.4,"turns":3},"session":{"id":"gem-456"},"metadata":{"model":"gemini-3-pro"}}`)
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
	line := []byte(`{"event":"message","message":{"content":"Refactor complete"},"usage":{"total_cost_usd":0.12,"turns":2},"session":{"id":"cx-123"},"metadata":{"model":"gpt-5.4-xhigh"}}`)
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
	err := validateLaunchOptions(LaunchOptions{
		Provider: ProviderCodex,
		Resume:   "sess-123",
	})
	if err == nil {
		t.Fatal("expected error for codex resume")
	}
	if !strings.Contains(err.Error(), "does not support resume") {
		t.Errorf("error = %q", err)
	}
}

func TestUnsupportedOptionsWarningsCodexResume(t *testing.T) {
	warnings := UnsupportedOptionsWarnings(ProviderCodex, LaunchOptions{
		Resume: "sess-123",
	})
	if len(warnings) == 0 {
		t.Fatal("expected warning")
	}
	if !strings.Contains(warnings[0], "unsupported") {
		t.Fatalf("warning = %q", warnings[0])
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
		runSessionOutput(context.Background(), s, stdout)
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
			// (2000/1M)*2.50 + (1000/1M)*15.00 = 0.005 + 0.015 = 0.02
			wantCost: 0.02,
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
			wantCost: 0.0025, // (1000/1M)*2.50
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

	// (1000/1M)*0.30 + (500/1M)*2.50 = 0.00155
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

	// (2000/1M)*2.50 + (1000/1M)*15.00 = 0.02
	want := 0.02
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
