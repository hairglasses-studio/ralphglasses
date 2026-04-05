package chains

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Registry manages chain definitions
type Registry struct {
	mu           sync.RWMutex
	chains       map[string]*ChainDefinition
	byCategory   map[ChainCategory][]string
	byTag        map[string][]string
	byTrigger    map[TriggerType][]string
}

// NewRegistry creates a new chain registry
func NewRegistry() *Registry {
	return &Registry{
		chains:     make(map[string]*ChainDefinition),
		byCategory: make(map[ChainCategory][]string),
		byTag:      make(map[string][]string),
		byTrigger:  make(map[TriggerType][]string),
	}
}

// Register adds a chain definition to the registry
func (r *Registry) Register(chain *ChainDefinition) error {
	if chain.Name == "" {
		return fmt.Errorf("chain name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate
	if _, exists := r.chains[chain.Name]; exists {
		return fmt.Errorf("chain %q already registered", chain.Name)
	}

	// Store chain
	r.chains[chain.Name] = chain

	// Index by category
	r.byCategory[chain.Category] = append(r.byCategory[chain.Category], chain.Name)

	// Index by tags
	for _, tag := range chain.Tags {
		r.byTag[tag] = append(r.byTag[tag], chain.Name)
	}

	// Index by trigger type
	r.byTrigger[chain.Trigger.Type] = append(r.byTrigger[chain.Trigger.Type], chain.Name)

	return nil
}

// Get retrieves a chain definition by name
func (r *Registry) Get(name string) (*ChainDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chain, exists := r.chains[name]
	if !exists {
		return nil, fmt.Errorf("chain %q not found", name)
	}
	return chain, nil
}

// List returns all registered chain names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.chains))
	for name := range r.chains {
		names = append(names, name)
	}
	return names
}

// ListByCategory returns chain names for a specific category
func (r *Registry) ListByCategory(category ChainCategory) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.byCategory[category]
	result := make([]string, len(names))
	copy(result, names)
	return result
}

// ListByTag returns chain names with a specific tag
func (r *Registry) ListByTag(tag string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.byTag[tag]
	result := make([]string, len(names))
	copy(result, names)
	return result
}

// ListByTrigger returns chain names with a specific trigger type
func (r *Registry) ListByTrigger(triggerType TriggerType) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.byTrigger[triggerType]
	result := make([]string, len(names))
	copy(result, names)
	return result
}

// GetSummaries returns summaries for all chains
func (r *Registry) GetSummaries() []ChainSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]ChainSummary, 0, len(r.chains))
	for _, chain := range r.chains {
		summaries = append(summaries, chain.ToSummary())
	}
	return summaries
}

// GetSummariesByCategory returns summaries for chains in a category
func (r *Registry) GetSummariesByCategory(category ChainCategory) []ChainSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := r.byCategory[category]
	summaries := make([]ChainSummary, 0, len(names))
	for _, name := range names {
		if chain, exists := r.chains[name]; exists {
			summaries = append(summaries, chain.ToSummary())
		}
	}
	return summaries
}

// Unregister removes a chain from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	chain, exists := r.chains[name]
	if !exists {
		return fmt.Errorf("chain %q not found", name)
	}

	// Remove from category index
	r.removeFromSlice(r.byCategory, chain.Category, name)

	// Remove from tag indexes
	for _, tag := range chain.Tags {
		r.removeFromTagIndex(tag, name)
	}

	// Remove from trigger index
	r.removeFromTriggerIndex(chain.Trigger.Type, name)

	// Remove chain
	delete(r.chains, name)

	return nil
}

func (r *Registry) removeFromSlice(m map[ChainCategory][]string, key ChainCategory, value string) {
	slice := m[key]
	for i, v := range slice {
		if v == value {
			m[key] = append(slice[:i], slice[i+1:]...)
			return
		}
	}
}

func (r *Registry) removeFromTagIndex(tag, name string) {
	slice := r.byTag[tag]
	for i, v := range slice {
		if v == name {
			r.byTag[tag] = append(slice[:i], slice[i+1:]...)
			return
		}
	}
}

func (r *Registry) removeFromTriggerIndex(triggerType TriggerType, name string) {
	slice := r.byTrigger[triggerType]
	for i, v := range slice {
		if v == name {
			r.byTrigger[triggerType] = append(slice[:i], slice[i+1:]...)
			return
		}
	}
}

// LoadFromFile loads a chain definition from a YAML file
func (r *Registry) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", path, err)
	}

	var chain ChainDefinition
	if err := yaml.Unmarshal(data, &chain); err != nil {
		return fmt.Errorf("failed to parse YAML %s: %w", path, err)
	}

	// Set default step type
	for i := range chain.Steps {
		if chain.Steps[i].Type == "" {
			chain.Steps[i].Type = StepTypeTool
		}
	}

	return r.Register(&chain)
}

// LoadFromDirectory loads all chain definitions from a directory
func (r *Registry) LoadFromDirectory(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-YAML files
		if info.IsDir() {
			return nil
		}

		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		if err := r.LoadFromFile(path); err != nil {
			return fmt.Errorf("failed to load %s: %w", path, err)
		}

		return nil
	})
}

// Count returns the number of registered chains
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.chains)
}

// Categories returns all categories that have chains
func (r *Registry) Categories() []ChainCategory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	categories := make([]ChainCategory, 0, len(r.byCategory))
	for cat := range r.byCategory {
		if len(r.byCategory[cat]) > 0 {
			categories = append(categories, cat)
		}
	}
	return categories
}

// Global registry instance
var globalRegistry *Registry
var registryOnce sync.Once

// GetRegistry returns the global chain registry with built-in chains registered
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = NewRegistry()
		// Register built-in chains so they're available via MCP tools
		if err := RegisterBuiltInChains(globalRegistry); err != nil {
			// Log but don't fail - chains will just be empty
			_ = err
		}
	})
	return globalRegistry
}
