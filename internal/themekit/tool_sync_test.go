package themekit

import (
	"os"
	"strings"
	"testing"
)

func TestExportAlacrittyValidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := Catppuccin(Mocha)
	path, content, err := exportAlacritty(p)
	if err != nil {
		t.Fatal(err)
	}

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Error("written content does not match returned content")
	}

	// Check TOML structure
	sections := []string{
		"[colors.primary]",
		"[colors.cursor]",
		"[colors.selection]",
		"[colors.normal]",
		"[colors.bright]",
	}
	for _, s := range sections {
		if !strings.Contains(content, s) {
			t.Errorf("missing TOML section: %s", s)
		}
	}

	// Check Mocha hex colors
	if !strings.Contains(content, "#1e1e2e") { // base
		t.Error("missing Mocha base color")
	}
	if !strings.Contains(content, "#cdd6f4") { // text
		t.Error("missing Mocha text color")
	}
	if !strings.Contains(content, "#f38ba8") { // red
		t.Error("missing Mocha red color")
	}

	// Check it names the palette
	if !strings.Contains(content, "Catppuccin Mocha") {
		t.Error("missing palette name in header")
	}
}

func TestExportAlacrittyAllFlavors(t *testing.T) {
	for _, flavor := range AllFlavors() {
		t.Run(string(flavor), func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			_, content, err := exportAlacritty(Catppuccin(flavor))
			if err != nil {
				t.Fatalf("exportAlacritty(%s): %v", flavor, err)
			}
			if !strings.Contains(content, "[colors.primary]") {
				t.Error("missing [colors.primary] section")
			}
		})
	}
}

func TestExportKittyValidConf(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := Catppuccin(Mocha)
	path, content, err := exportKitty(p)
	if err != nil {
		t.Fatal(err)
	}

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Error("written content does not match returned content")
	}

	// Check kitty config keys
	keys := []string{
		"foreground ",
		"background ",
		"cursor ",
		"cursor_text_color ",
		"url_color ",
		"active_border_color ",
		"color0 ",
		"color7 ",
		"color8 ",
		"color15 ",
	}
	for _, k := range keys {
		if !strings.Contains(content, k) {
			t.Errorf("missing kitty key: %s", strings.TrimSpace(k))
		}
	}

	// Check hex colors
	if !strings.Contains(content, "#1e1e2e") { // base
		t.Error("missing Mocha base color")
	}
	if !strings.Contains(content, "#cdd6f4") { // text
		t.Error("missing Mocha text color")
	}

	// Check header
	if !strings.Contains(content, "Catppuccin Mocha") {
		t.Error("missing palette name in header")
	}
	if !strings.Contains(content, "include catppuccin.conf") {
		t.Error("missing include instruction")
	}
}

func TestExportKittyAllFlavors(t *testing.T) {
	for _, flavor := range AllFlavors() {
		t.Run(string(flavor), func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			_, content, err := exportKitty(Catppuccin(flavor))
			if err != nil {
				t.Fatalf("exportKitty(%s): %v", flavor, err)
			}
			if !strings.Contains(content, "foreground ") {
				t.Error("missing foreground key")
			}
		})
	}
}

func TestExportTmuxValidConf(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	p := Catppuccin(Mocha)
	path, content, err := exportTmux(p)
	if err != nil {
		t.Fatal(err)
	}

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Error("written content does not match returned content")
	}

	// Check tmux directives
	directives := []string{
		"set -g status-style",
		"set -g pane-border-style",
		"set -g pane-active-border-style",
		"set -g message-style",
		"set -g clock-mode-colour",
		"set -g window-status-style",
		"set -g window-status-current-style",
	}
	for _, d := range directives {
		if !strings.Contains(content, d) {
			t.Errorf("missing tmux directive: %s", d)
		}
	}

	// Check Mocha colors
	if !strings.Contains(content, "#181825") { // mantle
		t.Error("missing Mocha mantle color")
	}
	if !strings.Contains(content, "#b4befe") { // lavender
		t.Error("missing Mocha lavender color")
	}

	// Check header
	if !strings.Contains(content, "Catppuccin Mocha") {
		t.Error("missing palette name in header")
	}
	if !strings.Contains(content, "source-file") {
		t.Error("missing source-file instruction")
	}
}

func TestExportTmuxAllFlavors(t *testing.T) {
	for _, flavor := range AllFlavors() {
		t.Run(string(flavor), func(t *testing.T) {
			tmpDir := t.TempDir()
			t.Setenv("HOME", tmpDir)

			_, content, err := exportTmux(Catppuccin(flavor))
			if err != nil {
				t.Fatalf("exportTmux(%s): %v", flavor, err)
			}
			if !strings.Contains(content, "set -g status-style") {
				t.Error("missing status-style directive")
			}
		})
	}
}

func TestExportAllAllEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := SyncConfig{
		Alacritty: true,
		Kitty:     true,
		Tmux:      true,
		Starship:  true,
		Bat:       true,
	}

	results, err := ExportAll(Catppuccin(Mocha), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	expectedTools := []string{"alacritty", "kitty", "tmux", "starship", "bat"}
	for i, want := range expectedTools {
		if results[i].Tool != want {
			t.Errorf("result[%d].Tool = %q, want %q", i, results[i].Tool, want)
		}
		if !results[i].Applied {
			t.Errorf("result[%d] (%s) not applied: %v", i, want, results[i].Error)
		}
		if results[i].Path == "" {
			t.Errorf("result[%d] (%s) has empty path", i, want)
		}
		if results[i].Content == "" {
			t.Errorf("result[%d] (%s) has empty content", i, want)
		}
	}
}

func TestExportAllSubset(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := SyncConfig{
		Alacritty: true,
		Kitty:     false,
		Tmux:      true,
		Starship:  false,
		Bat:       false,
	}

	results, err := ExportAll(Catppuccin(Frappe), cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Tool != "alacritty" {
		t.Errorf("result[0].Tool = %q, want alacritty", results[0].Tool)
	}
	if results[1].Tool != "tmux" {
		t.Errorf("result[1].Tool = %q, want tmux", results[1].Tool)
	}

	for _, r := range results {
		if !r.Applied {
			t.Errorf("%s not applied: %v", r.Tool, r.Error)
		}
	}
}

func TestExportAllNoneEnabled(t *testing.T) {
	results, err := ExportAll(Catppuccin(Mocha), SyncConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestExportAllHandlesErrors(t *testing.T) {
	// Set HOME to a non-writable path to trigger errors
	t.Setenv("HOME", "/dev/null/nonexistent")

	cfg := SyncConfig{
		Alacritty: true,
		Kitty:     true,
	}

	results, err := ExportAll(Catppuccin(Mocha), cfg)
	if err == nil {
		t.Fatal("expected error from non-writable HOME")
	}

	// Should still get results for all attempted tools
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Applied {
			t.Errorf("%s should not be applied with invalid HOME", r.Tool)
		}
		if r.Error == nil {
			t.Errorf("%s should have error with invalid HOME", r.Tool)
		}
	}
}
