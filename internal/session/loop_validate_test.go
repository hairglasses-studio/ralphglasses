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

func TestValidateLoopConfig_WorkerEnhancementProviderAgnostic(t *testing.T) {
	tests := []struct {
		name           string
		workerProvider Provider
		enableEnhance  bool
	}{
		{"claude worker with enhancement: no warn", ProviderClaude, true},
		{"gemini worker with enhancement: no warn", ProviderGemini, true},
		{"codex worker with enhancement: no warn", ProviderCodex, true},
		{"gemini worker no enhancement: no warn", ProviderGemini, false},
		{"empty provider with enhancement: no warn", "", true},
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
			if hasWarn {
				t.Errorf("unexpected worker enhancement warning for provider %q: %v", tt.workerProvider, warnings)
			}
		})
	}
}

func TestValidateLoopProfile_WorkerEnhancementProviderAgnostic(t *testing.T) {
	p := validProfile()
	p.EnableWorkerEnhancement = true
	p.WorkerProvider = ProviderGemini
	p.WorkerModel = "gemini-2.5-pro"
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("unexpected error for gemini worker enhancement: %v", err)
	}

	// Claude worker + enhancement should be fine.
	p2 := validProfile()
	p2.EnableWorkerEnhancement = true
	p2.WorkerProvider = ProviderClaude
	if err := ValidateLoopProfile(p2); err != nil {
		t.Errorf("unexpected error for Claude worker with enhancement: %v", err)
	}

	// Codex worker + enhancement should also be fine.
	p3 := validProfile()
	p3.EnableWorkerEnhancement = true
	p3.WorkerProvider = ProviderCodex
	p3.WorkerModel = "gpt-5.4"
	if err := ValidateLoopProfile(p3); err != nil {
		t.Errorf("unexpected error for Codex worker with enhancement: %v", err)
	}

	// Enhancement disabled with non-Claude: no error.
	p4 := validProfile()
	p4.EnableWorkerEnhancement = false
	p4.WorkerProvider = ProviderGemini
	p4.WorkerModel = "gemini-2.5-pro"
	if err := ValidateLoopProfile(p4); err != nil {
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
		PlannerModel:         "claude-sonnet-4-6",
		WorkerProvider:       ProviderClaude,
		WorkerModel:          "claude-sonnet-4-6",
		VerifierProvider:     ProviderClaude,
		VerifierModel:        "claude-sonnet-4-6",
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
	modelForProvider := map[Provider]string{
		ProviderClaude: "claude-sonnet-4-6",
		ProviderGemini: "gemini-2.5-pro",
		ProviderCodex:  "gpt-4o",
		"":             "",
	}
	for _, prov := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex, ""} {
		p := validProfile()
		p.PlannerProvider = prov
		p.PlannerModel = modelForProvider[prov]
		p.WorkerProvider = prov
		p.WorkerModel = modelForProvider[prov]
		p.VerifierProvider = prov
		p.VerifierModel = modelForProvider[prov]
		if err := ValidateLoopProfile(p); err != nil {
			t.Errorf("provider %q should be valid, got: %v", prov, err)
		}
	}
}

func TestValidateLoopProfile_InvalidVerifierProvider(t *testing.T) {
	p := validProfile()
	p.VerifierProvider = "invalid"
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for invalid verifier_provider")
	}

	// Valid verifier providers should pass.
	modelForProvider := map[Provider]string{
		ProviderClaude: "claude-sonnet-4-6",
		ProviderGemini: "gemini-2.5-pro",
		ProviderCodex:  "gpt-4o",
		"":             "",
	}
	for _, prov := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex, ""} {
		p2 := validProfile()
		p2.VerifierProvider = prov
		p2.VerifierModel = modelForProvider[prov]
		if err := ValidateLoopProfile(p2); err != nil {
			t.Errorf("verifier_provider %q should be valid, got: %v", prov, err)
		}
	}
}

