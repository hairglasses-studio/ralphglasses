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
	for range 20 {
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
	for range 20 {
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

// ---------------------------------------------------------------------------
// Security: Vuln 4 — shell metacharacter rejection in LoadConfig
// ---------------------------------------------------------------------------

func TestLoadConfig_RejectsShellMetacharacters(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		command string
	}{
		{"semicolon", "echo hello; rm -rf /"},
		{"pipe", "echo hello | cat"},
		{"ampersand", "echo hello && rm -rf /"},
		{"backtick", "echo `whoami`"},
		{"dollar-paren", "echo $(id)"},
		{"curly-braces", "echo ${PATH}"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			ralphDir := filepath.Join(dir, ".ralph")
			_ = os.MkdirAll(ralphDir, 0755)

			hooksYaml := `hooks:
  session.started:
    - name: bad-hook
      command: "` + tc.command + `"
`
			_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0644)

			bus := events.NewBus(100)
			e := NewExecutor(bus)
			err := e.LoadConfig(dir)
			if err == nil {
				t.Fatalf("expected error for metacharacter command %q, got nil", tc.command)
			}
			if !strings.Contains(err.Error(), "metacharacters") {
				t.Errorf("error should contain 'metacharacters', got: %v", err)
			}
		})
	}
}

func TestLoadConfig_RejectsEmptyHookName(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	hooksYaml := `hooks:
  session.started:
    - name: ""
      command: "echo hello"
`
	_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0644)

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	err := e.LoadConfig(dir)
	if err == nil {
		t.Fatal("expected error for empty hook name, got nil")
	}
	if !strings.Contains(err.Error(), "empty name") {
		t.Errorf("error should contain 'empty name', got: %v", err)
	}
}

func TestLoadConfig_AllowsCleanCommand(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	hooksYaml := `hooks:
  session.started:
    - name: safe-hook
      command: "echo started"
      sync: true
`
	_ = os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0644)

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	if err := e.LoadConfig(dir); err != nil {
		t.Fatalf("expected clean command to pass, got: %v", err)
	}
}

func TestDispatchHook_ToolCalledPayloadAndEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	payloadFile := filepath.Join(dir, "tool_payload.json")
	envFile := filepath.Join(dir, "tool_env.txt")
	scriptPath := filepath.Join(dir, "capture-tool-hook.sh")
	script := "#!/bin/sh\ncat > \"" + payloadFile + "\"\nenv > \"" + envFile + "\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0o755)
	hooksYaml := "hooks:\n  tool.called:\n    - name: capture\n      command: \"" + scriptPath + "\"\n      sync: true\n      timeout: 5\n"
	if err := os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0o644); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}

	bus := events.NewBus(100)
	e := NewExecutor(bus)
	if err := e.LoadConfig(dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	e.Start()
	defer e.Stop()

	bus.Publish(events.Event{
		Type:     events.ToolCalled,
		RepoPath: dir,
		RepoName: "tool-repo",
		Data: map[string]any{
			"tool":                 "demo_tool",
			"tool_input_json":      `{"repo":"demo","count":2}`,
			"tool_output":          "done",
			"tool_result_is_error": false,
		},
	})

	var payloadData []byte
	var envData []byte
	for range 20 {
		var payloadErr error
		payloadData, payloadErr = os.ReadFile(payloadFile)
		envData, _ = os.ReadFile(envFile)
		if payloadErr == nil && len(envData) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if len(payloadData) == 0 || len(envData) == 0 {
		t.Fatal("tool hook did not capture payload and environment")
	}

	payloadText := string(payloadData)
	if strings.Contains(payloadText, `"hook_event_name":"PostToolUse"`) == false {
		t.Fatalf("missing hook event in payload: %s", payloadText)
	}
	if strings.Contains(payloadText, `"tool_name":"demo_tool"`) == false {
		t.Fatalf("missing tool name in payload: %s", payloadText)
	}
	if strings.Contains(payloadText, `"tool_output":"done"`) == false {
		t.Fatalf("missing tool output in payload: %s", payloadText)
	}

	envText := string(envData)
	if strings.Contains(envText, "HOOK_TOOL_NAME=demo_tool") == false {
		t.Fatalf("missing HOOK_TOOL_NAME in env: %s", envText)
	}
	if strings.Contains(envText, "HOOK_TOOL_OUTPUT=done") == false {
		t.Fatalf("missing HOOK_TOOL_OUTPUT in env: %s", envText)
	}
	if strings.Contains(envText, "HOOK_TOOL_IS_ERROR=0") == false {
		t.Fatalf("missing HOOK_TOOL_IS_ERROR in env: %s", envText)
	}
	if strings.Contains(envText, "RALPH_EVENT_TYPE=tool.called") == false {
		t.Fatalf("missing RALPH_EVENT_TYPE in env: %s", envText)
	}
}

func TestDispatchHook_ToolCalledExitTwoPublishesBlocked(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "deny-tool-hook.sh")
	script := "#!/bin/sh\nprintf 'blocked by hook'\nexit 2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0o755)
	hooksYaml := "hooks:\n  tool.called:\n    - name: deny\n      command: \"" + scriptPath + "\"\n      sync: true\n      timeout: 5\n"
	if err := os.WriteFile(filepath.Join(ralphDir, "hooks.yaml"), []byte(hooksYaml), 0o644); err != nil {
		t.Fatalf("write hooks config: %v", err)
	}

	bus := events.NewBus(100)
	watch := bus.Subscribe("watch")
	e := NewExecutor(bus)
	if err := e.LoadConfig(dir); err != nil {
		t.Fatalf("load: %v", err)
	}
	e.Start()
	defer e.Stop()
	defer bus.Unsubscribe("watch")

	bus.Publish(events.Event{
		Type:     events.ToolCalled,
		RepoPath: dir,
		RepoName: "tool-repo",
		Data: map[string]any{
			"tool":                 "demo_tool",
			"tool_input_json":      `{}`,
			"tool_result_is_error": false,
		},
	})

	deadline := time.After(2 * time.Second)
	for {
		select {
		case evt := <-watch:
			if evt.Type != events.HookBlocked {
				continue
			}
			if evt.Data["hook"] != "deny" {
				t.Fatalf("hook name = %v, want deny", evt.Data["hook"])
			}
			reason, _ := evt.Data["reason"].(string)
			if strings.Contains(reason, "blocked by hook") == false {
				t.Fatalf("reason = %q, want blocked by hook", reason)
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for hook.blocked event")
		}
	}
}
