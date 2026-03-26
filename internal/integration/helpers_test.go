//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestRepo creates a temp directory structure that mimics a ralph-managed
// repo: .ralph/ directory, optional .ralphrc, and a ralph_loop.sh script.
// The scriptBody is written as the body of a bash script (#!/bin/bash is prepended).
// Returns the repo path.
func setupTestRepo(t *testing.T, scriptBody string) string {
	t.Helper()

	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatalf("create .ralph dir: %v", err)
	}

	// Write a minimal .ralphrc so the scanner picks it up.
	rcPath := filepath.Join(repoPath, ".ralphrc")
	if err := os.WriteFile(rcPath, []byte("# minimal config\n"), 0644); err != nil {
		t.Fatalf("write .ralphrc: %v", err)
	}

	// Write ralph_loop.sh with the provided body.
	scriptPath := filepath.Join(repoPath, "ralph_loop.sh")
	content := "#!/bin/bash\n" + scriptBody + "\n"
	if err := os.WriteFile(scriptPath, []byte(content), 0755); err != nil {
		t.Fatalf("write ralph_loop.sh: %v", err)
	}

	return repoPath
}

// writeStatusJSON writes a status.json file into .ralph/ for the given repo.
func writeStatusJSON(t *testing.T, repoPath string, status map[string]any) {
	t.Helper()
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status: %v", err)
	}
	statusPath := filepath.Join(repoPath, ".ralph", "status.json")
	if err := os.WriteFile(statusPath, data, 0644); err != nil {
		t.Fatalf("write status.json: %v", err)
	}
}

// waitForCondition polls check at 20ms intervals until it returns true or
// timeout elapses. Calls t.Fatal on timeout.
func waitForCondition(t *testing.T, timeout time.Duration, msg string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition: %s", msg)
}

// pidFileExists checks whether .ralph/ralphglasses.pid exists for the repo.
func pidFileExists(repoPath string) bool {
	_, err := os.Stat(filepath.Join(repoPath, ".ralph", "ralphglasses.pid"))
	return err == nil
}
