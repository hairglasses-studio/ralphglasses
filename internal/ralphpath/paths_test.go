package ralphpath

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPath_UsesLegacyHomeDirWhenAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	if got, want := ConfigPath(), filepath.Join(home, ".ralphglasses", "config.json"); got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestConfigPath_FallsBackToXDGConfigDirWithoutHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if got, want := ConfigPath(), filepath.Join(xdg, "ralphglasses", "config.json"); got != want {
		t.Fatalf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestStateAndSQLitePaths_UseLegacyHomeDirWhenAvailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	if got, want := StateDir(), filepath.Join(home, ".ralphglasses"); got != want {
		t.Fatalf("StateDir() = %q, want %q", got, want)
	}
	if got, want := SessionsDir(), filepath.Join(home, ".ralphglasses", "sessions"); got != want {
		t.Fatalf("SessionsDir() = %q, want %q", got, want)
	}
	if got, want := SQLiteStorePath(), filepath.Join(home, ".ralphglasses", "state.db"); got != want {
		t.Fatalf("SQLiteStorePath() = %q, want %q", got, want)
	}
}

func TestCoordinationDir_PrefersEnvAndRuntimeAndHome(t *testing.T) {
	override := filepath.Join(t.TempDir(), "coord")
	t.Setenv(CoordinationDirEnv, override)
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(t.TempDir(), "runtime"))
	t.Setenv("HOME", t.TempDir())

	if got := CoordinationDir(); got != override {
		t.Fatalf("CoordinationDir() with env override = %q, want %q", got, override)
	}

	t.Setenv(CoordinationDirEnv, "")
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	if got, want := CoordinationDir(), filepath.Join(runtimeDir, "ralphglasses", "coordination"); got != want {
		t.Fatalf("CoordinationDir() with XDG runtime = %q, want %q", got, want)
	}

	t.Setenv("XDG_RUNTIME_DIR", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	if got, want := CoordinationDir(), filepath.Join(home, ".ralphglasses", "coordination"); got != want {
		t.Fatalf("CoordinationDir() with home fallback = %q, want %q", got, want)
	}
}

func TestCoordinationDir_FallsBackToTempWithoutHome(t *testing.T) {
	t.Setenv(CoordinationDirEnv, "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HOME", "")

	if got, want := CoordinationDir(), filepath.Join(os.TempDir(), "ralphglasses-coordination"); got != want {
		t.Fatalf("CoordinationDir() = %q, want %q", got, want)
	}
}

func TestActiveStatePath_PrefersOverrideThenRuntimeDir(t *testing.T) {
	override := filepath.Join(t.TempDir(), "active.json")
	t.Setenv(activeStateEnv, override)
	if got := ActiveStatePath(); got != override {
		t.Fatalf("ActiveStatePath() override = %q, want %q", got, override)
	}

	t.Setenv(activeStateEnv, "")
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	if got, want := ActiveStatePath(), filepath.Join(runtimeDir, "ralphglasses", "ralphglasses-active.json"); got != want {
		t.Fatalf("ActiveStatePath() = %q, want %q", got, want)
	}
}
