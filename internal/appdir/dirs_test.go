package appdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheDir_PrefersXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("HOME", "")

	if got, want := CacheDir("ralphglasses"), filepath.Join(xdg, "ralphglasses"); got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_FallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	if got, want := ConfigDir("ralphglasses"), filepath.Join(home, ".config", "ralphglasses"); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestStateDir_FallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	if got, want := StateDir("ralphglasses"), filepath.Join(home, ".local", "state", "ralphglasses"); got != want {
		t.Fatalf("StateDir() = %q, want %q", got, want)
	}
}

func TestRuntimeDir_FallsBackToTemp(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	if got, want := RuntimeDir("ralphglasses"), filepath.Join(os.TempDir(), "ralphglasses"); got != want {
		t.Fatalf("RuntimeDir() = %q, want %q", got, want)
	}
}
