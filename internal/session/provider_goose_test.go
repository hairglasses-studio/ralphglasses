package session

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestGooseProviderConstants(t *testing.T) {
	if ProviderGoose != "goose" {
		t.Fatalf("ProviderGoose = %q, want %q", ProviderGoose, "goose")
	}
	if got := providerBinary(ProviderGoose); got != "goose" {
		t.Errorf("providerBinary(goose) = %q, want %q", got, "goose")
	}
	if got := ProviderDefaults(ProviderGoose); got != "claude-sonnet-4-6" {
		t.Errorf("ProviderDefaults(goose) = %q, want %q", got, "claude-sonnet-4-6")
	}
	if got := providerEnvVar(ProviderGoose); got != "GOOSE_API_KEY" {
		t.Errorf("providerEnvVar(goose) = %q, want %q", got, "GOOSE_API_KEY")
	}
}

func TestGooseBuildCmd(t *testing.T) {
	ctx := context.Background()

	t.Run("basic", func(t *testing.T) {
		cmd := buildGooseCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Prompt:   "refactor the module",
			Model:    "claude-sonnet-4-6",
		})

		cmdStr := strings.Join(cmd.Args, " ")
		for _, want := range []string{"session", "run", "--output-format", "json", "--model", "claude-sonnet-4-6", "--prompt", "refactor the module"} {
			if !strings.Contains(cmdStr, want) {
				t.Errorf("goose cmd %q missing %q", cmdStr, want)
			}
		}
		if cmd.Dir != "/tmp/repo" {
			t.Errorf("cmd.Dir = %q, want /tmp/repo", cmd.Dir)
		}
		if cmd.Args[0] != "goose" {
			t.Errorf("binary = %q, want goose", cmd.Args[0])
		}
	})

	t.Run("with resume", func(t *testing.T) {
		cmd := buildGooseCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Resume:   "sess-goose-1",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--resume sess-goose-1") {
			t.Errorf("goose cmd %q missing --resume", cmdStr)
		}
	})

	t.Run("no model no prompt", func(t *testing.T) {
		cmd := buildGooseCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if strings.Contains(cmdStr, "--model") {
			t.Errorf("goose cmd %q should not have --model when empty", cmdStr)
		}
		if strings.Contains(cmdStr, "--prompt") {
			t.Errorf("goose cmd %q should not have --prompt when empty", cmdStr)
		}
		// Should still have session run and --output-format json
		if !strings.Contains(cmdStr, "session run") {
			t.Errorf("goose cmd %q missing 'session run'", cmdStr)
		}
	})
}

func TestGooseNormalizeEvent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantText string
		wantCost float64
		wantSID  string
	}{
		{
			name:     "result with cost",
			input:    `{"type":"result","session_id":"goose-123","cost_usd":0.08,"result":"Task complete"}`,
			wantType: "result",
			wantText: "Task complete",
			wantCost: 0.08,
			wantSID:  "goose-123",
		},
		{
			name:     "assistant content",
			input:    `{"type":"assistant","content":"Working on it...","model":"claude-sonnet-4-6"}`,
			wantType: "assistant",
			wantText: "Working on it...",
		},
		{
			name:     "error event",
			input:    `{"type":"error","error":"connection timeout","is_error":true}`,
			wantType: "result",
			wantText: "connection timeout",
		},
		{
			name:     "metrics-based cost",
			input:    `{"type":"result","metrics":{"cost_usd":0.15}}`,
			wantType: "result",
			wantCost: 0.15,
		},
		{
			name:     "token estimation",
			input:    `{"type":"result","usage":{"input_tokens":10000,"output_tokens":3000}}`,
			wantType: "result",
			wantCost: estimateCostFromTokens(ProviderGoose, map[string]any{"usage": map[string]any{"input_tokens": float64(10000), "output_tokens": float64(3000)}}),
		},
		{
			name:     "kind field as type",
			input:    `{"kind":"message","content":"hello","session_id":"g1"}`,
			wantType: "assistant",
			wantText: "hello",
			wantSID:  "g1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeGooseEvent([]byte(tt.input))
			if err != nil {
				t.Fatalf("normalizeGooseEvent returned error: %v", err)
			}
			if tt.wantType != "" && event.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", event.Type, tt.wantType)
			}
			if tt.wantText != "" && event.Text != tt.wantText {
				t.Errorf("Text = %q, want %q", event.Text, tt.wantText)
			}
			if tt.wantCost > 0 && math.Abs(event.CostUSD-tt.wantCost) > 1e-9 {
				t.Errorf("CostUSD = %v, want %v", event.CostUSD, tt.wantCost)
			}
			if tt.wantSID != "" && event.SessionID != tt.wantSID {
				t.Errorf("SessionID = %q, want %q", event.SessionID, tt.wantSID)
			}
			if event.Raw == nil {
				t.Error("Raw should be populated")
			}
		})
	}
}

func TestGooseNormalizeEventFallback(t *testing.T) {
	event, err := normalizeGooseEvent([]byte("Goose plain text output"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "assistant" {
		t.Errorf("Type = %q, want assistant", event.Type)
	}
}

func TestGooseNormalizeEventViaDispatch(t *testing.T) {
	line := []byte(`{"type":"result","session_id":"g1","cost_usd":0.10}`)
	event, err := normalizeEvent(ProviderGoose, line)
	if err != nil {
		t.Fatalf("normalizeEvent returned error: %v", err)
	}
	if event.Type != "result" {
		t.Errorf("Type = %q, want result", event.Type)
	}
	if event.CostUSD != 0.10 {
		t.Errorf("CostUSD = %v, want 0.10", event.CostUSD)
	}
}

func TestGooseSanitizeStderr(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "clean output",
			input: "Error: connection refused",
			want:  "Error: connection refused",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "strips rust backtrace header",
			input: "Error: API key invalid\nstack backtrace:\n0: goose::main\n1: std::rt::lang_start",
			want:  "Error: API key invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeGooseStderr(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeGooseStderr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGooseCostRate(t *testing.T) {
	rate, ok := getProviderCostRate(ProviderGoose)
	if !ok {
		t.Fatal("no cost rate for goose provider")
	}
	if rate.InputPer1M <= 0 || rate.OutputPer1M <= 0 {
		t.Errorf("goose cost rates should be positive: input=%v, output=%v",
			rate.InputPer1M, rate.OutputPer1M)
	}
}

func TestGooseUnsupportedOptions(t *testing.T) {
	warnings := UnsupportedOptionsWarnings(ProviderGoose, LaunchOptions{
		SystemPrompt: "test",
		MaxBudgetUSD: 5.0,
		Agent:        "test-agent",
		MaxTurns:     10,
		AllowedTools: []string{"bash"},
		Worktree:     "true",
	})
	if len(warnings) == 0 {
		t.Error("expected warnings for unsupported options")
	}
	// Goose should warn about system_prompt
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "system_prompt") {
			found = true
		}
	}
	if !found {
		t.Error("expected system_prompt warning for goose provider")
	}
}
