package process

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateToJSON_NoLegacyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0o755); err != nil {
		t.Fatal(err)
	}

	info := migrateToJSON(repoPath)
	if info != nil {
		t.Errorf("expected nil for missing legacy PID file, got %+v", info)
	}
}

func TestMigrateToJSON_DeadProcess(t *testing.T) {
	// Not parallel: mutates global aliveFnPtr.

	// Stub aliveFn to report all processes as dead.
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)
	setAliveFn(func(pid int) bool { return false })

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a legacy integer PID file.
	pidPath := filepath.Join(ralphDir, pidFileName)
	if err := os.WriteFile(pidPath, []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := migrateToJSON(repoPath)
	if info != nil {
		t.Errorf("expected nil for dead process, got %+v", info)
	}

	// Legacy file should have been removed.
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("legacy PID file should be removed for dead process")
	}
}

func TestMigrateToJSON_AliveProcess(t *testing.T) {
	// Not parallel: mutates global aliveFnPtr.

	// Stub aliveFn to report all processes as alive.
	origAlive := *aliveFnPtr.Load()
	defer setAliveFn(origAlive)
	setAliveFn(func(pid int) bool { return true })

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a legacy integer PID file.
	pidPath := filepath.Join(ralphDir, pidFileName)
	if err := os.WriteFile(pidPath, []byte("54321"), 0o644); err != nil {
		t.Fatal(err)
	}

	info := migrateToJSON(repoPath)
	if info == nil {
		t.Fatal("expected non-nil info for alive process")
	}
	if info.PID != 54321 {
		t.Errorf("PID = %d, want 54321", info.PID)
	}
	if info.RepoPath != repoPath {
		t.Errorf("RepoPath = %q, want %q", info.RepoPath, repoPath)
	}

	// JSON PID file should have been created.
	jsonPath := filepath.Join(ralphDir, SessionDir, fmt.Sprintf("%d.json", info.PID))
	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON PID file should have been created")
	}
}
