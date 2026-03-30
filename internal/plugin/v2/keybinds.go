// Package v2 provides YAML-based plugin definitions for ralphglasses.
package v2

import (
	"fmt"
	"sort"
	"sync"
)

// KeybindDef describes a keybind contributed by a plugin.
type KeybindDef struct {
	Key    string `yaml:"key"`    // e.g. "ctrl+p", "g"
	Scope  string `yaml:"scope"`  // e.g. "global", "overview", "detail"
	Action string `yaml:"action"` // command name or shell command
}

// KeybindEntry is a resolved keybind with its originating plugin name.
type KeybindEntry struct {
	Key    string
	Scope  string
	Action string
	Plugin string // plugin that registered this keybind
}

// KeybindRegistry maps scope+key pairs to actions. It is safe for
// concurrent use.
type KeybindRegistry struct {
	mu       sync.RWMutex
	bindings map[string]KeybindEntry // key: "scope\x00key"
}

// NewKeybindRegistry creates an empty keybind registry.
func NewKeybindRegistry() *KeybindRegistry {
	return &KeybindRegistry{bindings: make(map[string]KeybindEntry)}
}

func bindingKey(scope, key string) string {
	return scope + "\x00" + key
}

// Register adds a keybind for the given scope and key. Returns an error
// if the same scope+key combination is already registered (duplicate
// detection).
func (r *KeybindRegistry) Register(scope, key, action, plugin string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	bk := bindingKey(scope, key)
	if existing, ok := r.bindings[bk]; ok {
		return fmt.Errorf("keybind %q in scope %q already registered by plugin %q", key, scope, existing.Plugin)
	}
	r.bindings[bk] = KeybindEntry{
		Key:    key,
		Scope:  scope,
		Action: action,
		Plugin: plugin,
	}
	return nil
}

// Lookup returns the action for a scope+key pair, or ("", false) if none.
func (r *KeybindRegistry) Lookup(scope, key string) (action string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.bindings[bindingKey(scope, key)]
	if !exists {
		return "", false
	}
	return entry.Action, true
}

// AllForScope returns all keybind entries registered for the given scope,
// sorted by key for deterministic output.
func (r *KeybindRegistry) AllForScope(scope string) []KeybindEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var entries []KeybindEntry
	prefix := scope + "\x00"
	for bk, entry := range r.bindings {
		if len(bk) > len(prefix) && bk[:len(prefix)] == prefix {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})
	return entries
}

// All returns every registered keybind entry, sorted by scope then key.
func (r *KeybindRegistry) All() []KeybindEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entries := make([]KeybindEntry, 0, len(r.bindings))
	for _, entry := range r.bindings {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Scope != entries[j].Scope {
			return entries[i].Scope < entries[j].Scope
		}
		return entries[i].Key < entries[j].Key
	})
	return entries
}

// RegisterKeybinds registers all keybinds from the given plugins into
// the registry. It returns an error if any duplicate scope+key is detected.
func RegisterKeybinds(reg *KeybindRegistry, plugins []*PluginDef) error {
	var errs []error
	for _, p := range plugins {
		for _, kb := range p.Keybinds {
			if err := reg.Register(kb.Scope, kb.Key, kb.Action, p.Name); err != nil {
				errs = append(errs, fmt.Errorf("plugin %q: %w", p.Name, err))
			}
		}
	}
	if len(errs) > 0 {
		// Combine all errors.
		combined := errs[0]
		for _, e := range errs[1:] {
			combined = fmt.Errorf("%w; %v", combined, e)
		}
		return combined
	}
	return nil
}
