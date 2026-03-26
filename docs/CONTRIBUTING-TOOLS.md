# Contributing MCP Tools

Guide for adding new tools to the ralphglasses MCP server (110+ tools across 13 namespaces with deferred loading).

## Architecture Overview

All MCP tools live in `internal/mcpserver/`. The system uses:

- **Tool groups** (`ToolGroup`) — namespaced collections of related tools (e.g., `session`, `fleet`, `repo`)
- **Deferred loading** — only the `core` group (10 tools) loads at startup; others load on demand via `ralphglasses_load_tool_group`
- **Annotations** — MCP behavioral hints (`ReadOnlyHint`, `DestructiveHint`, etc.) in `annotations.go`
- **Output schemas** — JSON Schema definitions for structured responses in `schemas.go`
- **Structured errors** — machine-parseable error codes via `codedError()` in `errors.go`

Key files:

| File | Purpose |
|------|---------|
| `tools_builders.go` | Tool definitions (`mcp.NewTool`) and group builders |
| `tools_dispatch.go` | Registration, deferred loading, group management handlers |
| `tools.go` | `Server` struct, helper functions (`getStringArg`, `jsonResult`, etc.) |
| `annotations.go` | `ToolAnnotations` map (behavioral hints per tool) |
| `schemas.go` | `OutputSchemas` map (JSON Schema output definitions) |
| `errors.go` | `ErrorCode` constants and `codedError()` helper |
| `handler_*.go` | Handler implementations grouped by domain |

## Quick Start Checklist

1. Pick a tool group in `tools_builders.go` (or create a new one)
2. Add a `mcp.NewTool()` definition with parameters to the group's builder function
3. Write a handler method `func (s *Server) handleYourTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)` in the appropriate `handler_*.go` file
4. Add an annotation entry in `annotations.go`
5. (Optional) Add an output schema in `schemas.go` for high-value tools
6. Write tests in the corresponding `handler_*_test.go` file
7. Update `docs/MCP-TOOLS.md` with the new tool
8. Run `go build ./...` and `go test ./internal/mcpserver/...`

## Step-by-step Guide

### 1. Define the Tool in a Group Builder

Open `tools_builders.go` and find the appropriate `build*Group()` method. Add a `ToolEntry` to its `Tools` slice:

```go
{mcp.NewTool("ralphglasses_repo_stats",
    mcp.WithDescription("Get statistics for a repo: file count, line count, commit count"),
    mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
    mcp.WithNumber("days", mcp.Description("Lookback period in days (default 30)")),
), s.handleRepoStats},
```

Parameter helpers from `mcp-go`:
- `mcp.WithString(name, ...opts)` — string parameter
- `mcp.WithNumber(name, ...opts)` — numeric parameter
- `mcp.WithBoolean(name, ...opts)` — boolean parameter
- `mcp.Required()` — marks the parameter as required
- `mcp.Description(text)` — parameter description

### 2. Write the Handler

Create or edit the appropriate `handler_*.go` file. Handlers are methods on `*Server`:

```go
func (s *Server) handleRepoStats(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // 1. Extract and validate parameters
    name := getStringArg(req, "repo")
    if name == "" {
        return codedError(ErrInvalidParams, "repo name required"), nil
    }
    if err := ValidateRepoName(name); err != nil {
        return codedError(ErrRepoNameInvalid, fmt.Sprintf("invalid repo name: %v", err)), nil
    }

    // 2. Lazy-scan repos if needed
    if s.reposNil() {
        if err := s.scan(); err != nil {
            return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
        }
    }

    // 3. Find the repo
    r := s.findRepo(name)
    if r == nil {
        return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
    }

    // 4. Business logic
    days := int(getNumberArg(req, "days", 30))
    stats := computeStats(r.Path, days)

    // 5. Return structured JSON
    return jsonResult(map[string]any{
        "repo":         r.Name,
        "files":        stats.Files,
        "lines":        stats.Lines,
        "commits":      stats.Commits,
        "lookback_days": days,
    }), nil
}
```

### 3. Add Annotations

In `annotations.go`, add an entry to the `ToolAnnotations` map:

```go
"ralphglasses_repo_stats": {Title: "Repo Stats", ReadOnlyHint: boolPtr(true)},
```

Available annotation fields:
- `Title` — human-readable tool name
- `ReadOnlyHint` — tool does not modify state
- `DestructiveHint` — tool may perform destructive updates
- `IdempotentHint` — repeated calls with same args have no additional effect
- `OpenWorldHint` — tool interacts with external systems (filesystem, network, subprocesses)

### 4. Add Output Schema (Optional)

For tools with stable structured output, add a JSON Schema in `schemas.go`:

```go
"ralphglasses_repo_stats": {
    "type": "object",
    "properties": map[string]any{
        "repo":          map[string]any{"type": "string"},
        "files":         map[string]any{"type": "integer"},
        "lines":         map[string]any{"type": "integer"},
        "commits":       map[string]any{"type": "integer"},
        "lookback_days": map[string]any{"type": "integer"},
    },
},
```

Schemas are automatically wired to tools via `applyToolMetadata()` in `tools_dispatch.go`.

### 5. Write Tests

Tests call handler methods directly on a `*Server` created with `setupTestServer()`:

