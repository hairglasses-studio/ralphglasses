package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PromptVersion represents a versioned snapshot of a prompt.
type PromptVersion struct {
	Version   int       `json:"version"`
	Template  string    `json:"template"`
	CreatedAt time.Time `json:"created_at"`
	Comment   string    `json:"comment,omitempty"`
}

// PromptVersionHistory tracks versions of a named prompt.
type PromptVersionHistory struct {
	Name     string          `json:"name"`
	Versions []PromptVersion `json:"versions"`
}

// PromptVersionStore manages versioned prompts.
type PromptVersionStore struct {
	dir string
}

// NewPromptVersionStore creates a version store.
func NewPromptVersionStore(dir string) *PromptVersionStore {
	return &PromptVersionStore{dir: dir}
}

// Save creates a new version of a named prompt.
func (pvs *PromptVersionStore) Save(name, template, comment string) (int, error) {
	if pvs.dir == "" {
		return 0, fmt.Errorf("version store directory not set")
	}

	history := pvs.loadHistory(name)
	version := len(history.Versions) + 1

	history.Versions = append(history.Versions, PromptVersion{
		Version:   version,
		Template:  template,
		CreatedAt: time.Now(),
		Comment:   comment,
	})

	return version, pvs.saveHistory(name, history)
}

// Get retrieves a specific version. Version 0 means latest.
func (pvs *PromptVersionStore) Get(name string, version int) (*PromptVersion, error) {
	history := pvs.loadHistory(name)
	if len(history.Versions) == 0 {
		return nil, fmt.Errorf("no versions for prompt %q", name)
	}

	if version <= 0 {
		return &history.Versions[len(history.Versions)-1], nil
	}

	for i := range history.Versions {
		if history.Versions[i].Version == version {
			return &history.Versions[i], nil
		}
	}
	return nil, fmt.Errorf("version %d not found for prompt %q", version, name)
}

// ListVersions returns all versions of a named prompt.
func (pvs *PromptVersionStore) ListVersions(name string) []PromptVersion {
	return pvs.loadHistory(name).Versions
}

func (pvs *PromptVersionStore) loadHistory(name string) *PromptVersionHistory {
	path := filepath.Join(pvs.dir, name+".versions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return &PromptVersionHistory{Name: name}
	}
	var h PromptVersionHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return &PromptVersionHistory{Name: name}
	}
	return &h
}

func (pvs *PromptVersionStore) saveHistory(name string, h *PromptVersionHistory) error {
	if err := os.MkdirAll(pvs.dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pvs.dir, name+".versions.json"), data, 0644)
}
