# S7 — MCP Namespace Restructure Plan

**Date**: 2026-04-04
**Branch context**: self-improve-review-20260404-165828
**Status**: Draft

---

## 1. Problem Statement

The `advanced` namespace currently contains 24 tools spanning 7+ unrelated domains: remote control, event bus queries, HITL scoring, autonomy management, feedback/provider intelligence, journals, workflows, ML model status (bandit, confidence calibration), and circuit-breaker operations. This violates the org-wide MCP guideline: one clear user intent per tool group, with high-signal descriptions so clients can load only what they need.

Additionally, 7 tools are confirmed misplaced in namespaces that don't match their semantic domain.

---

## 2. Current `advanced` Namespace Inventory (24 tools)

Source: `internal/mcpserver/tools_builders_misc.go` `buildAdvancedGroup()` (line 154)

| # | Tool | Domain |
|---|------|--------|
| 1 | `ralphglasses_rc_status` | Remote control |
| 2 | `ralphglasses_rc_send` | Remote control |
| 3 | `ralphglasses_rc_read` | Remote control |
| 4 | `ralphglasses_rc_act` | Remote control |
| 5 | `ralphglasses_event_list` | Event bus |
| 6 | `ralphglasses_event_poll` | Event bus |
| 7 | `ralphglasses_hitl_score` | HITL / autonomy |
| 8 | `ralphglasses_hitl_history` | HITL / autonomy |
| 9 | `ralphglasses_autonomy_level` | Autonomy control |
| 10 | `ralphglasses_supervisor_status` | Autonomy control |
| 11 | `ralphglasses_autonomy_decisions` | Autonomy control |
| 12 | `ralphglasses_autonomy_override` | Autonomy control |
| 13 | `ralphglasses_feedback_profiles` | Provider intelligence |
| 14 | `ralphglasses_provider_recommend` | Provider intelligence |
| 15 | `ralphglasses_tool_benchmark` | Provider intelligence |
| 16 | `ralphglasses_journal_read` | Journals |
| 17 | `ralphglasses_journal_write` | Journals |
| 18 | `ralphglasses_journal_prune` | Journals |
| 19 | `ralphglasses_workflow_define` | Workflows |
| 20 | `ralphglasses_workflow_run` | Workflows |
| 21 | `ralphglasses_workflow_delete` | Workflows |
| 22 | `ralphglasses_bandit_status` | ML model status |
| 23 | `ralphglasses_confidence_calibration` | ML model status |
| 24 | `ralphglasses_circuit_reset` | Circuit breaker ops |

---

## 3. Proposed Split

### 3a. New namespace: `rc`

**Description**: Remote control — send prompts, read output, and act on sessions from mobile or scripted contexts.

| Tool | Notes |
|------|-------|
| `ralphglasses_rc_status` | Compact fleet overview for mobile |
| `ralphglasses_rc_send` | Send prompt to repo, auto-stop/launch |
| `ralphglasses_rc_read` | Read recent output from active session |
| `ralphglasses_rc_act` | Quick fleet action: stop, pause, resume, retry |

**Rationale**: The RC tools form a self-contained mobile/headless remote-control surface. They are tightly coupled (rc_send → rc_read → rc_act is a workflow loop) and frequently used together. Separating them from autonomy and workflow tools aids discoverability.

**Builder**: New `buildRCGroup()` function in `tools_builders_misc.go` or a new `tools_builders_rc.go`.

---

### 3b. New namespace: `autonomy`

**Description**: Autonomy management — view/set autonomy level, inspect supervisor status, review and override autonomous decisions, track HITL events.

| Tool | Notes |
|------|-------|
| `ralphglasses_autonomy_level` | View/set autonomy level 0–3 |
| `ralphglasses_supervisor_status` | Autonomous supervisor running state |
| `ralphglasses_autonomy_decisions` | Recent autonomous decisions with rationale |
| `ralphglasses_autonomy_override` | Override/reverse an autonomous decision |
| `ralphglasses_hitl_score` | HITL score: manual vs autonomous ratio |
| `ralphglasses_hitl_history` | Recent HITL events |

**Rationale**: HITL tools are the measurement side of autonomy (how often humans intervene). They belong alongside autonomy level and decisions rather than in `advanced`. Together these 6 tools cover the full autonomy control plane.

**Builder**: New `buildAutonomyGroup()` function.

---

### 3c. New namespace: `workflow`

**Description**: Workflow automation — define, run, and delete multi-step YAML workflows that sequence agent sessions.

| Tool | Notes |
|------|-------|
| `ralphglasses_workflow_define` | Define a workflow as YAML |
| `ralphglasses_workflow_run` | Execute a defined workflow |
| `ralphglasses_workflow_delete` | Delete a workflow definition |

