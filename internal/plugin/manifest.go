package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// PluginManifest describes a plugin binary on disk.
// Each plugin directory contains a plugin.json with these fields.
type PluginManifest struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Path     string `json:"path"`      // path to binary (relative to manifest dir or absolute)
	Checksum string `json:"checksum"`  // SHA-256 hex digest of binary
	Protocol string `json:"protocol"`  // "grpc" or "builtin"
	Handshake string `json:"handshake"` // magic cookie value
}

// LoadManifest reads a plugin.json manifest from the given path.
func LoadManifest(path string) (*PluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}

	var m PluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}

	// Resolve relative binary path against the manifest's directory.
	if m.Path != "" && !filepath.IsAbs(m.Path) {
		m.Path = filepath.Join(filepath.Dir(path), m.Path)
	}

	return &m, nil
}

// ValidateManifest checks that required fields are present and, if the binary
// exists on disk, verifies its SHA-256 checksum matches.
func ValidateManifest(m *PluginManifest) error {
	if m.Name == "" {
		return fmt.Errorf("manifest missing required field: name")
	}
	if m.Version == "" {
		return fmt.Errorf("manifest missing required field: version")
	}
	if m.Protocol == "" {
		return fmt.Errorf("manifest missing required field: protocol")
	}
	if m.Protocol != "grpc" && m.Protocol != "builtin" {
		return fmt.Errorf("manifest protocol must be %q or %q, got %q", "grpc", "builtin", m.Protocol)
	}

	// If the binary path is set and exists, verify checksum.
	if m.Path != "" && m.Checksum != "" {
		if _, err := os.Stat(m.Path); err == nil {
			actual, err := fileSHA256(m.Path)
			if err != nil {
				return fmt.Errorf("checksum binary %s: %w", m.Path, err)
			}
			if actual != m.Checksum {
				return fmt.Errorf("checksum mismatch for %s: got %s, want %s", m.Path, actual, m.Checksum)
			}
		}
	}

	return nil
}

// ScanPluginDir scans a directory for plugin.json manifests.
// It looks one level deep: each subdirectory should contain a plugin.json.
// Manifests that fail to load or validate are logged and skipped.
func ScanPluginDir(dir string) ([]*PluginManifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan plugin dir %s: %w", dir, err)
	}

	var manifests []*PluginManifest
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, e.Name(), "plugin.json")
		m, err := LoadManifest(manifestPath)
		if err != nil {
			continue // skip directories without valid manifests
		}
		if err := ValidateManifest(m); err != nil {
			continue // skip invalid manifests
		}
		manifests = append(manifests, m)
	}

	return manifests, nil
}

// fileSHA256 computes the hex-encoded SHA-256 digest of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
