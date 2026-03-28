package model

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_BasicKeyValue(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	content := `MODEL=claude-sonnet-4-20250514
MAX_CALLS=100
BUDGET=5.00
`
	if err := os.WriteFile(rc, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	tests := []struct {
		key  string
		want string
	}{
		{"MODEL", "claude-sonnet-4-20250514"},
		{"MAX_CALLS", "100"},
		{"BUDGET", "5.00"},
	}
	for _, tt := range tests {
		if got := cfg.Values[tt.key]; got != tt.want {
			t.Errorf("Values[%q] = %q, want %q", tt.key, got, tt.want)
		}
	}
}

func TestLoadConfig_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	content := `MODEL="claude-sonnet-4-20250514"
SPEC_FILE="spec.md"
`
	if err := os.WriteFile(rc, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if got := cfg.Values["MODEL"]; got != "claude-sonnet-4-20250514" {
		t.Errorf("quoted value: got %q, want %q", got, "claude-sonnet-4-20250514")
	}
	if got := cfg.Values["SPEC_FILE"]; got != "spec.md" {
		t.Errorf("quoted value: got %q, want %q", got, "spec.md")
	}
}

func TestLoadConfig_CommentsAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	content := `# This is a comment
MODEL=sonnet

# Another comment

BUDGET=10
`
	if err := os.WriteFile(rc, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if len(cfg.Values) != 2 {
		t.Errorf("expected 2 values, got %d: %v", len(cfg.Values), cfg.Values)
	}
	if got := cfg.Values["MODEL"]; got != "sonnet" {
		t.Errorf("MODEL = %q, want %q", got, "sonnet")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for missing .ralphrc, got nil")
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	if err := os.WriteFile(rc, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Values) != 0 {
		t.Errorf("expected 0 values for empty file, got %d", len(cfg.Values))
	}
}

func TestLoadConfig_LineWithoutEquals(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	content := `GOOD=value
bad_line_no_equals
ALSO_GOOD=yes
`
	if err := os.WriteFile(rc, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if len(cfg.Values) != 2 {
		t.Errorf("expected 2 values (skipping bad line), got %d", len(cfg.Values))
	}
}

func TestLoadConfig_WhitespaceAroundKeyValue(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	content := `  KEY  =  value
`
	if err := os.WriteFile(rc, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Values["KEY"]; got != "value" {
		t.Errorf("trimmed key/value: got %q, want %q", got, "value")
	}
}

func TestLoadConfig_PathIsSet(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".ralphrc")
	if err := os.WriteFile(rc, []byte("K=V\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Path != rc {
		t.Errorf("Path = %q, want %q", cfg.Path, rc)
	}
}

func TestRalphConfig_Get(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *RalphConfig
		key        string
		defaultVal string
		want       string
	}{
		{
			name:       "nil config returns default",
			cfg:        nil,
			key:        "foo",
			defaultVal: "bar",
			want:       "bar",
		},
		{
			name: "existing key returns value",
			cfg: &RalphConfig{
				Values: map[string]string{"MODEL": "sonnet"},
			},
			key:        "MODEL",
			defaultVal: "default",
			want:       "sonnet",
		},
		{
			name: "missing key returns default",
			cfg: &RalphConfig{
				Values: map[string]string{"MODEL": "sonnet"},
			},
			key:        "MISSING",
			defaultVal: "fallback",
			want:       "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.Get(tt.key, tt.defaultVal)
			if got != tt.want {
				t.Errorf("Get(%q, %q) = %q, want %q", tt.key, tt.defaultVal, got, tt.want)
			}
		})
	}
}

func TestSave_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".ralphrc")

	tests := []struct {
		name string
		key  string
	}{
		{"spaces", "BAD KEY"},
		{"lowercase", "lowercase"},
		{"special chars", "KEY!@#"},
		{"starts with number", "1KEY"},
		{"mixed case", "myKey"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RalphConfig{
				Path:   rcPath,
				Values: map[string]string{tt.key: "value"},
			}
			err := cfg.Save()
			if err == nil {
				t.Errorf("expected error for key %q, got nil", tt.key)
			}
		})
	}
}

