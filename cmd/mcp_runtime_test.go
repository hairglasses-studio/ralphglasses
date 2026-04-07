package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestConfigureMCPRuntimeRestoresFullSupervisor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	scanRoot := filepath.Join(home, "hairglasses-studio", "repo")
	if err := os.MkdirAll(filepath.Join(scanRoot, ".ralph"), 0o755); err != nil {
		t.Fatalf("mkdir scan root: %v", err)
	}
	docsRoot := filepath.Join(home, "hairglasses-studio", "docs")
	if err := os.MkdirAll(docsRoot, 0o755); err != nil {
		t.Fatalf("mkdir docs root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsRoot, ".docs.sqlite"), nil, 0o644); err != nil {
		t.Fatalf("seed docs sqlite file: %v", err)
	}
	if err := session.SaveAutonomyLevel(filepath.Join(scanRoot, ".ralph"), int(session.LevelAutoOptimize)); err != nil {
		t.Fatalf("save autonomy level: %v", err)
	}

	bus := events.NewBus(16)
	rg := mcpserver.NewServerWithBus(scanRoot, bus)
	cleanup := configureMCPRuntime(scanRoot, bus, rg)
	defer cleanup()

	if rg.SessMgr == nil {
		t.Fatal("expected session manager to be configured")
	}
	if got := rg.SessMgr.GetAutonomyLevel(); got != session.LevelAutoOptimize {
		t.Fatalf("autonomy level = %v, want %v", got, session.LevelAutoOptimize)
	}

	status := rg.SessMgr.SupervisorStatus()
	if status == nil {
		t.Fatal("expected restored supervisor to be running")
	}
	if !status.ResearchDaemonActive {
		t.Error("expected restored supervisor to have research daemon attached")
	}
	if !status.CrashRecoveryActive {
		t.Error("expected restored supervisor to have crash recovery attached")
	}
}
