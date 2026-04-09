package mcpserver

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	os.Exit(runWithCleanGitEnv(m))
}

func runWithCleanGitEnv(m *testing.M) int {
	keys := []string{
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_INDEX_FILE",
		"GIT_PREFIX",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_COMMON_DIR",
		"GIT_SUPER_PREFIX",
		"GIT_NAMESPACE",
		"GIT_CEILING_DIRECTORIES",
	}

	saved := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			saved[key] = value
			_ = os.Unsetenv(key)
		}
	}

	code := m.Run()

	for key, value := range saved {
		_ = os.Setenv(key, value)
	}
	return code
}
