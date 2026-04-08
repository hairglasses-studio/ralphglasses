package tui

import (
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

func TestDefaultAliasPath_UsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")
	t.Setenv("HOME", "")

	want := filepath.Join("/custom/config", "ralphglasses", "aliases.json")
	if got := DefaultAliasPath(); got != want {
		t.Errorf("DefaultAliasPath() = %q, want %q", got, want)
	}
	if got := DefaultAliasPath(); got != ralphpath.AliasesJSONPath() {
		t.Errorf("DefaultAliasPath() = %q, want shared resolver path %q", got, ralphpath.AliasesJSONPath())
	}
}

func TestDefaultAliasPath_FallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".config", "ralphglasses", "aliases.json")
	if got := DefaultAliasPath(); got != want {
		t.Errorf("DefaultAliasPath() = %q, want %q", got, want)
	}
}
