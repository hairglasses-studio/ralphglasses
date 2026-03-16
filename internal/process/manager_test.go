package process

import (
	"os"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.procs == nil {
		t.Fatal("procs map not initialized")
	}
}

func TestManager_IsRunning_Empty(t *testing.T) {
	m := NewManager()
	if m.IsRunning("/some/path") {
		t.Error("IsRunning should return false for unknown path")
	}
}

func TestManager_IsPaused_Empty(t *testing.T) {
	m := NewManager()
	if m.IsPaused("/some/path") {
		t.Error("IsPaused should return false for unknown path")
	}
}

func TestManager_RunningPaths_Empty(t *testing.T) {
	m := NewManager()
	paths := m.RunningPaths()
	if len(paths) != 0 {
		t.Errorf("RunningPaths should be empty, got %d", len(paths))
	}
}

func TestManager_StopAll_Empty(t *testing.T) {
	m := NewManager()
	// Should not panic
	m.StopAll()
}

func TestManager_Stop_NotRunning(t *testing.T) {
	m := NewManager()
	err := m.Stop("/not/running")
	if err == nil {
		t.Fatal("expected error when stopping non-running process")
	}
}

func TestManager_TogglePause_NotRunning(t *testing.T) {
	m := NewManager()
	_, err := m.TogglePause("/not/running")
	if err == nil {
		t.Fatal("expected error when pausing non-running process")
	}
}

func writeTestScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/bash\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestManager_Start_DuplicateReturnsError(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	err := m.Start(repoPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.StopAll()

	err = m.Start(repoPath)
	if err == nil {
		t.Fatal("expected error when starting duplicate process")
	}
}

func TestManager_StartStopLifecycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	err := m.Start(repoPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !m.IsRunning(repoPath) {
		t.Error("expected IsRunning to be true after Start")
	}

	paths := m.RunningPaths()
	if len(paths) != 1 {
		t.Errorf("expected 1 running path, got %d", len(paths))
	}

	err = m.Stop(repoPath)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the goroutine time to clean up
	time.Sleep(200 * time.Millisecond)

	if m.IsRunning(repoPath) {
		t.Error("expected IsRunning to be false after Stop")
	}
}

func TestManager_StopAll_StopsRunning(t *testing.T) {
	m := NewManager()

	repo1 := t.TempDir()
	repo2 := t.TempDir()

	writeTestScript(t, repo1+"/ralph_loop.sh", "sleep 60")
	writeTestScript(t, repo2+"/ralph_loop.sh", "sleep 60")

	if err := m.Start(repo1); err != nil {
		t.Fatalf("Start repo1: %v", err)
	}
	if err := m.Start(repo2); err != nil {
		t.Fatalf("Start repo2: %v", err)
	}

	if len(m.RunningPaths()) != 2 {
		t.Fatalf("expected 2 running, got %d", len(m.RunningPaths()))
	}

	m.StopAll()

	if len(m.RunningPaths()) != 0 {
		t.Errorf("expected 0 running after StopAll, got %d", len(m.RunningPaths()))
	}
}

func TestManager_TogglePause(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	if err := m.Start(repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.StopAll()

	if m.IsPaused(repoPath) {
		t.Error("should not be paused initially")
	}

	paused, err := m.TogglePause(repoPath)
	if err != nil {
		t.Fatalf("TogglePause (pause): %v", err)
	}
	if !paused {
		t.Error("expected paused=true after first toggle")
	}
	if !m.IsPaused(repoPath) {
		t.Error("IsPaused should return true")
	}

	paused, err = m.TogglePause(repoPath)
	if err != nil {
		t.Fatalf("TogglePause (resume): %v", err)
	}
	if paused {
		t.Error("expected paused=false after second toggle")
	}
	if m.IsPaused(repoPath) {
		t.Error("IsPaused should return false after resume")
	}
}
