package styles

import (
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

func TestThemeDir_WithXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg-config")
	t.Setenv("HOME", "")

	want := filepath.Join("/tmp/test-xdg-config", "ralphglasses", "themes")
	if got := ThemeDir(); got != want {
		t.Errorf("ThemeDir() = %q, want %q", got, want)
	}
	if got := ThemeDir(); got != ralphpath.ThemesDir() {
		t.Errorf("ThemeDir() = %q, want shared resolver path %q", got, ralphpath.ThemesDir())
	}
}

func TestThemeDir_WithoutXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".config", "ralphglasses", "themes")
	if got := ThemeDir(); got != want {
		t.Errorf("ThemeDir() = %q, want %q", got, want)
	}
}

func TestLoadExternalThemes_NonExistentDir(t *testing.T) {
	themes := LoadExternalThemes("/nonexistent/path/that/does/not/exist")
	if len(themes) != 0 {
		t.Errorf("LoadExternalThemes(nonexistent) = %v, want empty", themes)
	}
}
