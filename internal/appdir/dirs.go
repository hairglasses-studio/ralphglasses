package appdir

import (
	"os"
	"path/filepath"
	"strings"
)

func CacheDir(app string) string {
	return joinAppDir(baseDir("XDG_CACHE_HOME", filepath.Join(homeDir(), ".cache"), os.TempDir()), app)
}

func ConfigDir(app string) string {
	return joinAppDir(baseDir("XDG_CONFIG_HOME", filepath.Join(homeDir(), ".config"), os.TempDir()), app)
}

func StateDir(app string) string {
	return joinAppDir(baseDir("XDG_STATE_HOME", filepath.Join(homeDir(), ".local", "state"), os.TempDir()), app)
}

func RuntimeDir(app string) string {
	return joinAppDir(baseDir("XDG_RUNTIME_DIR", "", os.TempDir()), app)
}

func baseDir(envName, homeFallback, tempFallback string) string {
	if dir := strings.TrimSpace(os.Getenv(envName)); dir != "" {
		return dir
	}
	if homeFallback != "" {
		return homeFallback
	}
	return tempFallback
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

func joinAppDir(base, app string) string {
	if strings.TrimSpace(app) == "" {
		return base
	}
	return filepath.Join(base, app)
}
