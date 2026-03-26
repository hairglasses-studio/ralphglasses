package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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

// WorkflowRun represents a single workflow execution.
type WorkflowRun struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	RepoPath  string               `json:"repo_path"`
	Status    string               `json:"status"`
	CreatedAt time.Time            `json:"created_at"`
	UpdatedAt time.Time            `json:"updated_at"`
	Steps     []WorkflowStepResult `json:"steps"`

	mu sync.Mutex
}

// WorkflowStepResult tracks execution state for one workflow step.
type WorkflowStepResult struct {
	Name      string     `json:"name"`
	Status    string     `json:"status"`
	SessionID string     `json:"session_id,omitempty"`
	Provider  Provider   `json:"provider,omitempty"`
	Error     string     `json:"error,omitempty"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Lock locks the workflow run mutex for external callers.
func (r *WorkflowRun) Lock() { r.mu.Lock() }

// Unlock unlocks the workflow run mutex.
func (r *WorkflowRun) Unlock() { r.mu.Unlock() }

// ParseWorkflow parses a workflow definition from YAML bytes.
func ParseWorkflow(name string, data []byte) (WorkflowDef, error) {
	var wf WorkflowDef
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return wf, fmt.Errorf("parse workflow: %w", err)
	}
	if wf.Name == "" {
		wf.Name = name
	}
	if err := ValidateWorkflow(wf); err != nil {
		return wf, err
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
	if err := ValidateWorkflow(wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

// DeleteWorkflow removes a workflow definition file from .ralph/workflows/<name>.yaml.
func DeleteWorkflow(repoPath, name string) error {
	name = strings.ReplaceAll(name, " ", "-")
	path := filepath.Join(repoPath, ".ralph", "workflows", name+".yaml")
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete workflow: %w", err)
	}
	return nil
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

// ValidateWorkflow validates workflow structure and dependency graph.
func ValidateWorkflow(wf WorkflowDef) error {
	if strings.TrimSpace(wf.Name) == "" {
		return fmt.Errorf("workflow name required")
	}
	if len(wf.Steps) == 0 {
		return fmt.Errorf("workflow must contain at least one step")
	}

	seen := make(map[string]struct{}, len(wf.Steps))
	for _, step := range wf.Steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("workflow step name required")
		}
		if strings.TrimSpace(step.Prompt) == "" {
			return fmt.Errorf("workflow step %q prompt required", step.Name)
		}
		if _, ok := seen[step.Name]; ok {
			return fmt.Errorf("workflow step names must be unique: %s", step.Name)
		}
		seen[step.Name] = struct{}{}
		if step.Provider != "" && providerBinary(Provider(step.Provider)) == "" {
			return fmt.Errorf("workflow step %q has unknown provider %q", step.Name, step.Provider)
		}
	}

	for _, step := range wf.Steps {
		for _, dep := range step.DependsOn {
			if _, ok := seen[dep]; !ok {
				return fmt.Errorf("workflow step %q depends on unknown step %q", step.Name, dep)
			}
		}
	}

	visiting := make(map[string]bool, len(wf.Steps))
	visited := make(map[string]bool, len(wf.Steps))
	stepIndex := make(map[string]WorkflowStep, len(wf.Steps))
	for _, step := range wf.Steps {
		stepIndex[step.Name] = step
	}

	var visit func(string) error
	visit = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("workflow contains dependency cycle at step %q", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		for _, dep := range stepIndex[name].DependsOn {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}

	for _, step := range wf.Steps {
		if err := visit(step.Name); err != nil {
			return err
		}
	}

	return nil
}

func newWorkflowRun(repoPath string, wf WorkflowDef) *WorkflowRun {
	now := time.Now()
	steps := make([]WorkflowStepResult, len(wf.Steps))
	for i, step := range wf.Steps {
		steps[i] = WorkflowStepResult{
			Name:   step.Name,
			Status: "pending",
		}
	}
	return &WorkflowRun{
		ID:        uuid.NewString(),
		Name:      wf.Name,
		RepoPath:  repoPath,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
		Steps:     steps,
	}
}

func (r *WorkflowRun) stepByName(name string) *WorkflowStepResult {
	for i := range r.Steps {
		if r.Steps[i].Name == name {
			return &r.Steps[i]
		}
	}
	return nil
}

func (r *WorkflowRun) updateStep(name, status string, mutate func(*WorkflowStepResult)) {
	r.mu.Lock()
	defer r.mu.Unlock()

	step := r.stepByName(name)
	if step == nil {
		return
	}
	step.Status = status
	if mutate != nil {
		mutate(step)
	}
	r.UpdatedAt = time.Now()
}

func (r *WorkflowRun) setStatus(status string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = status
	r.UpdatedAt = time.Now()
}

func detachContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
