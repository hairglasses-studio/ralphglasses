package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDirManifests_NonexistentDir(t *testing.T) {
	t.Parallel()
	manifests, err := LoadDirManifests("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got %v", err)
	}
	if manifests != nil {
		t.Errorf("expected nil manifests for nonexistent dir, got %v", manifests)
	}
}

func TestLoadDirManifests_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	manifests, err := LoadDirManifests(dir)
	if err != nil {
		t.Fatalf("LoadDirManifests empty dir: %v", err)
	}
	if len(manifests) != 0 {
		t.Errorf("expected 0 manifests from empty dir, got %d", len(manifests))
	}
}

func TestLoadDirManifests_WithManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a plugin subdirectory with a plugin.json manifest.
	pluginDir := filepath.Join(dir, "my-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := PluginManifest{
		Name:     "my-plugin",
		Version:  "1.0.0",
		Path:     "./my-plugin",
		Protocol: "grpc",
	}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	manifests, err := LoadDirManifests(dir)
	if err != nil {
		t.Fatalf("LoadDirManifests: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(manifests))
	}
	if manifests[0].Name != "my-plugin" {
		t.Errorf("manifest name = %q, want %q", manifests[0].Name, "my-plugin")
	}
	if manifests[0].Version != "1.0.0" {
		t.Errorf("manifest version = %q, want %q", manifests[0].Version, "1.0.0")
	}
}

func TestLoadDirManifests_MultiplePlugins(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	for _, name := range []string{"plugin-a", "plugin-b"} {
		pDir := filepath.Join(dir, name)
		if err := os.MkdirAll(pDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := PluginManifest{Name: name, Version: "1.0.0", Protocol: "builtin"}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(pDir, "plugin.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manifests, err := LoadDirManifests(dir)
	if err != nil {
		t.Fatalf("LoadDirManifests: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("expected 2 manifests, got %d", len(manifests))
	}
}
