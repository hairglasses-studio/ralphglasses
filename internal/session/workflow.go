package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowDef defines a multi-step workflow.
type WorkflowDef struct {
	Name  string         `json:"name" yaml:"name"`
	Steps []WorkflowStep `json:"steps" yaml:"steps"`
}

// WorkflowStep is a single step in a workflow.
type WorkflowStep struct {
	Name      string   `json:"name" yaml:"name"`
	Prompt    string   `json:"prompt" yaml:"prompt"`
	Provider  string   `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model     string   `json:"model,omitempty" yaml:"model,omitempty"`
	Agent     string   `json:"agent,omitempty" yaml:"agent,omitempty"`
	DependsOn []string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
	Parallel  bool     `json:"parallel,omitempty" yaml:"parallel,omitempty"`
}

// ParseWorkflow parses a workflow definition from YAML bytes.
func ParseWorkflow(name string, data []byte) (WorkflowDef, error) {
	var wf WorkflowDef
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return wf, fmt.Errorf("parse workflow: %w", err)
	}
	if wf.Name == "" {
		wf.Name = name
	}
	return wf, nil
}

// SaveWorkflow writes a workflow definition to .ralph/workflows/<name>.yaml.
func SaveWorkflow(repoPath string, wf WorkflowDef) error {
	dir := filepath.Join(repoPath, ".ralph", "workflows")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create workflows dir: %w", err)
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		return fmt.Errorf("marshal workflow: %w", err)
	}

	name := strings.ReplaceAll(wf.Name, " ", "-")
	path := filepath.Join(dir, name+".yaml")
	return os.WriteFile(path, data, 0644)
}

// LoadWorkflow reads a workflow definition from .ralph/workflows/<name>.yaml.
func LoadWorkflow(repoPath, name string) (*WorkflowDef, error) {
	name = strings.ReplaceAll(name, " ", "-")
	path := filepath.Join(repoPath, ".ralph", "workflows", name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow: %w", err)
	}

	var wf WorkflowDef
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}
	return &wf, nil
}

// ListWorkflows returns all workflow definitions in .ralph/workflows/.
func ListWorkflows(repoPath string) ([]WorkflowDef, error) {
	dir := filepath.Join(repoPath, ".ralph", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	var workflows []WorkflowDef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var wf WorkflowDef
		if err := yaml.Unmarshal(data, &wf); err != nil {
			continue
		}
		workflows = append(workflows, wf)
	}
	return workflows, nil
}