```go
func TestHandleRepoStats(t *testing.T) {
    t.Parallel()

    t.Run("missing repo param", func(t *testing.T) {
        t.Parallel()
        srv, _ := setupTestServer(t)

        result, err := srv.handleRepoStats(context.Background(), makeRequest(map[string]any{}))
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if !result.IsError {
            t.Fatal("expected error for missing repo")
        }
        code := parseErrorCode(t, getResultText(result))
        if code != string(ErrInvalidParams) {
            t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
        }
    })

    t.Run("valid repo", func(t *testing.T) {
        t.Parallel()
        srv, _ := setupTestServer(t)

        result, err := srv.handleRepoStats(context.Background(), makeRequest(map[string]any{
            "repo": "test-repo",
        }))
        if err != nil {
            t.Fatalf("unexpected error: %v", err)
        }
        if result.IsError {
            t.Fatalf("unexpected tool error: %s", getResultText(result))
        }

        // Parse and validate the JSON response.
        var resp map[string]any
        if err := json.Unmarshal([]byte(getResultText(result)), &resp); err != nil {
            t.Fatalf("invalid JSON: %v", err)
        }
        if resp["repo"] != "test-repo" {
            t.Errorf("repo = %v, want test-repo", resp["repo"])
        }
    })
}
```

Test helpers (defined in `tools_core_test.go`):
- `setupTestServer(t)` — creates a `*Server` with a temp dir containing a `test-repo` with `.ralph/` structure
- `makeRequest(map[string]any)` — builds a `mcp.CallToolRequest` from argument map
- `getResultText(result)` — extracts text content from a `*mcp.CallToolResult`
- `parseErrorCode(t, text)` — extracts `error_code` from a coded error response (in `handler_session_test.go`)

## Handler Patterns

### Parameter Extraction

```go
name := getStringArg(req, "repo")          // returns "" if missing
days := getNumberArg(req, "days", 30)       // returns default if missing
verbose := getBoolArg(req, "verbose")       // returns false if missing
```

### Repo Lookup (Common Boilerplate)

Most repo-scoped tools follow this pattern:

```go
if s.reposNil() {
    if err := s.scan(); err != nil {
        return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
    }
}
r := s.findRepo(name)
if r == nil {
    return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
}
```

### Response Helpers

```go
return jsonResult(myStruct)                 // JSON-serialized struct or map
return textResult("plain text response")    // raw text
return emptyResult("sessions")              // standardized empty-collection response
return codedError(ErrInvalidParams, "msg")  // structured error with code
```

### Error Codes

Use constants from `errors.go`. Common ones:

| Code | When to use |
|------|-------------|
| `ErrInvalidParams` | Missing or invalid parameters |
| `ErrRepoNotFound` | Repo name does not match any discovered repo |
| `ErrRepoNameInvalid` | Repo name fails validation (path traversal, etc.) |
| `ErrSessionNotFound` | Session ID not found |
| `ErrNotRunning` | Required subsystem not initialized |
| `ErrInternal` | Unexpected internal failure |
| `ErrFilesystem` | File I/O error |
| `ErrScanFailed` | Repo scan failure |

Handlers always return `(result, nil)` — errors are communicated through `codedError()`, not the Go error return. The error return is reserved for transport-level failures.

## Tool Groups and Deferred Loading

### Existing Groups

Defined in `ToolGroupNames` in `tools.go`:

```
core, session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced, eval, fleet_h, observability
```

### Adding a New Group

1. Add the group name to `ToolGroupNames` in `tools.go`
2. Create a `build*Group()` method in `tools_builders.go`
3. Call it from `buildToolGroups()`

```go
// In tools.go — add to ToolGroupNames:
var ToolGroupNames = []string{
    "core", "session", "loop", /* ..., */ "mygroup",
}

// In tools_builders.go — add builder call:
func (s *Server) buildToolGroups() []ToolGroup {
    return []ToolGroup{
        // ... existing groups ...
        s.buildMyGroup(),
    }
}

func (s *Server) buildMyGroup() ToolGroup {
    return ToolGroup{
        Name:        "mygroup",
        Description: "My new tool group: does X, Y, Z",
        Tools: []ToolEntry{
            {mcp.NewTool("ralphglasses_my_tool",
                mcp.WithDescription("Does something useful"),
                mcp.WithString("param", mcp.Required(), mcp.Description("A parameter")),
            ), s.handleMyTool},
        },
    }
}
```

### How Deferred Loading Works

1. `Server.Register()` checks `DeferredLoading` flag
2. If true, only `core` group + two meta-tools (`ralphglasses_tool_groups`, `ralphglasses_load_tool_group`) are registered
3. When a client calls `ralphglasses_load_tool_group` with a group name, `RegisterToolGroup()` builds that group and calls `srv.AddTool()` for each entry
4. `applyToolMetadata()` automatically wires annotations and output schemas during registration
5. `loadedGroups` map tracks what has been loaded (idempotent)

## Naming Conventions

- Tool names: `ralphglasses_{group}_{action}` (e.g., `ralphglasses_repo_stats`)
- Handler methods: `handle{Group}{Action}` in PascalCase (e.g., `handleRepoStats`)
- Handler files: `handler_{group}.go` (e.g., `handler_repo.go`)
- Test files: `handler_{group}_test.go` (e.g., `handler_repo_test.go`)

## Validation

The codebase includes input validation helpers in `validate.go`:
- `ValidateRepoName(name)` — rejects path traversal, empty names, OS-invalid chars
- `ValidateStringLength(s, max, fieldName)` — enforces max length on user input
- `ValidateProvider(provider)` — checks provider is claude/gemini/codex

Always validate user input before using it in filesystem operations or subprocess calls.
