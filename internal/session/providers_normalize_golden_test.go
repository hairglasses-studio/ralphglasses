package session

import (
	"math"
	"testing"
)

// TestGoldenCostExtraction is a table-driven golden-file test that verifies the
// three-tier cost extraction pipeline across providers and auth modes:
//   1. Direct cost_usd field (flat or nested in usage)
//   2. Token-based estimation via estimateCostFromTokens
//   3. Stderr regex fallback via ParseProviderCostFromStderr
func TestGoldenCostExtraction(t *testing.T) {
	const tolerance = 1e-9

	tests := []struct {
		name      string
		provider  Provider
		input     string
		wantCost  float64
		wantType  string
		wantSID   string
	}{
		{
			name:     "claude/api_key_nested_usage_cost",
			provider: ProviderClaude,
			input:    `{"type":"result","session_id":"abc","usage":{"input_tokens":1000,"output_tokens":500,"cost_usd":0.045}}`,
			wantCost: 0.045,
			wantType: "result",
			wantSID:  "abc",
		},
		{
			name:     "claude/max_account_flat_cost",
			provider: ProviderClaude,
			input:    `{"type":"result","session_id":"abc","cost_usd":0.12,"content":"done"}`,
			wantCost: 0.12,
			wantType: "result",
			wantSID:  "abc",
		},
		{
			name:     "claude/token_only_estimated",
			provider: ProviderClaude,
			input:    `{"type":"result","session_id":"abc","usage":{"input_tokens":5000,"output_tokens":2000}}`,
			wantCost: (5000.0/1_000_000)*CostClaudeSonnetInput + (2000.0/1_000_000)*CostClaudeSonnetOutput,
			wantType: "result",
			wantSID:  "abc",
		},
		{
			name:     "gemini/explicit_cost",
			provider: ProviderGemini,
			input:    `{"type":"result","session_id":"abc","usage":{"cost_usd":0.03,"input_tokens":800,"output_tokens":200}}`,
			wantCost: 0.03,
			wantType: "result",
			wantSID:  "abc",
		},
		{
			name:     "gemini/token_only_estimated",
			provider: ProviderGemini,
			input:    `{"type":"result","session_id":"abc","usage":{"input_tokens":1000,"output_tokens":500}}`,
			wantCost: (1000.0/1_000_000)*CostGeminiFlashInput + (500.0/1_000_000)*CostGeminiFlashOutput,
			wantType: "result",
			wantSID:  "abc",
		},
		{
			name:     "codex/explicit_cost",
			provider: ProviderCodex,
			input:    `{"type":"result","session_id":"abc","usage":{"cost_usd":0.08}}`,
			wantCost: 0.08,
			wantType: "result",
			wantSID:  "abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeEvent(tt.provider, []byte(tt.input))
			if err != nil {
				t.Fatalf("normalizeEvent returned error: %v", err)
			}
			if event.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", event.Type, tt.wantType)
			}
			if event.SessionID != tt.wantSID {
				t.Errorf("SessionID = %q, want %q", event.SessionID, tt.wantSID)
			}
			if math.Abs(event.CostUSD-tt.wantCost) > tolerance {
				t.Errorf("CostUSD = %.12f, want %.12f (diff=%.2e)",
					event.CostUSD, tt.wantCost, math.Abs(event.CostUSD-tt.wantCost))
			}
			if event.Raw == nil {
				t.Error("Raw should be populated")
			}
		})
	}
}

// TestGoldenStderrCostFallback verifies tier-3 cost extraction from stderr
// output using ParseProviderCostFromStderr.
func TestGoldenStderrCostFallback(t *testing.T) {
	const tolerance = 1e-9

	tests := []struct {
		name     string
		provider Provider
		stderr   string
		wantCost float64
		wantOK   bool
	}{
		{
			name:     "claude/total_cost_with_dollar",
			provider: ProviderClaude,
			stderr:   "Session complete.\nTotal cost: $0.1234\nDone.",
			wantCost: 0.1234,
			wantOK:   true,
		},
		{
			name:     "gemini/cost_with_dollar",
			provider: ProviderGemini,
			stderr:   "Finished generation.\nCost: $0.0567\n",
			wantCost: 0.0567,
			wantOK:   true,
		},
		{
			name:     "codex/cost_usd_label",
			provider: ProviderCodex,
			stderr:   "cost_usd: $0.0890",
			wantCost: 0.0890,
			wantOK:   true,
		},
		{
			name:     "claude/no_cost_in_stderr",
			provider: ProviderClaude,
			stderr:   "Some random output without cost info",
			wantCost: 0,
			wantOK:   false,
		},
		{
			name:     "empty_stderr",
			provider: ProviderClaude,
			stderr:   "",
			wantCost: 0,
			wantOK:   false,
		},
		{
			name:     "claude/ansi_escape_stripped",
			provider: ProviderClaude,
			stderr:   "\x1b[32mTotal cost: $0.0456\x1b[0m",
			wantCost: 0.0456,
			wantOK:   true,
		},
		{
			name:     "gemini/token_count_stderr",
			provider: ProviderGemini,
			stderr:   "prompt_token_count: 2000, candidates_token_count: 800",
			wantCost: (2000.0/1_000_000)*CostGeminiFlashInput + (800.0/1_000_000)*CostGeminiFlashOutput,
			wantOK:   true,
		},
		{
			name:     "codex/token_count_stderr",
			provider: ProviderCodex,
			stderr:   "5000 input tokens, 1000 output tokens used",
			wantCost: (5000.0/1_000_000)*CostCodexInput + (1000.0/1_000_000)*CostCodexOutput,
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, ok := ParseProviderCostFromStderr(tt.provider, tt.stderr)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if math.Abs(cost-tt.wantCost) > tolerance {
				t.Errorf("cost = %.12f, want %.12f (diff=%.2e)",
					cost, tt.wantCost, math.Abs(cost-tt.wantCost))
			}
		})
	}
}

// TestGoldenCostTierPrecedence verifies that explicit cost_usd takes precedence
// over token estimation — i.e., when both cost_usd and token counts are present,
// the explicit cost wins.
func TestGoldenCostTierPrecedence(t *testing.T) {
	const tolerance = 1e-9

	tests := []struct {
		name     string
		provider Provider
		input    string
		wantCost float64
	}{
		{
			name:     "claude/explicit_cost_beats_tokens",
			provider: ProviderClaude,
			input:    `{"type":"result","session_id":"x","cost_usd":0.99,"usage":{"input_tokens":1,"output_tokens":1}}`,
			wantCost: 0.99,
		},
		{
			name:     "gemini/explicit_cost_beats_tokens",
			provider: ProviderGemini,
			input:    `{"type":"result","session_id":"x","usage":{"cost_usd":0.55,"input_tokens":1,"output_tokens":1}}`,
			wantCost: 0.55,
		},
		{
			name:     "codex/explicit_cost_beats_tokens",
			provider: ProviderCodex,
			input:    `{"type":"result","session_id":"x","usage":{"cost_usd":0.77,"input_tokens":1,"output_tokens":1}}`,
			wantCost: 0.77,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := normalizeEvent(tt.provider, []byte(tt.input))
			if err != nil {
				t.Fatalf("normalizeEvent returned error: %v", err)
			}
			if math.Abs(event.CostUSD-tt.wantCost) > tolerance {
				t.Errorf("CostUSD = %.12f, want %.12f — explicit cost should take precedence over token estimation",
					event.CostUSD, tt.wantCost)
			}
		})
	}
}
