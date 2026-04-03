// Package workflow provides a DAG-based workflow engine for executing
// multi-step task pipelines with dependency resolution and parallel execution.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// StepStatus represents the current state of a workflow step.
type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepSucceeded StepStatus = "succeeded"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

// Step defines a single unit of work within a workflow.
type Step struct {
	Name      string   `yaml:"name" json:"name"`
	Command   string   `yaml:"command,omitempty" json:"command,omitempty"`
	Type      string   `yaml:"type,omitempty" json:"type,omitempty"` // maps to StepRegistry
	DependsOn []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`

	// Execution policy
	Timeout    time.Duration `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	RetryCount int           `yaml:"retry_count,omitempty" json:"retry_count,omitempty"`
	RetryDelay time.Duration `yaml:"retry_delay,omitempty" json:"retry_delay,omitempty"`
	Condition  string        `yaml:"condition,omitempty" json:"condition,omitempty"`

	// Params is an opaque bag for step-type-specific configuration.
	Params map[string]string `yaml:"params,omitempty" json:"params,omitempty"`
}

// StepResult captures the outcome of executing a step.
type StepResult struct {
	Name     string        `json:"name"`
	Status   StepStatus    `json:"status"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Retries  int           `json:"retries"`
}

// Workflow is a resolved, executable graph of steps.
type Workflow struct {
	Name  string `yaml:"name" json:"name"`
	Steps []Step `yaml:"steps" json:"steps"`
}

// WorkflowResult is the aggregate outcome of a workflow run.
type WorkflowResult struct {
	Name    string                `json:"name"`
	Status  StepStatus            `json:"status"` // succeeded if all steps succeeded
	Results map[string]StepResult `json:"results"`
	Elapsed time.Duration         `json:"elapsed"`
}

// Engine executes workflows using topological ordering and parallel dispatch.
type Engine struct {
	mu       sync.RWMutex
	registry *StepRegistry
	executor *StepExecutor
}

// NewEngine returns an Engine with the default StepRegistry and StepExecutor.
func NewEngine() *Engine {
	reg := NewStepRegistry()
	return &Engine{
		registry: reg,
		executor: NewStepExecutor(reg),
	}
}

// NewEngineWithRegistry returns an Engine using a caller-supplied registry.
func NewEngineWithRegistry(reg *StepRegistry) *Engine {
	return &Engine{
		registry: reg,
		executor: NewStepExecutor(reg),
	}
}

// Registry returns the engine's step registry for registering custom handlers.
func (e *Engine) Registry() *StepRegistry {
	return e.registry
}

// Run executes the workflow respecting dependency ordering.
// Independent steps are dispatched in parallel.
func (e *Engine) Run(ctx context.Context, wf *Workflow) (*WorkflowResult, error) {
	order, err := topoSort(wf.Steps)
	if err != nil {
		return nil, fmt.Errorf("workflow %q: %w", wf.Name, err)
	}

	start := time.Now()
	results := make(map[string]StepResult, len(wf.Steps))
	var mu sync.Mutex

	// Build lookup maps.
	stepMap := make(map[string]Step, len(wf.Steps))
	for _, s := range wf.Steps {
		stepMap[s.Name] = s
	}

	// Run in topological layers: each layer contains steps whose
	// dependencies are already complete.
	layers := toLayers(order, wf.Steps)
	overall := StepSucceeded

	for _, layer := range layers {
		if ctx.Err() != nil {
			break
		}

		var wg sync.WaitGroup
		for _, name := range layer {
			step := stepMap[name]

			mu.Lock()
			// Check if any dependency failed -> skip.
			skip := false
			for _, dep := range step.DependsOn {
				if r, ok := results[dep]; ok && r.Status == StepFailed {
					skip = true
					break
				}
			}

			// Evaluate condition expression if present.
			if !skip && step.Condition != "" {
				if !EvalCondition(step.Condition, results) {
					skip = true
				}
			}

			// Output forwarding: inject dependency outputs into step params.
			// Steps can reference "${dep_name.output}" in their params.
			if !skip {
				step = injectOutputRefs(step, results)
			}
			mu.Unlock()

			if skip {
				mu.Lock()
				results[name] = StepResult{Name: name, Status: StepSkipped}
				if step.Condition == "" {
					// Only mark overall failed if skipped due to dep failure, not condition.
					overall = StepFailed
				}
				mu.Unlock()
				continue
			}

			wg.Add(1)
			go func(s Step) {
				defer wg.Done()
				r := e.executor.Execute(ctx, s)
				mu.Lock()
				results[s.Name] = r
				if r.Status == StepFailed {
					overall = StepFailed
				}
				mu.Unlock()
			}(step)
		}
		wg.Wait()
	}

	return &WorkflowResult{
		Name:    wf.Name,
		Status:  overall,
		Results: results,
		Elapsed: time.Since(start),
	}, nil
}

// injectOutputRefs replaces "${step_name.output}" references in step params
// and command with actual outputs from completed dependency steps.
func injectOutputRefs(step Step, results map[string]StepResult) Step {
	// Deep-copy params to avoid mutating the original step.
	if len(step.Params) > 0 {
		params := make(map[string]string, len(step.Params))
		for k, v := range step.Params {
			params[k] = replaceOutputRefs(v, results)
		}
		step.Params = params
	}
	if step.Command != "" {
		step.Command = replaceOutputRefs(step.Command, results)
	}
	return step
}

// replaceOutputRefs replaces all "${step_name.output}" patterns in s with
// the actual output from the named step's result.
func replaceOutputRefs(s string, results map[string]StepResult) string {
	for name, r := range results {
		placeholder := "${" + name + ".output}"
		if strings.Contains(s, placeholder) {
			s = strings.ReplaceAll(s, placeholder, r.Output)
		}
	}
	return s
}

// topoSort returns step names in a valid topological order or an error if the
// graph contains a cycle.
func topoSort(steps []Step) ([]string, error) {
	// Build adjacency list + in-degree map.
	inDeg := make(map[string]int, len(steps))
	adj := make(map[string][]string, len(steps))
	names := make(map[string]bool, len(steps))

	for _, s := range steps {
		names[s.Name] = true
		if _, ok := inDeg[s.Name]; !ok {
			inDeg[s.Name] = 0
		}
		for _, dep := range s.DependsOn {
			adj[dep] = append(adj[dep], s.Name)
			inDeg[s.Name]++
		}
	}

	// Validate that all dependencies reference existing steps.
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if !names[dep] {
				return nil, fmt.Errorf("step %q depends on unknown step %q", s.Name, dep)
			}
		}
	}

	// Kahn's algorithm.
	var queue []string
	for _, s := range steps {
		if inDeg[s.Name] == 0 {
			queue = append(queue, s.Name)
		}
	}

	var order []string
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, m := range adj[n] {
			inDeg[m]--
			if inDeg[m] == 0 {
				queue = append(queue, m)
			}
		}
	}

	if len(order) != len(steps) {
		return nil, errors.New("cycle detected in workflow steps")
	}
	return order, nil
}

// toLayers groups a topological order into parallel layers.
// Steps in the same layer have no mutual dependencies.
func toLayers(order []string, steps []Step) [][]string {
	depSet := make(map[string]map[string]bool, len(steps))
	for _, s := range steps {
		m := make(map[string]bool, len(s.DependsOn))
		for _, d := range s.DependsOn {
			m[d] = true
		}
		depSet[s.Name] = m
	}

	placed := make(map[string]int) // name -> layer index
	var layers [][]string

	for _, name := range order {
		layer := 0
		for dep := range depSet[name] {
			if l, ok := placed[dep]; ok && l+1 > layer {
				layer = l + 1
			}
		}
		for len(layers) <= layer {
			layers = append(layers, nil)
		}
		layers[layer] = append(layers[layer], name)
		placed[name] = layer
	}
	return layers
}