**Rationale**: Workflows are a distinct primitive (YAML-defined multi-step sequences) unrelated to RC, autonomy, or journals. 3 tools is small but matches the `plugin` namespace precedent (4 tools). If `event_list`/`event_poll` are also split out, they could be co-located here as "workflow plumbing" or form their own `events` namespace.

**Builder**: New `buildWorkflowGroup()` function.

---

### 3d. Residual `advanced` namespace (what stays)

After the splits above, `advanced` retains tools that don't yet justify their own namespace:

| Tool | Domain | Future home |
|------|--------|-------------|
| `ralphglasses_event_list` | Event bus | `events` namespace (future) |
| `ralphglasses_event_poll` | Event bus | `events` namespace (future) |
| `ralphglasses_feedback_profiles` | Provider intelligence | `intelligence` or merge into `fleet_h` |
| `ralphglasses_provider_recommend` | Provider intelligence | `intelligence` or merge into `fleet_h` |
| `ralphglasses_tool_benchmark` | Benchmarking | `eval` namespace (fits alongside eval_ab_test etc.) |
| `ralphglasses_journal_read` | Journals | `observability` (journals are loop artifacts) |
| `ralphglasses_journal_write` | Journals | `observability` |
| `ralphglasses_journal_prune` | Journals | `observability` |
| `ralphglasses_bandit_status` | ML model status | `intelligence` or `fleet_h` |
| `ralphglasses_confidence_calibration` | ML model status | `intelligence` or `fleet_h` |
| `ralphglasses_circuit_reset` | Operations | `core` (circuit breakers are operational) |

**Immediate recommendation**: Residual `advanced` should drop from 24 to 11 tools after the three splits. A follow-up pass can further dissolve it by moving journals to `observability`, `tool_benchmark` to `eval`, events to a new `events` group, and ML-status tools into `fleet_h`.

---

## 4. Misplaced Tools (Current vs Correct Namespace)

These tools are currently registered in namespaces that don't match their semantic domain.

| Tool | Current Namespace | Correct Namespace | Evidence |
|------|------------------|-------------------|----------|
| `ralphglasses_session_handoff` | `loop` (tools_builders_session.go line 197) | `session` | Operates on sessions (source_session_id param), not loop state machines. Annotation file marks it in `session` section (line 43). |
| `ralphglasses_worktree_create` | `observability` (tools_builders_misc.go line 583) | `repo` or `loop` | Worktree creation is a repo/workspace operation, not observability. Annotation marked in `observability` section (line 185). |
| `ralphglasses_worktree_cleanup` | `observability` (tools_builders_misc.go line 590) | `repo` or `loop` | Same as above — cleanup of loop worktrees is a loop or repo lifecycle operation. |
| `ralphglasses_loop_await` | `observability` (tools_builders_misc.go line 538) | `loop` | Awaiting loop/session completion is a loop lifecycle primitive, not an observation query. Annotation marks it in `observability` (line 180). |
| `ralphglasses_loop_poll` | `observability` (tools_builders_misc.go line 545) | `loop` | Same as loop_await — non-blocking status check belongs in `loop`. |
| `ralphglasses_provider_benchmark` | `rdcycle` (tools_builders_misc.go line 403) | `eval` or `advanced/intelligence` | Benchmarks providers using a standardized task suite — that's evaluation/intelligence, not R&D cycle management. Annotation marks it in `observability` (line 138 comment says `advanced`). |
| `ralphglasses_observation_correlate` | `rdcycle` (tools_builders_misc.go line 397) | `observability` | Correlates observations to git commits — pure observation query tool, should be in observability with `observation_query` and `observation_summary`. Annotation marks it in `observability` (line 200). |

**Annotation mismatches**: `provider_benchmark` and `observation_correlate` both appear in the `annotations.go` file under comments that suggest a different home than where they are actually registered in the builder.

---

## 5. Files That Need Modification

### Core registration files

