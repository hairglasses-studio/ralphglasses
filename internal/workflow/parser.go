package workflow

import (
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkflowDef is the top-level YAML schema for defining workflows.
type WorkflowDef struct {
	Name  string    `yaml:"name"`
	Steps []StepDef `yaml:"steps"`
}

// StepDef is the YAML representation of a single step.
type StepDef struct {
	Name       string            `yaml:"name"`
	Type       string            `yaml:"type,omitempty"`
	Command    string            `yaml:"command,omitempty"`
	DependsOn  []string          `yaml:"depends_on,omitempty"`
	Timeout    string            `yaml:"timeout,omitempty"`
	RetryCount int               `yaml:"retry_count,omitempty"`
	RetryDelay string            `yaml:"retry_delay,omitempty"`
	Condition  string            `yaml:"condition,omitempty"`
	Params     map[string]string `yaml:"params,omitempty"`
}

// ParseFile reads and parses a YAML workflow definition from a file path.
func ParseFile(path string) (*Workflow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open workflow file: %w", err)
	}
	defer f.Close()
	return Parse(f)
}

// Parse reads a YAML workflow definition from a reader and returns a resolved Workflow.
func Parse(r io.Reader) (*Workflow, error) {
	var def WorkflowDef
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&def); err != nil {
		return nil, fmt.Errorf("decode workflow YAML: %w", err)
	}
	return resolve(def)
}

// ParseBytes parses a YAML workflow definition from raw bytes.
func ParseBytes(data []byte) (*Workflow, error) {
	var def WorkflowDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("unmarshal workflow YAML: %w", err)
	}
	return resolve(def)
}

// resolve converts a WorkflowDef into an executable Workflow, parsing
// duration strings and validating required fields.
func resolve(def WorkflowDef) (*Workflow, error) {
	if def.Name == "" {
		return nil, fmt.Errorf("workflow name is required")
	}
	if len(def.Steps) == 0 {
		return nil, fmt.Errorf("workflow %q has no steps", def.Name)
	}

	seen := make(map[string]bool, len(def.Steps))
	steps := make([]Step, 0, len(def.Steps))

	for i, sd := range def.Steps {
		if sd.Name == "" {
			return nil, fmt.Errorf("step %d: name is required", i)
		}
		if seen[sd.Name] {
			return nil, fmt.Errorf("duplicate step name %q", sd.Name)
		}
		seen[sd.Name] = true

		s := Step{
			Name:       sd.Name,
			Type:       sd.Type,
			Command:    sd.Command,
			DependsOn:  sd.DependsOn,
			RetryCount: sd.RetryCount,
			Condition:  sd.Condition,
			Params:     sd.Params,
		}

		if sd.Timeout != "" {
			d, err := time.ParseDuration(sd.Timeout)
			if err != nil {
				return nil, fmt.Errorf("step %q: invalid timeout %q: %w", sd.Name, sd.Timeout, err)
			}
			s.Timeout = d
		}
		if sd.RetryDelay != "" {
			d, err := time.ParseDuration(sd.RetryDelay)
			if err != nil {
				return nil, fmt.Errorf("step %q: invalid retry_delay %q: %w", sd.Name, sd.RetryDelay, err)
			}
			s.RetryDelay = d
		}

		// Default type to shell_exec when a command is provided.
		if s.Type == "" && s.Command != "" {
			s.Type = "shell_exec"
		}

		steps = append(steps, s)
	}

	return &Workflow{Name: def.Name, Steps: steps}, nil
}
