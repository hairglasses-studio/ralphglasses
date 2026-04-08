package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultActiveStateFile_UsesRuntimeDir(t *testing.T) {
	override := filepath.Join(t.TempDir(), "active.json")
	t.Setenv("RALPHGLASSES_ACTIVE_STATE_FILE", override)
	if got := defaultActiveStateFile(); got != override {
		t.Fatalf("defaultActiveStateFile() override = %q, want %q", got, override)
	}

	t.Setenv("RALPHGLASSES_ACTIVE_STATE_FILE", "")
	runtimeDir := filepath.Join(t.TempDir(), "runtime")
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	if got, want := defaultActiveStateFile(), filepath.Join(runtimeDir, "ralphglasses", "ralphglasses-active.json"); got != want {
		t.Fatalf("defaultActiveStateFile() = %q, want %q", got, want)
	}
}

func useTempActiveStateFile(t *testing.T) string {
	t.Helper()

	origActiveStateFile := activeStateFile
	activeStateFile = filepath.Join(t.TempDir(), "ralphglasses-active.json")
	t.Cleanup(func() { activeStateFile = origActiveStateFile })
	return activeStateFile
}

func TestStateWriterWriteActiveState(t *testing.T) {
	activePath := useTempActiveStateFile(t)

	// Clean up after test.
	t.Cleanup(func() { os.Remove(activeStateFile) })

	tests := []struct {
		name     string
		session  *Session
		wantName string
		wantProv string
		wantCost string
	}{
		{
			name: "basic session",
			session: &Session{
				RepoName: "dotfiles",
				Provider: "claude",
				Status:   "running",
				SpentUSD: 0.0042,
			},
			wantName: "dotfiles",
			wantProv: "claude",
			wantCost: "$0.0042",
		},
		{
			name: "zero cost",
			session: &Session{
				RepoName: "ralphglasses",
				Provider: "gemini",
				Status:   "pending",
				SpentUSD: 0,
			},
			wantName: "ralphglasses",
			wantProv: "gemini",
			wantCost: "$0.0000",
		},
		{
			name: "high cost",
			session: &Session{
				RepoName: "big-project",
				Provider: "openai",
				Status:   "completed",
				SpentUSD: 12.3456,
			},
			wantName: "big-project",
			wantProv: "openai",
			wantCost: "$12.3456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := WriteActiveState(tt.session); err != nil {
				t.Fatalf("WriteActiveState() error: %v", err)
			}

			data, err := os.ReadFile(activePath)
			if err != nil {
				t.Fatalf("failed to read active state file: %v", err)
			}

			var state ActiveState
			if err := json.Unmarshal(data, &state); err != nil {
				t.Fatalf("invalid JSON in state file: %v", err)
			}

			if state.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", state.Name, tt.wantName)
			}
			if state.Provider != tt.wantProv {
				t.Errorf("Provider = %q, want %q", state.Provider, tt.wantProv)
			}
			if state.Status != string(tt.session.Status) {
				t.Errorf("Status = %q, want %q", state.Status, tt.session.Status)
			}
			if state.Cost != tt.wantCost {
				t.Errorf("Cost = %q, want %q", state.Cost, tt.wantCost)
			}
		})
	}
}

func TestStateWriterRemoveActiveState(t *testing.T) {
	activePath := useTempActiveStateFile(t)

	// Write a state file first.
	s := &Session{
		RepoName: "test-repo",
		Provider: "claude",
		Status:   "running",
	}
	if err := WriteActiveState(s); err != nil {
		t.Fatalf("WriteActiveState() setup error: %v", err)
	}

	// Verify it exists.
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("state file should exist before removal: %v", err)
	}

	RemoveActiveState()

	// Verify it's gone.
	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Fatalf("state file should not exist after removal, got err: %v", err)
	}
}

func TestStateWriterRemoveActiveStateIdempotent(t *testing.T) {
	activePath := useTempActiveStateFile(t)

	// Removing when file doesn't exist should not panic.
	os.Remove(activePath) // ensure it doesn't exist
	RemoveActiveState()   // should not panic
}

func TestStateWriterAtomicWrite(t *testing.T) {
	activePath := useTempActiveStateFile(t)
	t.Cleanup(func() { os.Remove(activeStateFile) })

	s := &Session{
		RepoName:     "atomic-test",
		Provider:     "claude",
		Status:       "running",
		SpentUSD:     1.5,
		LaunchedAt:   time.Now(),
		LastActivity: time.Now(),
	}

	if err := WriteActiveState(s); err != nil {
		t.Fatalf("WriteActiveState() error: %v", err)
	}

	// Verify the file is valid JSON (atomic write should never produce partial content).
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	var state ActiveState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("state file contains invalid JSON (possible partial write): %v", err)
	}

	if state.Name != "atomic-test" {
		t.Errorf("Name = %q, want %q", state.Name, "atomic-test")
	}

	// Verify no temp files were left behind.
	entries, err := os.ReadDir(filepath.Dir(activePath))
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	tmpPrefix := filepath.Base(activePath) + "."
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), tmpPrefix) && strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file found: %s", e.Name())
		}
	}
}
