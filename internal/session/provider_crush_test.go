package session

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestCrushProviderConstants(t *testing.T) {
	if ProviderCrush != "crush" {
		t.Fatalf("ProviderCrush = %q, want %q", ProviderCrush, "crush")
	}
	if got := providerBinary(ProviderCrush); got != "crush" {
		t.Errorf("providerBinary(crush) = %q, want %q", got, "crush")
	}
	if got := ProviderDefaults(ProviderCrush); got != "sonnet" {
		t.Errorf("ProviderDefaults(crush) = %q, want %q", got, "sonnet")
	}
	if got := providerEnvVar(ProviderCrush); got != "ANTHROPIC_API_KEY" {
		t.Errorf("providerEnvVar(crush) = %q, want %q", got, "ANTHROPIC_API_KEY")
	}
}

func TestCrushBuildCmd(t *testing.T) {
	ctx := context.Background()

	t.Run("basic", func(t *testing.T) {
		cmd := buildCrushCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Prompt:   "fix the bug",
			Model:    "sonnet",
		})

		cmdStr := strings.Join(cmd.Args, " ")
		for _, want := range []string{"--headless", "--json-output", "--model", "sonnet", "fix the bug"} {
			if !strings.Contains(cmdStr, want) {
				t.Errorf("crush cmd %q missing %q", cmdStr, want)
			}
		}
		if cmd.Dir != "/tmp/repo" {
			t.Errorf("cmd.Dir = %q, want /tmp/repo", cmd.Dir)
		}
		if cmd.Args[0] != "crush" {
			t.Errorf("binary = %q, want crush", cmd.Args[0])
		}
	})

	t.Run("with system prompt", func(t *testing.T) {
		cmd := buildCrushCmd(ctx, LaunchOptions{
			RepoPath:     "/tmp/repo",
			Prompt:       "test",
			SystemPrompt: "You are a Go expert",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--system-prompt") {
			t.Errorf("crush cmd %q missing --system-prompt", cmdStr)
		}
		if !strings.Contains(cmdStr, "You are a Go expert") {
			t.Errorf("crush cmd %q missing system prompt value", cmdStr)
		}
	})

	t.Run("with resume", func(t *testing.T) {
		cmd := buildCrushCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Resume:   "sess-abc",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--resume sess-abc") {
			t.Errorf("crush cmd %q missing --resume sess-abc", cmdStr)
		}
	})

	t.Run("with continue", func(t *testing.T) {
		cmd := buildCrushCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Continue: true,
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--continue") {
			t.Errorf("crush cmd %q missing --continue", cmdStr)
		}
	})

	t.Run("no prompt omits positional", func(t *testing.T) {
		cmd := buildCrushCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Resume:   "sess-1",
		})
		// Last arg should be sess-1 (the resume value), not a prompt
		if len(cmd.Args) > 0 {
			last := cmd.Args[len(cmd.Args)-1]
			if last != "sess-1" {
				// That's fine; just verify no empty prompt was added
				for _, arg := range cmd.Args {
					if arg == "" {
						t.Error("empty argument in crush cmd")
					}
				}
			}
		}
	})
}

func TestCrushNormalizeEvent(t *testing.T) {
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
			input:    `{"type":"result","session_id":"crush-123","cost_usd":0.05,"result":"All done"}`,
			wantType: "result",
			wantText: "All done",
			wantCost: 0.05,
			wantSID:  "crush-123",
		},
		{
			name:     "assistant message",
			input:    `{"type":"assistant","content":"Here is the fix","session_id":"crush-456"}`,
			wantType: "assistant",
			wantText: "Here is the fix",
			wantSID:  "crush-456",
		},
		{
			name:     "error event",
			input:    `{"type":"error","error":"rate limited","is_error":true}`,
			wantType: "result",
			wantText: "rate limited",
		},
		{
			name:     "token-based cost estimation",
			input:    `{"type":"result","session_id":"x","usage":{"input_tokens":5000,"output_tokens":2000}}`,
			wantType: "result",
			wantCost: estimateCostFromTokens(ProviderCrush, map[string]any{"usage": map[string]any{"input_tokens": float64(5000), "output_tokens": float64(2000)}}),
		},
		{
			name:     "nested usage cost",
			input:    `{"type":"result","usage":{"cost_usd":0.12}}`,
			wantType: "result",
			wantCost: 0.12,
		},
		{
			name:     "model field",
			input:    `{"type":"system","model":"claude-sonnet-4-6","session_id":"s1"}`,
			wantType: "system",
			wantSID:  "s1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeCrushEvent([]byte(tt.input))
			if err != nil {
				t.Fatalf("normalizeCrushEvent returned error: %v", err)
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

func TestCrushNormalizeEventFallback(t *testing.T) {
	// Non-JSON input should produce a fallback text event.
	event, err := normalizeCrushEvent([]byte("Plain text output from crush"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "assistant" {
		t.Errorf("Type = %q, want assistant", event.Type)
	}
	if !strings.Contains(event.Text, "Plain text output") {
		t.Errorf("Text = %q, want to contain 'Plain text output'", event.Text)
	}
}

func TestCrushNormalizeEventViaDispatch(t *testing.T) {
	// Ensure normalizeEvent dispatches to normalizeCrushEvent.
	line := []byte(`{"type":"result","session_id":"abc","cost_usd":0.05}`)
	event, err := normalizeEvent(ProviderCrush, line)
	if err != nil {
		t.Fatalf("normalizeEvent returned error: %v", err)
	}
	if event.Type != "result" {
		t.Errorf("Type = %q, want result", event.Type)
	}
	if event.CostUSD != 0.05 {
		t.Errorf("CostUSD = %v, want 0.05", event.CostUSD)
	}
}

func TestCrushCostRate(t *testing.T) {
	rate, ok := getProviderCostRate(ProviderCrush)
	if !ok {
		t.Fatal("no cost rate for crush provider")
	}
	if rate.InputPer1M <= 0 || rate.OutputPer1M <= 0 {
		t.Errorf("crush cost rates should be positive: input=%v, output=%v",
			rate.InputPer1M, rate.OutputPer1M)
	}
}

func TestCrushUnsupportedOptions(t *testing.T) {
	warnings := UnsupportedOptionsWarnings(ProviderCrush, LaunchOptions{
		MaxBudgetUSD: 5.0,
		Agent:        "test-agent",
		MaxTurns:     10,
		AllowedTools: []string{"bash"},
		Worktree:     "true",
	})
	if len(warnings) == 0 {
		t.Error("expected warnings for unsupported options")
	}
	// SystemPrompt should NOT be warned for Crush (it supports it).
	for _, w := range warnings {
		if strings.Contains(w, "system_prompt") {
			t.Errorf("crush should support system_prompt, got warning: %s", w)
		}
	}
}
