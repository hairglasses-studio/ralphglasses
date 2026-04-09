# Task 06: 8.3.2 — Built-in Workflow Definitions

**ROADMAP ID**: 8.3.2  
**Priority**: P1 | **Size**: M  
**Assigned to**: openrouter/free agent

---

## Goal

Add 3 built-in named workflows to the workflow engine: "fix-all-lint", "increase-coverage", and "migrate-dependency".

## Acceptance Criteria (from ROADMAP)

> **Acceptance:** `workflow_run name=fix-all-lint` runs a 3-step loop: lint → fix → verify

## Context

The `internal/workflow/` package exists with a workflow engine. Built-in workflows are pre-defined multi-step loop sequences that agents can invoke by name. They should be registered at startup.

## Workflow Definitions

### `fix-all-lint`
Steps:
1. `observe` — run `golangci-lint run ./...`, capture output
2. `fix` — loop task: "Fix all lint issues found in the observe step"
3. `verify` — run `golangci-lint run ./...`, assert exit 0

### `increase-coverage`
Steps:
1. `baseline` — run `go test -coverprofile=cov.out ./...`, extract coverage %
2. `fix` — loop task: "Add tests to reach 85% coverage for packages below threshold"
3. `verify` — run `go test -coverprofile=cov.out ./...`, assert coverage >= baseline + 2%

### `migrate-dependency`
Steps:
1. `analyze` — `go list -m all | grep <dep>`, find all usages
2. `migrate` — loop task: "Replace all usages of <dep> with <replacement>"
3. `verify` — `go build ./...` and `go test ./...`

## Files to Create/Modify

### New files:
- `internal/workflow/builtins.go` — `RegisterBuiltinWorkflows(registry *WorkflowRegistry)`
- `internal/workflow/builtins_test.go` — tests that builtins register and run step 1

### Modified files:
- `internal/workflow/workflow.go` — call `RegisterBuiltinWorkflows` on init/startup
- `internal/mcpserver/handler_workflow.go` (or similar) — ensure `workflow_run` uses the registry

## Verification

```bash
go build ./...
go test ./internal/workflow/... -run TestBuiltinWorkflows -v
```
