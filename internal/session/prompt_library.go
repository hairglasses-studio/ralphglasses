package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

// PromptEntry is a saved reusable prompt.
type PromptEntry struct {
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Template    string            `json:"template"`
	Variables   map[string]string `json:"variables,omitempty"` // default values
	Tags        []string          `json:"tags,omitempty"`
}

// PromptLibrary manages saved prompts in the shared Ralph prompt library path.
type PromptLibrary struct {
	dir string
}

// NewPromptLibrary creates a prompt library.
func NewPromptLibrary() *PromptLibrary {
	return &PromptLibrary{
		dir: ralphpath.PromptsDir(),
	}
}

// NewPromptLibraryAt creates a prompt library at a specific directory.
func NewPromptLibraryAt(dir string) *PromptLibrary {
	return &PromptLibrary{dir: dir}
}

// Save writes a prompt entry to disk.
func (pl *PromptLibrary) Save(entry PromptEntry) error {
	if pl.dir == "" {
		return fmt.Errorf("prompt library directory not configured")
	}
	if err := os.MkdirAll(pl.dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pl.dir, entry.Name+".json"), data, 0644)
}

// Load reads a named prompt entry.
func (pl *PromptLibrary) Load(name string) (*PromptEntry, error) {
	if pl.dir == "" {
		return nil, fmt.Errorf("prompt library directory not configured")
	}
	data, err := os.ReadFile(filepath.Join(pl.dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var entry PromptEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// List returns all saved prompt names.
func (pl *PromptLibrary) List() ([]string, error) {
	if pl.dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(pl.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	sort.Strings(names)
	return names, nil
}

// Delete removes a named prompt.
func (pl *PromptLibrary) Delete(name string) error {
	if pl.dir == "" {
		return fmt.Errorf("prompt library directory not configured")
	}
	return os.Remove(filepath.Join(pl.dir, name+".json"))
}
