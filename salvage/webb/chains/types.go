package chains

import (
	"time"
)

// ChainCategory represents the category of a chain
type ChainCategory string

const (
	CategoryOperational   ChainCategory = "operational"
	CategoryInvestigative ChainCategory = "investigative"
	CategoryCustomer      ChainCategory = "customer"
	CategoryDevelopment   ChainCategory = "development"
	CategoryRemediation   ChainCategory = "remediation"
)

// TriggerType represents how a chain is triggered
type TriggerType string

const (
	TriggerManual    TriggerType = "manual"
	TriggerScheduled TriggerType = "scheduled"
	TriggerEvent     TriggerType = "event"
)

// StepType represents the type of step execution
type StepType string

const (
	StepTypeTool     StepType = "tool"     // Execute an MCP tool
	StepTypeChain    StepType = "chain"    // Execute a sub-chain
	StepTypeParallel StepType = "parallel" // Execute steps in parallel
	StepTypeBranch   StepType = "branch"   // Conditional branching
	StepTypeGate     StepType = "gate"     // Human-in-the-loop approval
)

// ExecutionStatus represents the status of a chain execution
type ExecutionStatus string

const (
	StatusPending    ExecutionStatus = "pending"
	StatusRunning    ExecutionStatus = "running"
	StatusPaused     ExecutionStatus = "paused"     // Waiting for gate approval
	StatusCompleted  ExecutionStatus = "completed"
	StatusFailed     ExecutionStatus = "failed"
	StatusCancelled  ExecutionStatus = "cancelled"
)

// ChainDefinition defines a complete workflow chain
type ChainDefinition struct {
	Name        string            `json:"name" yaml:"name"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Category    ChainCategory     `json:"category" yaml:"category"`
	Trigger     ChainTrigger      `json:"trigger" yaml:"trigger"`
	Input       []ChainInput      `json:"input,omitempty" yaml:"input,omitempty"`
	Variables   map[string]string `json:"variables,omitempty" yaml:"variables,omitempty"`
	Steps       []ChainStep       `json:"steps" yaml:"steps"`
	OnError     *ErrorHandler     `json:"on_error,omitempty" yaml:"on_error,omitempty"`
	Timeout     string            `json:"timeout,omitempty" yaml:"timeout,omitempty"` // e.g., "30m", "1h"
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
}

// ChainTrigger defines how a chain is triggered
type ChainTrigger struct {
	Type   TriggerType `json:"type" yaml:"type"`
	Cron   string      `json:"cron,omitempty" yaml:"cron,omitempty"`     // For scheduled triggers
	Event  string      `json:"event,omitempty" yaml:"event,omitempty"`   // For event triggers (e.g., "incident.created")
	Filter string      `json:"filter,omitempty" yaml:"filter,omitempty"` // Optional filter expression
}

// ChainInput defines an input parameter for the chain
type ChainInput struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`                                   // string, int, bool, list
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Default     string `json:"default,omitempty" yaml:"default,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ChainStep represents a single step in a chain
type ChainStep struct {
	ID          string            `json:"id" yaml:"id"`
	Type        StepType          `json:"type,omitempty" yaml:"type,omitempty"` // Defaults to "tool"
	Name        string            `json:"name,omitempty" yaml:"name,omitempty"` // Human-readable name

	// For tool steps
	Tool   string            `json:"tool,omitempty" yaml:"tool,omitempty"`
	Params map[string]string `json:"params,omitempty" yaml:"params,omitempty"`

	// For chain steps (sub-chain)
	Chain string `json:"chain,omitempty" yaml:"chain,omitempty"`

	// For parallel steps
	Steps []ChainStep `json:"steps,omitempty" yaml:"steps,omitempty"`

	// For branch steps
	Condition string              `json:"condition,omitempty" yaml:"condition,omitempty"`
	Branches  map[string][]ChainStep `json:"branches,omitempty" yaml:"branches,omitempty"`

	// For gate steps
	GateType   string `json:"gate_type,omitempty" yaml:"gate_type,omitempty"` // "human", "approval"
	Message    string `json:"message,omitempty" yaml:"message,omitempty"`
	GateTimeout string `json:"gate_timeout,omitempty" yaml:"gate_timeout,omitempty"`
	OnTimeout  string `json:"on_timeout,omitempty" yaml:"on_timeout,omitempty"` // "continue", "abort", "default"

	// Common fields
	Retry       *RetryPolicy `json:"retry,omitempty" yaml:"retry,omitempty"`
	ContinueOn  string       `json:"continue_on,omitempty" yaml:"continue_on,omitempty"` // "success", "failure", "always"
	StoreAs     string       `json:"store_as,omitempty" yaml:"store_as,omitempty"`       // Variable name to store result
}

// RetryPolicy defines retry behavior for a step
type RetryPolicy struct {
	MaxAttempts int    `json:"max_attempts" yaml:"max_attempts"`
	Delay       string `json:"delay,omitempty" yaml:"delay,omitempty"`         // e.g., "5s", "1m"
	BackoffRate float64 `json:"backoff_rate,omitempty" yaml:"backoff_rate,omitempty"` // Multiplier for exponential backoff
}

// ErrorHandler defines how to handle errors in the chain
type ErrorHandler struct {
	Action  string `json:"action" yaml:"action"`   // "abort", "continue", "retry", "fallback"
	Fallback string `json:"fallback,omitempty" yaml:"fallback,omitempty"` // Chain or step to run on error
	Notify  string `json:"notify,omitempty" yaml:"notify,omitempty"`     // Slack channel or user to notify
}

// ChainExecution represents a running or completed chain execution
type ChainExecution struct {
	ID           string                 `json:"id"`
	ChainName    string                 `json:"chain_name"`
	Status       ExecutionStatus        `json:"status"`
	StartedAt    time.Time              `json:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	CurrentStep  string                 `json:"current_step,omitempty"`
	Input        map[string]interface{} `json:"input,omitempty"`
	Variables    map[string]interface{} `json:"variables"`
	StepResults  map[string]StepResult  `json:"step_results"`
	Error        string                 `json:"error,omitempty"`
	TriggeredBy  string                 `json:"triggered_by,omitempty"` // "manual", "schedule", "event:<name>"
	ParentExecID string                 `json:"parent_exec_id,omitempty"` // For sub-chain executions
}

