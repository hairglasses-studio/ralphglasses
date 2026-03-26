package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultCostRatesNonEmpty(t *testing.T) {
	rates := DefaultCostRates()
	if len(rates.InputPerMToken) == 0 {
		t.Fatal("DefaultCostRates().InputPerMToken is empty")
	}
	if len(rates.OutputPerMToken) == 0 {
		t.Fatal("DefaultCostRates().OutputPerMToken is empty")
	}

	// Verify specific keys from compiled-in constants.
	wantInput := map[string]float64{
		"gemini_flash_lite": CostGeminiFlashLiteInput,
		"gemini_flash":      CostGeminiFlashInput,
		"claude_sonnet":     CostClaudeSonnetInput,
		"claude_opus":       CostClaudeOpusInput,
		"codex":             CostCodexInput,
	}
	for k, want := range wantInput {
		if got, ok := rates.InputPerMToken[k]; !ok {
			t.Errorf("missing input key %q", k)
		} else if got != want {
			t.Errorf("input[%q] = %v, want %v", k, got, want)
		}
	}
}

func TestLoadCostRatesCustomFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cost_rates.json")

	data := `{
		"input_per_m_token": {"claude_sonnet": 5.00, "codex": 3.50},
		"output_per_m_token": {"claude_sonnet": 20.00}
	}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	rates, err := LoadCostRates(path)
	if err != nil {
		t.Fatalf("LoadCostRates error: %v", err)
	}

	// Overridden values.
	if got := rates.InputPerMToken["claude_sonnet"]; got != 5.00 {
		t.Errorf("input[claude_sonnet] = %v, want 5.00", got)
	}
	if got := rates.InputPerMToken["codex"]; got != 3.50 {
		t.Errorf("input[codex] = %v, want 3.50", got)
	}
	if got := rates.OutputPerMToken["claude_sonnet"]; got != 20.00 {
		t.Errorf("output[claude_sonnet] = %v, want 20.00", got)
	}

	// Default-filled values (not in JSON but merged from defaults).
	if got := rates.InputPerMToken["gemini_flash"]; got != CostGeminiFlashInput {
		t.Errorf("input[gemini_flash] = %v, want %v (default)", got, CostGeminiFlashInput)
	}
	if got := rates.OutputPerMToken["codex"]; got != CostCodexOutput {
		t.Errorf("output[codex] = %v, want %v (default)", got, CostCodexOutput)
	}
}

func TestLoadCostRatesMissingFileFallsBack(t *testing.T) {
	rates, err := LoadCostRates("/nonexistent/path/cost_rates.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}

	defaults := DefaultCostRates()
	if len(rates.InputPerMToken) != len(defaults.InputPerMToken) {
		t.Errorf("input map len = %d, want %d", len(rates.InputPerMToken), len(defaults.InputPerMToken))
	}
	for k, want := range defaults.InputPerMToken {
		if got := rates.InputPerMToken[k]; got != want {
			t.Errorf("input[%q] = %v, want %v", k, got, want)
		}
	}
}

func TestLoadCostRatesMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte("{not valid json!"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadCostRates(path)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestProviderCostRateFromMatchesDefaults(t *testing.T) {
	rates := DefaultCostRates()

	for _, tc := range []struct {
		provider  Provider
		wantIn    float64
		wantOut   float64
	}{
		{ProviderClaude, CostClaudeSonnetInput, CostClaudeSonnetOutput},
		{ProviderGemini, CostGeminiFlashInput, CostGeminiFlashOutput},
		{ProviderCodex, CostCodexInput, CostCodexOutput},
	} {
		got := rates.ProviderCostRateFrom(tc.provider)
		if got.InputPer1M != tc.wantIn {
			t.Errorf("%s input = %v, want %v", tc.provider, got.InputPer1M, tc.wantIn)
		}
		if got.OutputPer1M != tc.wantOut {
			t.Errorf("%s output = %v, want %v", tc.provider, got.OutputPer1M, tc.wantOut)
		}
	}
}

func TestProviderCostRateFromCustom(t *testing.T) {
	rates := &CostRates{
		InputPerMToken:  map[string]float64{"claude_sonnet": 99.00},
		OutputPerMToken: map[string]float64{"claude_sonnet": 199.00},
	}

	got := rates.ProviderCostRateFrom(ProviderClaude)
	if got.InputPer1M != 99.00 {
		t.Errorf("custom input = %v, want 99.00", got.InputPer1M)
	}
	if got.OutputPer1M != 199.00 {
		t.Errorf("custom output = %v, want 199.00", got.OutputPer1M)
	}

	// Gemini not in custom rates => falls back to compiled-in defaults.
	got2 := rates.ProviderCostRateFrom(ProviderGemini)
	if got2.InputPer1M != CostGeminiFlashInput {
		t.Errorf("gemini fallback input = %v, want %v", got2.InputPer1M, CostGeminiFlashInput)
	}
}
