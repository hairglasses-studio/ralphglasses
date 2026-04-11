package session

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// LoopConfig captures the user-facing configuration for a loop run,
// consolidating fields from LoopProfile and LaunchOptions that benefit
// from pre-flight validation.
type LoopConfig struct {
	Provider                Provider      `json:"provider"`
	Model                   string        `json:"model"`
	BudgetUSD               float64       `json:"budget_usd,omitempty"`
	EnhancePrompt           bool          `json:"enhance_prompt,omitempty"`
	EnhancerProvider        string        `json:"enhancer_provider,omitempty"`         // provider configured for enhancement
	WorkerProvider          Provider      `json:"worker_provider,omitempty"`           // worker session provider
	EnableWorkerEnhancement bool          `json:"enable_worker_enhancement,omitempty"` // prompt enhancement before worker calls
	Timeout                 time.Duration `json:"timeout,omitempty"`
	ReviewPatience          int           `json:"review_patience,omitempty"`
}

// LoopValidationWarning describes a single validation issue with a loop config.
type LoopValidationWarning struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// String returns a human-readable representation of the warning.
func (w LoopValidationWarning) String() string {
	return fmt.Sprintf("%s: %s", w.Field, w.Message)
}

// knownModelPrefixes maps each provider to valid model name prefixes.
// Includes shorthand names (sonnet, opus, haiku) used by Claude and
// all OpenAI model families (o4- for newer reasoning models).
var knownModelPrefixes = map[Provider][]string{
	ProviderClaude: {"claude-", "sonnet", "opus", "haiku"},
	ProviderGemini: {"gemini-"},
	ProviderCodex:  {"gpt-", "o1-", "o3-", "o4-", "codex-"},
}

var codexSupportedModelPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^gpt-5\.4(-mini|-pro)?$`),
	regexp.MustCompile(`^gpt-5\.3-codex(-spark)?$`),
	regexp.MustCompile(`^gpt-5\.2(-codex)?$`),
	regexp.MustCompile(`^gpt-5\.1-codex$`),
	regexp.MustCompile(`^codex-mini-latest$`),
	regexp.MustCompile(`^o3(-mini)?$`),
	regexp.MustCompile(`^o4-mini$`),
	regexp.MustCompile(`^gpt-4o$`),
	regexp.MustCompile(`^o1-preview$`),
}

var codexSupportedModelExamples = []string{
	"gpt-5.4",
	"gpt-5.4-mini",
	"gpt-5.3-codex",
	"gpt-5.2",
	"codex-mini-latest",
	"o3",
	"o4-mini",
}

var codexUnsupportedModels = map[string]string{
	"gpt-5.4-xhigh": `reasoning effort belongs in the effort setting; use "gpt-5.4" with effort "max" instead`,
	"o1-pro":        `the installed Codex runtime does not support "o1-pro"; use a supported Codex model such as "gpt-5.4", "gpt-5.4-mini", "o3", or "o4-mini"`,
}

var codexReasoningEffortSuffixPattern = regexp.MustCompile(`^(gpt-[a-z0-9.-]+|o[0-9][a-z0-9.-]*|codex-[a-z0-9.-]+)-(low|medium|high|xhigh)$`)

// ValidateLoopConfig checks a LoopConfig for common misconfigurations and
// returns warnings. An empty slice means the config looks valid.
func ValidateLoopConfig(cfg LoopConfig) []LoopValidationWarning {
	var warnings []LoopValidationWarning

	// 1. Model name validation: check model matches known patterns for provider.
	if cfg.Provider != "" && cfg.Model != "" {
		prefixes, known := knownModelPrefixes[cfg.Provider]
		if known {
			matched := false
			for _, p := range prefixes {
				if strings.HasPrefix(cfg.Model, p) {
					matched = true
					break
				}
			}
			if !matched {
				warnings = append(warnings, LoopValidationWarning{
					Field:   "model",
					Message: fmt.Sprintf("model %q does not match expected prefixes for provider %q (%s)", cfg.Model, cfg.Provider, strings.Join(prefixes, ", ")),
				})
			}
		}
	}

	// 2. Budget range validation.
	if cfg.BudgetUSD != 0 {
		if cfg.BudgetUSD < 0.01 {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "budget_usd",
				Message: fmt.Sprintf("budget $%.4f is below minimum recommended $0.01", cfg.BudgetUSD),
			})
		}
		if cfg.BudgetUSD > 100 {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "budget_usd",
				Message: fmt.Sprintf("budget $%.2f exceeds maximum recommended $100.00", cfg.BudgetUSD),
			})
		}
	}

	// 3. Enhancement flag checks.
	if cfg.EnhancePrompt && cfg.EnhancerProvider == "" {
		warnings = append(warnings, LoopValidationWarning{
			Field:   "enhance_prompt",
			Message: "prompt enhancement enabled but no enhancer provider is configured",
		})
	}

	// 4. Timeout validation.
	if cfg.Timeout != 0 {
		if cfg.Timeout < 10*time.Second {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "timeout",
				Message: fmt.Sprintf("timeout %s is below minimum recommended 10s", cfg.Timeout),
			})
		}
		if cfg.Timeout > 3600*time.Second {
			warnings = append(warnings, LoopValidationWarning{
				Field:   "timeout",
				Message: fmt.Sprintf("timeout %s exceeds maximum recommended 1h", cfg.Timeout),
			})
		}
	}

	return warnings
}

// validLoopProfileProviders is the set of accepted provider values for loop profiles.
var validLoopProfileProviders = map[Provider]bool{
	ProviderClaude: true,
	ProviderGemini: true,
	ProviderCodex:  true,
	"":             true, // empty = use default
}

// validateModelProviderMatch checks that a model name matches the expected prefixes
// for the given provider. Returns an error if the model doesn't match, or nil if valid.
// Skips validation when either provider or model is empty.
func validateModelProviderMatch(role string, provider Provider, model string) error {
	provider = normalizeSessionProvider(provider)
	if provider == "" || model == "" {
		return nil
	}
	// Allow well-known test/mock model names used in E2E harness.
	if model == "mock" || model == "test" {
		return nil
	}
	prefixes, known := knownModelPrefixes[provider]
	if !known {
		return nil
	}
	for _, p := range prefixes {
		if strings.HasPrefix(model, p) {
			if err := ValidateModelName(provider, model); err != nil {
				return fmt.Errorf("%s_model %q is invalid for %s_provider %q: %w", role, model, role, provider, err)
			}
			return nil
		}
	}
	return fmt.Errorf("%s_model %q does not match expected prefixes for %s_provider %q (%s)",
		role, model, role, provider, strings.Join(prefixes, ", "))
}

// ValidateModelName enforces provider-specific hard validation for explicit
// model overrides. It is intentionally stricter for Codex, where invalid
// model IDs fail immediately at launch time.
func ValidateModelName(provider Provider, model string) error {
	provider = normalizeSessionProvider(provider)
	model = strings.TrimSpace(model)
	if provider == "" || model == "" {
		return nil
	}
	if model == "mock" || model == "test" {
		return nil
	}

	switch provider {
	case ProviderCodex:
		if reason, ok := codexUnsupportedModels[model]; ok {
			return fmt.Errorf("%s", reason)
		}
		if codexReasoningEffortSuffixPattern.MatchString(model) {
			return fmt.Errorf("reasoning effort belongs in the effort setting, not the model name")
		}
		for _, pattern := range codexSupportedModelPatterns {
			if pattern.MatchString(model) {
				return nil
			}
		}
		return fmt.Errorf("not in the supported Codex model allowlist (%s)", strings.Join(codexSupportedModelExamples, ", "))
	default:
		return nil
	}
}

// CheckModelRegistry returns a warning message if the model passes prefix
// validation but is not found in the known model registry. Returns "" if
// the model is in the registry, has no matching prefix, or either argument
// is empty. This is a soft check — the registry is not exhaustive.
func CheckModelRegistry(provider Provider, model string) string {
	provider = normalizeSessionProvider(provider)
	if provider == "" || model == "" {
		return ""
	}
	prefixes, known := knownModelPrefixes[provider]
	if !known {
		return ""
	}
	matched := false
	for _, p := range prefixes {
		if strings.HasPrefix(model, p) {
			matched = true
			break
		}
	}
	if !matched {
		return "" // prefix mismatch is handled by validateModelProviderMatch
	}
	if LookupModel(model) == nil {
		return fmt.Sprintf("model %q is not in the known registry for %s — verify it exists with the provider", model, provider)
	}
	return ""
}

// InferProviderFromModel returns the most likely provider for an explicit model
// identifier using the built-in registry first and provider prefix heuristics as
// a fallback. It returns false when the model does not imply a known provider.
func InferProviderFromModel(model string) (Provider, bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", false
	}
	if info := LookupModel(model); info != nil {
		return normalizeSessionProvider(info.Provider), true
	}
	for provider, prefixes := range knownModelPrefixes {
		for _, prefix := range prefixes {
			if strings.HasPrefix(model, prefix) {
				return normalizeSessionProvider(provider), true
			}
		}
	}
	return "", false
}

// ValidateLoopProfile checks a LoopProfile for invalid settings before loop execution.
// Returns an error describing the first invalid field found, or nil if valid.
func ValidateLoopProfile(p LoopProfile) error {
	if !validLoopProfileProviders[p.PlannerProvider] {
		return fmt.Errorf("invalid planner_provider %q: must be one of claude, gemini, codex, or empty", p.PlannerProvider)
	}
	if !validLoopProfileProviders[p.WorkerProvider] {
		return fmt.Errorf("invalid worker_provider %q: must be one of claude, gemini, codex, or empty", p.WorkerProvider)
	}
	if !validLoopProfileProviders[p.VerifierProvider] {
		return fmt.Errorf("invalid verifier_provider %q: must be one of claude, gemini, codex, or empty", p.VerifierProvider)
	}

	defaults := DefaultLoopProfile()
	plannerProvider := p.PlannerProvider
	if plannerProvider == "" && p.PlannerModel != "" {
		plannerProvider = defaults.PlannerProvider
	}
	workerProvider := p.WorkerProvider
	if workerProvider == "" && p.WorkerModel != "" {
		workerProvider = defaults.WorkerProvider
	}
	verifierProvider := p.VerifierProvider
	if verifierProvider == "" && p.VerifierModel != "" {
		verifierProvider = defaults.VerifierProvider
	}

	// Model-provider prefix mismatch checks.
	if err := validateModelProviderMatch("planner", plannerProvider, p.PlannerModel); err != nil {
		return err
	}
	if err := validateModelProviderMatch("worker", workerProvider, p.WorkerModel); err != nil {
		return err
	}
	if err := validateModelProviderMatch("verifier", verifierProvider, p.VerifierModel); err != nil {
		return err
	}

	if p.MaxIterations < 0 {
		return fmt.Errorf("max_iterations must be non-negative, got %d", p.MaxIterations)
	}
	if p.MaxDurationSecs < 0 {
		return fmt.Errorf("max_duration_secs must be non-negative, got %d", p.MaxDurationSecs)
	}

	if p.MaxConcurrentWorkers < 0 || p.MaxConcurrentWorkers > 10 {
		return fmt.Errorf("max_concurrent_workers must be 0-10, got %d", p.MaxConcurrentWorkers)
	}

	// Budget: either both set (> 0) or both zero (no budget).
	if (p.PlannerBudgetUSD > 0) != (p.WorkerBudgetUSD > 0) {
		return fmt.Errorf("planner_budget_usd and worker_budget_usd must both be set or both be zero")
	}
	if p.PlannerBudgetUSD < 0 {
		return fmt.Errorf("planner_budget_usd must be non-negative, got %f", p.PlannerBudgetUSD)
	}
	if p.WorkerBudgetUSD < 0 {
		return fmt.Errorf("worker_budget_usd must be non-negative, got %f", p.WorkerBudgetUSD)
	}

	if p.RetryLimit < 0 || p.RetryLimit > 10 {
		return fmt.Errorf("retry_limit must be 0-10, got %d", p.RetryLimit)
	}

	if p.StallTimeout < 0 {
		return fmt.Errorf("stall_timeout must be non-negative, got %s", p.StallTimeout)
	}

	if p.HardBudgetCapUSD < 0 {
		return fmt.Errorf("hard_budget_cap_usd must be non-negative, got %f", p.HardBudgetCapUSD)
	}
	if p.NoopPlateauLimit < 0 {
		return fmt.Errorf("noop_plateau_limit must be non-negative, got %d", p.NoopPlateauLimit)
	}

	return nil
}
