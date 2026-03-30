// Package builtins provides embedded YAML plugin definitions that ship with ralphglasses.
package builtins

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	v2 "github.com/hairglasses-studio/ralphglasses/internal/plugin/v2"
	"gopkg.in/yaml.v3"
)

//go:embed *.yml
var builtinFS embed.FS

// LoadBuiltins parses all embedded YAML plugin definitions and returns them.
func LoadBuiltins() ([]v2.PluginDef, error) {
	entries, err := builtinFS.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("read embedded builtins: %w", err)
	}

	var plugins []v2.PluginDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}

		data, err := builtinFS.ReadFile(e.Name())
		if err != nil {
			return nil, fmt.Errorf("read builtin %s: %w", e.Name(), err)
		}

		var p v2.PluginDef
		if err := yaml.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("parse builtin %s: %w", e.Name(), err)
		}

		if err := v2.Validate(&p); err != nil {
			return nil, fmt.Errorf("validate builtin %s: %w", e.Name(), err)
		}

		plugins = append(plugins, p)
	}

	return plugins, nil
}

// BuiltinNames returns the names of all available builtin plugins, sorted alphabetically.
func BuiltinNames() []string {
	plugins, err := LoadBuiltins()
	if err != nil {
		return nil
	}

	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name
	}
	sort.Strings(names)
	return names
}
