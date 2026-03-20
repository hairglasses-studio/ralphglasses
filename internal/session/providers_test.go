package session

import (
	"context"
	"os"
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
		provider Provider
		wantModel string
	}{
		{ProviderClaude, "sonnet"},
		{ProviderGemini, "gemini-2.5-pro"},
		{ProviderCodex, "o4-mini"},
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
		Model:    "gemini-2.5-pro",
		Resume:   "sess-123",
	})

	cmdStr := strings.Join(cmd.Args, " ")
	for _, want := range []string{"-p", "--output-format", "stream-json", "--model", "gemini-2.5-pro", "--resume", "sess-123", "--yolo"} {
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

func TestBuildCodexCmdResume(t *testing.T) {
	ctx := context.Background()
	cmd := buildCodexCmd(ctx, LaunchOptions{
		RepoPath: "/tmp/repo",
		Resume:   "sess-456",
	})

	cmdStr := strings.Join(cmd.Args, " ")
	for _, want := range []string{"exec", "--json", "--full-auto", "resume", "sess-456"} {
		if !strings.Contains(cmdStr, want) {
			t.Errorf("codex resume cmd %q missing %q", cmdStr, want)
		}
	}
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

func TestNormalizeEventEmptyLine(t *testing.T) {
	_, err := normalizeEvent(ProviderClaude, []byte{})
	if err == nil {
		t.Error("expected error for empty line")
	}
}

func TestNormalizeEventInvalidJSON(t *testing.T) {
	_, err := normalizeEvent(ProviderGemini, []byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
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
		runSessionOutput(s, stdout)
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
	os.Unsetenv("GOOGLE_API_KEY")
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
