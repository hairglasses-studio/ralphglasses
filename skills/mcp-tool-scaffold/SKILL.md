---
name: mcp-tool-scaffold
description: Scaffold new MCP tool modules and handlers following mcpkit and hg-mcp patterns. Use when creating a new MCP tool, adding a handler, scaffolding a module, or discussing MCP tool boilerplate in Go. Covers the ToolModule interface, handler skeleton, parameter validation, test generation, and naming conventions.
---

# MCP Tool Scaffolding

Generate production-ready MCP tool modules following mcpkit patterns used across hg-mcp (1,190 tools), mesmer (1,790 tools), and webb (1,371 tools).

## Module Template

```go
package mymodule

import (
    "context"
    "fmt"

    mcp "github.com/mark3labs/mcp-go/mcp"
    "github.com/hairglasses-studio/<project>/pkg/tools"
)

type Module struct{}

func init() { tools.GetRegistry().RegisterModule(&Module{}) }

func (m *Module) Name() string        { return "mymodule" }
func (m *Module) Description() string { return "Brief module description" }
func (m *Module) Tools() []tools.ToolDefinition {
    return []tools.ToolDefinition{
        {
            Tool: mcp.NewTool("mymodule_list",
                mcp.WithDescription("List all items. Returns JSON array with id, name, and status fields."),
                mcp.WithString("filter", mcp.Description("Filter by status: active, inactive, all"), mcp.DefaultString("all")),
                mcp.WithNumber("limit", mcp.Description("Max results to return (default 25, max 100)")),
            ),
            Handler:  handleList,
            Category: "mymodule",
            Tags:     []string{"read", "list"},
        },
    }
}
```

## Handler Template

```go
func handleList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // 1. Extract and validate parameters
    filter := tools.GetStringParam(req, "filter")
    if filter == "" {
        filter = "all"
    }
    limit := tools.GetIntParam(req, "limit", 25)

    // 2. Initialize client (lazy)
    client, err := getClient()
    if err != nil {
        return tools.ErrorResult(fmt.Errorf("client init: %w", err)), nil
    }

    // 3. Call service
    items, err := client.List(ctx, filter, limit)
    if err != nil {
        return tools.ErrorResult(fmt.Errorf("list items: %w", err)), nil
    }

    // 4. Return structured result
    return tools.JSONResult(items), nil
}
```

## TypedHandler Template (mcpkit native)

```go
type ListInput struct {
    Filter string `json:"filter,omitempty" jsonschema:"description=Filter by status: active|inactive|all"`
    Limit  int    `json:"limit,omitempty"  jsonschema:"description=Max results (default 25),minimum=1,maximum=100"`
}

func handleList(ctx context.Context, req handler.TypedRequest[ListInput]) (*mcp.CallToolResult, error) {
    input := req.Input
    if input.Limit == 0 {
        input.Limit = 25
    }
    // ...
    return handler.JSONResult(items), nil
}
```

## Test Template

```go
package mymodule_test

import (
    "testing"

    "github.com/hairglasses-studio/<project>/pkg/tools"
    _ "github.com/hairglasses-studio/<project>/internal/mymodule" // register via init()
)

func TestMyModuleList(t *testing.T) {
    srv := tools.NewTestServer(t)
    result, err := srv.CallTool("mymodule_list", map[string]any{
        "filter": "active",
        "limit":  10,
    })
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.IsError {
        t.Fatalf("tool error: %s", tools.ExtractText(result))
    }
}

func TestMyModuleList_MissingRequired(t *testing.T) {
    srv := tools.NewTestServer(t)
    result, _ := srv.CallTool("mymodule_list", map[string]any{})
    // Should succeed with defaults, not error
    if result.IsError {
        t.Errorf("expected success with defaults, got error")
    }
}
```

## Naming Conventions

- Tool names: `<service>_<module>_<action>` (e.g., `hgmcp_inventory_search`)
- Parameters: descriptive names (`user_id` not `id`, `search_query` not `q`)
- Descriptions: onboarding-quality docs (72% -> 90% accuracy improvement with good descriptions)

## Common Anti-Patterns to Avoid

| Anti-Pattern | Use Instead |
|-------------|-------------|
| `json.MarshalIndent(data, "", "  ")` + manual result | `tools.JSONResult(data)` |
| `tools.ErrorResult(fmt.Errorf("..."))` | `handler.CodedErrorResult(handler.ErrInvalidParam, err)` |
| Direct `request.Params` map access | `tools.GetStringParam(req, "name")` |
| `os.Getenv("KEY")` scattered in handlers | `config.GetRequired("KEY")` or lazy client init |
| Copy-pasted init/registration | Follow Module interface pattern |

## Deferred Loading

Mark lower-priority tools with `DeferLoading: true` when a module has 10+ tools:

```go
{
    Tool:         mcp.NewTool("mymodule_debug_dump", ...),
    Handler:      handleDebugDump,
    DeferLoading: true,
}
```
