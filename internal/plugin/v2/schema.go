package v2

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// PluginsFile is the top-level structure for ~/.config/ralphglasses/plugins.yml.
type PluginsFile struct {
	Version int         `yaml:"version"`
	Plugins []PluginDef `yaml:"plugins"`
}

// DefaultPluginsPath returns the default path for the user plugins file:
// ~/.config/ralphglasses/plugins.yml
func DefaultPluginsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "ralphglasses", "plugins.yml")
	}
	return filepath.Join(home, ".config", "ralphglasses", "plugins.yml")
}

// LoadPluginsFile reads, parses, and validates a plugins file from path.
func LoadPluginsFile(path string) (*PluginsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugins file %s: %w", path, err)
	}

	var pf PluginsFile
	if err := yaml.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("parse plugins file %s: %w", path, err)
	}

	if err := ValidatePluginsFile(&pf); err != nil {
		return nil, fmt.Errorf("validate plugins file %s: %w", path, err)
	}

	for i := range pf.Plugins {
		applyConfigDefaults(&pf.Plugins[i])
	}

	return &pf, nil
}

// SavePluginsFile marshals a PluginsFile to YAML and writes it to path,
// creating parent directories as needed.
func SavePluginsFile(path string, pf *PluginsFile) error {
	if err := ValidatePluginsFile(pf); err != nil {
		return fmt.Errorf("validate before save: %w", err)
	}

	data, err := yaml.Marshal(pf)
	if err != nil {
		return fmt.Errorf("marshal plugins file: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write plugins file %s: %w", path, err)
	}

	return nil
}

// ValidatePluginsFile checks the top-level structure and all contained plugins.
func ValidatePluginsFile(pf *PluginsFile) error {
	if pf.Version != 1 {
		return fmt.Errorf("unsupported plugins file version %d (want 1)", pf.Version)
	}

	seen := make(map[string]bool)
	var errs []error

	for i := range pf.Plugins {
		p := &pf.Plugins[i]

		if strings.TrimSpace(p.Name) == "" {
			errs = append(errs, fmt.Errorf("plugin[%d]: name is required", i))
			continue
		}

		if seen[p.Name] {
			errs = append(errs, fmt.Errorf("plugin %q: duplicate name", p.Name))
			continue
		}
		seen[p.Name] = true

		if err := Validate(p); err != nil {
			errs = append(errs, fmt.Errorf("plugin %q: %w", p.Name, err))
		}
	}

	return errors.Join(errs...)
}
