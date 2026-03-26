package mcpserver

// ToolGroupBuilder builds a ToolGroup for a specific namespace.
type ToolGroupBuilder interface {
	// Name returns the unique namespace identifier for this tool group.
	Name() string
	// Build constructs the ToolGroup with all tool definitions and handlers.
	Build(s *Server) ToolGroup
}

// FuncBuilder is a convenience adapter that turns a plain function into a
// ToolGroupBuilder. Use it for simple registrations where a full interface
// implementation would be boilerplate.
type FuncBuilder struct {
	name    string
	buildFn func(s *Server) ToolGroup
}

// Name returns the namespace identifier.
func (f *FuncBuilder) Name() string { return f.name }

// Build delegates to the wrapped function.
func (f *FuncBuilder) Build(s *Server) ToolGroup { return f.buildFn(s) }

// NewFuncBuilder creates a FuncBuilder from a name and build function.
func NewFuncBuilder(name string, fn func(s *Server) ToolGroup) *FuncBuilder {
	return &FuncBuilder{name: name, buildFn: fn}
}

// ToolGroupRegistry collects builders and produces tool groups on demand.
// Builders are invoked in registration order so that the output of BuildAll
// is deterministic and matches the previous hard-coded ordering.
type ToolGroupRegistry struct {
	builders []ToolGroupBuilder
}

// NewToolGroupRegistry creates an empty registry.
func NewToolGroupRegistry() *ToolGroupRegistry {
	return &ToolGroupRegistry{}
}

// Register appends a builder to the registry.
func (r *ToolGroupRegistry) Register(b ToolGroupBuilder) {
	r.builders = append(r.builders, b)
}

// BuildAll invokes every registered builder and returns the resulting groups
// keyed by namespace name.
func (r *ToolGroupRegistry) BuildAll(s *Server) map[string]ToolGroup {
	m := make(map[string]ToolGroup, len(r.builders))
	for _, b := range r.builders {
		m[b.Name()] = b.Build(s)
	}
	return m
}

// BuildAllOrdered invokes every registered builder and returns the resulting
// groups as an ordered slice, preserving registration order.
func (r *ToolGroupRegistry) BuildAllOrdered(s *Server) []ToolGroup {
	groups := make([]ToolGroup, 0, len(r.builders))
	for _, b := range r.builders {
		groups = append(groups, b.Build(s))
	}
	return groups
}

// Names returns the namespace names of all registered builders in order.
func (r *ToolGroupRegistry) Names() []string {
	names := make([]string, len(r.builders))
	for i, b := range r.builders {
		names[i] = b.Name()
	}
	return names
}

// Len returns the number of registered builders.
func (r *ToolGroupRegistry) Len() int {
	return len(r.builders)
}
