package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

func TestPidDir_EmptyStateDir(t *testing.T) {
	m := &Manager{
		sessions: make(map[string]*Session),
	}
	m.stateDir = ""
	if got := m.pidDir(); got != "" {
		t.Errorf("pidDir() = %q, want empty", got)
	}
}

func TestPidDir_ReturnsCorrectPath(t *testing.T) {
	m := &Manager{
		sessions: make(map[string]*Session),
	}
	m.stateDir = "/tmp/ralph/sessions"
	got := m.pidDir()
	want := "/tmp/ralph/pids"
	if got != want {
		t.Errorf("pidDir() = %q, want %q", got, want)
	}
}

func TestInitPIDFiles_CleansDeadProcesses(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	pidDir := filepath.Join(tmp, "pids")
	stateDir := filepath.Join(tmp, "sessions")

	// Write a PID file for a dead process (PID 999999 is almost certainly dead).
	err := process.WritePIDFile(pidDir, process.PIDInfo{
		PID:      999999,
		RepoPath: "/fake/repo",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the PID file exists before init.
	pidFilePath := filepath.Join(pidDir, "999999.json")
	if _, err := os.Stat(pidFilePath); os.IsNotExist(err) {
		t.Fatal("PID file should exist before init")
	}

	m := &Manager{
		sessions: make(map[string]*Session),
		stateDir: stateDir,
	}

	m.InitPIDFiles()

	// The PID file for the dead process should have been cleaned up.
	if _, err := os.Stat(pidFilePath); !os.IsNotExist(err) {
		t.Error("PID file for dead process should have been removed")
	}
}

func TestInitPIDFiles_NoPIDDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	stateDir := filepath.Join(tmp, "sessions")

	m := &Manager{
		sessions: make(map[string]*Session),
		stateDir: stateDir,
	}

	// Should not panic or error when pids directory doesn't exist.
	m.InitPIDFiles()
}

func TestInitPIDFiles_EmptyStateDir(t *testing.T) {
	t.Parallel()

	m := &Manager{
		sessions: make(map[string]*Session),
	}
	m.stateDir = ""

	// Should return immediately without error.
	m.InitPIDFiles()
}
