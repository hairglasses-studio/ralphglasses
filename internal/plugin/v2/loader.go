// Package v2 provides YAML-based plugin definitions for ralphglasses.
package v2

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// PluginDef describes a v2 plugin loaded from YAML.
type PluginDef struct {
	Name        string                `yaml:"name"`
	Version     string                `yaml:"version"`
	Description string                `yaml:"description"`
	Author      string                `yaml:"author"`
	Commands    []CommandDef          `yaml:"commands"`
	Hooks       []HookDef             `yaml:"hooks"`
	Config      map[string]*ConfigDef `yaml:"config"`
	Keybinds    []KeybindDef          `yaml:"keybinds"`
}

// CommandDef defines a command exposed by a plugin.
type CommandDef struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Run         string `yaml:"run"` // shell command or Go func name
}

// HookDef defines an event hook.
type HookDef struct {
	Event    string `yaml:"event"`
	Command  string `yaml:"command"`
	Priority int    `yaml:"priority"`
}

// ConfigDef defines a configuration value accepted by a plugin.
type ConfigDef struct {
	Type        string `yaml:"type"`
	Default     any    `yaml:"default"`
	Description string `yaml:"description"`
	Required    bool   `yaml:"required"`
}

// LoadPlugin reads and parses a single YAML plugin definition from path.
func LoadPlugin(path string) (*PluginDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plugin %s: %w", path, err)
	}

	var p PluginDef
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse plugin %s: %w", path, err)
	}

	if err := Validate(&p); err != nil {
		return nil, fmt.Errorf("validate plugin %s: %w", path, err)
	}

	applyConfigDefaults(&p)
	return &p, nil
}

// LoadDir loads all .yml and .yaml plugin files from dir.
func LoadDir(dir string) ([]*PluginDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read plugin dir %s: %w", dir, err)
	}

	var plugins []*PluginDef
	var errs []error

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yml" && ext != ".yaml" {
			continue
		}
		p, err := LoadPlugin(filepath.Join(dir, e.Name()))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		plugins = append(plugins, p)
	}

	if len(errs) > 0 {
		return plugins, errors.Join(errs...)
	}
	return plugins, nil
}

// Validate checks a PluginDef for required fields and consistency.
func Validate(p *PluginDef) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("plugin name is required")
	}
	if strings.TrimSpace(p.Version) == "" {
		return errors.New("plugin version is required")
	}

	seen := make(map[string]bool)
	for i, cmd := range p.Commands {
		if strings.TrimSpace(cmd.Name) == "" {
			return fmt.Errorf("command[%d]: name is required", i)
		}
		if strings.TrimSpace(cmd.Run) == "" {
			return fmt.Errorf("command %q: run is required", cmd.Name)
		}
		if seen[cmd.Name] {
			return fmt.Errorf("command %q: duplicate name", cmd.Name)
		}
		seen[cmd.Name] = true
	}

	for i, hook := range p.Hooks {
		if strings.TrimSpace(hook.Event) == "" {
			return fmt.Errorf("hook[%d]: event is required", i)
		}
		if strings.TrimSpace(hook.Command) == "" {
			return fmt.Errorf("hook[%d]: command is required", i)
		}
	}

	seenBinds := make(map[string]bool)
	for i, kb := range p.Keybinds {
		if strings.TrimSpace(kb.Key) == "" {
			return fmt.Errorf("keybind[%d]: key is required", i)
		}
		if strings.TrimSpace(kb.Scope) == "" {
			return fmt.Errorf("keybind[%d]: scope is required", i)
		}
		if strings.TrimSpace(kb.Action) == "" {
			return fmt.Errorf("keybind[%d]: action is required", i)
		}
		bk := kb.Scope + "\x00" + kb.Key
		if seenBinds[bk] {
			return fmt.Errorf("keybind[%d]: duplicate key %q in scope %q", i, kb.Key, kb.Scope)
		}
		seenBinds[bk] = true
	}

	validTypes := map[string]bool{
		"string": true, "int": true, "float": true, "bool": true,
	}
	for key, cfg := range p.Config {
		if cfg.Type != "" && !validTypes[cfg.Type] {
			return fmt.Errorf("config %q: invalid type %q (want string|int|float|bool)", key, cfg.Type)
		}
	}

	return nil
}

// applyConfigDefaults fills in zero-valued config entries with their defaults.
func applyConfigDefaults(p *PluginDef) {
	for _, cfg := range p.Config {
		if cfg.Type == "" {
			cfg.Type = "string"
		}
	}
}

// Registry manages loaded v2 plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*PluginDef
}

// NewRegistry creates an empty plugin registry.
func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]*PluginDef)}
}

// Register adds a plugin to the registry. Returns an error if a plugin
// with the same name is already registered.
func (r *Registry) Register(p *PluginDef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[p.Name]; exists {
		return fmt.Errorf("plugin %q already registered", p.Name)
	}
	r.plugins[p.Name] = p
	return nil
}

// Get returns a plugin by name, or nil if not found.
func (r *Registry) Get(name string) *PluginDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.plugins[name]
}

// All returns all registered plugins.
func (r *Registry) All() []*PluginDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*PluginDef, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	return out
}

// LoadDir loads all plugins from a directory into the registry.
func (r *Registry) LoadDir(dir string) error {
	plugins, err := LoadDir(dir)
	for _, p := range plugins {
		if regErr := r.Register(p); regErr != nil {
			err = errors.Join(err, regErr)
		}
	}
	return err
}
