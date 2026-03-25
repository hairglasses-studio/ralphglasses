package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const selfTestEnvVar = "RALPH_SELF_TEST"

// IsSelfTestTarget checks if the given repo path contains the ralphglasses project
// by reading its go.mod for the module path.
func IsSelfTestTarget(repoPath string) bool {
	data, err := os.ReadFile(filepath.Join(repoPath, "go.mod"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "hairglasses-studio/ralphglasses")
}

// RecursionGuard returns an error if RALPH_SELF_TEST is already set,
// indicating we're inside a recursive self-test loop.
func RecursionGuard() error {
	if os.Getenv(selfTestEnvVar) == "1" {
		return fmt.Errorf("recursive self-test detected: %s is already set", selfTestEnvVar)
	}
	return nil
}

// SetSelfTestEnv returns environment variables with RALPH_SELF_TEST=1 added.
// Used when launching child processes during self-test runs.
func SetSelfTestEnv(env []string) []string {
	return append(env, selfTestEnvVar+"=1")
}
