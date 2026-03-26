package plugin

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// LoadDir scans dir for plugin manifests and legacy .so files.
//
// For each subdirectory containing a plugin.json, the manifest is loaded and
// validated. Valid manifests are returned so the caller can decide how to
// initialize the plugin (builtin registration, gRPC client launch, etc.).
//
// Legacy .so files at the top level are still detected and logged but not
// loaded. Actual gRPC plugin execution requires hashicorp/go-plugin wiring
// which is a follow-up integration.
func LoadDir(dir string) ([]Plugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin LoadDir %q: %w", dir, err)
	}

	// Log any legacy .so files found at the top level.
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".so" {
			slog.Info("plugin stub: legacy .so file found (use manifest-based plugins instead)",
				"path", filepath.Join(dir, e.Name()))
		}
	}

	// Scan for manifest-based plugins.
	manifests, err := ScanPluginDir(dir)
	if err != nil {
		return nil, fmt.Errorf("plugin LoadDir scan manifests: %w", err)
	}

	for _, m := range manifests {
		slog.Info("plugin manifest discovered",
			"name", m.Name,
			"version", m.Version,
			"protocol", m.Protocol,
			"path", m.Path,
		)
	}

	// Return nil — actual plugin instantiation requires either:
	//   - "builtin" protocol: lookup in a builtin registry (handled by caller)
	//   - "grpc" protocol: hashicorp/go-plugin client launch (future)
	return nil, nil
}

// LoadDirManifests is like LoadDir but returns the raw manifests instead of
// instantiated Plugin values. This is useful for inspection and for callers
// that handle plugin instantiation themselves.
func LoadDirManifests(dir string) ([]*PluginManifest, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	return ScanPluginDir(dir)
}
