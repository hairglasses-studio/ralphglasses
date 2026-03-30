package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LaunchTemplate is a named, reusable launch configuration.
type LaunchTemplate struct {
	Name        string  `json:"name"`
	Provider    string  `json:"provider"`
	Model       string  `json:"model,omitempty"`
	Prompt      string  `json:"prompt,omitempty"`
	BudgetUSD   float64 `json:"budget_usd,omitempty"`
	MaxTurns    int     `json:"max_turns,omitempty"`
	RepoPath    string  `json:"repo_path,omitempty"`
}

// TemplateDir returns the directory for stored templates.
func TemplateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ralphglasses", "templates")
}

// SaveTemplate writes a launch template to disk.
func SaveTemplate(t LaunchTemplate) error {
	dir := TemplateDir()
	if dir == "" {
		return fmt.Errorf("cannot determine template directory")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, t.Name+".json"), data, 0644)
}

// LoadTemplate reads a named template.
func LoadTemplate(name string) (*LaunchTemplate, error) {
	dir := TemplateDir()
	if dir == "" {
		return nil, fmt.Errorf("cannot determine template directory")
	}
	data, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return nil, err
	}
	var t LaunchTemplate
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ListTemplates returns all saved template names.
func ListTemplates() ([]string, error) {
	dir := TemplateDir()
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
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

// DeleteTemplate removes a named template.
func DeleteTemplate(name string) error {
	dir := TemplateDir()
	if dir == "" {
		return fmt.Errorf("cannot determine template directory")
	}
	return os.Remove(filepath.Join(dir, name+".json"))
}