| File | Change |
|------|--------|
| `internal/mcpserver/tools.go` | Add `"rc"`, `"autonomy"`, `"workflow"` to `ToolGroupNames` slice (line 127). Preserve ordering. |
| `internal/mcpserver/tools_builders.go` | Register three new `FuncBuilder` entries in `defaultRegistry()` for `rc`, `autonomy`, `workflow`. |
| `internal/mcpserver/tools_builders_misc.go` | Remove tools from `buildAdvancedGroup()`. Add `buildRCGroup()`, `buildAutonomyGroup()`, `buildWorkflowGroup()`. Move `session_handoff` from `buildLoopGroup()` to `buildSessionGroup()`. Move `loop_await`/`loop_poll` from `buildObservabilityGroup()` to `buildLoopGroup()`. Move `worktree_create`/`worktree_cleanup` to their target namespace builder. Move `observation_correlate` to `buildObservabilityGroup()`. Move `provider_benchmark` to `buildEvalGroup()` or a new builder. |
| `internal/mcpserver/tools_builders_session.go` | Add `session_handoff` entry to `buildSessionGroup()` (currently in loop builder). |
| `internal/mcpserver/annotations.go` | Add section comments for `rc`, `autonomy`, `workflow`. Reassign tools from `// ── advanced ──` to their new sections. Fix misplaced tool comments (`loop_await`/`loop_poll` move from `observability` to `loop`; `worktree_*` move; `session_handoff` already in correct section). |
| `docs/MCP-TOOLS.md` | Update tool table — new namespaces, corrected group assignments. |
| `docs/ARCHITECTURE.md` | Update namespace list. |

### Optional but recommended

| File | Change |
|------|--------|
| `internal/mcpserver/tools_builders_misc.go` | Consider splitting into `tools_builders_advanced.go`, `tools_builders_rdcycle.go` (file is already 600+ lines). |
| `internal/mcpserver/tools_builders_rc.go` | New file for `buildRCGroup()` (cleaner separation). |

---

## 6. Migration Risk

### Risk level: **Low-Medium**

**Reasoning**:

1. **Tool names do not change.** All `ralphglasses_*` tool names stay identical. MCP clients that call tools by name are unaffected.

2. **Breaking change only for deferred loading.** The namespace (group) name is only surfaced in two ways:
   - `ralphglasses_tool_groups` — lists available namespaces
   - `ralphglasses_load_tool_group` — loads a namespace by name

   Any client or script that calls `ralphglasses_load_tool_group("advanced")` expecting it to include RC, autonomy, or workflow tools will need to be updated to load the new namespace names instead. This includes:
   - The `CLAUDE.md` skill (`ralphglasses-ops`) if it documents the `advanced` group
   - Any sweep scripts or automation that load `advanced` explicitly
   - `docs/MCP-TOOLS.md` references to the `advanced` group

3. **`ToolGroupNames` ordering matters for registration.** New entries must be appended or inserted consistently. The `defaultRegistry()` build order determines which groups are available.

4. **Annotation file stays consistent.** The annotations map is keyed by tool name, not namespace — no functional impact. Only the section comments need updating.

5. **No handler changes.** All handler functions (`handleRCSend`, `handleAutonomyLevel`, etc.) stay exactly where they are. Only the builder scaffolding moves.

### Estimated breaking change surface

- Clients using `RegisterAllTools` (non-deferred mode): **zero impact**
- Clients using `RegisterCoreTools` + explicit group loads: **must update group names** for `advanced` → `rc`/`autonomy`/`workflow`
- Grep for `"advanced"` in MCP client configs and scripts: `grep -r '"advanced"' ~/hairglasses-studio`

---

## 7. Estimated Effort

| Task | Effort |
|------|--------|
| Split `buildAdvancedGroup` into 3 new builders + residual | 45 min |
| Move 7 misplaced tools to correct builders | 30 min |
| Update `ToolGroupNames` and `defaultRegistry` | 10 min |
| Update `annotations.go` section comments | 15 min |
| Update `docs/MCP-TOOLS.md` | 30 min |
| Update `docs/ARCHITECTURE.md` | 10 min |
| Run build + vet + test to verify no breakage | 15 min |
| **Total** | **~2.5 hours** |

Follow-up pass (dissolve residual `advanced`):

| Task | Effort |
|------|--------|
| Move journals → `observability` | 20 min |
| Move `tool_benchmark` → `eval` | 10 min |
| Move event tools → new `events` namespace or `workflow` | 20 min |
| Move ML status tools → `fleet_h` or new `intelligence` namespace | 20 min |
| Move `circuit_reset` → `core` | 10 min |
| **Total follow-up** | **~1.5 hours** |

---

## 8. Implementation Order (Recommended)

1. Create `buildRCGroup()` — smallest, cleanest split (4 tools, zero ambiguity)
2. Create `buildAutonomyGroup()` — 6 tools, clear semantic boundary
3. Create `buildWorkflowGroup()` — 3 tools
4. Update `ToolGroupNames` and `defaultRegistry`
5. Fix 7 misplaced tools (highest correctness value, low risk)
6. Update annotations and docs
7. Run `go build ./... && go vet ./... && go test ./... -count=1`
8. Schedule follow-up pass to fully dissolve residual `advanced`
