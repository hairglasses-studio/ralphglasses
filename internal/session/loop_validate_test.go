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

func TestValidateLoopConfig_WorkerEnhancementNonClaude(t *testing.T) {
	tests := []struct {
		name           string
		workerProvider Provider
		enableEnhance  bool
		wantWarn       bool
	}{
		{"claude worker with enhancement: no warn", ProviderClaude, true, false},
		{"gemini worker with enhancement: warn", ProviderGemini, true, true},
		{"codex worker with enhancement: warn", ProviderCodex, true, true},
		{"gemini worker no enhancement: no warn", ProviderGemini, false, false},
		{"empty provider with enhancement: no warn", "", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := LoopConfig{
				WorkerProvider:          tt.workerProvider,
				EnableWorkerEnhancement: tt.enableEnhance,
			}
			warnings := ValidateLoopConfig(cfg)
			hasWarn := false
			for _, w := range warnings {
				if w.Field == "enable_worker_enhancement" {
					hasWarn = true
				}
			}
			if hasWarn != tt.wantWarn {
				t.Errorf("worker enhancement warning: got %v, want %v (warnings: %v)", hasWarn, tt.wantWarn, warnings)
			}
		})
	}
}

func TestValidateLoopProfile_WorkerEnhancementNonClaude(t *testing.T) {
	p := validProfile()
	p.EnableWorkerEnhancement = true
	p.WorkerProvider = ProviderGemini
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for worker enhancement with non-Claude provider")
	}

	// Claude worker + enhancement should be fine.
	p2 := validProfile()
	p2.EnableWorkerEnhancement = true
	p2.WorkerProvider = ProviderClaude
	if err := ValidateLoopProfile(p2); err != nil {
		t.Errorf("unexpected error for Claude worker with enhancement: %v", err)
	}

	// Enhancement disabled with non-Claude: no error.
	p3 := validProfile()
	p3.EnableWorkerEnhancement = false
	p3.WorkerProvider = ProviderGemini
	if err := ValidateLoopProfile(p3); err != nil {
		t.Errorf("unexpected error for disabled enhancement: %v", err)
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

// --- ValidateLoopProfile tests ---

func validProfile() LoopProfile {
	return LoopProfile{
		PlannerProvider:      ProviderClaude,
		WorkerProvider:       ProviderClaude,
		MaxIterations:        10,
		MaxConcurrentWorkers: 2,
		PlannerBudgetUSD:     5.0,
		WorkerBudgetUSD:      10.0,
		RetryLimit:           3,
		StallTimeout:         10 * time.Minute,
	}
}

func TestValidateLoopProfile_ValidProfile(t *testing.T) {
	if err := ValidateLoopProfile(validProfile()); err != nil {
		t.Errorf("expected nil error for valid profile, got: %v", err)
	}
}

func TestValidateLoopProfile_ValidDefaults(t *testing.T) {
	// Minimal valid profile: empty providers, one limit set, zero budgets.
	p := LoopProfile{MaxDurationSecs: 3600}
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for minimal valid profile, got: %v", err)
	}
}

func TestValidateLoopProfile_InvalidPlannerProvider(t *testing.T) {
	p := validProfile()
	p.PlannerProvider = "invalid"
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for invalid planner_provider")
	}
}

func TestValidateLoopProfile_InvalidWorkerProvider(t *testing.T) {
	p := validProfile()
	p.WorkerProvider = "invalid"
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for invalid worker_provider")
	}
}

func TestValidateLoopProfile_ZeroLimitsValid(t *testing.T) {
	// Zero means unlimited — valid for both fields.
	p := validProfile()
	p.MaxIterations = 0
	p.MaxDurationSecs = 0
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for zero limits (unlimited), got: %v", err)
	}
}

func TestValidateLoopProfile_NegativeIterations(t *testing.T) {
	p := validProfile()
	p.MaxIterations = -1
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for negative max_iterations")
	}
}

func TestValidateLoopProfile_NegativeDuration(t *testing.T) {
	p := validProfile()
	p.MaxDurationSecs = -1
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for negative max_duration_secs")
	}
}

func TestValidateLoopProfile_MaxConcurrentWorkersOutOfRange(t *testing.T) {
	tests := []struct {
		name    string
		val     int
		wantErr bool
	}{
		{"zero ok", 0, false},
		{"one ok", 1, false},
		{"ten ok", 10, false},
		{"eleven invalid", 11, true},
		{"negative invalid", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validProfile()
			p.MaxConcurrentWorkers = tt.val
			err := ValidateLoopProfile(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxConcurrentWorkers=%d: got err=%v, wantErr=%v", tt.val, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLoopProfile_BudgetMismatch(t *testing.T) {
	// Only planner budget set.
	p := validProfile()
	p.PlannerBudgetUSD = 5.0
	p.WorkerBudgetUSD = 0
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error when only planner_budget_usd is set")
	}

	// Only worker budget set.
	p2 := validProfile()
	p2.PlannerBudgetUSD = 0
	p2.WorkerBudgetUSD = 10.0
	if err := ValidateLoopProfile(p2); err == nil {
		t.Error("expected error when only worker_budget_usd is set")
	}

	// Both zero is fine.
	p3 := validProfile()
	p3.PlannerBudgetUSD = 0
	p3.WorkerBudgetUSD = 0
	if err := ValidateLoopProfile(p3); err != nil {
		t.Errorf("expected nil error when both budgets are zero, got: %v", err)
	}
}

func TestValidateLoopProfile_RetryLimitOutOfRange(t *testing.T) {
	tests := []struct {
		name    string
		val     int
		wantErr bool
	}{
		{"zero ok", 0, false},
		{"ten ok", 10, false},
		{"eleven invalid", 11, true},
		{"negative invalid", -1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validProfile()
			p.RetryLimit = tt.val
			err := ValidateLoopProfile(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("RetryLimit=%d: got err=%v, wantErr=%v", tt.val, err, tt.wantErr)
			}
		})
	}
}

func TestValidateLoopProfile_NegativeStallTimeout(t *testing.T) {
	p := validProfile()
	p.StallTimeout = -1 * time.Second
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for negative stall_timeout")
	}

	// Zero is fine.
	p.StallTimeout = 0
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for zero stall_timeout, got: %v", err)
	}
}

func TestValidateLoopProfile_AllProviders(t *testing.T) {
	for _, prov := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex, ""} {
		p := validProfile()
		p.PlannerProvider = prov
		p.WorkerProvider = prov
		if err := ValidateLoopProfile(p); err != nil {
			t.Errorf("provider %q should be valid, got: %v", prov, err)
		}
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
