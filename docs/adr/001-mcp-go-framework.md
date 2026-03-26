# ADR 001: Choice of mcp-go as MCP Framework

## Status

Accepted

## Context

Ralphglasses exposes 110+ tools across 13 namespaces via the Model Context Protocol (MCP). We needed a Go MCP SDK that provides:

- Compliant MCP server implementation (tool registration, JSON-RPC transport)
- Ergonomic tool definition API (schema, descriptions, required params)
- Active maintenance tracking the evolving MCP specification
- Composability with our own abstractions (tool groups, deferred loading, annotations)

Alternatives considered included writing a custom MCP server from scratch or using other early-stage Go MCP libraries. A custom implementation would give full control but require ongoing spec-tracking effort. Other libraries were less mature or less actively maintained at the time of evaluation.

## Decision

We adopted `github.com/mark3labs/mcp-go` (v0.45.0) as the MCP framework.

All tool registration flows through mcp-go's `server.MCPServer` and `mcp.Tool` types. Our `internal/mcpserver/` package wraps mcp-go with project-specific concerns:

- `Server` struct (`internal/mcpserver/tools.go`) holds application state and delegates tool registration to `server.MCPServer`
- `ToolEntry` pairs an `mcp.Tool` definition with a `server.ToolHandlerFunc` handler
- `ToolGroup` / `ToolGroupRegistry` (`internal/mcpserver/registry.go`) organize tools into namespaces built on top of mcp-go primitives
- `addToolWithMetadata()` (`internal/mcpserver/tools_dispatch.go`) enriches tools with annotations and output schemas before calling `srv.AddTool()`

Tool definitions use mcp-go's builder API (`mcp.NewTool`, `mcp.WithString`, `mcp.WithDescription`, `mcp.Required()`) throughout the handler files.

## Consequences

**Positive:**

- Rapid tool registration with declarative schema builders
- Automatic JSON-RPC transport handling (stdio and SSE)
- Annotations and output schemas layer cleanly on top of mcp-go's `mcp.Tool` struct
- Deferred loading works by calling `srv.AddTool()` at any time, not just at startup

**Negative:**

- Coupled to mcp-go's release cadence for MCP spec changes
- Some features (e.g., `RawOutputSchema`) required working with mcp-go's internal types
- Breaking changes in mcp-go require coordinated updates across all 13 namespace builders

**Risks:**

- If mcp-go is abandoned, we would need to fork or migrate to another SDK
- The v0.x version signals the API is not yet stable
