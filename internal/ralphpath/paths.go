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

func ConfigDirDefaultDescription() string {
	return "~/.ralphglasses when HOME is available; otherwise the XDG config dir"
}

func ConfigPathDefaultDescription() string {
	return "~/.ralphglasses/config.json when HOME is available; otherwise config.json in the XDG config dir"
}

func XDGConfigDir() string {
	return appdir.ConfigDir("ralphglasses")
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

func ExternalSessionSearchDirs(scanRoot string) []string {
	dirs := []string{SessionsDir(), LegacyScanRootSessionsDir(scanRoot)}
	seen := make(map[string]struct{}, len(dirs))
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if _, ok := seen[dir]; ok {
			continue
		}
		seen[dir] = struct{}{}
		out = append(out, dir)
	}
	return out
}

func LegacyScanRootSessionsDir(scanRoot string) string {
	scanRoot = strings.TrimSpace(scanRoot)
	if scanRoot == "" {
		return ""
	}
	return filepath.Join(scanRoot, ".session-state")
}

func SQLiteStorePath() string {
	return filepath.Join(StateDir(), "state.db")
}

func StateDirDefaultDescription() string {
	return "~/.ralphglasses when HOME is available; otherwise the XDG state dir"
}

func SQLiteStoreDefaultDescription() string {
	return "~/.ralphglasses/state.db when HOME is available; otherwise state.db in the XDG state dir"
}

func PromptsDir() string {
	return filepath.Join(StateDir(), "prompts")
}

func CostEventsPath() string {
	if legacy := existingLegacyHomePath(".ralph", "cost_events.jsonl"); legacy != "" {
		return legacy
	}
	return filepath.Join(StateDir(), "cost_events.jsonl")
}

func CommandHistoryPath() string {
	return filepath.Join(XDGConfigDir(), "command_history.json")
}

func ThemesDir() string {
	return filepath.Join(XDGConfigDir(), "themes")
}

func AliasesYAMLPath() string {
	return filepath.Join(XDGConfigDir(), "aliases.yml")
}

func AliasesJSONPath() string {
	return filepath.Join(XDGConfigDir(), "aliases.json")
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

func existingLegacyHomePath(parts ...string) string {
	home := homeDir()
	if home == "" {
		return ""
	}
	legacyPath := filepath.Join(append([]string{home}, parts...)...)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return ""
}
