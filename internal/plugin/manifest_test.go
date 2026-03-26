package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifest := PluginManifest{
		Name:      "test-plugin",
		Version:   "1.2.3",
		Path:      "bin/plugin",
		Checksum:  "abc123",
		Protocol:  "grpc",
		Handshake: MagicCookieValue,
	}
	data, _ := json.Marshal(manifest)
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest error: %v", err)
	}
	if m.Name != "test-plugin" {
		t.Errorf("Name = %q, want %q", m.Name, "test-plugin")
	}
	if m.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", m.Version, "1.2.3")
	}
	if m.Protocol != "grpc" {
		t.Errorf("Protocol = %q, want %q", m.Protocol, "grpc")
	}
	// Path should be resolved to absolute.
	wantPath := filepath.Join(dir, "bin/plugin")
	if m.Path != wantPath {
		t.Errorf("Path = %q, want %q", m.Path, wantPath)
	}
}

func TestLoadManifest_AbsolutePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifest := PluginManifest{
		Name:     "abs-path",
		Version:  "1.0",
		Path:     "/usr/local/bin/plugin",
		Protocol: "grpc",
	}
	data, _ := json.Marshal(manifest)
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	// Absolute path should remain unchanged.
	if m.Path != "/usr/local/bin/plugin" {
		t.Errorf("Path = %q, want %q", m.Path, "/usr/local/bin/plugin")
	}
}

func TestLoadManifest_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadManifest("/nonexistent/plugin.json")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadManifest_InvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "plugin.json")
	if err := os.WriteFile(path, []byte("{bad json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadManifest(path)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestValidateManifest_Valid(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{
		Name:     "valid",
		Version:  "1.0",
		Protocol: "grpc",
	}
	if err := ValidateManifest(m); err != nil {
		t.Errorf("ValidateManifest error: %v", err)
	}
}

func TestValidateManifest_BuiltinProtocol(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{
		Name:     "valid-builtin",
		Version:  "1.0",
		Protocol: "builtin",
	}
	if err := ValidateManifest(m); err != nil {
		t.Errorf("ValidateManifest error: %v", err)
	}
}

func TestValidateManifest_MissingName(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{Version: "1.0", Protocol: "grpc"}
	if err := ValidateManifest(m); err == nil {
		t.Error("expected error for missing name")
	}
}

func TestValidateManifest_MissingVersion(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{Name: "x", Protocol: "grpc"}
	if err := ValidateManifest(m); err == nil {
		t.Error("expected error for missing version")
	}
}

func TestValidateManifest_MissingProtocol(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{Name: "x", Version: "1.0"}
	if err := ValidateManifest(m); err == nil {
		t.Error("expected error for missing protocol")
	}
}

func TestValidateManifest_InvalidProtocol(t *testing.T) {
	t.Parallel()

	m := &PluginManifest{Name: "x", Version: "1.0", Protocol: "http"}
	if err := ValidateManifest(m); err == nil {
		t.Error("expected error for invalid protocol")
	}
}

func TestValidateManifest_ChecksumMatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binPath := filepath.Join(dir, "plugin-bin")
	content := []byte("fake plugin binary content")
	if err := os.WriteFile(binPath, content, 0o755); err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(content)
	checksum := hex.EncodeToString(h[:])

	m := &PluginManifest{
		Name:     "checksum-test",
		Version:  "1.0",
		Path:     binPath,
		Checksum: checksum,
		Protocol: "grpc",
	}

	if err := ValidateManifest(m); err != nil {
		t.Errorf("ValidateManifest error: %v", err)
	}
}

func TestValidateManifest_ChecksumMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	binPath := filepath.Join(dir, "plugin-bin")
	if err := os.WriteFile(binPath, []byte("content"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := &PluginManifest{
		Name:     "bad-checksum",
		Version:  "1.0",
		Path:     binPath,
		Checksum: "0000000000000000000000000000000000000000000000000000000000000000",
		Protocol: "grpc",
	}

	if err := ValidateManifest(m); err == nil {
		t.Error("expected error for checksum mismatch")
	}
}

func TestScanPluginDir_Empty(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifests, err := ScanPluginDir(dir)
	if err != nil {
		t.Fatalf("ScanPluginDir error: %v", err)
	}
	if manifests != nil {
		t.Errorf("ScanPluginDir returned %v, want nil", manifests)
	}
}

func TestScanPluginDir_NonExistent(t *testing.T) {
	t.Parallel()

	manifests, err := ScanPluginDir("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanPluginDir error: %v", err)
	}
	if manifests != nil {
		t.Errorf("ScanPluginDir returned %v, want nil", manifests)
	}
}

func TestScanPluginDir_FindsManifests(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create two plugin subdirectories with manifests.
	for _, name := range []string{"plugin-a", "plugin-b"} {
		pluginDir := filepath.Join(dir, name)
		if err := os.Mkdir(pluginDir, 0o755); err != nil {
			t.Fatal(err)
		}
		m := PluginManifest{
			Name:     name,
			Version:  "1.0",
			Protocol: "grpc",
		}
		data, _ := json.Marshal(m)
		if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Also create a non-plugin directory (no plugin.json).
	if err := os.Mkdir(filepath.Join(dir, "not-a-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}

	// And a regular file at top level.
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	manifests, err := ScanPluginDir(dir)
	if err != nil {
		t.Fatalf("ScanPluginDir error: %v", err)
	}
	if len(manifests) != 2 {
		t.Fatalf("ScanPluginDir found %d manifests, want 2", len(manifests))
	}

	names := map[string]bool{}
	for _, m := range manifests {
		names[m.Name] = true
	}
	if !names["plugin-a"] || !names["plugin-b"] {
		t.Errorf("expected plugin-a and plugin-b, got %v", names)
	}
}

func TestScanPluginDir_SkipsInvalid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Valid plugin.
	validDir := filepath.Join(dir, "valid")
	if err := os.Mkdir(validDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(PluginManifest{Name: "valid", Version: "1.0", Protocol: "grpc"})
	if err := os.WriteFile(filepath.Join(validDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Invalid plugin (missing required fields).
	invalidDir := filepath.Join(dir, "invalid")
	if err := os.Mkdir(invalidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	badData, _ := json.Marshal(PluginManifest{Name: "invalid"}) // missing version and protocol
	if err := os.WriteFile(filepath.Join(invalidDir, "plugin.json"), badData, 0o644); err != nil {
		t.Fatal(err)
	}

	manifests, err := ScanPluginDir(dir)
	if err != nil {
		t.Fatalf("ScanPluginDir error: %v", err)
	}
	if len(manifests) != 1 {
		t.Fatalf("ScanPluginDir found %d manifests, want 1 (invalid should be skipped)", len(manifests))
	}
	if manifests[0].Name != "valid" {
		t.Errorf("expected valid plugin, got %q", manifests[0].Name)
	}
}
