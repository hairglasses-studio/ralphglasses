package themekit

import (
	"os"
	"strings"
	"testing"
)

func TestExportWezTerm(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ExportWezTerm(Catppuccin(Mocha))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `background = "#1e1e2e"`) {
		t.Error("missing background color")
	}
	if !strings.Contains(content, `foreground = "#cdd6f4"`) {
		t.Error("missing foreground color")
	}
	if !strings.Contains(content, `"#f38ba8"`) {
		t.Error("missing red in ansi palette")
	}
	if !strings.Contains(content, "Catppuccin Mocha") {
		t.Error("missing palette name in header")
	}
}

func TestExportWezTermAllFlavors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	for _, f := range AllFlavors() {
		_, err := ExportWezTerm(Catppuccin(f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
		}
	}
}