func TestValidateConfig_UnknownKeys(t *testing.T) {
	cfg := &RalphConfig{
		Values: map[string]string{
			"MODEL":       "sonnet",
			"UNKNOWN_KEY": "value",
		},
	}
	warnings, errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("expected no errors for unknown key, got %v", errs)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if warnings[0].Key != "UNKNOWN_KEY" {
		t.Errorf("warning key = %q, want UNKNOWN_KEY", warnings[0].Key)
	}
}

func TestValidateConfig_TypeMismatch(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"int as string", "MAX_CALLS_PER_HOUR", "abc"},
		{"float as string", "BUDGET", "not-a-number"},
		{"bool as string", "AUTO_ENHANCE", "maybe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RalphConfig{Values: map[string]string{tt.key: tt.val}}
			_, errs := ValidateConfig(cfg)
			if len(errs) == 0 {
				t.Errorf("expected error for %s=%q", tt.key, tt.val)
			}
		})
	}
}

func TestValidateConfig_RangeViolation(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{"int below min", "MAX_CALLS_PER_HOUR", "0"},
		{"int above max", "MAX_CALLS_PER_HOUR", "9999"},
		{"float below min", "BUDGET", "0"},
		{"float above max", "BUDGET", "5000"},
		{"autonomy too high", "AUTONOMY_LEVEL", "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &RalphConfig{Values: map[string]string{tt.key: tt.val}}
			_, errs := ValidateConfig(cfg)
			if len(errs) == 0 {
				t.Errorf("expected range error for %s=%q", tt.key, tt.val)
			}
		})
	}
}

