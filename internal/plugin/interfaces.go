package plugin

import "context"

// ProviderPlugin extends Plugin with custom LLM provider capabilities.
// Implementations can back alternative model providers such as Ollama,
// vLLM, or any locally-hosted inference endpoint.
type ProviderPlugin interface {
	Plugin

	// ProviderName returns the unique identifier for this provider
	// (e.g., "ollama", "vllm", "local-llama").
	ProviderName() string

	// Complete sends a prompt to the provider and returns the completion text.
	// The opts map carries provider-specific parameters (model, temperature, etc.).
	Complete(ctx context.Context, prompt string, opts map[string]any) (string, error)
}

// ToolDef describes a single MCP tool exposed by a ToolPlugin.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

// ToolPlugin extends Plugin with runtime MCP tool registration.
// Unlike GRPCPlugin (which uses string tool names and string results),
// ToolPlugin uses structured ToolDef descriptors and returns typed results.
type ToolPlugin interface {
	Plugin

	// Tools returns the set of tool definitions this plugin provides.
	Tools() []ToolDef

	// HandleToolCall dispatches a tool invocation by name with the given arguments.
	// The return value is the structured tool result (typically JSON-serializable).
	HandleToolCall(ctx context.Context, name string, args map[string]any) (any, error)
}

// Task represents a unit of work returned by a StrategyPlugin's Plan method.
type Task struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Priority    int            `json:"priority"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// PlanContext carries the information a StrategyPlugin needs to formulate a plan.
type PlanContext struct {
	Repo     string         `json:"repo"`
	Provider string         `json:"provider"`
	History  []Event        `json:"history,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// StrategyPlugin extends Plugin with alternative loop planning strategies.
// Implementations can replace or augment the default task planning logic
// with custom heuristics, ML-based prioritization, or external orchestration.
type StrategyPlugin interface {
	Plugin

	// Plan produces an ordered list of tasks given the current context and
	// a set of candidate tasks. The strategy may reorder, filter, split,
	// or synthesize new tasks.
	Plan(ctx context.Context, pc PlanContext, tasks []Task) ([]Task, error)
}

// --- Type assertion helpers ---

// AsProvider checks whether p implements ProviderPlugin and returns it.
// Returns nil, false if p does not implement the interface.
func AsProvider(p Plugin) (ProviderPlugin, bool) {
	pp, ok := p.(ProviderPlugin)
	return pp, ok
}

// AsTool checks whether p implements ToolPlugin and returns it.
// Returns nil, false if p does not implement the interface.
func AsTool(p Plugin) (ToolPlugin, bool) {
	tp, ok := p.(ToolPlugin)
	return tp, ok
}

// AsStrategy checks whether p implements StrategyPlugin and returns it.
// Returns nil, false if p does not implement the interface.
func AsStrategy(p Plugin) (StrategyPlugin, bool) {
	sp, ok := p.(StrategyPlugin)
	return sp, ok
}

// AsGRPC checks whether p implements GRPCPlugin and returns it.
// Returns nil, false if p does not implement the interface.
func AsGRPC(p Plugin) (GRPCPlugin, bool) {
	gp, ok := p.(GRPCPlugin)
	return gp, ok
}
