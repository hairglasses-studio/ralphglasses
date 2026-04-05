package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeManifest is a test helper that writes a plugin.json manifest into a
// subdirectory of dir, creating the subdirectory if needed.
func writeManifest(t *testing.T, dir, name, version string) string {
	t.Helper()
	subdir := filepath.Join(dir, name)
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := PluginManifest{
		Name:     name,
		Version:  version,
		Protocol: "builtin",
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(subdir, "plugin.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReload_DetectsNewPlugins(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRegistry()
	r.AddPluginDir(dir)

	// Initial reload with no plugins.
	if err := r.Reload(); err != nil {
		t.Fatalf("initial Reload: %v", err)
	}

	// Add a new plugin manifest.
	writeManifest(t, dir, "new-plugin", "1.0")

	var added, removed []string
	r.OnReload(func(a, rm []string) {
		added = append(added, a...)
		removed = append(removed, rm...)
	})

	if err := r.Reload(); err != nil {
		t.Fatalf("Reload after adding plugin: %v", err)
	}

	if len(added) != 1 || added[0] != "new-plugin" {
		t.Errorf("added = %v, want [new-plugin]", added)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want []", removed)
	}
}

func TestReload_DetectsRemovedPlugins(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRegistry()
	r.AddPluginDir(dir)

	// Create a plugin and do initial scan.
	writeManifest(t, dir, "doomed", "1.0")
	if err := r.Reload(); err != nil {
		t.Fatalf("initial Reload: %v", err)
	}

	// Remove the plugin directory.
	if err := os.RemoveAll(filepath.Join(dir, "doomed")); err != nil {
		t.Fatal(err)
	}

	var added, removed []string
	r.OnReload(func(a, rm []string) {
		added = append(added, a...)
		removed = append(removed, rm...)
	})

	if err := r.Reload(); err != nil {
		t.Fatalf("Reload after removing plugin: %v", err)
	}

	if len(added) != 0 {
		t.Errorf("added = %v, want []", added)
	}
	// The removed list should contain the plugin name or path reference.
	if len(removed) != 1 {
		t.Errorf("removed = %v, want 1 entry", removed)
	}
}

func TestReload_CallbackFiresWithCorrectLists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRegistry()
	r.AddPluginDir(dir)

	// Start with two plugins.
	writeManifest(t, dir, "keep", "1.0")
	writeManifest(t, dir, "remove-me", "1.0")
	if err := r.Reload(); err != nil {
		t.Fatalf("initial Reload: %v", err)
	}

	// Remove one, add another.
	os.RemoveAll(filepath.Join(dir, "remove-me"))
	writeManifest(t, dir, "brand-new", "1.0")

	var mu sync.Mutex
	var cbAdded, cbRemoved []string
	r.OnReload(func(a, rm []string) {
		mu.Lock()
		defer mu.Unlock()
		cbAdded = append(cbAdded, a...)
		cbRemoved = append(cbRemoved, rm...)
	})

	if err := r.Reload(); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// brand-new should be added.
	foundNew := false
	for _, n := range cbAdded {
		if n == "brand-new" {
			foundNew = true
		}
	}
	if !foundNew {
		t.Errorf("added = %v, expected to contain 'brand-new'", cbAdded)
	}

	// remove-me should be removed (may show as name or path).
	if len(cbRemoved) == 0 {
		t.Error("removed callback list is empty, expected remove-me entry")
	}
}

func TestReload_UnchangedPluginsNotReloaded(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRegistry()
	r.AddPluginDir(dir)

	// Create a plugin and do initial scan.
	writeManifest(t, dir, "stable", "1.0")
	if err := r.Reload(); err != nil {
		t.Fatalf("initial Reload: %v", err)
	}

	callbackFired := false
	r.OnReload(func(a, rm []string) {
		callbackFired = true
	})

	// Reload without any changes.
	if err := r.Reload(); err != nil {
		t.Fatalf("Reload without changes: %v", err)
	}

	if callbackFired {
		t.Error("OnReload callback fired even though no plugins changed")
	}
}

func TestReload_DetectsModifiedPlugins(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRegistry()
	r.AddPluginDir(dir)

	// Create initial plugin.
	manifestPath := writeManifest(t, dir, "evolving", "1.0")
	if err := r.Reload(); err != nil {
		t.Fatalf("initial Reload: %v", err)
	}

	// Ensure file system time resolution is exceeded.
	time.Sleep(50 * time.Millisecond)

	// Modify the manifest (bump version).
	m := PluginManifest{Name: "evolving", Version: "2.0", Protocol: "builtin"}
	data, _ := json.Marshal(m)
	// Touch with a future mtime to guarantee detection.
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Second)
	os.Chtimes(manifestPath, future, future)

	var added, removed []string
	r.OnReload(func(a, rm []string) {
		added = append(added, a...)
		removed = append(removed, rm...)
	})

	if err := r.Reload(); err != nil {
		t.Fatalf("Reload after modification: %v", err)
	}

	// Modified plugins appear in both added and removed.
	if len(added) != 1 || added[0] != "evolving" {
		t.Errorf("added = %v, want [evolving]", added)
	}
	if len(removed) != 1 || removed[0] != "evolving" {
		t.Errorf("removed = %v, want [evolving]", removed)
	}
}

func TestReload_MultipleCallbacks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	r := NewRegistry()
	r.AddPluginDir(dir)

	// Initial scan.
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}

	var count int
	var mu sync.Mutex
	for range 3 {
		r.OnReload(func(_, _ []string) {
			mu.Lock()
			count++
			mu.Unlock()
		})
	}

	writeManifest(t, dir, "trigger", "1.0")
	if err := r.Reload(); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if count != 3 {
		t.Errorf("callback count = %d, want 3", count)
	}
}

func TestReload_NonExistentDirSkipped(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.AddPluginDir("/nonexistent/path/for/reload/test")

	if err := r.Reload(); err != nil {
		t.Errorf("Reload with nonexistent dir returned error: %v", err)
	}
}