func TestValidateConfig_ValidValues(t *testing.T) {
	cfg := &RalphConfig{
		Values: map[string]string{
			"MODEL":              "sonnet",
			"MAX_CALLS_PER_HOUR": "120",
			"BUDGET":             "5.00",
			"AUTO_ENHANCE":       "true",
			"AUTONOMY_LEVEL":     "2",
		},
	}
	warnings, errs := ValidateConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateConfig_BoolVariants(t *testing.T) {
	for _, v := range []string{"true", "false", "1", "0", "yes", "no"} {
		cfg := &RalphConfig{Values: map[string]string{"AUTO_ENHANCE": v}}
		_, errs := ValidateConfig(cfg)
		if len(errs) != 0 {
			t.Errorf("AUTO_ENHANCE=%q should be valid, got %v", v, errs)
		}
	}
}

func TestValidateConfig_Nil(t *testing.T) {
	warnings, errs := ValidateConfig(nil)
	if warnings != nil || errs != nil {
		t.Errorf("nil config should return nil, nil")
	}
}

func TestValidateConfig_DeprecatedKeys(t *testing.T) {
	cfg := &RalphConfig{
		Values: map[string]string{
			"CLAUDE_MODEL":    "sonnet",
			"GEMINI_MODEL":    "flash",
			"MAX_RETRIES":     "3",
			"TIMEOUT_SECONDS": "60",
		},
	}
	warnings, errs := ValidateConfig(cfg)
	if len(errs) != 0 {
		t.Errorf("deprecated keys should not produce errors, got %v", errs)
	}
	if len(warnings) != 4 {
		t.Fatalf("expected 4 deprecated warnings, got %d: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if _, ok := DeprecatedKeys[w.Key]; !ok {
			t.Errorf("unexpected warning key %q", w.Key)
		}
		if w.Message == "unknown config key" {
			t.Errorf("deprecated key %q should not get 'unknown config key' warning", w.Key)
		}
	}
}

func TestValidateConfig_DeprecatedKeyNotDuplicated(t *testing.T) {
	// A deprecated key that is NOT in KnownKeys should get only one warning (deprecated), not also "unknown".
	cfg := &RalphConfig{
		Values: map[string]string{
			"CLAUDE_MODEL": "sonnet",
		},
	}
	warnings, _ := ValidateConfig(cfg)
	count := 0
	for _, w := range warnings {
		if w.Key == "CLAUDE_MODEL" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 warning for deprecated key CLAUDE_MODEL, got %d", count)
	}
}

func TestConfigDiff_AddedKeys(t *testing.T) {
	old := &RalphConfig{Values: map[string]string{"MODEL": "sonnet"}}
	new := &RalphConfig{Values: map[string]string{"MODEL": "sonnet", "BUDGET": "10"}}
	changes := ConfigDiff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0].Type != "added" || changes[0].Key != "BUDGET" || changes[0].NewVal != "10" {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestConfigDiff_RemovedKeys(t *testing.T) {
	old := &RalphConfig{Values: map[string]string{"MODEL": "sonnet", "BUDGET": "10"}}
	new := &RalphConfig{Values: map[string]string{"MODEL": "sonnet"}}
	changes := ConfigDiff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0].Type != "removed" || changes[0].Key != "BUDGET" || changes[0].OldVal != "10" {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestConfigDiff_ChangedKeys(t *testing.T) {
	old := &RalphConfig{Values: map[string]string{"MODEL": "sonnet"}}
	new := &RalphConfig{Values: map[string]string{"MODEL": "opus"}}
	changes := ConfigDiff(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(changes), changes)
	}
	if changes[0].Type != "changed" || changes[0].OldVal != "sonnet" || changes[0].NewVal != "opus" {
		t.Errorf("unexpected change: %+v", changes[0])
	}
}

func TestConfigDiff_NoChanges(t *testing.T) {
	cfg := &RalphConfig{Values: map[string]string{"MODEL": "sonnet"}}
	changes := ConfigDiff(cfg, cfg)
	if len(changes) != 0 {
		t.Errorf("expected no changes, got %v", changes)
	}
}

func TestConfigDiff_NilConfigs(t *testing.T) {
	new := &RalphConfig{Values: map[string]string{"MODEL": "sonnet"}}
	changes := ConfigDiff(nil, new)
	if len(changes) != 1 || changes[0].Type != "added" {
		t.Errorf("nil old should show all keys as added, got %v", changes)
	}

	old := &RalphConfig{Values: map[string]string{"MODEL": "sonnet"}}
	changes = ConfigDiff(old, nil)
	if len(changes) != 1 || changes[0].Type != "removed" {
		t.Errorf("nil new should show all keys as removed, got %v", changes)
	}

	changes = ConfigDiff(nil, nil)
	if len(changes) != 0 {
		t.Errorf("both nil should have no changes, got %v", changes)
	}
}

func TestConfigDiff_Mixed(t *testing.T) {
	old := &RalphConfig{Values: map[string]string{"A": "1", "B": "2", "C": "3"}}
	new := &RalphConfig{Values: map[string]string{"A": "1", "B": "99", "D": "4"}}
	changes := ConfigDiff(old, new)

	byKey := map[string]ConfigChange{}
	for _, c := range changes {
		byKey[c.Key] = c
	}

	if len(changes) != 3 {
		t.Fatalf("expected 3 changes, got %d: %v", len(changes), changes)
	}
	if byKey["B"].Type != "changed" {
		t.Errorf("B should be changed, got %v", byKey["B"])
	}
	if byKey["C"].Type != "removed" {
		t.Errorf("C should be removed, got %v", byKey["C"])
	}
	if byKey["D"].Type != "added" {
		t.Errorf("D should be added, got %v", byKey["D"])
	}
}

func TestRalphConfig_Save(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".ralphrc")

	cfg := &RalphConfig{
		Path: rcPath,
		Values: map[string]string{
			"MODEL":  "sonnet",
			"BUDGET": "10",
		},
	}

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Re-load and verify
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}

	if got := loaded.Values["MODEL"]; got != "sonnet" {
		t.Errorf("after save, MODEL = %q, want %q", got, "sonnet")
	}
	if got := loaded.Values["BUDGET"]; got != "10" {
		t.Errorf("after save, BUDGET = %q, want %q", got, "10")
	}
}
