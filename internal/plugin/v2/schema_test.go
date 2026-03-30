package v2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPluginsPath(t *testing.T) {
	p := DefaultPluginsPath()
	if !strings.HasSuffix(p, filepath.Join(".config", "ralphglasses", "plugins.yml")) {
		t.Errorf("DefaultPluginsPath() = %q, want suffix .config/ralphglasses/plugins.yml", p)
	}
}

func TestLoadPluginsFile_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 1
plugins:
  - name: alpha
    version: "1.0.0"
    description: First plugin
    commands:
      - name: run
        run: echo alpha
  - name: beta
    version: "2.0.0"
    description: Second plugin
    hooks:
      - event: session.start
        command: init
        priority: 1
`
	path := filepath.Join(dir, "plugins.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	pf, err := LoadPluginsFile(path)
	if err != nil {
		t.Fatalf("LoadPluginsFile: %v", err)
	}

	if pf.Version != 1 {
		t.Errorf("Version = %d, want 1", pf.Version)
	}
	if len(pf.Plugins) != 2 {
		t.Fatalf("Plugins len = %d, want 2", len(pf.Plugins))
	}
	if pf.Plugins[0].Name != "alpha" {
		t.Errorf("Plugins[0].Name = %q, want %q", pf.Plugins[0].Name, "alpha")
	}
	if pf.Plugins[1].Name != "beta" {
		t.Errorf("Plugins[1].Name = %q, want %q", pf.Plugins[1].Name, "beta")
	}
}

func TestLoadPluginsFile_InvalidVersion(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 99
plugins: []
`
	path := filepath.Join(dir, "plugins.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPluginsFile(path)
	if err == nil {
		t.Error("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported plugins file version") {
		t.Errorf("error = %q, want mention of unsupported version", err.Error())
	}
}

func TestLoadPluginsFile_DuplicatePluginNames(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 1
plugins:
  - name: dupe
    version: "1.0.0"
  - name: dupe
    version: "2.0.0"
`
	path := filepath.Join(dir, "plugins.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPluginsFile(path)
	if err == nil {
		t.Error("expected error for duplicate plugin names")
	}
	if !strings.Contains(err.Error(), "duplicate name") {
		t.Errorf("error = %q, want mention of duplicate name", err.Error())
	}
}

func TestLoadPluginsFile_EmptyPluginName(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 1
plugins:
  - name: ""
    version: "1.0.0"
`
	path := filepath.Join(dir, "plugins.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPluginsFile(path)
	if err == nil {
		t.Error("expected error for empty plugin name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want mention of name required", err.Error())
	}
}

func TestLoadPluginsFile_InvalidCommand(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 1
plugins:
  - name: bad-cmd
    version: "1.0.0"
    commands:
      - name: ""
        run: echo hi
`
	path := filepath.Join(dir, "plugins.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPluginsFile(path)
	if err == nil {
		t.Error("expected error for empty command name")
	}
}

func TestLoadPluginsFile_FileNotFound(t *testing.T) {
	_, err := LoadPluginsFile("/nonexistent/plugins.yml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSavePluginsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "plugins.yml")

	pf := &PluginsFile{
		Version: 1,
		Plugins: []PluginDef{
			{
				Name:    "saved-plugin",
				Version: "1.0.0",
				Commands: []CommandDef{
					{Name: "hello", Run: "echo hello"},
				},
			},
		},
	}

	if err := SavePluginsFile(path, pf); err != nil {
		t.Fatalf("SavePluginsFile: %v", err)
	}

	// Round-trip: load back what we saved.
	loaded, err := LoadPluginsFile(path)
	if err != nil {
		t.Fatalf("LoadPluginsFile after save: %v", err)
	}

	if len(loaded.Plugins) != 1 {
		t.Fatalf("Plugins len = %d, want 1", len(loaded.Plugins))
	}
	if loaded.Plugins[0].Name != "saved-plugin" {
		t.Errorf("Name = %q, want %q", loaded.Plugins[0].Name, "saved-plugin")
	}
}

func TestSavePluginsFile_RejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")

	pf := &PluginsFile{
		Version: 0, // invalid
		Plugins: nil,
	}

	if err := SavePluginsFile(path, pf); err == nil {
		t.Error("expected error for invalid PluginsFile on save")
	}
}

func TestSaveAndLoad_WithConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plugins.yml")

	pf := &PluginsFile{
		Version: 1,
		Plugins: []PluginDef{
			{
				Name:    "config-test",
				Version: "1.0.0",
				Config: map[string]*ConfigDef{
					"timeout": {
						Type:        "int",
						Default:     30,
						Description: "Timeout in seconds",
						Required:    true,
					},
				},
			},
		},
	}

	if err := SavePluginsFile(path, pf); err != nil {
		t.Fatalf("SavePluginsFile: %v", err)
	}

	loaded, err := LoadPluginsFile(path)
	if err != nil {
		t.Fatalf("LoadPluginsFile: %v", err)
	}

	cfg := loaded.Plugins[0].Config["timeout"]
	if cfg == nil {
		t.Fatal("config 'timeout' not found")
	}
	if cfg.Type != "int" {
		t.Errorf("Type = %q, want %q", cfg.Type, "int")
	}
	if !cfg.Required {
		t.Error("Required = false, want true")
	}
}

func TestValidatePluginsFile_EmptyPlugins(t *testing.T) {
	pf := &PluginsFile{
		Version: 1,
		Plugins: nil,
	}
	if err := ValidatePluginsFile(pf); err != nil {
		t.Errorf("expected no error for empty plugins list, got: %v", err)
	}
}
