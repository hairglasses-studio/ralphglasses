//go:build integration

package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

func TestScanRefreshCycle(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "integration-repo")
	ralphDir := filepath.Join(repoDir, ".ralph")
	logDir := filepath.Join(ralphDir, "logs")
	_ = os.MkdirAll(logDir, 0755)

	status := model.LoopStatus{Status: "running", LoopCount: 1, CallsMadeThisHr: 5, MaxCallsPerHour: 80}
	writeJSON(t, filepath.Join(ralphDir, "status.json"), status)
	writeJSON(t, filepath.Join(ralphDir, ".circuit_breaker_state"), model.CircuitBreakerState{State: "CLOSED"})
	writeJSON(t, filepath.Join(ralphDir, "progress.json"), model.Progress{Iteration: 1, CompletedIDs: []string{"setup"}, Status: "in_progress"})
	_ = os.WriteFile(filepath.Join(repoDir, ".ralphrc"), []byte("PROJECT_NAME=\"integration\"\n"), 0644)
	_ = os.WriteFile(filepath.Join(logDir, "ralph.log"), []byte("Starting loop 1\nTask completed\n"), 0644)

	repos, err := discovery.Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	repo := repos[0]
	if repo.Status == nil || repo.Status.Status != "running" {
		t.Fatal("status not loaded")
	}
	if repo.Circuit == nil || repo.Circuit.State != "CLOSED" {
		t.Fatal("circuit breaker not loaded")
	}

	status.LoopCount = 5
	status.Status = "idle"
	writeJSON(t, filepath.Join(ralphDir, "status.json"), status)
	model.RefreshRepo(repo)
	if repo.Status.LoopCount != 5 {
		t.Errorf("loop = %d, want 5", repo.Status.LoopCount)
	}

	lines, err := process.ReadFullLog(repo.Path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(lines) != 2 {
		t.Errorf("log lines = %d, want 2", len(lines))
	}

	repo.Config.Values["NEW_KEY"] = "new_value"
	repo.Config.Save()
	reloaded, _ := model.LoadConfig(repo.Path)
	if reloaded.Values["NEW_KEY"] != "new_value" {
		t.Error("config change not persisted")
	}
}

func TestProcessManagerLifecycle(t *testing.T) {
	mgr := process.NewManager()
	if mgr.IsRunning("/any/path") {
		t.Error("should not be running initially")
	}
	mgr.StopAll()
	if err := mgr.Stop("/nonexistent"); err == nil {
		t.Error("expected error for non-running stop")
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, _ := json.Marshal(v)
	_ = os.WriteFile(path, data, 0644)
}
