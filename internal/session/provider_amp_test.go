package session

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestAmpProviderConstants(t *testing.T) {
	if ProviderAmp != "amp" {
		t.Fatalf("ProviderAmp = %q, want %q", ProviderAmp, "amp")
	}
	if got := providerBinary(ProviderAmp); got != "amp" {
		t.Errorf("providerBinary(amp) = %q, want %q", got, "amp")
	}
	if got := ProviderDefaults(ProviderAmp); got != "amp-default" {
		t.Errorf("ProviderDefaults(amp) = %q, want %q", got, "amp-default")
	}
	if got := providerEnvVar(ProviderAmp); got != "AMP_ACCESS_TOKEN" {
		t.Errorf("providerEnvVar(amp) = %q, want %q", got, "AMP_ACCESS_TOKEN")
	}
}

func TestAmpBuildCmd(t *testing.T) {
	ctx := context.Background()

	t.Run("basic", func(t *testing.T) {
		cmd := buildAmpCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Prompt:   "analyze the codebase",
			Model:    "amp-default",
		})

		cmdStr := strings.Join(cmd.Args, " ")
		for _, want := range []string{"run", "--output-format", "json", "--non-interactive", "--model", "amp-default", "analyze the codebase"} {
			if !strings.Contains(cmdStr, want) {
				t.Errorf("amp cmd %q missing %q", cmdStr, want)
			}
		}
		if cmd.Dir != "/tmp/repo" {
			t.Errorf("cmd.Dir = %q, want /tmp/repo", cmd.Dir)
		}
		if cmd.Args[0] != "amp" {
			t.Errorf("binary = %q, want amp", cmd.Args[0])
		}
	})

	t.Run("with resume", func(t *testing.T) {
		cmd := buildAmpCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
			Resume:   "thread-amp-1",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if !strings.Contains(cmdStr, "--resume thread-amp-1") {
			t.Errorf("amp cmd %q missing --resume", cmdStr)
		}
	})

	t.Run("no model no prompt", func(t *testing.T) {
		cmd := buildAmpCmd(ctx, LaunchOptions{
			RepoPath: "/tmp/repo",
		})
		cmdStr := strings.Join(cmd.Args, " ")
		if strings.Contains(cmdStr, "--model") {
			t.Errorf("amp cmd %q should not have --model when empty", cmdStr)
		}
		// Should have run, --output-format json, --non-interactive
		if !strings.Contains(cmdStr, "run") {
			t.Errorf("amp cmd %q missing 'run'", cmdStr)
		}
		if !strings.Contains(cmdStr, "--non-interactive") {
			t.Errorf("amp cmd %q missing --non-interactive", cmdStr)
		}
	})
}

func TestAmpNormalizeEvent(t *testing.T) {
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
			input:    `{"type":"result","session_id":"amp-123","cost_usd":0.10,"result":"Analysis complete"}`,
			wantType: "result",
			wantText: "Analysis complete",
			wantCost: 0.10,
			wantSID:  "amp-123",
		},
		{
			name:     "assistant content",
			input:    `{"type":"assistant","content":"Scanning codebase...","model":"gpt-5.4"}`,
			wantType: "assistant",
			wantText: "Scanning codebase...",
		},
		{
			name:     "error event",
			input:    `{"type":"error","error":"authentication failed","is_error":true}`,
			wantType: "result",
			wantText: "authentication failed",
		},
		{
			name:     "thread_id as session_id",
			input:    `{"type":"system","thread_id":"amp-thread-1"}`,
			wantType: "system",
			wantSID:  "amp-thread-1",
		},
		{
			name:     "token estimation",
			input:    `{"type":"result","usage":{"input_tokens":8000,"output_tokens":4000}}`,
			wantType: "result",
			wantCost: estimateCostFromTokens(ProviderAmp, map[string]any{"usage": map[string]any{"input_tokens": float64(8000), "output_tokens": float64(4000)}}),
		},
		{
			name:     "nested usage cost",
			input:    `{"type":"result","usage":{"cost_usd":0.22}}`,
			wantType: "result",
			wantCost: 0.22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeAmpEvent([]byte(tt.input))
			if err != nil {
				t.Fatalf("normalizeAmpEvent returned error: %v", err)
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

func TestAmpNormalizeEventFallback(t *testing.T) {
	event, err := normalizeAmpEvent([]byte("Amp plain text output"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "assistant" {
		t.Errorf("Type = %q, want assistant", event.Type)
	}
}

func TestAmpNormalizeEventViaDispatch(t *testing.T) {
	line := []byte(`{"type":"result","session_id":"a1","cost_usd":0.15}`)
	event, err := normalizeEvent(ProviderAmp, line)
	if err != nil {
		t.Fatalf("normalizeEvent returned error: %v", err)
	}
	if event.Type != "result" {
		t.Errorf("Type = %q, want result", event.Type)
	}
	if event.CostUSD != 0.15 {
		t.Errorf("CostUSD = %v, want 0.15", event.CostUSD)
	}
}

func TestAmpCostRate(t *testing.T) {
	rate, ok := getProviderCostRate(ProviderAmp)
	if !ok {
		t.Fatal("no cost rate for amp provider")
	}
	if rate.InputPer1M <= 0 || rate.OutputPer1M <= 0 {
		t.Errorf("amp cost rates should be positive: input=%v, output=%v",
			rate.InputPer1M, rate.OutputPer1M)
	}
}

func TestAmpUnsupportedOptions(t *testing.T) {
	warnings := UnsupportedOptionsWarnings(ProviderAmp, LaunchOptions{
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
	// Amp should warn about system_prompt
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "system_prompt") {
			found = true
		}
	}
	if !found {
		t.Error("expected system_prompt warning for amp provider")
	}
}