// StepResult represents the result of a step execution
type StepResult struct {
	StepID      string                 `json:"step_id"`
	Status      ExecutionStatus        `json:"status"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Output      map[string]interface{} `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Attempts    int                    `json:"attempts"`
}

// Checkpoint represents a saved execution state for resume
type Checkpoint struct {
	ExecutionID string          `json:"execution_id"`
	ChainName   string          `json:"chain_name"`
	StepID      string          `json:"step_id"`
	Variables   map[string]interface{} `json:"variables"`
	StepResults map[string]StepResult  `json:"step_results"`
	CreatedAt   time.Time       `json:"created_at"`
}

// GateApproval represents an approval for a gate step
type GateApproval struct {
	ExecutionID string    `json:"execution_id"`
	StepID      string    `json:"step_id"`
	Approved    bool      `json:"approved"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
	ApprovedAt  time.Time `json:"approved_at"`
	Comment     string    `json:"comment,omitempty"`
}

// ChainSummary provides a brief overview of a chain
type ChainSummary struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Category    ChainCategory `json:"category"`
	TriggerType TriggerType   `json:"trigger_type"`
	StepCount   int           `json:"step_count"`
	Tags        []string      `json:"tags,omitempty"`
}

// ToSummary converts a ChainDefinition to a ChainSummary
func (c *ChainDefinition) ToSummary() ChainSummary {
	return ChainSummary{
		Name:        c.Name,
		Description: c.Description,
		Category:    c.Category,
		TriggerType: c.Trigger.Type,
		StepCount:   len(c.Steps),
		Tags:        c.Tags,
	}
}
