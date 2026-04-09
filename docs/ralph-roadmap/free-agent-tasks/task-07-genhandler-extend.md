# Task 07: Extend Handler Codegen Tool

**ROADMAP ID**: 1.2.5.4  
**Priority**: P2 | **Size**: M  
**Assigned to**: openrouter/free agent

---

## Goal

Extend `tools/genhandler` to generate both the handler stub AND the test file skeleton for new MCP tools.

## Acceptance Criteria

> **Acceptance:** `go run ./tools/genhandler -name foo_bar -group core` generates `handler_core_foo_bar.go` + `handler_core_foo_bar_test.go`

## Context

`tools/genhandler/main.go` already exists and generates handler stubs. It needs to also emit a companion `_test.go` with basic table-driven test structure.

## Template for Generated Test

```go
package mcpserver

import (
    "context"
    "testing"
)

func TestHandle{{Name}}(t *testing.T) {
    t.Parallel()
    srv, _ := setupTestServer(t)

    tests := []struct{
        name    string
        args    map[string]any
        wantErr bool
    }{
        {"missing required param", map[string]any{}, true},
        {"valid request", map[string]any{"repo_path": t.TempDir()}, false},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            res, err := srv.handle{{Name}}(context.Background(), makeRequest(tc.args))
            if (err != nil) != tc.wantErr {
                t.Errorf("err = %v, wantErr %v", err, tc.wantErr)
            }
            if !tc.wantErr && res.IsError {
                t.Errorf("unexpected error: %s", getResultText(res))
            }
        })
    }
}
```

## Files to Modify

- `tools/genhandler/main.go` — add `-test` flag (default: true) that emits the `_test.go` alongside the handler

## Verification

```bash
go build ./tools/genhandler/...
go run ./tools/genhandler -name my_tool -group core
ls internal/mcpserver/handler_core_my_tool.go
ls internal/mcpserver/handler_core_my_tool_test.go
go test ./tools/genhandler/... -v
```
