package styles

import (
	"strings"
	"testing"
)

func TestThemeDir_WithXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg-config")
	got := ThemeDir()
	if !strings.Contains(got, "ralphglasses") || !strings.Contains(got, "themes") {
		t.Errorf("ThemeDir with XDG = %q, want path containing ralphglasses/themes", got)
	}
	if !strings.HasPrefix(got, "/tmp/test-xdg-config") {
		t.Errorf("ThemeDir with XDG = %q, want XDG-based path", got)
	}
}

func TestThemeDir_WithoutXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	got := ThemeDir()
	// Without XDG, should return home-based path or empty string on failure.
	// Just verify it doesn't panic and contains expected suffix when non-empty.
	if got != "" && !strings.Contains(got, "ralphglasses") {
		t.Errorf("ThemeDir without XDG = %q, want path containing ralphglasses or empty", got)
	}
}

func TestLoadExternalThemes_NonExistentDir(t *testing.T) {
	themes := LoadExternalThemes("/nonexistent/path/that/does/not/exist")
	if len(themes) != 0 {
		t.Errorf("LoadExternalThemes(nonexistent) = %v, want empty", themes)
	}
}
