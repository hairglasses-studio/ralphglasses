package session

import (
	"testing"
	"time"
)

func TestValidateLoopConfig_ValidConfig(t *testing.T) {
	cfg := LoopConfig{
		Provider:         ProviderClaude,
		Model:            "claude-sonnet-4-20250514",
		BudgetUSD:        5.0,
		EnhancePrompt:    true,
		EnhancerProvider: "claude",
		Timeout:          5 * time.Minute,
	}
	warnings := ValidateLoopConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid config, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateLoopConfig_ModelProviderMismatch(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		model    string
		wantWarn bool
	}{
		{"claude model on claude", ProviderClaude, "claude-sonnet-4-20250514", false},
		{"gemini model on gemini", ProviderGemini, "gemini-2.5-pro", false},
		{"gpt model on codex", ProviderCodex, "gpt-4o", false},
		{"o1 model on codex", ProviderCodex, "o1-preview", false},
		{"o3 model on codex", ProviderCodex, "o3-mini", false},
		{"codex model on codex", ProviderCodex, "codex-mini-latest", false},
		{"claude model on gemini", ProviderGemini, "claude-sonnet-4-20250514", true},
		{"gemini model on claude", ProviderClaude, "gemini-2.5-pro", true},
		{"gpt model on claude", ProviderClaude, "gpt-4o", true},
		{"empty provider skips check", "", "anything", false},
		{"empty model skips check", ProviderClaude, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LoopConfig{Provider: tt.provider, Model: tt.model}
			warnings := ValidateLoopConfig(cfg)
			hasModelWarn := false
			for _, w := range warnings {
				if w.Field == "model" {
					hasModelWarn = true
				}
			}
			if hasModelWarn != tt.wantWarn {
				t.Errorf("model warning: got %v, want %v (warnings: %v)", hasModelWarn, tt.wantWarn, warnings)
			}
		})
	}
}

func TestValidateLoopConfig_BudgetOutOfRange(t *testing.T) {
	tests := []struct {
		name      string
		budget    float64
		wantWarns int
	}{
		{"zero budget no warn", 0, 0},
		{"normal budget", 10.0, 0},
		{"min boundary", 0.01, 0},
		{"max boundary", 100.0, 0},
		{"too low", 0.001, 1},
		{"too high", 150.0, 1},
		{"negative", -1.0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LoopConfig{BudgetUSD: tt.budget}
			warnings := ValidateLoopConfig(cfg)
			budgetWarns := 0
			for _, w := range warnings {
				if w.Field == "budget_usd" {
					budgetWarns++
				}
			}
			if budgetWarns != tt.wantWarns {
				t.Errorf("budget warnings: got %d, want %d (warnings: %v)", budgetWarns, tt.wantWarns, warnings)
			}
		})
	}
}

func TestValidateLoopConfig_EnhancementWithoutProvider(t *testing.T) {
	// Enhancement enabled with provider: no warning.
	cfg := LoopConfig{EnhancePrompt: true, EnhancerProvider: "claude"}
	warnings := ValidateLoopConfig(cfg)
	for _, w := range warnings {
		if w.Field == "enhance_prompt" {
			t.Errorf("unexpected enhancement warning when provider is set: %v", w)
		}
	}

	// Enhancement enabled without provider: warning.
	cfg = LoopConfig{EnhancePrompt: true}
	warnings = ValidateLoopConfig(cfg)
	found := false
	for _, w := range warnings {
		if w.Field == "enhance_prompt" {
			found = true
		}
	}
	if !found {
		t.Error("expected enhancement warning when no enhancer provider is configured")
	}

	// Enhancement disabled without provider: no warning.
	cfg = LoopConfig{EnhancePrompt: false}
	warnings = ValidateLoopConfig(cfg)
	for _, w := range warnings {
		if w.Field == "enhance_prompt" {
			t.Errorf("unexpected enhancement warning when enhancement is disabled: %v", w)
		}
	}
}

func TestValidateLoopConfig_TimeoutValidation(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		wantWarn bool
	}{
		{"zero timeout no warn", 0, false},
		{"normal timeout", 5 * time.Minute, false},
		{"min boundary", 10 * time.Second, false},
		{"max boundary", 3600 * time.Second, false},
		{"too short", 5 * time.Second, true},
		{"too long", 2 * time.Hour, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LoopConfig{Timeout: tt.timeout}
			warnings := ValidateLoopConfig(cfg)
			hasTimeoutWarn := false
			for _, w := range warnings {
				if w.Field == "timeout" {
					hasTimeoutWarn = true
				}
			}
			if hasTimeoutWarn != tt.wantWarn {
				t.Errorf("timeout warning: got %v, want %v (warnings: %v)", hasTimeoutWarn, tt.wantWarn, warnings)
			}
		})
	}
}

func TestLoopValidationWarning_String(t *testing.T) {
	w := LoopValidationWarning{Field: "model", Message: "mismatch"}
	got := w.String()
	want := "model: mismatch"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