func TestValidateLoopProfile_ModelProviderMismatch(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		provider Provider
		model    string
		wantErr  bool
	}{
		// Planner
		{"planner claude model on claude", "planner", ProviderClaude, "claude-sonnet-4-6", false},
		{"planner gemini model on claude", "planner", ProviderClaude, "gemini-2.5-pro", true},
		{"planner empty model skips", "planner", ProviderClaude, "", false},
		{"planner empty provider skips", "planner", "", "claude-sonnet-4-6", false},
		// Worker
		{"worker gpt model on codex", "worker", ProviderCodex, "gpt-4o", false},
		{"worker claude model on codex", "worker", ProviderCodex, "claude-sonnet-4-6", true},
		{"worker o3 model on codex", "worker", ProviderCodex, "o3-mini", false},
		{"worker codex model on codex", "worker", ProviderCodex, "codex-mini-latest", false},
		// Verifier
		{"verifier gemini model on gemini", "verifier", ProviderGemini, "gemini-2.5-pro", false},
		{"verifier gpt model on gemini", "verifier", ProviderGemini, "gpt-4o", true},
		{"verifier empty model skips", "verifier", ProviderGemini, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validProfile()
			switch tt.role {
			case "planner":
				p.PlannerProvider = tt.provider
				p.PlannerModel = tt.model
			case "worker":
				p.WorkerProvider = tt.provider
				p.WorkerModel = tt.model
			case "verifier":
				p.VerifierProvider = tt.provider
				p.VerifierModel = tt.model
			}
			err := ValidateLoopProfile(p)
			if (err != nil) != tt.wantErr {
				t.Errorf("got err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLoopProfile_HardBudgetCapNonNegative(t *testing.T) {
	p := validProfile()
	p.HardBudgetCapUSD = -1.0
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for negative hard_budget_cap_usd")
	}

	// Zero (disabled) is fine.
	p.HardBudgetCapUSD = 0
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for zero hard_budget_cap_usd, got: %v", err)
	}

	// Positive is fine.
	p.HardBudgetCapUSD = 95.0
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for positive hard_budget_cap_usd, got: %v", err)
	}
}

func TestValidateLoopProfile_NoopPlateauLimitNonNegative(t *testing.T) {
	p := validProfile()
	p.NoopPlateauLimit = -1
	if err := ValidateLoopProfile(p); err == nil {
		t.Error("expected error for negative noop_plateau_limit")
	}

	// Zero (disabled) is fine.
	p.NoopPlateauLimit = 0
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for zero noop_plateau_limit, got: %v", err)
	}

	// Positive is fine.
	p.NoopPlateauLimit = 5
	if err := ValidateLoopProfile(p); err != nil {
		t.Errorf("expected nil error for positive noop_plateau_limit, got: %v", err)
	}
}

func TestValidateModelProviderMatch(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		provider Provider
		model    string
		wantErr  bool
	}{
		{"match", "planner", ProviderClaude, "claude-opus-4-6", false},
		{"mismatch", "worker", ProviderGemini, "claude-sonnet-4-6", true},
		{"empty provider", "verifier", "", "anything", false},
		{"empty model", "planner", ProviderClaude, "", false},
		{"both empty", "worker", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateModelProviderMatch(tt.role, tt.provider, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("got err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckModelRegistry(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		model    string
		wantWarn bool
	}{
		{"known model", ProviderGemini, "gemini-2.5-pro", false},
		{"unknown model with valid prefix", ProviderGemini, "gemini-3-pro", true},
		{"known claude model", ProviderClaude, "claude-opus-4-20250514", false},
		{"unknown claude model", ProviderClaude, "claude-unknown-99", true},
		{"prefix mismatch returns empty", ProviderGemini, "claude-sonnet-4-6", false},
		{"empty provider", "", "gemini-2.5-pro", false},
		{"empty model", ProviderGemini, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckModelRegistry(tt.provider, tt.model)
			if (got != "") != tt.wantWarn {
				t.Errorf("CheckModelRegistry(%q, %q) = %q, wantWarn=%v", tt.provider, tt.model, got, tt.wantWarn)
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
