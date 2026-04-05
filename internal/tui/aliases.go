package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// builtinCommandAliases are pre-defined aliases that ship with ralphglasses.
// Users cannot overwrite these via Set.
var builtinCommandAliases = map[string]string{
	"q": "quit",
	"h": "help",
	"s": "status",
}

// builtinCommands is the set of command names that cannot be used as alias keys
// (because they would shadow real commands).
var builtinCommands = map[string]bool{
	"quit":     true,
	"help":     true,
	"status":   true,
	"start":    true,
	"stop":     true,
	"scan":     true,
	"repos":    true,
	"sessions": true,
	"teams":    true,
	"fleet":    true,
	"config":   true,
	"logs":     true,
}

// AliasStore manages user-defined command aliases, persisted as JSON.
type AliasStore struct {
	mu      sync.RWMutex
	path    string
	aliases map[string]string
}

// DefaultAliasPath returns the default path for the aliases JSON file,
// respecting XDG_CONFIG_HOME.
func DefaultAliasPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "ralphglasses", "aliases.json")
}

// NewAliasStore loads aliases from the given JSON file path.
// If the file does not exist, an empty store is created (the file
// will be written on the first Save call).
func NewAliasStore(path string) (*AliasStore, error) {
	s := &AliasStore{
		path:    path,
		aliases: make(map[string]string),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("read aliases: %w", err)
	}
	if len(data) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(data, &s.aliases); err != nil {
		return nil, fmt.Errorf("parse aliases: %w", err)
	}
	return s, nil
}

// Set defines or updates a user alias. It returns an error if the alias
// key conflicts with a built-in command name or a built-in alias.
func (s *AliasStore) Set(alias, command string) error {
	alias = strings.TrimSpace(alias)
	command = strings.TrimSpace(command)
	if alias == "" {
		return errors.New("alias must not be empty")
	}
	if command == "" {
		return errors.New("command must not be empty")
	}
	if _, ok := builtinCommandAliases[alias]; ok {
		return fmt.Errorf("cannot redefine built-in alias %q", alias)
	}
	if builtinCommands[alias] {
		return fmt.Errorf("cannot alias built-in command %q", alias)
	}
	s.mu.Lock()
	s.aliases[alias] = command
	s.mu.Unlock()
	return nil
}

// Get returns the expanded command for the given alias.
// It checks user aliases first, then built-in aliases.
func (s *AliasStore) Get(alias string) (string, bool) {
	s.mu.RLock()
	cmd, ok := s.aliases[alias]
	s.mu.RUnlock()
	if ok {
		return cmd, true
	}
	cmd, ok = builtinCommandAliases[alias]
	return cmd, ok
}

// Delete removes a user-defined alias. Built-in aliases cannot be deleted.
func (s *AliasStore) Delete(alias string) error {
	if _, ok := builtinCommandAliases[alias]; ok {
		return fmt.Errorf("cannot delete built-in alias %q", alias)
	}
	s.mu.Lock()
	delete(s.aliases, alias)
	s.mu.Unlock()
	return nil
}

// List returns a combined map of all aliases (user + built-in).
// User aliases override built-in aliases with the same key (though Set
// prevents that today).
func (s *AliasStore) List() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := make(map[string]string, len(builtinCommandAliases)+len(s.aliases))
	maps.Copy(m, builtinCommandAliases)
	maps.Copy(m, s.aliases)
	return m
}

// Resolve expands the first word of input if it matches an alias.
// The remainder of the input is preserved. If no alias matches, the
// original input is returned unchanged.
func (s *AliasStore) Resolve(input string) string {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return ""
	}
	first := fields[0]

	if expanded, ok := s.Get(first); ok {
		if len(fields) > 1 {
			return expanded + " " + strings.Join(fields[1:], " ")
		}
		return expanded
	}
	return strings.Join(fields, " ")
}

// Save persists the user aliases to the JSON file, creating parent
// directories as needed. Built-in aliases are not written to disk.
func (s *AliasStore) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.aliases, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("marshal aliases: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create alias dir: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write aliases: %w", err)
	}
	return nil
}
