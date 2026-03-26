package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultProviderCostsNonEmpty(t *testing.T) {
	costs := DefaultProviderCosts()
	if len(costs.InputPerMToken) == 0 {
		t.Fatal("DefaultProviderCosts().InputPerMToken is empty")
	}
	if len(costs.OutputPerMToken) == 0 {
		t.Fatal("DefaultProviderCosts().OutputPerMToken is empty")
	}

	// Verify specific keys match compiled-in constants.
	wantInput := map[string]float64{
		"gemini_flash_lite": CostGeminiFlashLiteInput,
		"gemini_flash":      CostGeminiFlashInput,
		"claude_sonnet":     CostClaudeSonnetInput,
		"claude_opus":       CostClaudeOpusInput,
		"codex":             CostCodexInput,
	}
	for k, want := range wantInput {
		if got, ok := costs.InputPerMToken[k]; !ok {
			t.Errorf("missing input key %q", k)
		} else if got != want {
			t.Errorf("input[%q] = %v, want %v", k, got, want)
		}
	}
}

func TestLoadProviderCostsCustomFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "costs.json")

	data := `{
		"input_per_m_token": {"claude_sonnet": 5.00, "codex": 3.50},
		"output_per_m_token": {"claude_sonnet": 20.00}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	costs, err := LoadProviderCosts(path)
	if err != nil {
		t.Fatalf("LoadProviderCosts error: %v", err)
	}

	// Overridden values.
	if got := costs.InputPerMToken["claude_sonnet"]; got != 5.00 {
		t.Errorf("input[claude_sonnet] = %v, want 5.00", got)
	}
	if got := costs.InputPerMToken["codex"]; got != 3.50 {
		t.Errorf("input[codex] = %v, want 3.50", got)
	}
	if got := costs.OutputPerMToken["claude_sonnet"]; got != 20.00 {
		t.Errorf("output[claude_sonnet] = %v, want 20.00", got)
	}

	// Default-filled values (not in JSON but merged from defaults).
	if got := costs.InputPerMToken["gemini_flash"]; got != CostGeminiFlashInput {
		t.Errorf("input[gemini_flash] = %v, want %v (default)", got, CostGeminiFlashInput)
	}
	if got := costs.OutputPerMToken["codex"]; got != CostCodexOutput {
		t.Errorf("output[codex] = %v, want %v (default)", got, CostCodexOutput)
	}
}

func TestLoadProviderCostsMissingFileFallsBack(t *testing.T) {
	costs, err := LoadProviderCosts("/nonexistent/path/costs.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	defaults := DefaultProviderCosts()
	if len(costs.InputPerMToken) != len(defaults.InputPerMToken) {
		t.Errorf("input map len = %d, want %d", len(costs.InputPerMToken), len(defaults.InputPerMToken))
	}
	for k, want := range defaults.InputPerMToken {
		if got := costs.InputPerMToken[k]; got != want {
			t.Errorf("input[%q] = %v, want %v", k, got, want)
		}
	}
}

func TestLoadProviderCostsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte("{not valid json!"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProviderCosts(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestInputRateAndOutputRate(t *testing.T) {
	costs := DefaultProviderCosts()

	// Known key returns map value.
	if got := costs.InputRate("claude_sonnet", 0); got != CostClaudeSonnetInput {
		t.Errorf("InputRate(claude_sonnet) = %v, want %v", got, CostClaudeSonnetInput)
	}

	// Unknown key returns fallback.
	if got := costs.InputRate("unknown_model", 42.0); got != 42.0 {
		t.Errorf("InputRate(unknown_model) = %v, want 42.0", got)
	}

	// Nil receiver returns fallback.
	var nilCosts *ProviderCosts
	if got := nilCosts.InputRate("claude_sonnet", 99.0); got != 99.0 {
		t.Errorf("nil InputRate = %v, want 99.0", got)
	}
	if got := nilCosts.OutputRate("claude_sonnet", 88.0); got != 88.0 {
		t.Errorf("nil OutputRate = %v, want 88.0", got)
	}
}

func TestCostRateForProvider(t *testing.T) {
	costs := DefaultProviderCosts()

	tests := []struct {
		provider  string
		wantIn    float64
		wantOut   float64
	}{
		{"claude", CostClaudeSonnetInput, CostClaudeSonnetOutput},
		{"gemini", CostGeminiFlashInput, CostGeminiFlashOutput},
		{"codex", CostCodexInput, CostCodexOutput},
		{"openai", CostCodexInput, CostCodexOutput},
	}

	for _, tc := range tests {
		inRate, outRate := costs.CostRateForProvider(tc.provider)
		if inRate != tc.wantIn {
			t.Errorf("%s input = %v, want %v", tc.provider, inRate, tc.wantIn)
		}
		if outRate != tc.wantOut {
			t.Errorf("%s output = %v, want %v", tc.provider, outRate, tc.wantOut)
		}
	}
}

func TestCostRateForProviderCustomOverride(t *testing.T) {
	costs := &ProviderCosts{
		InputPerMToken:  map[string]float64{"claude_sonnet": 99.00},
		OutputPerMToken: map[string]float64{"claude_sonnet": 199.00},
	}

	inRate, outRate := costs.CostRateForProvider("claude")
	if inRate != 99.00 {
		t.Errorf("custom claude input = %v, want 99.00", inRate)
	}
	if outRate != 199.00 {
		t.Errorf("custom claude output = %v, want 199.00", outRate)
	}

	// Gemini not in custom map => falls back to compiled-in defaults.
	inRate2, _ := costs.CostRateForProvider("gemini")
	if inRate2 != CostGeminiFlashInput {
		t.Errorf("gemini fallback input = %v, want %v", inRate2, CostGeminiFlashInput)
	}
}

func TestCostRateForProviderUnknown(t *testing.T) {
	costs := DefaultProviderCosts()
	inRate, outRate := costs.CostRateForProvider("unknown_provider")
	// Unknown defaults to Claude rates.
	if inRate != CostClaudeSonnetInput {
		t.Errorf("unknown provider input = %v, want %v", inRate, CostClaudeSonnetInput)
	}
	if outRate != CostClaudeSonnetOutput {
		t.Errorf("unknown provider output = %v, want %v", outRate, CostClaudeSonnetOutput)
	}
}
