package v2

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
	"gopkg.in/yaml.v3"
)

// AliasFile represents a user-defined alias configuration file.
type AliasFile struct {
	Version int                  `yaml:"version"`
	Aliases map[string]*AliasDef `yaml:"aliases"`
}

// AliasDef defines a single command alias.
type AliasDef struct {
	Command     string   `yaml:"command"`
	Description string   `yaml:"description"`
	Args        []string `yaml:"args,omitempty"`
}

// DefaultAliasPath returns the default path for the aliases configuration file.
func DefaultAliasPath() string {
	return ralphpath.AliasesYAMLPath()
}

// LoadAliasFile reads and parses an alias file from the given path.
func LoadAliasFile(path string) (*AliasFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read alias file %s: %w", path, err)
	}

	var af AliasFile
	if err := yaml.Unmarshal(data, &af); err != nil {
		return nil, fmt.Errorf("parse alias file %s: %w", path, err)
	}

	if af.Version != 1 {
		return nil, fmt.Errorf("unsupported alias file version: %d", af.Version)
	}

	if af.Aliases == nil {
		af.Aliases = make(map[string]*AliasDef)
	}

	for name, def := range af.Aliases {
		if err := validateAlias(name, def); err != nil {
			return nil, fmt.Errorf("alias %q: %w", name, err)
		}
	}

	return &af, nil
}

// SaveAliasFile writes an alias file to the given path.
func SaveAliasFile(path string, af *AliasFile) error {
	if af.Version == 0 {
		af.Version = 1
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create alias dir: %w", err)
	}

	data, err := yaml.Marshal(af)
	if err != nil {
		return fmt.Errorf("marshal alias file: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write alias file %s: %w", path, err)
	}

	return nil
}

// validateAlias checks a single alias definition for correctness.
func validateAlias(name string, def *AliasDef) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("alias name must not be empty")
	}
	if def == nil {
		return errors.New("alias definition must not be nil")
	}
	if strings.TrimSpace(def.Command) == "" {
		return errors.New("command must not be empty")
	}
	return nil
}

// AliasRegistry manages user-defined command aliases.
type AliasRegistry struct {
	mu      sync.RWMutex
	aliases map[string]*AliasDef
}

// NewAliasRegistry creates an empty alias registry.
func NewAliasRegistry() *AliasRegistry {
	return &AliasRegistry{aliases: make(map[string]*AliasDef)}
}

// Register adds an alias to the registry. Returns an error if validation
// fails or an alias with the same name already exists.
func (r *AliasRegistry) Register(name, command, description string) error {
	def := &AliasDef{
		Command:     command,
		Description: description,
	}
	if err := validateAlias(name, def); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.aliases[name]; exists {
		return fmt.Errorf("alias %q already registered", name)
	}

	// Check for circular alias: the command must not reference the alias name
	// as its first token (direct self-reference).
	parts := strings.Fields(command)
	if len(parts) > 0 && parts[0] == name {
		return fmt.Errorf("alias %q is circular: command references itself", name)
	}

	r.aliases[name] = def
	return nil
}

// Resolve looks up an alias by name. Returns the definition and true if
// found, nil and false otherwise.
func (r *AliasRegistry) Resolve(name string) (*AliasDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, ok := r.aliases[name]
	return def, ok
}

// All returns a copy of all registered aliases.
func (r *AliasRegistry) All() map[string]*AliasDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]*AliasDef, len(r.aliases))
	maps.Copy(out, r.aliases)
	return out
}

// LoadFromFile populates the registry from an alias file on disk.
func (r *AliasRegistry) LoadFromFile(path string) error {
	af, err := LoadAliasFile(path)
	if err != nil {
		return err
	}

	for name, def := range af.Aliases {
		if regErr := r.Register(name, def.Command, def.Description); regErr != nil {
			err = errors.Join(err, regErr)
		}
	}
	return err
}
