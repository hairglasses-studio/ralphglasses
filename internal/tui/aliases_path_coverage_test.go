package tui

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultAliasPath_UsesXDGConfigHome(t *testing.T) {
	// Set a custom XDG_CONFIG_HOME and verify it's used.
	original := os.Getenv("XDG_CONFIG_HOME")
	t.Cleanup(func() { os.Setenv("XDG_CONFIG_HOME", original) })

	os.Setenv("XDG_CONFIG_HOME", "/custom/config")
	got := DefaultAliasPath()
	if !strings.HasPrefix(got, "/custom/config") {
		t.Errorf("DefaultAliasPath() = %q, want prefix /custom/config", got)
	}
	if !strings.HasSuffix(got, "aliases.json") {
		t.Errorf("DefaultAliasPath() = %q, want suffix aliases.json", got)
	}
}

func TestDefaultAliasPath_FallsBackToHome(t *testing.T) {
	// Unset XDG_CONFIG_HOME to fall back to home directory.
	original := os.Getenv("XDG_CONFIG_HOME")
	t.Cleanup(func() { os.Setenv("XDG_CONFIG_HOME", original) })

	os.Unsetenv("XDG_CONFIG_HOME")
	got := DefaultAliasPath()
	// On CI or dev box, home is always set; result should be non-empty.
	if got == "" {
		// Only fail if UserHomeDir is actually available.
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			t.Errorf("DefaultAliasPath() returned empty string with home=%q", home)
		}
	}
	if got != "" && !strings.HasSuffix(got, "aliases.json") {
		t.Errorf("DefaultAliasPath() = %q, want suffix aliases.json", got)
	}
}
