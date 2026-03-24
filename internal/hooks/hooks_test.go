package hooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestLoadConfigNotExist(t *testing.T) {
	bus := events.NewBus(100)
	e := NewExecutor(bus)
	// Should not error on missing file
	if err := e.LoadConfig("/nonexistent/path"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadConfigValid(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	hooksYaml := `hooks:
  session.started:
    - name: notify
      command: "echo started"
      sync: false
`
	_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0644)

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	if err := e.LoadConfig(dir); err != nil {
		t.Fatalf("load config: %v", err)
	}

	e.mu.RLock()
	cfg, ok := e.configs[dir]
	e.mu.RUnlock()
	if !ok {
		t.Fatal("config not loaded")
	}
	hooks := cfg.Hooks[events.SessionStarted]
	if len(hooks) != 1 {
		t.Fatalf("hooks count = %d, want 1", len(hooks))
	}
	if hooks[0].Name != "notify" {
		t.Errorf("hook name = %q, want notify", hooks[0].Name)
	}
}

func TestDispatchHook(t *testing.T) {
	dir := t.TempDir()
	markerFile := filepath.Join(dir, "hook_ran")
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	hooksYaml := `hooks:
  session.started:
    - name: marker
      command: "touch ` + markerFile + `"
      sync: true
      timeout: 5
`
	_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0644)

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	if err := e.LoadConfig(dir); err != nil {
		t.Fatalf("load: %v", err)
	}

	e.Start()
	defer e.Stop()

	bus.Publish(events.Event{
		Type:     events.SessionStarted,
		RepoPath: dir,
		RepoName: "test",
	})

	// Wait for hook to execute
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(markerFile); err == nil {
			return // success
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Error("hook did not create marker file")
}

func TestStartStop(t *testing.T) {
	bus := events.NewBus(100)
	e := NewExecutor(bus)
	e.Start()
	e.Stop()
	// Should not panic
}
