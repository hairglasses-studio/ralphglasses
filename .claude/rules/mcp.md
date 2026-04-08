---
paths:
  - "internal/mcpserver/**"
---

MCP server patterns (222 tools across 30 deferred-loaded groups):
- Tool groups registered via `ToolGroupRegistry` / `ToolGroupBuilder` in `registry.go`
- Each group has a `build*Group()` method on `*Server` in domain-specific `tools_builders_*.go` files
- Error handling: always use `codedError(ErrCode, msg)` returning `(*mcp.CallToolResult, nil)`
- Middleware stack: Concurrency(32) → Trace → Instrumentation → EventBus → Validation → ResponseSizeLimit(4096) → Hooks
- Deferred loading: groups load on first access via `ralphglasses_load_tool_group`
- ValidatePath guards required on all handlers accepting file/repo path params (security: path traversal)
- Response size limit is 4KB — return summaries with follow-up detail tools for large payloads
