# ADR 002: Deferred Tool Group Loading

## Status

Accepted

## Context

Ralphglasses registers 110+ MCP tools across 13 namespaces. Loading all tools at startup has two costs:

1. **Token overhead** -- LLM clients receive the full tool catalog in their system prompt, consuming context window tokens even for tools that will never be used in a given session.
2. **Startup latency** -- Building all tool groups involves initializing handlers and wiring dependencies that may not be needed.

Claude Code and similar MCP clients work best when the visible tool set is small and focused. A session that only needs session management should not see fleet analytics or roadmap tools.

## Decision

We implemented deferred (lazy) tool group loading controlled by the `Server.DeferredLoading` flag.

When `DeferredLoading` is true, `Server.Register()` calls `RegisterCoreTools()` instead of `RegisterAllTools()` (`internal/mcpserver/tools_dispatch.go`). This registers:

- Two meta-tools: `ralphglasses_tool_groups` (lists available groups) and `ralphglasses_load_tool_group` (loads a group by name)
- The `core` namespace (~10 essential tools: scan, list, start, stop, status, config, logs, snapshot, pause, stop_all)

Additional namespaces (session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced) are loaded on demand via `RegisterToolGroup()`. The `loadedGroups` map on `Server` ensures idempotent loading -- calling `load_tool_group` twice for the same group is a no-op.

The `ToolGroupRegistry` (`internal/mcpserver/registry.go`) provides the `BuildAll` and `BuildAllOrdered` methods that construct groups lazily from registered `ToolGroupBuilder` instances.

## Consequences

**Positive:**

- Core startup exposes ~12 tools instead of 110+, saving thousands of context tokens
- LLM agents discover and load only the namespaces they need
- New tool groups can be added without increasing baseline startup cost
- `ToolGroupBuilder` interface allows groups to defer expensive initialization until loaded

**Negative:**

- Agents must make an extra tool call to discover and load non-core tools
- Tool names from unloaded groups are invisible to the LLM until explicitly loaded
- Testing must cover both eager and deferred registration paths

**Mitigations:**

- The `ralphglasses_tool_groups` tool returns group names with descriptions, guiding the LLM
- Integration tests (`internal/mcpserver/tools_deferred_test.go`) verify deferred loading behavior
- `RegisterAllTools()` remains available for contexts where deferred loading is unnecessary
