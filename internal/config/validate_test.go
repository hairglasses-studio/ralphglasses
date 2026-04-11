package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// ValidationResult helpers
// ---------------------------------------------------------------------------

func TestValidationResult_OK(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
		want   bool
	}{
		{"empty", &ValidationResult{}, true},
		{"warnings only", &ValidationResult{
			Warnings: []ValidationIssue{{Field: "x", Severity: SeverityWarning, Message: "w"}},
		}, true},
		{"errors present", &ValidationResult{
			Errors: []ValidationIssue{{Field: "x", Severity: SeverityError, Message: "e"}},
		}, false},
		{"both", &ValidationResult{
			Errors:   []ValidationIssue{{Field: "x", Severity: SeverityError, Message: "e"}},
			Warnings: []ValidationIssue{{Field: "y", Severity: SeverityWarning, Message: "w"}},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.OK(); got != tt.want {
				t.Errorf("OK() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidationResult_HasWarnings(t *testing.T) {
	tests := []struct {
		name   string
		result *ValidationResult
		want   bool
	}{
		{"empty", &ValidationResult{}, false},
		{"warnings only", &ValidationResult{
			Warnings: []ValidationIssue{{Field: "x"}},
		}, true},
		{"errors only", &ValidationResult{
			Errors: []ValidationIssue{{Field: "x"}},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.HasWarnings(); got != tt.want {
				t.Errorf("HasWarnings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidationResult_AllIssues(t *testing.T) {
	r := &ValidationResult{
		Errors:   []ValidationIssue{{Field: "a"}, {Field: "b"}},
		Warnings: []ValidationIssue{{Field: "c"}},
	}
	all := r.AllIssues()
	if len(all) != 3 {
		t.Fatalf("AllIssues() len = %d, want 3", len(all))
	}
	// Errors come first.
	if all[0].Field != "a" || all[1].Field != "b" || all[2].Field != "c" {
		t.Errorf("AllIssues() order wrong: %v", all)
	}
}

func TestValidationIssue_String(t *testing.T) {
	v := ValidationIssue{Field: "budget", Severity: SeverityError, Message: "too high"}
	want := "[error] budget: too high"
	if got := v.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// ValidateStruct — nil and empty
// ---------------------------------------------------------------------------

func TestValidateStruct_NilConfig(t *testing.T) {
	r := ValidateStruct(nil)
	if !r.OK() {
		t.Errorf("nil config should produce no errors")
	}
	if r.HasWarnings() {
		t.Errorf("nil config should produce no warnings")
	}
}

func TestValidateStruct_EmptyConfig(t *testing.T) {
	r := ValidateStruct(&Config{})
	if !r.OK() {
		t.Errorf("empty config should produce no errors, got %v", r.Errors)
	}
}

// ---------------------------------------------------------------------------
// Budget range rule
// ---------------------------------------------------------------------------

func TestRule_BudgetRange(t *testing.T) {
	tests := []struct {
		name       string
		budget     float64
		wantErrors int
		wantWarns  int
	}{
		{"zero", 0, 0, 0},
		{"small valid", 0.50, 0, 0},
		{"typical", 10.0, 0, 0},
		{"max boundary", 10000, 0, 1},
		{"negative", -1, 1, 0},
		{"exceeds max", 10001, 1, 0},
		{"way over", 50000, 1, 0},
		{"high but valid warns", 5000, 0, 1},
		{"exactly 1000 no warn", 1000, 0, 0},
		{"just over 1000 warns", 1001, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DefaultBudgetUSD: tt.budget}
			r := ValidateStructWithRules(cfg, []ValidationRule{
				{Name: "budget_range", Check: ruleBudgetRange},
			})
			if len(r.Errors) != tt.wantErrors {
				t.Errorf("errors = %d, want %d: %v", len(r.Errors), tt.wantErrors, r.Errors)
			}
			if len(r.Warnings) != tt.wantWarns {
				t.Errorf("warnings = %d, want %d: %v", len(r.Warnings), tt.wantWarns, r.Warnings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Cost rate multiplier rule
// ---------------------------------------------------------------------------

func TestRule_CostRateMultiplier(t *testing.T) {
	tests := []struct {
		name       string
		rate       float64
		wantErrors int
	}{
		{"zero", 0, 0},
		{"one", 1.0, 0},
		{"half", 0.5, 0},
		{"large", 100, 0},
		{"negative", -0.1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{CostRateMultiplier: tt.rate}
			r := ValidateStructWithRules(cfg, []ValidationRule{
				{Name: "cost_rate_multiplier", Check: ruleCostRateMultiplier},
			})
			if len(r.Errors) != tt.wantErrors {
				t.Errorf("errors = %d, want %d: %v", len(r.Errors), tt.wantErrors, r.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Provider names rule
// ---------------------------------------------------------------------------

func TestRule_ProviderNames(t *testing.T) {
	tests := []struct {
		name       string
		defProv    string
		workerProv string
		wantErrors int
	}{
		{"both empty", "", "", 0},
		{"claude/gemini", "claude", "gemini", 0},
		{"openai/claude", "openai", "claude", 0},
		{"unknown default", "gpt5", "", 1},
		{"unknown worker", "", "llama9000", 1},
		{"both unknown", "foo", "bar", 2},
		{"default valid worker unknown", "claude", "invalid", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				DefaultProvider: tt.defProv,
				WorkerProvider:  tt.workerProv,
			}
			r := ValidateStructWithRules(cfg, []ValidationRule{
				{Name: "provider_names", Check: ruleProviderNames},
			})
			if len(r.Errors) != tt.wantErrors {
				t.Errorf("errors = %d, want %d: %v", len(r.Errors), tt.wantErrors, r.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Scan paths rule
// ---------------------------------------------------------------------------

func TestRule_ScanPathsExist(t *testing.T) {
	existingDir := t.TempDir()

	existingFile := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(existingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		paths      []string
		wantErrors int
		wantWarns  int
	}{
		{"no paths", nil, 0, 0},
		{"existing directory", []string{existingDir}, 0, 0},
		{"nonexistent path", []string{"/nonexistent/path/abc123"}, 1, 0},
		{"empty string path", []string{""}, 1, 0},
		{"file not dir", []string{existingFile}, 0, 1},
		{"mixed valid and invalid", []string{existingDir, "/no/such/path"}, 1, 0},
		{"multiple nonexistent", []string{"/no/a", "/no/b"}, 2, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{ScanPaths: tt.paths}
			r := ValidateStructWithRules(cfg, []ValidationRule{
				{Name: "scan_paths_exist", Check: ruleScanPathsExist},
			})
			if len(r.Errors) != tt.wantErrors {
				t.Errorf("errors = %d, want %d: %v", len(r.Errors), tt.wantErrors, r.Errors)
			}
			if len(r.Warnings) != tt.wantWarns {
				t.Errorf("warnings = %d, want %d: %v", len(r.Warnings), tt.wantWarns, r.Warnings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Duration formats rule
// ---------------------------------------------------------------------------

func TestRule_DurationFormats(t *testing.T) {
	tests := []struct {
		name          string
		sessionTO     time.Duration
		healthInterval time.Duration
		wantErrors    int
		wantWarns     int
	}{
		{"both zero", 0, 0, 0, 0},
		{"valid durations", 10 * time.Minute, 30 * time.Second, 0, 0},
		{"negative session timeout", -1 * time.Second, 0, 1, 0},
		{"negative health interval", 0, -5 * time.Second, 1, 0},
		{"both negative", -1 * time.Second, -1 * time.Second, 2, 0},
		{"session over 24h warns", 25 * time.Hour, 0, 0, 1},
		{"exactly 24h no warn", 24 * time.Hour, 0, 0, 0},
		{"health sub-second warns", 0, 500 * time.Millisecond, 0, 1},
		{"health exactly 1s ok", 0, time.Second, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				SessionTimeout:      tt.sessionTO,
				HealthCheckInterval: tt.healthInterval,
			}
			r := ValidateStructWithRules(cfg, []ValidationRule{
				{Name: "duration_formats", Check: ruleDurationFormats},
			})
			if len(r.Errors) != tt.wantErrors {
				t.Errorf("errors = %d, want %d: %v", len(r.Errors), tt.wantErrors, r.Errors)
			}
			if len(r.Warnings) != tt.wantWarns {
				t.Errorf("warnings = %d, want %d: %v", len(r.Warnings), tt.wantWarns, r.Warnings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Worker count rule
// ---------------------------------------------------------------------------

func TestRule_WorkerCount(t *testing.T) {
	tests := []struct {
		name       string
		workers    int
		wantErrors int
	}{
		{"zero", 0, 0},
		{"one", 1, 0},
		{"max boundary", 50, 0},
		{"over max", 51, 1},
		{"way over", 200, 1},
		{"negative", -1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{MaxWorkers: tt.workers}
			r := ValidateStructWithRules(cfg, []ValidationRule{
				{Name: "worker_count", Check: ruleWorkerCount},
			})
			if len(r.Errors) != tt.wantErrors {
				t.Errorf("errors = %d, want %d: %v", len(r.Errors), tt.wantErrors, r.Errors)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Conflicting flags rule
// ---------------------------------------------------------------------------

func TestRule_ConflictingFlags(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantWarns int
	}{
		{"no conflict — both empty", Config{}, 0},
		{"workers with default provider", Config{MaxWorkers: 4, DefaultProvider: "claude"}, 0},
		{"workers with worker provider", Config{MaxWorkers: 4, WorkerProvider: "gemini"}, 0},
		{"workers no provider", Config{MaxWorkers: 4}, 1},
		{"same provider both roles", Config{DefaultProvider: "claude", WorkerProvider: "claude"}, 1},
		{"different providers", Config{DefaultProvider: "claude", WorkerProvider: "gemini"}, 0},
		{"zero workers no provider ok", Config{MaxWorkers: 0}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := tt.cfg
			r := ValidateStructWithRules(&cfg, []ValidationRule{
				{Name: "conflicting_flags", Check: ruleConflictingFlags},
			})
			if len(r.Warnings) != tt.wantWarns {
				t.Errorf("warnings = %d, want %d: %v", len(r.Warnings), tt.wantWarns, r.Warnings)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Full ValidateStruct integration
// ---------------------------------------------------------------------------

func TestValidateStruct_FullyValid(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ScanPaths:           []string{dir},
		DefaultProvider:     "claude",
		WorkerProvider:      "gemini",
		MaxWorkers:          4,
		DefaultBudgetUSD:    10.0,
		CostRateMultiplier:  1.0,
		SessionTimeout:      10 * time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
	r := ValidateStruct(cfg)
	if !r.OK() {
		t.Errorf("fully valid config should have no errors: %v", r.Errors)
	}
	// Warnings are acceptable but not errors.
}

func TestValidateStruct_MultipleErrors(t *testing.T) {
	cfg := &Config{
		ScanPaths:       []string{"/nonexistent/alpha"},
		DefaultProvider: "nonexistent_ai",
		MaxWorkers:      -5,
		DefaultBudgetUSD: -100,
		SessionTimeout:  -1 * time.Second,
	}
	r := ValidateStruct(cfg)
	if r.OK() {
		t.Fatal("config with multiple errors should not be OK")
	}
	if len(r.Errors) < 4 {
		t.Errorf("expected at least 4 errors, got %d: %v", len(r.Errors), r.Errors)
	}
}

// ---------------------------------------------------------------------------
// ValidateProviderName helper
// ---------------------------------------------------------------------------

func TestValidateProviderName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"", false},
		{"claude", false},
		{"gemini", false},
		{"codex", false},
		{"openai", false},
		{"gpt5", true},
		{"Claude", true}, // case-sensitive
		{"GEMINI", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateProviderName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProviderName(%q) err = %v, wantErr = %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateModelForProvider helper
// ---------------------------------------------------------------------------

func TestValidateModelForProvider(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		wantErr  bool
	}{
		// Empty skips.
		{"", "claude-sonnet-4-6", false},
		{"claude", "", false},
		{"", "", false},

		// Claude models.
		{"claude", "claude-sonnet-4-6", false},
		{"claude", "claude-opus-4-20250514", false},
		{"claude", "claude-haiku-3-5-20241022", false},
		{"claude", "sonnet-4", false},
		{"claude", "opus-4", false},
		{"claude", "haiku-3.5", false},
		{"claude", "gpt-4", true}, // wrong provider

		// Gemini models.
		{"gemini", "gemini-3.1-pro", false},
		{"gemini", "gemini-3.1-flash", false},
		{"gemini", "claude-sonnet-4-6", true}, // wrong provider

		// Codex/OpenAI models.
		{"codex", "gpt-4-turbo", false},
		{"codex", "o1-preview", false},
		{"codex", "o3-mini", false},
		{"codex", "o4-mini", false},
		{"codex", "codex-mini", false},
		{"codex", "gemini-flash", true},

		{"openai", "gpt-4o", false},
		{"openai", "o4-mini", false},
		{"openai", "claude-sonnet-4-6", true},

		// Ollama models.

		// Unknown provider — skip validation.
		{"future_ai", "anything-goes", false},
	}
	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			err := ValidateModelForProvider(tt.provider, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateModelForProvider(%q, %q) err = %v, wantErr = %v",
					tt.provider, tt.model, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateBudget helper
// ---------------------------------------------------------------------------

func TestValidateBudget(t *testing.T) {
	tests := []struct {
		name    string
		budget  float64
		wantErr bool
	}{
		{"zero", 0, false},
		{"small", 0.01, false},
		{"typical", 10, false},
		{"large valid", 10000, false},
		{"negative", -1, true},
		{"over max", 10001, true},
		{"way over", 1000000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBudget(tt.budget)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBudget(%g) err = %v, wantErr = %v", tt.budget, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidateDurationString helper
// ---------------------------------------------------------------------------

func TestValidateDurationString(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", false},
		{"10s", false},
		{"5m", false},
		{"1h30m", false},
		{"500ms", false},
		{"2h45m30s", false},
		{"0s", false},
		{"not-a-duration", true},
		{"10", true},
		{"abc", true},
		{"-5s", false}, // Go parses negative durations
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := ValidateDurationString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateDurationString(%q) err = %v, wantErr = %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ValidatePort helper
// ---------------------------------------------------------------------------

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"min valid", 1, false},
		{"common http", 80, false},
		{"common https", 443, false},
		{"high port", 8080, false},
		{"max valid", 65535, false},
		{"zero", 0, true},
		{"negative", -1, true},
		{"over max", 65536, true},
		{"way over", 100000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%d) err = %v, wantErr = %v", tt.port, err, tt.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Custom rules via ValidateStructWithRules
// ---------------------------------------------------------------------------

func TestValidateStructWithRules_CustomRule(t *testing.T) {
	customRule := ValidationRule{
		Name: "custom_budget_floor",
		Check: func(cfg *Config, r *ValidationResult) {
			if cfg.DefaultBudgetUSD > 0 && cfg.DefaultBudgetUSD < 1.0 {
				r.addError("default_budget_usd", "minimum budget is $1.00 in this org")
			}
		},
	}

	cfg := &Config{DefaultBudgetUSD: 0.50}
	r := ValidateStructWithRules(cfg, []ValidationRule{customRule})
	if r.OK() {
		t.Error("custom rule should have triggered an error")
	}
	if len(r.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(r.Errors))
	}
}

func TestValidateStructWithRules_NoRules(t *testing.T) {
	cfg := &Config{DefaultBudgetUSD: -999}
	r := ValidateStructWithRules(cfg, nil)
	if !r.OK() {
		t.Error("no rules should produce no errors")
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestValidateStruct_ScanPathIsFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "regular.txt")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{ScanPaths: []string{file}}
	r := ValidateStruct(cfg)
	// Should warn, not error.
	if !r.OK() {
		t.Errorf("file scan path should be a warning, not error: %v", r.Errors)
	}
	if !r.HasWarnings() {
		t.Error("expected warning for file-as-scan-path")
	}
}

func TestValidateStruct_EmptyScanPath(t *testing.T) {
	cfg := &Config{ScanPaths: []string{""}}
	r := ValidateStruct(cfg)
	if r.OK() {
		t.Error("empty scan path should produce an error")
	}
}

func TestValidateStruct_HighBudgetWarnsNotErrors(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		ScanPaths:       []string{dir},
		DefaultProvider: "claude",
		DefaultBudgetUSD: 5000,
	}
	r := ValidateStruct(cfg)
	if !r.OK() {
		t.Errorf("high budget should only warn, not error: %v", r.Errors)
	}
	if !r.HasWarnings() {
		t.Error("expected warning for high budget")
	}
}
