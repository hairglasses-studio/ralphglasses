# Task 02: finding_to_task — Convert Scratchpad Findings to Loop Tasks

**ROADMAP ID**: 9.1.1  
**Priority**: P0 | **Size**: M  
**Assigned to**: openrouter/free agent

---

## Goal

Implement the `finding_to_task` MCP tool. This converts findings from the scratchpad (`.ralph/scratchpad.jsonl`) into structured loop tasks with priority and category classification.

## Acceptance Criteria (from ROADMAP)

> **Acceptance:** `finding_to_task` converts scratchpad findings → loop task list with priority + category

## Context

The scratchpad stores findings from R&D cycle analysis. Each finding has a description, severity, and category. `finding_to_task` should:
1. Read findings from `.ralph/scratchpad.jsonl` (or a provided path)
2. Filter to unresolved/actionable findings
3. Classify each finding into a task type (bug_fix, refactor, feature, test, docs)
4. Assign priority based on severity (P0=critical, P1=high, P2=medium, P3=low)
5. Return a list of `LoopTask` objects ready for the planner

## Files to Create/Modify

### New files:
- `internal/session/finding_to_task.go` — `FindingToTask(findings []ScratchpadFinding) []LoopTask`
- `internal/session/finding_to_task_test.go` — unit tests with fixture findings

### Modified files:
- `internal/mcpserver/tools_dispatch.go` — register `finding_to_task` in `rdcycle` group
- `internal/mcpserver/handler_rdcycle.go` — `handleFindingToTask` handler

## Schema

Input (finding from scratchpad):
```go
type ScratchpadFinding struct {
    ID          string    `json:"id"`
    Timestamp   time.Time `json:"timestamp"`
    Severity    string    `json:"severity"` // "critical", "high", "medium", "low"
    Category    string    `json:"category"` // "bug", "perf", "coverage", "quality"
    Description string    `json:"description"`
    Resolved    bool      `json:"resolved"`
}
```

Output:
```go
type LoopTask struct {
    ID          string `json:"id"`
    Title       string `json:"title"`
    TaskType    string `json:"task_type"` // "bug_fix", "refactor", "test", "feature"
    Priority    int    `json:"priority"`  // 0-3
    FindingID   string `json:"finding_id"`
    Description string `json:"description"`
}
```

## Verification

```bash
go build ./...
go test ./internal/session/... -run TestFindingToTask -v
```

## Classification Rules

| Finding Category | Task Type | Severity→Priority |
|-----------------|-----------|-------------------|
| bug | bug_fix | critical→P0, high→P1, medium→P2, low→P3 |
| perf | refactor | critical→P0, high→P1, rest→P2 |
| coverage | test | all→P2 |
| quality | refactor | high→P1, rest→P2 |
| docs | docs | all→P3 |
| other | feature | all→P2 |
