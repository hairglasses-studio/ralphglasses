// Package remote provides SSH remote host management for ralphglasses fleet control.
package remote

import (
	"fmt"
	"sort"
	"sync"
)

// Host represents a remote machine accessible via SSH.
type Host struct {
	Address string            `json:"address"`
	User    string            `json:"user"`
	Port    int               `json:"port"`
	KeyPath string            `json:"key_path,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// HostRegistry is a thread-safe registry of named remote hosts.
type HostRegistry struct {
	mu    sync.RWMutex
	hosts map[string]*Host
}

// NewHostRegistry creates an empty host registry.
func NewHostRegistry() *HostRegistry {
	return &HostRegistry{
		hosts: make(map[string]*Host),
	}
}

// Register adds a host under the given name. Returns an error if the name is
// already registered.
func (r *HostRegistry) Register(name string, host Host) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.hosts[name]; exists {
		return fmt.Errorf("host %q already registered", name)
	}
	h := host // copy
	r.hosts[name] = &h
	return nil
}

// Get returns the host registered under name and true, or nil and false if not
// found.
func (r *HostRegistry) Get(name string) (*Host, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.hosts[name]
	if !ok {
		return nil, false
	}
	cp := *h
	return &cp, true
}

// List returns all registered hosts sorted by address.
func (r *HostRegistry) List() []Host {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Host, 0, len(r.hosts))
	for _, h := range r.hosts {
		out = append(out, *h)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Address < out[j].Address
	})
	return out
}

// Remove deletes the host registered under name. Returns an error if the name
// is not found.
func (r *HostRegistry) Remove(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.hosts[name]; !exists {
		return fmt.Errorf("host %q not found", name)
	}
	delete(r.hosts, name)
	return nil
}

// FilterByLabel returns all hosts whose Labels contain the given key/value pair.
func (r *HostRegistry) FilterByLabel(key, value string) []Host {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []Host
	for _, h := range r.hosts {
		if h.Labels != nil && h.Labels[key] == value {
			out = append(out, *h)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Address < out[j].Address
	})
	return out
}
