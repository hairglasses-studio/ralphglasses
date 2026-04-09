package session

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TaskType classifies the kind of work a task represents.
type TaskType string

const (
	TaskBugfix   TaskType = "bugfix"
	TaskFeature  TaskType = "feature"
	TaskRefactor TaskType = "refactor"
	TaskTest     TaskType = "test"
	TaskDocs     TaskType = "docs"
	TaskResearch TaskType = "research"
	TaskPublish  TaskType = "publish"
)

// validTaskTypes is the set of recognized task types for validation.
var validTaskTypes = map[TaskType]bool{
	TaskBugfix:   true,
	TaskFeature:  true,
	TaskRefactor: true,
	TaskTest:     true,
	TaskDocs:     true,
	TaskResearch: true,
	TaskPublish:  true,
}

// Priority indicates urgency. P0 is highest, P3 is lowest.
type Priority string

const (
	PriorityP0 Priority = "P0"
	PriorityP1 Priority = "P1"
	PriorityP2 Priority = "P2"
	PriorityP3 Priority = "P3"
)

// validPriorities is the set of recognized priority levels.
var validPriorities = map[Priority]bool{
	PriorityP0: true,
	PriorityP1: true,
	PriorityP2: true,
	PriorityP3: true,
}

// Complexity estimates the relative size of a task.
type Complexity string

const (
	ComplexityS  Complexity = "S"
	ComplexityM  Complexity = "M"
	ComplexityL  Complexity = "L"
	ComplexityXL Complexity = "XL"
)

// validComplexities is the set of recognized complexity levels.
var validComplexities = map[Complexity]bool{
	ComplexityS:  true,
	ComplexityM:  true,
	ComplexityL:  true,
	ComplexityXL: true,
}

// TaskInput describes one input to a task (file, context, or constraint).
type TaskInput struct {
	Type     string `json:"type"`     // "file", "context", "constraint"
	Value    string `json:"value"`
	Required bool   `json:"required"`
}

// TaskOutput describes an expected artifact produced by a task.
type TaskOutput struct {
	Type       string `json:"type"`                 // "file", "test", "doc"
	Path       string `json:"path"`
	Validation string `json:"validation,omitempty"` // validation command or description
}

// TaskSpec is a structured, typed task specification that replaces untyped
// string/map task descriptions in the session loop pipeline. It captures
// all metadata needed for planning, routing, and verification.
type TaskSpec struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	Type                TaskType          `json:"type"`
	Priority            Priority          `json:"priority"`
	Inputs              []TaskInput       `json:"inputs,omitempty"`
	Outputs             []TaskOutput      `json:"outputs,omitempty"`
	Dependencies        []string          `json:"dependencies,omitempty"`
	EstimatedComplexity Complexity        `json:"estimated_complexity"`
	Tags                []string          `json:"tags,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
}

// Validate checks that all required fields are populated and that enum
// values are within their allowed sets. Returns nil if the spec is valid.
func (ts *TaskSpec) Validate() error {
	var errs []string

	if strings.TrimSpace(ts.ID) == "" {
		errs = append(errs, "id is required")
	}
	if strings.TrimSpace(ts.Name) == "" {
		errs = append(errs, "name is required")
	}
	if strings.TrimSpace(ts.Description) == "" {
		errs = append(errs, "description is required")
	}
	if ts.Type == "" {
		errs = append(errs, "type is required")
	} else if !validTaskTypes[ts.Type] {
		errs = append(errs, fmt.Sprintf("invalid type %q", ts.Type))
	}
	if ts.Priority == "" {
		errs = append(errs, "priority is required")
	} else if !validPriorities[ts.Priority] {
		errs = append(errs, fmt.Sprintf("invalid priority %q", ts.Priority))
	}
	if ts.EstimatedComplexity == "" {
		errs = append(errs, "estimated_complexity is required")
	} else if !validComplexities[ts.EstimatedComplexity] {
		errs = append(errs, fmt.Sprintf("invalid estimated_complexity %q", ts.EstimatedComplexity))
	}

	if len(errs) > 0 {
		return fmt.Errorf("task spec validation: %s", strings.Join(errs, "; "))
	}
	return nil
}

// MarshalJSON implements json.Marshaler (default behavior, included for symmetry).
func (ts TaskSpec) MarshalJSON() ([]byte, error) {
	type Alias TaskSpec
	return json.Marshal(Alias(ts))
}

// UnmarshalJSON implements json.Unmarshaler (default behavior, included for symmetry).
func (ts *TaskSpec) UnmarshalJSON(data []byte) error {
	type Alias TaskSpec
	var a Alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*ts = TaskSpec(a)
	return nil
}
