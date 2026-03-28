---
name: parallel-roadmap-sprint
description: Execute researched roadmap work across parallel workstreams — quick wins, doc fixes, bug fixes, and Phase 9 scaffolding
user-invocable: true
argument-hint: [workstream-filter]
---

You are executing a parallel roadmap sprint for the ralphglasses project. The work below was identified by a comprehensive 4-phase roadmap overhaul (codebase audit, external research, scratchpad synthesis, roadmap rewrite) completed 2026-03-28.

## Pre-flight

Before launching any agents, read these files to confirm current state:
- `ROADMAP.md` (1661 lines, 619 tasks, 29 phases)
- `.ralph/roadmap_overhaul_phase3.md` (finding synthesis)
- `docs/MCP-TOOLS.md` (outdated — says 110 tools, actual is 115)

## Workstream Definitions

### WS-1: Quick Wins (Phase 0.9 bug fixes)
**Goal:** Fix the 5 highest-impact bugs from scratchpad findings.
**Files:** `internal/mcpserver/handler_*.go`, `internal/session/loop*.go`, `.ralphrc`
**No shared writes with other workstreams.**

Tasks (in priority order):
1. **QW-1: JSON format enforcement** — Find where "not valid json" retry logic lives (25.7% retry rate, 26 occurrences in improvement_patterns.json). Add structured output enforcement or response validation. Files: grep for `not valid json` across `internal/`.
2. **QW-2: Enable cascade routing by default** — `.ralphrc` has `CASCADE_ENABLED=true` but `internal/session/cascade.go` reportedly isn't wired up. Verify and fix. Acceptance: `cascade.go` is called during provider selection in `loop_steps.go` or `providers.go`.
3. **QW-6: Fix loop_gates zero-baseline** (FINDING-226/238, 2nd cycle regression) — `loop_gates` returns pass when baseline has zero values. File: grep for `loop_gates` or `LoopGate` in `internal/`. Add guard: if baseline metric is zero, return `warn` not `pass`.
4. **QW-7: Fix prompt_analyze score inflation** (FINDING-240) — Scores cluster near maximum. File: `internal/enhancer/` scoring logic. Add calibration or normalize distribution.
5. **QW-12: improvement_patterns.json rules: null** — `ConsolidatePatterns` in `internal/session/journal.go` never populates `rules`. Fix: extract rules from suggestions appearing ≥3 times.

After each fix, run `go build ./...` and `go vet ./...`. Run relevant tests with `go test ./internal/session/... ./internal/mcpserver/... ./internal/enhancer/...`.

### WS-2: Documentation reconciliation
**Goal:** Update docs to match actual codebase state found by research agents.
**Files:** `docs/MCP-TOOLS.md`, `docs/ARCHITECTURE.md`, `CLAUDE.md`
**No shared writes with other workstreams.**

Tasks:
1. **Update MCP-TOOLS.md tool count** — Change 110→115. Add missing tools: `ralphglasses_loop_prune` (loop namespace, "Prune stale loop run files by status and age") and `ralphglasses_fleet_dlq` (fleet namespace, "Dead letter queue management"). Update namespace counts: loop 9→10, fleet 6→7.
2. **Update CLAUDE.md tool count** — Change `110 tools, 13 namespaces` → `115 tools (13 namespaces + 2 meta-tools)`.
3. **Update ARCHITECTURE.md** — If it references tool counts, update. Add note about official MCP Go SDK (`modelcontextprotocol/go-sdk` v1.4.1) as migration target from `mark3labs/mcp-go`.
4. **Verify** — Run `go build ./...` to ensure no doc-only changes break anything.

### WS-3: Self-improvement pipeline fixes
**Goal:** Fix the finding→learning pipeline so the self-improvement subsystem actually learns.
**Files:** `internal/session/journal.go`, `internal/session/reflexion.go`, `internal/session/episodic.go`, `.ralph/improvement_patterns.json`
**No shared writes with other workstreams.**

