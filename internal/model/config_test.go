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
