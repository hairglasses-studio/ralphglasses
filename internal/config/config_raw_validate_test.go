package config

import (
	"testing"
)

func TestValidateRawConfig_ValidConfig(t *testing.T) {
	cfg := map[string]any{
		"default_provider":      "claude",
		"worker_provider":       "gemini",
		"max_workers":           float64(4),
		"default_budget_usd":    float64(5.0),
		"cost_rate_multiplier":  float64(1.0),
		"session_timeout":       "10m",
		"health_check_interval": float64(30),
		"kill_timeout":          float64(10),
		"max_restarts":          float64(3),
		"scan_interval":         float64(60),
		"log_level":             "info",
		"auto_restart":          true,
		"provider":              "codex",
		"scan_paths":            []any{"/tmp", "/home"},
	}

	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid config, got %v", warnings)
	}
}

func TestValidateRawConfig_Nil(t *testing.T) {
	warnings := ValidateRawConfig(nil)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for nil, got %v", warnings)
	}
}

func TestValidateRawConfig_Empty(t *testing.T) {
	warnings := ValidateRawConfig(map[string]any{})
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for empty config, got %v", warnings)
	}
}

func TestValidateRawConfig_UnknownKey(t *testing.T) {
	cfg := map[string]any{
		"unknown_field": "surprise",
	}

	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Severity != "warning" {
		t.Errorf("expected severity 'warning', got %q", warnings[0].Severity)
	}
	if warnings[0].Key != "unknown_field" {
		t.Errorf("expected key 'unknown_field', got %q", warnings[0].Key)
	}
}

func TestValidateRawConfig_OutOfRangeNumeric(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  float64
	}{
		{"kill_timeout too low", "kill_timeout", float64(0)},
		{"kill_timeout too high", "kill_timeout", float64(61)},
		{"max_restarts too high", "max_restarts", float64(101)},
		{"scan_interval too low", "scan_interval", float64(0)},
		{"scan_interval too high", "scan_interval", float64(3601)},
		{"max_workers too high", "max_workers", float64(51)},
		{"max_workers negative", "max_workers", float64(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]any{tt.key: tt.val}
			warnings := ValidateRawConfig(cfg)
			if len(warnings) != 1 {
				t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
			}
			if warnings[0].Severity != "error" {
				t.Errorf("expected severity 'error', got %q", warnings[0].Severity)
			}
		})
	}
}

func TestValidateRawConfig_TypeMismatch(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  any
	}{
		{"int gets string", "kill_timeout", "ten"},
		{"string gets int", "log_level", float64(42)},
		{"bool gets string", "auto_restart", "yes"},
		{"float gets string", "default_budget_usd", "five"},
		{"[]string gets string", "scan_paths", "not-an-array"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]any{tt.key: tt.val}
			warnings := ValidateRawConfig(cfg)
			if len(warnings) != 1 {
				t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
			}
			if warnings[0].Severity != "error" {
				t.Errorf("expected severity 'error', got %q", warnings[0].Severity)
			}
		})
	}
}

func TestValidateRawConfig_InvalidEnumValue(t *testing.T) {
	cfg := map[string]any{
		"log_level": "trace",
	}
	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Severity != "error" {
		t.Errorf("expected severity 'error', got %q", warnings[0].Severity)
	}
}

func TestValidateRawConfig_BoundaryValues(t *testing.T) {
	// All boundary values that should pass.
	tests := []struct {
		name string
		key  string
		val  float64
	}{
		{"kill_timeout min", "kill_timeout", float64(1)},
		{"kill_timeout max", "kill_timeout", float64(60)},
		{"max_restarts min", "max_restarts", float64(0)},
		{"max_restarts max", "max_restarts", float64(100)},
		{"scan_interval min", "scan_interval", float64(1)},
		{"scan_interval max", "scan_interval", float64(3600)},
		{"max_workers min", "max_workers", float64(0)},
		{"max_workers max", "max_workers", float64(50)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := map[string]any{tt.key: tt.val}
			warnings := ValidateRawConfig(cfg)
			if len(warnings) != 0 {
				t.Errorf("expected no warnings for boundary value, got %v", warnings)
			}
		})
	}
}

func TestValidateRawConfig_FloatAsInt(t *testing.T) {
	// A non-integer float for an int field should error.
	cfg := map[string]any{
		"kill_timeout": float64(5.5),
	}
	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if warnings[0].Severity != "error" {
		t.Errorf("expected severity 'error', got %q", warnings[0].Severity)
	}
}

func TestValidateRawConfig_MultipleIssues(t *testing.T) {
	cfg := map[string]any{
		"unknown_key":  "value",
		"kill_timeout":  float64(0),
		"log_level":     float64(42),
	}
	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 3 {
		t.Errorf("expected 3 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateRawConfig_DurationTypes(t *testing.T) {
	// String duration should pass.
	cfg := map[string]any{"session_timeout": "10m"}
	if w := ValidateRawConfig(cfg); len(w) != 0 {
		t.Errorf("expected no warnings for string duration, got %v", w)
	}

	// Numeric duration should pass.
	cfg = map[string]any{"session_timeout": float64(600)}
	if w := ValidateRawConfig(cfg); len(w) != 0 {
		t.Errorf("expected no warnings for numeric duration, got %v", w)
	}

	// Bool duration should fail.
	cfg = map[string]any{"session_timeout": true}
	w := ValidateRawConfig(cfg)
	if len(w) != 1 {
		t.Fatalf("expected 1 warning for bool duration, got %d: %v", len(w), w)
	}
}

func TestValidateRawConfig_StringSliceElements(t *testing.T) {
	// Array with non-string element should fail.
	cfg := map[string]any{
		"scan_paths": []any{"/tmp", float64(42)},
	}
	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
}

func TestValidationWarning_String(t *testing.T) {
	w := ValidationWarning{Key: "foo", Message: "bar", Severity: "warning"}
	s := w.String()
	if s != "[warning] foo: bar" {
		t.Errorf("unexpected String() output: %q", s)
	}
}

func TestValidateRawConfig_AllProviders(t *testing.T) {
	for _, p := range []string{"claude", "gemini", "codex", "openai", "ollama"} {
		cfg := map[string]any{"provider": p}
		if w := ValidateRawConfig(cfg); len(w) != 0 {
			t.Errorf("provider %q should be valid, got %v", p, w)
		}
	}
}
