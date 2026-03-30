package v2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlugin_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `
name: test-plugin
version: "0.1.0"
description: A test plugin
author: tester
commands:
  - name: greet
    description: Say hello
    run: echo hello
hooks:
  - event: session.start
    command: greet
    priority: 5
config:
  verbose:
    type: bool
    default: false
    description: Enable verbose output
    required: false
`
	path := filepath.Join(dir, "test.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPlugin(path)
	if err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	if p.Name != "test-plugin" {
		t.Errorf("Name = %q, want %q", p.Name, "test-plugin")
	}
	if p.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", p.Version, "0.1.0")
	}
	if len(p.Commands) != 1 {
		t.Fatalf("Commands len = %d, want 1", len(p.Commands))
	}
	if p.Commands[0].Name != "greet" {
		t.Errorf("Command name = %q, want %q", p.Commands[0].Name, "greet")
	}
	if p.Commands[0].Run != "echo hello" {
		t.Errorf("Command run = %q, want %q", p.Commands[0].Run, "echo hello")
	}
	if len(p.Hooks) != 1 {
		t.Fatalf("Hooks len = %d, want 1", len(p.Hooks))
	}
	if p.Hooks[0].Event != "session.start" {
		t.Errorf("Hook event = %q, want %q", p.Hooks[0].Event, "session.start")
	}
	if p.Hooks[0].Priority != 5 {
		t.Errorf("Hook priority = %d, want 5", p.Hooks[0].Priority)
	}
	if cfg, ok := p.Config["verbose"]; !ok {
		t.Error("Config missing 'verbose' key")
	} else if cfg.Type != "bool" {
		t.Errorf("Config verbose type = %q, want %q", cfg.Type, "bool")
	}
}

func TestValidate_MissingName(t *testing.T) {
	p := &PluginDef{Version: "1.0.0"}
	if err := Validate(p); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidate_MissingVersion(t *testing.T) {
	p := &PluginDef{Name: "test"}
	if err := Validate(p); err == nil {
		t.Error("expected error for missing version")
	}
}

func TestValidate_InvalidCommand_MissingRun(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Commands: []CommandDef{
			{Name: "broken"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for command missing run")
	}
}

func TestValidate_InvalidCommand_MissingName(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Commands: []CommandDef{
			{Run: "echo hi"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for command missing name")
	}
}

func TestValidate_DuplicateCommand(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Commands: []CommandDef{
			{Name: "cmd", Run: "echo a"},
			{Name: "cmd", Run: "echo b"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for duplicate command name")
	}
}

func TestValidate_InvalidConfigType(t *testing.T) {
	p := &PluginDef{
		Name:    "test",
		Version: "1.0.0",
		Config: map[string]*ConfigDef{
			"bad": {Type: "complex64"},
		},
	}
	if err := Validate(p); err == nil {
		t.Error("expected error for invalid config type")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()

	for _, name := range []string{"a.yml", "b.yaml"} {
		content := `
name: ` + name + `
version: "1.0.0"
commands:
  - name: run
    run: echo ok
`
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Non-YAML file should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("ignore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	plugins, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(plugins) != 2 {
		t.Errorf("loaded %d plugins, want 2", len(plugins))
	}
}

func TestLoadDir_InvalidFile(t *testing.T) {
	dir := t.TempDir()

	// Invalid YAML plugin (missing name).
	if err := os.WriteFile(filepath.Join(dir, "bad.yml"), []byte("version: 1.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plugins, err := LoadDir(dir)
	if err == nil {
		t.Error("expected error for invalid plugin file")
	}
	if len(plugins) != 0 {
		t.Errorf("loaded %d plugins, want 0", len(plugins))
	}
}

func TestConfigDefaults(t *testing.T) {
	dir := t.TempDir()
	content := `
name: defaults-test
version: "1.0.0"
config:
  setting:
    description: A setting with no explicit type
    default: hello
`
	path := filepath.Join(dir, "plugin.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := LoadPlugin(path)
	if err != nil {
		t.Fatalf("LoadPlugin: %v", err)
	}

	cfg := p.Config["setting"]
	if cfg == nil {
		t.Fatal("config 'setting' not found")
	}
	if cfg.Type != "string" {
		t.Errorf("Type = %q, want %q (default)", cfg.Type, "string")
	}
	if cfg.Default != "hello" {
		t.Errorf("Default = %v, want %q", cfg.Default, "hello")
	}
}

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	p := &PluginDef{Name: "reg-test", Version: "1.0.0"}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Duplicate registration should fail.
	if err := r.Register(p); err == nil {
		t.Error("expected error for duplicate registration")
	}

	got := r.Get("reg-test")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "reg-test" {
		t.Errorf("Get name = %q, want %q", got.Name, "reg-test")
	}

	if r.Get("nonexistent") != nil {
		t.Error("Get should return nil for unknown plugin")
	}

	all := r.All()
	if len(all) != 1 {
		t.Errorf("All len = %d, want 1", len(all))
	}
}

func TestRegistry_LoadDir(t *testing.T) {
	dir := t.TempDir()
	content := `
name: dir-plugin
version: "2.0.0"
commands:
  - name: test
    run: echo test
`
	if err := os.WriteFile(filepath.Join(dir, "plugin.yml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewRegistry()
	if err := r.LoadDir(dir); err != nil {
		t.Fatalf("Registry.LoadDir: %v", err)
	}

	if r.Get("dir-plugin") == nil {
		t.Error("expected plugin to be registered after LoadDir")
	}
}
