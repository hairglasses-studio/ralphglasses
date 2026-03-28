package themekit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportBat(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ExportBat(Catppuccin(Mocha))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "--theme=Catppuccin Mocha") {
		t.Error("missing bat theme directive")
	}
	if !strings.Contains(content, "# claudekit:") {
		t.Error("missing claudekit marker comment")
	}
}

func TestExportBatPreservesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	configDir := filepath.Join(tmpDir, ".config", "bat")
	os.MkdirAll(configDir, 0o755)
	os.WriteFile(filepath.Join(configDir, "config"), []byte("--paging=never\n--theme=OneHalfDark\n"), 0o644)

	_, err := ExportBat(Catppuccin(Frappe))
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(configDir, "config"))
	content := string(data)

	if !strings.Contains(content, "--paging=never") {
		t.Error("existing paging config was lost")
	}
	if strings.Contains(content, "OneHalfDark") {
		t.Error("old theme should have been replaced")
	}
	if !strings.Contains(content, "Catppuccin Frappe") {
		t.Error("new theme should be Catppuccin Frappe")
	}
}

func TestExportDelta(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ExportDelta(Catppuccin(Mocha))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, "[delta]") {
		t.Error("missing [delta] section")
	}
	if !strings.Contains(content, "syntax-theme = Catppuccin Mocha") {
		t.Error("missing syntax-theme")
	}
	if !strings.Contains(content, "#f38ba8") {
		t.Error("missing red color for minus-style")
	}
}

func TestBatThemeName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Catppuccin Mocha", "Catppuccin Mocha"},
		{"Catppuccin Latte", "Catppuccin Latte"},
		{"Catppuccin Frappé", "Catppuccin Frappe"},
		{"Catppuccin Macchiato", "Catppuccin Macchiato"},
	}
	for _, tt := range tests {
		if got := batThemeName(tt.input); got != tt.want {
			t.Errorf("batThemeName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
