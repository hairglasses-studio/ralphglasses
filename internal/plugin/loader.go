package plugin

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// LoadDir scans dir for *.so plugin files and returns what would be loaded.
//
// NOTE: Actual Go plugin loading (plugin.Open) is intentionally not implemented here.
// The Go standard library's plugin package has significant limitations:
//   - Plugins must be compiled with the exact same Go version and module graph as the host.
//   - CGO is required, which complicates cross-compilation.
//
// TODO: For production plugin loading, use hashicorp/go-plugin (net/rpc or gRPC-based).
// See: https://github.com/hashicorp/go-plugin
// It provides versioned handshake, process isolation, and cross-language support.
func LoadDir(dir string) ([]Plugin, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin LoadDir %q: %w", dir, err)
	}

	var found []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".so" {
			found = append(found, filepath.Join(dir, e.Name()))
		}
	}

	if len(found) == 0 {
		return nil, nil
	}

	for _, path := range found {
		slog.Info("plugin stub: would load", "path", path)
	}

	// Return empty slice; actual loading requires hashicorp/go-plugin integration.
	return nil, nil
}
