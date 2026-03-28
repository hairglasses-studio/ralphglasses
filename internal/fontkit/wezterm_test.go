package fontkit

import (
	"os"
	"strings"
	"testing"
)

func TestConfigureWezTerm(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ConfigureWezTerm(WezTermOpts{FontSize: 16})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if !strings.Contains(content, `wezterm.font("MonaspiceNeNFM")`) {
		t.Error("missing font name")
	}
	if !strings.Contains(content, "font_size = 16.0") {
		t.Error("missing or wrong font_size")
	}
	if !strings.Contains(content, "local M = {}") {
		t.Error("missing module structure")
	}
	if !strings.Contains(content, "return M") {
		t.Error("missing module return")
	}
}

func TestConfigureWezTermDefault(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ConfigureWezTerm(WezTermOpts{})
	if err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, "font_size = 15.0") {
		t.Error("default font_size should be 15.0")
	}
}
