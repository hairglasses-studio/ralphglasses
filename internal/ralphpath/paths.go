package ralphpath

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/appdir"
)

const (
	CoordinationDirEnv = "RALPHGLASSES_COORD_DIR"
	activeStateEnv     = "RALPHGLASSES_ACTIVE_STATE_FILE"
)

func ConfigDir() string {
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".ralphglasses")
	}
	return appdir.ConfigDir("ralphglasses")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func StateDir() string {
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".ralphglasses")
	}
	return appdir.StateDir("ralphglasses")
}

func SessionsDir() string {
	return filepath.Join(StateDir(), "sessions")
}

func SQLiteStorePath() string {
	return filepath.Join(StateDir(), "state.db")
}

func CoordinationDir() string {
	if override := strings.TrimSpace(os.Getenv(CoordinationDirEnv)); override != "" {
		return override
	}
	if xdgRuntime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); xdgRuntime != "" {
		return filepath.Join(xdgRuntime, "ralphglasses", "coordination")
	}
	if home := homeDir(); home != "" {
		return filepath.Join(home, ".ralphglasses", "coordination")
	}
	return filepath.Join(os.TempDir(), "ralphglasses-coordination")
}

func ActiveStatePath() string {
	if override := strings.TrimSpace(os.Getenv(activeStateEnv)); override != "" {
		return override
	}
	return filepath.Join(appdir.RuntimeDir("ralphglasses"), "ralphglasses-active.json")
}

func homeDir() string {
	if home, ok := os.LookupEnv("HOME"); ok {
		return strings.TrimSpace(home)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(home)
}
