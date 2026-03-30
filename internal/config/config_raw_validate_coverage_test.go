package config

import (
	"testing"
)

func TestKnownKeys_ReturnsList(t *testing.T) {
	keys := KnownKeys()
	if len(keys) == 0 {
		t.Fatal("KnownKeys should return non-empty list")
	}
}

func TestKnownKeys_Sorted(t *testing.T) {
	keys := KnownKeys()
	for i := 1; i < len(keys); i++ {
		if keys[i].Name < keys[i-1].Name {
			t.Errorf("KnownKeys not sorted: %q comes before %q", keys[i-1].Name, keys[i].Name)
		}
	}
}

func TestKnownKeys_ContainsExpected(t *testing.T) {
	keys := KnownKeys()
	found := make(map[string]bool)
	for _, k := range keys {
		found[k.Name] = true
	}

	expected := []string{
		"scan_paths", "default_provider", "max_workers",
		"default_budget_usd", "log_level", "auto_restart",
	}
	for _, name := range expected {
		if !found[name] {
			t.Errorf("KnownKeys missing expected key %q", name)
		}
	}
}

func TestKnownKeys_TypesPopulated(t *testing.T) {
	keys := KnownKeys()
	for _, k := range keys {
		if k.Name == "" {
			t.Error("found KeyInfo with empty Name")
		}
		if k.Type == "" {
			t.Errorf("key %q has empty Type", k.Name)
		}
	}
}

func TestValidationWarning_StringFormat(t *testing.T) {
	w := ValidationWarning{Key: "max_workers", Message: "value 99 out of range", Severity: "error"}
	got := w.String()
	if got == "" {
		t.Error("String() returned empty string")
	}
	for _, substr := range []string{"error", "max_workers", "out of range"} {
		found := false
		for i := 0; i+len(substr) <= len(got); i++ {
			if got[i:i+len(substr)] == substr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("String() = %q, expected to contain %q", got, substr)
		}
	}
}

func TestValidateRawConfig_PluginKeyAllowed(t *testing.T) {
	cfg := map[string]any{
		"plugin_myapp_token": "abc123",
	}
	warnings := ValidateRawConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("plugin_ key should not produce warnings, got: %v", warnings)
	}
}

func TestValidateRawConfig_FloatWrongType(t *testing.T) {
	cfg2 := map[string]any{"default_budget_usd": "not-a-number"}
	if w := ValidateRawConfig(cfg2); len(w) == 0 {
		t.Error("expected warning for non-float budget")
	}
}

func TestValidateRawConfig_StringWrongType(t *testing.T) {
	cfg := map[string]any{"default_provider": 42.0}
	if w := ValidateRawConfig(cfg); len(w) == 0 {
		t.Error("expected warning for non-string provider")
	}
}

func TestValidateRawConfig_IntWrongType(t *testing.T) {
	cfg := map[string]any{"max_workers": "ten"}
	w := ValidateRawConfig(cfg)
	if len(w) == 0 {
		t.Error("expected error for string where int is expected")
	}
}

func TestValidateRawConfig_FloatForInt(t *testing.T) {
	cfg := map[string]any{"max_workers": float64(2.5)}
	w := ValidateRawConfig(cfg)
	if len(w) == 0 {
		t.Error("expected error for float 2.5 where int is expected")
	}
}

func TestValidateRawConfig_DurationBoolType(t *testing.T) {
	cfg3 := map[string]any{"session_timeout": true}
	if w := ValidateRawConfig(cfg3); len(w) == 0 {
		t.Error("expected warning for bool duration")
	}
}
