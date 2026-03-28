package themekit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportStarship(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ExportStarship(Catppuccin(Mocha))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Check for expected TOML sections
	sections := []string{
		"[character]",
		"[directory]",
		"[git_branch]",
		"[git_status]",
		"[golang]",
		"[nodejs]",
		"[python]",
		"[rust]",
		"[time]",
	}
	for _, section := range sections {
		if !strings.Contains(content, section) {
			t.Errorf("missing section: %s", section)
		}
	}

	// Check claudekit markers
	if !strings.Contains(content, "# claudekit:begin") {
		t.Error("missing claudekit:begin marker")
	}
	if !strings.Contains(content, "# claudekit:end") {
		t.Error("missing claudekit:end marker")
	}

	// Check Mocha-specific hex colors
	if !strings.Contains(content, "#a6e3a1") { // green
		t.Error("missing Mocha green color")
	}
	if !strings.Contains(content, "#89b4fa") { // blue
		t.Error("missing Mocha blue color")
	}

	// Check Nerd Font symbols
	if !strings.Contains(content, "\ue627") { // Go nerd font icon
		t.Error("missing Nerd Font Go symbol")
	}
}

func TestExportStarshipPreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config")
	os.MkdirAll(configDir, 0o755)

	existing := "# My custom config\n[custom.mymod]\ncommand = \"echo hello\"\n"
	os.WriteFile(filepath.Join(configDir, "starship.toml"), []byte(existing), 0o644)

	_, err := ExportStarship(Catppuccin(Mocha))
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(configDir, "starship.toml"))
	content := string(data)

	if !strings.Contains(content, "# My custom config") {
		t.Error("existing custom config was lost")
	}
	if !strings.Contains(content, "[custom.mymod]") {
		t.Error("existing custom module was lost")
	}
	if !strings.Contains(content, "[character]") {
		t.Error("claudekit character config missing")
	}
}

func TestExportStarshipReplacesExistingBlock(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config")
	os.MkdirAll(configDir, 0o755)

	// Existing file with a claudekit block and user config
	existing := "# User preamble\n\n# claudekit:begin\n[character]\nsuccess_symbol = \"old\"\n# claudekit:end\n\n# User postamble\n"
	os.WriteFile(filepath.Join(configDir, "starship.toml"), []byte(existing), 0o644)

	_, err := ExportStarship(Catppuccin(Frappe))
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(configDir, "starship.toml"))
	content := string(data)

	if !strings.Contains(content, "# User preamble") {
		t.Error("user preamble was lost")
	}
	if !strings.Contains(content, "# User postamble") {
		t.Error("user postamble was lost")
	}
	if strings.Contains(content, "success_symbol = \"old\"") {
		t.Error("old claudekit block should have been replaced")
	}
	if !strings.Contains(content, "Catppuccin Frappé") {
		t.Error("new config should reference Frappe")
	}
}

func TestExportStarshipAllFlavors(t *testing.T) {
	for _, flavor := range AllFlavors() {
		t.Run(string(flavor), func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			path, err := ExportStarship(Catppuccin(flavor))
			if err != nil {
				t.Fatalf("ExportStarship(%s) failed: %v", flavor, err)
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)

			if !strings.Contains(content, "[character]") {
				t.Error("missing [character] section")
			}
			if !strings.Contains(content, "# claudekit:begin") {
				t.Error("missing claudekit:begin marker")
			}
		})
	}
}
