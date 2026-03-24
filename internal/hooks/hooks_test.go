package hooks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestLoadConfigNotExist(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	e := NewExecutor(bus)
	// Should not error on missing file
	if err := e.LoadConfig("/nonexistent/path"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoadConfigValid(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
	bus := events.NewBus(100)
	e := NewExecutor(bus)
	e.Start()
	e.Stop()
	// Should not panic
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	// Write garbled YAML content
	_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte("{{{{not: valid: yaml: [[["), 0644)

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	err := e.LoadConfig(dir)
	if err == nil {
		t.Fatal("expected parse error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse hooks config") {
		t.Errorf("error should contain 'parse hooks config', got: %v", err)
	}
}

func TestDispatchHook_RepoPathFiltering(t *testing.T) {
	t.Parallel()

	// Set up repo A with a hook that creates a marker file
	repoA := t.TempDir()
	markerFile := filepath.Join(repoA, "should_not_exist")
	ralphDirA := filepath.Join(repoA, ".ralph")
	_ = os.MkdirAll(ralphDirA, 0755)

	hooksYaml := `hooks:
  session.started:
    - name: marker
      command: "touch ` + markerFile + `"
      sync: true
      timeout: 5
`
	_ = os.WriteFile(filepath.Join(ralphDirA, "hooks.yaml"), []byte(hooksYaml), 0644)

	// Repo B is a different path
	repoB := t.TempDir()

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	if err := e.LoadConfig(repoA); err != nil {
		t.Fatalf("load: %v", err)
	}

	e.Start()
	defer e.Stop()

	// Dispatch event for repo B — hook for repo A should NOT fire
	bus.Publish(events.Event{
		Type:     events.SessionStarted,
		RepoPath: repoB,
		RepoName: "repo-b",
	})

	// Give enough time for hook to potentially run
	time.Sleep(500 * time.Millisecond)

	if _, err := os.Stat(markerFile); err == nil {
		t.Error("marker file exists — hook ran for wrong repo")
	}
}

func TestDispatchHook_EnvironmentVariables(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	envFile := filepath.Join(dir, "env_output")
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	// Hook writes env vars to a file
	hooksYaml := `hooks:
  session.started:
    - name: env-check
      command: "env > ` + envFile + `"
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
		RepoName: "env-test",
	})

	// Wait for hook to execute
	var envData []byte
	for i := 0; i < 20; i++ {
		var err error
		envData, err = os.ReadFile(envFile)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if envData == nil {
		t.Fatal("env file was not created by hook")
	}

	envStr := string(envData)

	// Verify RALPH_EVENT_TYPE is set
	if !strings.Contains(envStr, "RALPH_EVENT_TYPE=session.started") {
		t.Error("RALPH_EVENT_TYPE not found or incorrect in hook environment")
	}

	// Verify RALPH_REPO_PATH is set to the correct directory
	if !strings.Contains(envStr, "RALPH_REPO_PATH="+dir) {
		t.Errorf("RALPH_REPO_PATH not found or incorrect in hook environment, want %s", dir)
	}
}

func TestLoadConfig_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	// Write empty file
	_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(""), 0644)

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	err := e.LoadConfig(dir)
	if err != nil {
		t.Fatalf("empty YAML should not error, got: %v", err)
	}

	// Config should be stored (possibly with nil/empty Hooks map)
	e.mu.RLock()
	cfg, ok := e.configs[dir]
	e.mu.RUnlock()
	if !ok {
		t.Fatal("config not stored for empty file")
	}
	if len(cfg.Hooks) != 0 {
		t.Errorf("expected empty hooks map, got %d entries", len(cfg.Hooks))
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	t.Parallel()

	// Use a temp dir that exists but has no .ralph/hooks.yaml
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)
	// Do NOT create hooks.yaml

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	err := e.LoadConfig(dir)
	if err != nil {
		t.Fatalf("missing hooks.yaml should return nil, got: %v", err)
	}

	// Should not store a config entry
	e.mu.RLock()
	_, ok := e.configs[dir]
	e.mu.RUnlock()
	if ok {
		t.Error("config should not be stored when hooks.yaml is missing")
	}
}
