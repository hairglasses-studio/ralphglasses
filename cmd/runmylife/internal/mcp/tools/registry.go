package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// ToolHandlerFunc is the function signature for tool handlers.
type ToolHandlerFunc func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

// ToolModule is the interface that all tool modules must implement.
type ToolModule interface {
	Name() string
	Description() string
	Tools() []ToolDefinition
}

// ToolComplexity indicates the complexity level of a tool.
type ToolComplexity string

const (
	ComplexitySimple   ToolComplexity = "simple"
	ComplexityModerate ToolComplexity = "moderate"
	ComplexityComplex  ToolComplexity = "complex"
)

// ToolDefinition represents a complete tool with metadata.
type ToolDefinition struct {
	Tool    mcp.Tool
	Handler ToolHandlerFunc

	Category    string
	Subcategory string
	Tags        []string
	UseCases    []string
	Complexity  ToolComplexity
	IsWrite     bool

	Deprecated    bool
	DeprecatedMsg string

	ProducesRefs []string
	ConsumesRefs []string

	Timeout             time.Duration
	CircuitBreakerGroup string
}

// MiddlewareFunc wraps a tool handler, adding cross-cutting behavior.
type MiddlewareFunc func(name string, td ToolDefinition, next ToolHandlerFunc) ToolHandlerFunc

// ToolRegistry manages all registered tool modules and definitions.
type ToolRegistry struct {
	mu          sync.RWMutex
	modules     map[string]ToolModule
	tools       map[string]ToolDefinition
	middlewares []MiddlewareFunc
}

var (
	globalRegistry     *ToolRegistry
	globalRegistryOnce sync.Once
)

// GetRegistry returns the global tool registry.
func GetRegistry() *ToolRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = &ToolRegistry{
			modules: make(map[string]ToolModule),
			tools:   make(map[string]ToolDefinition),
		}
	})
	return globalRegistry
}

// RegisterModule registers a tool module and all its tools.
func (r *ToolRegistry) RegisterModule(module ToolModule) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.modules[module.Name()] = module
	for _, tool := range module.Tools() {
		applyAnnotations(&tool)
		r.tools[tool.Tool.Name] = tool
	}
}

// SetMiddleware configures the middleware chain applied to all tool handlers.
func (r *ToolRegistry) SetMiddleware(mws []MiddlewareFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.middlewares = mws
}

// RegisterWithServer registers all tools with an MCP server.
func (r *ToolRegistry) RegisterWithServer(s *server.MCPServer) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, tool := range r.tools {
		s.AddTool(tool.Tool, server.ToolHandlerFunc(r.wrapHandler(tool.Tool.Name, tool)))
	}
}

// RegisterDiscoveryOnlyWithServer registers discovery tools with full schemas
// and all other tools with minimal schemas for token efficiency.
func (r *ToolRegistry) RegisterDiscoveryOnlyWithServer(s *server.MCPServer) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fullSchemaTools := map[string]bool{
		"runmylife_tool_discover": true,
		"runmylife_tool_schema":   true,
		"runmylife_tool_stats":    true,
		"runmylife_tool_help":     true,
		"runmylife_tool_usage":    true,
		"runmylife_tool_flow":     true,
		"runmylife_tool_next":     true,
		"runmylife_tool_workflow": true,
	}

	for _, tool := range r.tools {
		if fullSchemaTools[tool.Tool.Name] {
			s.AddTool(tool.Tool, server.ToolHandlerFunc(r.wrapHandler(tool.Tool.Name, tool)))
		} else {
			minimalTool := mcp.Tool{
				Name:        tool.Tool.Name,
				Description: truncateDesc(tool.Tool.Description, 80),
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			}
			s.AddTool(minimalTool, server.ToolHandlerFunc(r.wrapHandler(tool.Tool.Name, tool)))
		}
	}
}

// wrapHandler applies the middleware chain and built-in wrappers to a tool handler.
func (r *ToolRegistry) wrapHandler(toolName string, td ToolDefinition) ToolHandlerFunc {
	handler := td.Handler

	// Innermost: panic recovery
	wrapped := func(ctx context.Context, request mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				log.Printf("[runmylife] PANIC in %s: %v\n%s", toolName, rec, stack)
				result = mcp.NewToolResultError(fmt.Sprintf("[INTERNAL_ERROR] panic in %s: %v", toolName, rec))
				err = nil
			}
		}()
		return handler(ctx, request)
	}

	// Usage tracking
	withUsage := ToolHandlerFunc(func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		RecordUsage(toolName)
		return wrapped(ctx, request)
	})

	// Apply middlewares in reverse order
	final := withUsage
	for i := len(r.middlewares) - 1; i >= 0; i-- {
		final = r.middlewares[i](toolName, td, final)
	}
	return final
}

// GetAllToolDefinitions returns all registered tool definitions.
func (r *ToolRegistry) GetAllToolDefinitions() []ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]ToolDefinition, 0, len(r.tools))
	for _, tool := range r.tools {
		all = append(all, tool)
	}
	return all
}

// GetTool returns a tool definition by name.
func (r *ToolRegistry) GetTool(name string) (ToolDefinition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// ToolCount returns the number of registered tools.
func (r *ToolRegistry) ToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// ToolStats holds registry statistics.
type ToolStats struct {
	TotalTools  int
	ModuleCount int
	ByCategory  map[string]int
}

// GetToolStats returns registry statistics.
func (r *ToolRegistry) GetToolStats() ToolStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := ToolStats{
		TotalTools:  len(r.tools),
		ModuleCount: len(r.modules),
		ByCategory:  make(map[string]int),
	}
	for _, tool := range r.tools {
		stats.ByCategory[tool.Category]++
	}
	return stats
}

// UseLazyTools returns true if lazy tool loading is enabled.
func UseLazyTools() bool {
	return os.Getenv("RUNMYLIFE_LAZY_TOOLS") != "false"
}

func applyAnnotations(td *ToolDefinition) {
	if td.Tool.Annotations.Title == "" {
		td.Tool.Annotations.Title = toolNameToTitle(td.Tool.Name)
	}
	readOnly := !td.IsWrite
	destructive := td.IsWrite
	td.Tool.Annotations.ReadOnlyHint = &readOnly
	td.Tool.Annotations.DestructiveHint = &destructive
}

func toolNameToTitle(name string) string {
	title := strings.TrimPrefix(name, "runmylife_")
	parts := strings.Split(title, "_")
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func truncateDesc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// TextResult creates a text result for a tool response.
func TextResult(text string) *mcp.CallToolResult {
	return mcp.NewToolResultText(text)
}

// ErrorResult creates an error result for a tool response.
func ErrorResult(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(err.Error())
}

// JSONResult creates a JSON-formatted text result.
func JSONResult(data interface{}) *mcp.CallToolResult {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return common.CodedErrorResultf(common.ErrAPIError, "failed to marshal JSON: %v", err)
	}
	return mcp.NewToolResultText(string(jsonBytes))
}
