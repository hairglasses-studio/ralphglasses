package session

import (
	"context"
	"fmt"
	"sync"
)

// AgentRole defines the specialization of a micro-agent.
// Following 12-Factor Agent principle #10: small, focused agents.
type AgentRole string

const (
	RolePlanner      AgentRole = "planner"
	RoleImplementer  AgentRole = "implementer"
	RoleReviewer     AgentRole = "reviewer"
	RoleTester       AgentRole = "tester"
	RoleSynthesizer  AgentRole = "synthesizer"
)

// AgentTaskSpec describes work to be executed by a micro-agent.
type AgentTaskSpec struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	RepoPath    string            `json:"repo_path,omitempty"`
	Provider    Provider          `json:"provider,omitempty"`
	Model       string            `json:"model,omitempty"`
	MaxTurns    int               `json:"max_turns,omitempty"`
	BudgetUSD   float64           `json:"budget_usd,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
}

// TaskResult holds the output of a micro-agent execution.
type TaskResult struct {
	AgentRole   AgentRole `json:"agent_role"`
	Success     bool      `json:"success"`
	Output      string    `json:"output,omitempty"`
	Error       string    `json:"error,omitempty"`
	SpentUSD    float64   `json:"spent_usd"`
	TurnCount   int       `json:"turn_count"`
	Interrupted bool      `json:"interrupted"`
}

// MicroAgent is the interface for a focused, single-purpose agent.
// Each agent handles a specific phase of work (plan, implement, review, test, synthesize).
type MicroAgent interface {
	Role() AgentRole
	MaxConcurrency() int
	Execute(ctx context.Context, spec AgentTaskSpec) (*TaskResult, error)
}

// AgentRegistry manages registration and lookup of micro-agents by role.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[AgentRole]MicroAgent
}

// NewAgentRegistry creates an empty registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[AgentRole]MicroAgent),
	}
}

// Register adds a micro-agent for the given role.
func (r *AgentRegistry) Register(agent MicroAgent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agent.Role()] = agent
}

// Get returns the agent for the given role, or nil if not registered.
func (r *AgentRegistry) Get(role AgentRole) MicroAgent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.agents[role]
}

// Roles returns all registered roles.
func (r *AgentRegistry) Roles() []AgentRole {
	r.mu.RLock()
	defer r.mu.RUnlock()
	roles := make([]AgentRole, 0, len(r.agents))
	for role := range r.agents {
		roles = append(roles, role)
	}
	return roles
}

// Pipeline defines an ordered sequence of agent roles for a workflow.
type Pipeline struct {
	Steps []AgentRole `json:"steps"`
}

// DefaultPipeline returns the standard plan->implement->review->test pipeline.
func DefaultPipeline() Pipeline {
	return Pipeline{
		Steps: []AgentRole{RolePlanner, RoleImplementer, RoleReviewer, RoleTester},
	}
}

// ExecutePipeline runs a sequence of micro-agents, passing each result
// as context to the next. Returns all results and stops on first failure
// unless the agent was interrupted.
func (r *AgentRegistry) ExecutePipeline(ctx context.Context, pipeline Pipeline, spec AgentTaskSpec) ([]TaskResult, error) {
	var results []TaskResult

	for _, role := range pipeline.Steps {
		agent := r.Get(role)
		if agent == nil {
			return results, fmt.Errorf("no agent registered for role %s", role)
		}

		// Inject previous results as context.
		if spec.Context == nil {
			spec.Context = make(map[string]string)
		}
		if len(results) > 0 {
			last := results[len(results)-1]
			spec.Context["previous_output"] = last.Output
			spec.Context["previous_role"] = string(last.AgentRole)
		}

		result, err := agent.Execute(ctx, spec)
		if err != nil {
			return results, fmt.Errorf("agent %s failed: %w", role, err)
		}
		results = append(results, *result)

		if !result.Success && !result.Interrupted {
			break // stop pipeline on failure
		}
		if result.Interrupted {
			break // stop pipeline on interruption (can be resumed)
		}
	}

	return results, nil
}
