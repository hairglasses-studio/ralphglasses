package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Severity represents the importance level of a validation issue.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// ValidationIssue represents a single validation finding with its field,
// severity, and human-readable message.
type ValidationIssue struct {
	Field    string   `json:"field"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
}

// String formats the issue as "[severity] field: message".
func (v ValidationIssue) String() string {
	return fmt.Sprintf("[%s] %s: %s", v.Severity, v.Field, v.Message)
}

// ValidationResult collects all issues found during validation, separated
// into errors (fatal) and warnings (informational).
type ValidationResult struct {
	Errors   []ValidationIssue `json:"errors,omitempty"`
	Warnings []ValidationIssue `json:"warnings,omitempty"`
}

// OK returns true when the result contains no errors. Warnings alone
// do not make the result invalid.
func (r *ValidationResult) OK() bool {
	return len(r.Errors) == 0
}

// HasWarnings returns true when the result contains at least one warning.
func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// AllIssues returns a combined slice of errors followed by warnings.
func (r *ValidationResult) AllIssues() []ValidationIssue {
	out := make([]ValidationIssue, 0, len(r.Errors)+len(r.Warnings))
	out = append(out, r.Errors...)
	out = append(out, r.Warnings...)
	return out
}

// addError appends an error-severity issue.
func (r *ValidationResult) addError(field, msg string) {
	r.Errors = append(r.Errors, ValidationIssue{
		Field:    field,
		Severity: SeverityError,
		Message:  msg,
	})
}

// addWarning appends a warning-severity issue.
func (r *ValidationResult) addWarning(field, msg string) {
	r.Warnings = append(r.Warnings, ValidationIssue{
		Field:    field,
		Severity: SeverityWarning,
		Message:  msg,
	})
}

// ValidationRule is a single check applied to a Config. Each rule inspects
// the config and appends any issues to the result.
type ValidationRule struct {
	Name  string
	Check func(cfg *Config, r *ValidationResult)
}

// validProviders is the authoritative set of recognized LLM provider names
// for the rule-based validator.
var validProviders = map[string]bool{
	"claude":      true,
	"gemini":      true,
	"codex":       true,
	"antigravity": true,
	"openai":      true,
	"crush":       true,
	"goose":       true,
	"amp":         true,
	"a2a":         true,
}

// validModelPrefixes maps each provider to accepted model name prefixes.
// Includes shorthand names (sonnet, opus, haiku) used by Claude and all
// OpenAI model families.
var validModelPrefixes = map[string][]string{
	"claude": {"claude-", "sonnet", "opus", "haiku"},
	"gemini": {"gemini-"},
	"codex":  {"gpt-", "o1-", "o3-", "o4-", "codex-"},
	"openai": {"gpt-", "o1-", "o3-", "o4-", "codex-"},
	"crush":  {"sonnet", "claude-", "gpt-", "gemini-"},
	"goose":  {"claude-", "sonnet", "gpt-", "gemini-"},
	"amp":    {"amp-", "claude-", "gpt-", "gemini-"},
	"a2a":    {"a2a-"},
}

// durationRe matches Go-style duration strings like "10s", "5m30s", "1h".
var durationRe = regexp.MustCompile(`^(\d+h)?(\d+m)?(\d+s)?(\d+ms)?$`)

// Budget limits.
const (
	minBudgetUSD = 0
	maxBudgetUSD = 10000
)

// Port range.
const (
	minPort = 1
	maxPort = 65535
)

// defaultRules is the ordered list of validation rules applied by Validate.
var defaultRules = []ValidationRule{
	{Name: "budget_range", Check: ruleBudgetRange},
	{Name: "cost_rate_multiplier", Check: ruleCostRateMultiplier},
	{Name: "provider_names", Check: ruleProviderNames},
	{Name: "scan_paths_exist", Check: ruleScanPathsExist},
	{Name: "duration_formats", Check: ruleDurationFormats},
	{Name: "worker_count", Check: ruleWorkerCount},
	{Name: "conflicting_flags", Check: ruleConflictingFlags},
}

// ValidateStruct runs all default validation rules against the given Config
// and returns a structured ValidationResult.
func ValidateStruct(cfg *Config) *ValidationResult {
	return ValidateStructWithRules(cfg, defaultRules)
}

// ValidateStructWithRules runs a custom set of validation rules against cfg.
func ValidateStructWithRules(cfg *Config, rules []ValidationRule) *ValidationResult {
	r := &ValidationResult{}
	if cfg == nil {
		return r
	}
	for _, rule := range rules {
		rule.Check(cfg, r)
	}
	return r
}

// --- Individual validation rules ---

// ruleBudgetRange checks that DefaultBudgetUSD is within [0, 10000].
func ruleBudgetRange(cfg *Config, r *ValidationResult) {
	if cfg.DefaultBudgetUSD < minBudgetUSD {
		r.addError("default_budget_usd",
			fmt.Sprintf("must be non-negative, got %g", cfg.DefaultBudgetUSD))
	} else if cfg.DefaultBudgetUSD > maxBudgetUSD {
		r.addError("default_budget_usd",
			fmt.Sprintf("exceeds maximum of $%d, got $%g", maxBudgetUSD, cfg.DefaultBudgetUSD))
	} else if cfg.DefaultBudgetUSD > 1000 {
		r.addWarning("default_budget_usd",
			fmt.Sprintf("$%g is unusually high — verify this is intentional", cfg.DefaultBudgetUSD))
	}
}

// ruleCostRateMultiplier checks that CostRateMultiplier is non-negative.
func ruleCostRateMultiplier(cfg *Config, r *ValidationResult) {
	if cfg.CostRateMultiplier < 0 {
		r.addError("cost_rate_multiplier",
			fmt.Sprintf("must be non-negative, got %g", cfg.CostRateMultiplier))
	}
}

// ruleProviderNames checks that DefaultProvider and WorkerProvider are recognized.
func ruleProviderNames(cfg *Config, r *ValidationResult) {
	if cfg.DefaultProvider != "" && !validProviders[cfg.DefaultProvider] {
		r.addError("default_provider",
			fmt.Sprintf("unknown provider %q (valid: claude, gemini, codex, antigravity, openai, crush, goose, amp, a2a)", cfg.DefaultProvider))
	}
	if cfg.WorkerProvider != "" && !validProviders[cfg.WorkerProvider] {
		r.addError("worker_provider",
			fmt.Sprintf("unknown provider %q (valid: claude, gemini, codex, antigravity, openai, crush, goose, amp, a2a)", cfg.WorkerProvider))
	}
}

// ruleScanPathsExist checks that each scan path exists on disk.
func ruleScanPathsExist(cfg *Config, r *ValidationResult) {
	for _, p := range cfg.ScanPaths {
		if p == "" {
			r.addError("scan_paths", "empty path in scan_paths")
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			r.addError("scan_paths",
				fmt.Sprintf("path %q does not exist: %v", p, err))
			continue
		}
		if !info.IsDir() {
			r.addWarning("scan_paths",
				fmt.Sprintf("path %q is a file, not a directory", p))
		}
	}
}

// ruleDurationFormats checks that SessionTimeout and HealthCheckInterval
// are non-negative and within reasonable bounds.
func ruleDurationFormats(cfg *Config, r *ValidationResult) {
	if cfg.SessionTimeout < 0 {
		r.addError("session_timeout",
			fmt.Sprintf("must be non-negative, got %s", cfg.SessionTimeout))
	} else if cfg.SessionTimeout > 24*time.Hour {
		r.addWarning("session_timeout",
			fmt.Sprintf("%s exceeds 24h — verify this is intentional", cfg.SessionTimeout))
	}

	if cfg.HealthCheckInterval < 0 {
		r.addError("health_check_interval",
			fmt.Sprintf("must be non-negative, got %s", cfg.HealthCheckInterval))
	} else if cfg.HealthCheckInterval > 0 && cfg.HealthCheckInterval < time.Second {
		r.addWarning("health_check_interval",
			fmt.Sprintf("%s is below 1s — this may cause excessive polling", cfg.HealthCheckInterval))
	}
}

// ruleWorkerCount checks that MaxWorkers is in [0, 50].
func ruleWorkerCount(cfg *Config, r *ValidationResult) {
	if cfg.MaxWorkers < 0 {
		r.addError("max_workers",
			fmt.Sprintf("must be non-negative, got %d", cfg.MaxWorkers))
	} else if cfg.MaxWorkers > 50 {
		r.addError("max_workers",
			fmt.Sprintf("exceeds maximum of 50, got %d", cfg.MaxWorkers))
	}
}

// ruleConflictingFlags detects contradictory configuration combinations.
func ruleConflictingFlags(cfg *Config, r *ValidationResult) {
	// Workers configured but no worker provider specified.
	if cfg.MaxWorkers > 0 && cfg.WorkerProvider == "" && cfg.DefaultProvider == "" {
		r.addWarning("max_workers",
			"max_workers > 0 but no worker_provider or default_provider set")
	}

	// Same provider for orchestrator and worker is valid but worth noting
	// if both are explicitly set (not relying on defaults).
	if cfg.DefaultProvider != "" && cfg.WorkerProvider != "" &&
		cfg.DefaultProvider == cfg.WorkerProvider {
		r.addWarning("worker_provider",
			fmt.Sprintf("worker_provider %q is the same as default_provider — consider using a cheaper provider for workers", cfg.WorkerProvider))
	}
}

// --- Helpers for external validation ---

// ValidateProviderName returns an error if name is not a recognized provider.
func ValidateProviderName(name string) error {
	if name == "" {
		return nil
	}
	if !validProviders[name] {
		return fmt.Errorf("unknown provider %q (valid: claude, gemini, codex, antigravity, openai, crush, goose, amp, a2a)", name)
	}
	return nil
}

// ValidateModelForProvider returns an error if model does not match the
// known prefixes for provider. Returns nil when either argument is empty.
func ValidateModelForProvider(provider, model string) error {
	if provider == "" || model == "" {
		return nil
	}
	prefixes, ok := validModelPrefixes[provider]
	if !ok {
		return nil // unknown provider, skip prefix check
	}
	for _, p := range prefixes {
		if strings.HasPrefix(model, p) {
			return nil
		}
	}
	return fmt.Errorf("model %q does not match expected prefixes for provider %q (%s)",
		model, provider, strings.Join(prefixes, ", "))
}

// ValidateBudget returns an error if budget is outside [0, 10000].
func ValidateBudget(budget float64) error {
	if budget < minBudgetUSD {
		return fmt.Errorf("budget must be non-negative, got %g", budget)
	}
	if budget > maxBudgetUSD {
		return fmt.Errorf("budget exceeds maximum of $%d, got $%g", maxBudgetUSD, budget)
	}
	return nil
}

// ValidateDurationString checks whether s is a valid Go duration literal.
// Returns an error if it cannot be parsed.
func ValidateDurationString(s string) error {
	if s == "" {
		return nil
	}
	_, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return nil
}

// ValidatePort returns an error if port is outside [1, 65535].
func ValidatePort(port int) error {
	if port < minPort || port > maxPort {
		return fmt.Errorf("port %d out of range [%d, %d]", port, minPort, maxPort)
	}
	return nil
}
