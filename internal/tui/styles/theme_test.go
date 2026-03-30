package styles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultThemes_ReturnsExpectedNames(t *testing.T) {
	themes := DefaultThemes()
	expected := []string{"k9s", "dracula", "gruvbox", "nord", "snazzy", "catppuccin-macchiato", "catppuccin-mocha"}
	for _, name := range expected {
		if _, ok := themes[name]; !ok {
			t.Errorf("DefaultThemes() missing expected theme %q", name)
		}
	}
	if len(themes) != len(expected) {
		t.Errorf("DefaultThemes() returned %d themes, want %d", len(themes), len(expected))
	}
}

func TestDefaultThemes_AllFieldsNonEmpty(t *testing.T) {
	for name, theme := range DefaultThemes() {
		t.Run(name, func(t *testing.T) {
			if theme.Name == "" {
				t.Error("Name is empty")
			}
			if theme.Primary == "" {
				t.Error("Primary is empty")
			}
			if theme.Accent == "" {
				t.Error("Accent is empty")
			}
			if theme.Green == "" {
				t.Error("Green is empty")
			}
			if theme.Yellow == "" {
				t.Error("Yellow is empty")
			}
			if theme.Red == "" {
				t.Error("Red is empty")
			}
			if theme.Gray == "" {
				t.Error("Gray is empty")
			}
			if theme.DarkBg == "" {
				t.Error("DarkBg is empty")
			}
			if theme.BrightFg == "" {
				t.Error("BrightFg is empty")
			}
		})
	}
}

func TestLoadTheme_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	content := `name: test
primary: "39"
accent: "205"
green: "42"
yellow: "214"
red: "196"
gray: "244"
dark_bg: "236"
bright_fg: "255"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	theme, err := LoadTheme(path)
	if err != nil {
		t.Fatalf("LoadTheme() error: %v", err)
	}
	if theme.Name != "test" {
		t.Errorf("Name = %q, want %q", theme.Name, "test")
	}
	if theme.Primary != "39" {
		t.Errorf("Primary = %q, want %q", theme.Primary, "39")
	}
	if theme.BrightFg != "255" {
		t.Errorf("BrightFg = %q, want %q", theme.BrightFg, "255")
	}
}

func TestLoadTheme_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("{{{{not yaml"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTheme(path)
	if err == nil {
		t.Error("LoadTheme() with invalid YAML should return error")
	}
}

func TestLoadTheme_MissingFile(t *testing.T) {
	_, err := LoadTheme("/nonexistent/path/theme.yaml")
	if err == nil {
		t.Error("LoadTheme() with missing file should return error")
	}
}

func TestApplyTheme_ChangesPackageLevelVars(t *testing.T) {
	// Save originals
	origPrimary := ColorPrimary
	origAccent := ColorAccent

	theme := &Theme{
		Name:     "custom",
		Primary:  "#ff0000",
		Accent:   "#00ff00",
		Green:    "#00ff00",
		Yellow:   "#ffff00",
		Red:      "#ff0000",
		Gray:     "#808080",
		DarkBg:   "#000000",
		BrightFg: "#ffffff",
	}
	ApplyTheme(theme)

	if string(ColorPrimary) != "#ff0000" {
		t.Errorf("ColorPrimary = %q after ApplyTheme, want %q", string(ColorPrimary), "#ff0000")
	}
	if string(ColorAccent) != "#00ff00" {
		t.Errorf("ColorAccent = %q after ApplyTheme, want %q", string(ColorAccent), "#00ff00")
	}

	// Verify styles were rebuilt by checking they render non-empty
	if TitleStyle.Render("x") == "" {
		t.Error("TitleStyle.Render returned empty after ApplyTheme")
	}
	if ProviderClaudeStyle.Render("x") == "" {
		t.Error("ProviderClaudeStyle.Render returned empty after ApplyTheme")
	}

	// Restore defaults
	ApplyTheme(&Theme{
		Primary:  string(origPrimary),
		Accent:   string(origAccent),
		Green:    "42",
		Yellow:   "214",
		Red:      "196",
		Gray:     "244",
		DarkBg:   "236",
		BrightFg: "255",
	})
}

func TestLoadExternalThemes(t *testing.T) {
	dir := t.TempDir()

	// Write a valid theme YAML.
	content := `name: my-custom
primary: "#aabbcc"
accent: "#ddeeff"
green: "#00ff00"
yellow: "#ffff00"
red: "#ff0000"
gray: "#808080"
dark_bg: "#000000"
bright_fg: "#ffffff"
`
	if err := os.WriteFile(filepath.Join(dir, "my-custom.yaml"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	// Write a non-YAML file that should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("not a theme"), 0644); err != nil {
		t.Fatal(err)
	}

	themes := LoadExternalThemes(dir)
	if len(themes) != 1 {
		t.Fatalf("LoadExternalThemes() returned %d themes, want 1", len(themes))
	}
	if themes["my-custom"] == nil {
		t.Fatal("expected theme keyed as 'my-custom'")
	}
	if themes["my-custom"].Primary != "#aabbcc" {
		t.Errorf("Primary = %q, want %q", themes["my-custom"].Primary, "#aabbcc")
	}
}

func TestLoadExternalThemes_EmptyDir(t *testing.T) {
	themes := LoadExternalThemes(t.TempDir())
	if len(themes) != 0 {
		t.Errorf("LoadExternalThemes(empty) returned %d themes, want 0", len(themes))
	}
}

func TestLoadExternalThemes_NonexistentDir(t *testing.T) {
	themes := LoadExternalThemes("/nonexistent/dir")
	if len(themes) != 0 {
		t.Errorf("LoadExternalThemes(nonexistent) returned %d themes, want 0", len(themes))
	}
}

func TestResolveTheme_BuiltIn(t *testing.T) {
	th := ResolveTheme("dracula")
	if th == nil {
		t.Fatal("ResolveTheme('dracula') returned nil")
	}
	if th.Name != "dracula" {
		t.Errorf("Name = %q, want 'dracula'", th.Name)
	}
}

func TestResolveTheme_NotFound(t *testing.T) {
	th := ResolveTheme("nonexistent-theme-name")
	if th != nil {
		t.Error("ResolveTheme for unknown theme should return nil")
	}
}

func TestApplyTheme_NilIsNoOp(t *testing.T) {
	before := string(ColorPrimary)
	ApplyTheme(nil)
	after := string(ColorPrimary)
	if before != after {
		t.Errorf("ApplyTheme(nil) changed ColorPrimary from %q to %q", before, after)
	}
}
