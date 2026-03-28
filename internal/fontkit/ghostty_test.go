package fontkit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureGhostty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ConfigureGhostty(GhosttyOpts{FontSize: 16})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "font-family = MonaspiceNeNFM") {
		t.Error("missing font-family directive")
	}
	if !strings.Contains(content, "font-size = 16") {
		t.Error("missing or wrong font-size")
	}
	if !strings.Contains(content, "# claudekit:") {
		t.Error("missing claudekit comment marker")
	}
}

func TestConfigureGhosttyPreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "ghostty")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config"), []byte("theme = catppuccin-mocha\nwindow-padding-x = 8\n"), 0o644)

	_, err := ConfigureGhostty(GhosttyOpts{})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(configDir, "config"))
	content := string(data)

	if !strings.Contains(content, "theme = catppuccin-mocha") {
		t.Error("existing theme config was lost")
	}
	if !strings.Contains(content, "window-padding-x = 8") {
		t.Error("existing padding config was lost")
	}
}