Tasks:
1. **Fix ConsolidatePatterns rule extraction** — `improvement_patterns.json` has `"rules": null`. Read `journal.go`, find `ConsolidatePatterns`. It should extract rules from suggestions that appear ≥3 times. Implement this.
2. **Enrich reflexion verbal feedback** — Read `reflexion.go`. Per Reflexion paper (NeurIPS 2023), store structured verbal reflections (root cause + correction), not just failure classification. Ensure `ExtractReflection` writes actionable correction text that `FormatForPrompt` injects.
3. **Verify episodic retrieval** — Read `episodic.go`. Confirm `FindSimilar(k)` actually returns useful episodes (not empty). If similarity is broken (flat scores like relevance_scoring in research-audit), fix it.
4. **Run tests:** `go test ./internal/session/... -run "Journal|Reflexion|Episodic" -v`

### WS-4: Phase 9 scaffolding (R&D Cycle tools — Tier 1 only)
**Goal:** Scaffold the 5 critical-path R&D cycle tools as handler stubs with input schemas and documentation.
**Files:** NEW file `internal/mcpserver/handler_rdcycle.go`, modify `internal/mcpserver/tools_builders_misc.go`
**No shared writes with other workstreams except the builder registration.**

Tools to scaffold (handler + schema + builder registration):
1. **`finding_to_task`** — Input: `finding_id` (string), `scratchpad_name` (string). Output: task spec JSON with title, description, difficulty_score, provider_hint, estimated_cost. Logic: read scratchpad, find finding by ID, generate task spec.
2. **`cycle_baseline`** — Input: `repo` (string), `metrics` (string array, optional). Output: baseline_id, snapshot of current metrics (test count, coverage, build time, loop P50/P95). Logic: run `go test -count=1`, parse coverage, record to `.ralph/cycle_baselines/`.
3. **`cycle_plan`** — Input: `previous_cycle_id` (string, optional), `max_tasks` (int), `budget` (float). Output: prioritized task list from unresolved findings + roadmap items. Logic: read scratchpads, filter unresolved, sort by recurrence count and severity.
4. **`cycle_merge`** — Input: `worktree_paths` (string array), `conflict_strategy` (string: "ours"/"theirs"/"manual"). Output: merge result, conflicts list. Logic: git merge-tree or sequential merge with conflict detection.
5. **`cycle_schedule`** — Input: `cron_expr` (string), `cycle_config` (object). Output: schedule_id. Logic: write schedule to `.ralph/schedules/`, implement cron parsing.

For each tool:
- Write the handler function in `handler_rdcycle.go` with real input parsing and a TODO body that returns a stub response
- Register in a new `rdcycle` tool group in `tools_builders_misc.go`
- Add to the deferred loading registry
- Ensure `go build ./...` passes

### WS-5: Test coverage for new code
**Goal:** After WS-1, WS-3, and WS-4 complete, write tests for all new/modified code.
**Depends on:** WS-1, WS-3, WS-4 (run AFTER they merge)
**Files:** `*_test.go` files adjacent to modified packages

Tasks:
1. Test each QW fix from WS-1 (at minimum: regression test for the specific bug)
2. Test ConsolidatePatterns rule extraction from WS-3
3. Test Phase 9 handler stubs from WS-4 (schema validation, error cases)
4. Run full suite: `go test ./... -count=1 -timeout 120s`
5. Run coverage: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1`

## Execution Plan

```
Time →

Parallel:  WS-1 (bugs)     WS-2 (docs)     WS-3 (self-improve)     WS-4 (Phase 9 scaffold)
           ─────────────    ──────────────   ────────────────────     ───────────────────────
                    ↘            ↓                   ↘                        ↙
                     └───────────┴────────────────────┴───────────────────────┘
                                          ↓
Sequential:                         WS-5 (tests)
                                    ─────────────
                                          ↓
                                    Commit & verify
```

Launch WS-1 through WS-4 as parallel agents (each in its own worktree). After all 4 complete, launch WS-5 on the merged result.

## Constraints

- Each workstream operates on non-overlapping files (except WS-4 touches `tools_builders_misc.go` which no other WS modifies)
- Do NOT modify `ROADMAP.md` — it was just overhauled
- Do NOT add features beyond what's specified — no speculative abstractions
- Every workstream must end with `go build ./...` and `go vet ./...` passing
- If $ARGUMENTS contains a workstream filter (e.g., "ws-1" or "ws-1,ws-3"), only run those workstreams

## Post-Sprint

After all workstreams complete:
1. Run `go test ./... -count=1 -timeout 120s` on the merged result
2. Update `.ralph/tool_improvement_scratchpad.md` with any new findings
3. Report: which QW items are resolved, which Phase 9 tools are scaffolded, coverage delta
