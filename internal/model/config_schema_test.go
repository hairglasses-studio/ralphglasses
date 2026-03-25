package model

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSchema_ValidConfigPasses verifies that a fully valid config produces no
// warnings or errors from the schema validator.
func TestSchema_ValidConfigPasses(t *testing.T) {
	cfg := &RalphConfig{
		Values: map[string]string{
			"MODEL":                   "sonnet",
			"MAX_CALLS_PER_HOUR":      "60",
			"CLAUDE_TIMEOUT_MINUTES":  "30",
			"BUDGET":                  "5.00",
			"RALPH_SESSION_BUDGET":    "100.00",
			"MARATHON_DURATION_HOURS": "12",
			"CHECKPOINT_HOURS":        "3",
			"CB_FAIL_THRESHOLD":       "5",
			"CB_RESET_TIMEOUT":        "300",
			"CB_HALF_OPEN_MAX":        "2",
			"AUTO_ENHANCE":            "false",
			"AUTONOMY_LEVEL":          "0",
			"PROVIDER":                "claude",
		},
	}
	warnings, errs := ValidateConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got: %v", warnings)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

// TestSchema_OutOfRangeNumericRejected verifies that numeric fields outside
// their allowed range produce validation errors.
func TestSchema_OutOfRangeNumericRejected(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
	}{
		{"poll interval zero", "MAX_CALLS_PER_HOUR", "0"},
		{"poll interval negative", "MAX_CALLS_PER_HOUR", "-1"},
		{"poll interval over max", "MAX_CALLS_PER_HOUR", "1001"},
		{"budget zero", "BUDGET", "0.00"},
		{"budget over max", "BUDGET", "9999.99"},
		{"timeout zero", "CLAUDE_TIMEOUT_MINUTES", "0"},
		{"timeout over max", "CLAUDE_TIMEOUT_MINUTES", "999"},
		{"autonomy too high", "AUTONOMY_LEVEL", "4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &RalphConfig{Values: map[string]string{tc.key: tc.val}}
			_, errs := ValidateConfig(cfg)
			if len(errs) == 0 {
				t.Errorf("expected range error for %s=%q, got none", tc.key, tc.val)
			}
		})
	}
}

// TestSchema_UnknownKeyWarns verifies that unrecognized keys produce a warning
// but not an error, so configs remain forward-compatible.
func TestSchema_UnknownKeyWarns(t *testing.T) {
	cfg := &RalphConfig{
		Values: map[string]string{
			"MODEL":       "sonnet",  // known
			"FUTURE_FLAG": "enabled", // unknown
			"MY_CUSTOM":   "42",     // unknown
		},
	}
	warnings, errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("unknown keys must not produce errors, got: %v", errs)
	}
	if len(warnings) != 2 {
		t.Errorf("expected 2 warnings for unknown keys, got %d: %v", len(warnings), warnings)
	}
	keys := map[string]bool{}
	for _, w := range warnings {
		keys[w.Key] = true
	}
	if !keys["FUTURE_FLAG"] || !keys["MY_CUSTOM"] {
		t.Errorf("warnings should name the unknown keys, got: %v", warnings)
	}
}

// TestSchema_MissingOptionalFieldsOK verifies that a minimal config (omitting
// all optional fields) produces no warnings or errors.
func TestSchema_MissingOptionalFieldsOK(t *testing.T) {
	cfg := &RalphConfig{
		Values: map[string]string{},
	}
	warnings, errs := ValidateConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("empty config should produce no warnings, got: %v", warnings)
	}
	if len(errs) != 0 {
		t.Errorf("empty config should produce no errors, got: %v", errs)
	}
}

// TestSchema_LoadConfigEmitsWarnings verifies that LoadConfig succeeds even
// when the .ralphrc contains unknown or invalid entries — warnings are logged
// but do not cause a load failure.
func TestSchema_LoadConfigEmitsWarnings(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	content := `MODEL=sonnet
UNKNOWN_FUTURE_KEY=some_value
MAX_CALLS_PER_HOUR=99999
`
	if err := os.WriteFile(rc, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// LoadConfig must succeed (not return error) even with unknown/invalid values.
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig should not fail on bad values, got: %v", err)
	}
	if cfg.Values["MODEL"] != "sonnet" {
		t.Errorf("MODEL = %q, want %q", cfg.Values["MODEL"], "sonnet")
	}
}

// TestSchema_KnownKeysRegistryComplete verifies all KnownKeys entries have a
// non-empty description, catching accidental empty registrations.
func TestSchema_KnownKeysRegistryComplete(t *testing.T) {
	for key, spec := range KnownKeys {
		if spec.Description == "" {
			t.Errorf("KnownKeys[%q] has empty description", key)
		}
	}
}
