# Ralphglasses Skills

> 223 tools total: 218 grouped tools across 30 tool groups plus 5 always-available management tools

## Table of Contents

- [management](#management) (5 tools) — Always-available discovery and contract tools registered ahead of deferred tool-group loading.
- [core](#core) (20 tools) — Essential fleet management: scan, list, start, stop, pause, logs, config
- [session](#session) (19 tools) — LLM session lifecycle: launch, list, status, resume, stop, budget, retry, output, tail, diff, compare, errors, export
- [loop](#loop) (12 tools) — Perpetual development loops: start, status, step, stop, benchmark, baseline, gates, self-test, self-improve
- [prompt](#prompt) (9 tools) — Prompt enhancement: analyze, enhance, lint, improve, classify, should_enhance, templates, template_fill
- [fleet](#fleet) (12 tools) — Fleet operations: fleet_status, analytics, submit, budget, workers, dlq, marathon_dashboard, capacity_plan
- [repo](#repo) (9 tools) — Repo management: health, optimize, scaffold, claudemd_check, snapshot
- [roadmap](#roadmap) (6 tools) — Roadmap automation: parse, analyze, research, expand, export
- [team](#team) (6 tools) — Agent teams and definitions: team_create, team_status, team_delegate, agent_define, agent_list, agent_compose
- [tenant](#tenant) (5 tools) — Workspace tenant administration: list, create, status, rotate trigger token, and batch role leaderboards
- [awesome](#awesome) (5 tools) — Awesome-list research: fetch, analyze, diff, report, sync
- [advanced](#advanced) (5 tools) — Advanced: journals, tool benchmark, circuit breaker reset
- [events](#events) (2 tools) — Fleet event bus: query and poll for session, cost, loop, and circuit events
- [feedback](#feedback) (6 tools) — Provider feedback: performance profiles, recommendations, bandit stats, confidence calibration
- [eval](#eval) (7 tools) — Offline evaluation: counterfactual analysis, Bayesian A/B testing, frequentist significance testing, changepoint detection
- [fleet_h](#fleet-h) (5 tools) — Fleet intelligence: blackboard coordination, A2A task delegation, cost forecasting, cost recommendations
- [observability](#observability) (14 tools) — Observation queries, scratchpad automation, loop wait/poll, coverage, cost estimation, merge verification
- [rdcycle](#rdcycle) (16 tools) — R&D cycle automation — finding-to-task conversion, cycle planning, baselines, merging, and scheduling
- [plugin](#plugin) (4 tools) — Plugin management: list, info, enable, disable registered plugins
- [sweep](#sweep) (8 tools) — Cross-repo audit sweeps: generate optimized prompts, fan-out sessions, monitor, nudge stalled sessions, schedule recurring checks
- [rc](#rc) (6 tools) — Remote control — send prompts, read output, and act on sessions from mobile or scripted contexts
- [autonomy](#autonomy) (8 tools) — Autonomy management — view/set autonomy level, inspect supervisor status, review and override autonomous decisions, track HITL events
- [workflow](#workflow) (3 tools) — Workflow automation — define, run, and delete multi-step YAML workflows that sequence agent sessions
- [docs](#docs) (8 tools) — Docs repo integration: search research, check existing, write findings, push changes, meta-roadmap coordination, cross-repo roadmap management
- [recovery](#recovery) (6 tools) — Emergency session recovery: triage killed sessions, salvage partial output, generate recovery plans, batch re-launch, write incident reports, discover orphaned sessions
- [promptdj](#promptdj) (6 tools) — Prompt DJ: quality-aware prompt routing to optimal providers
- [a2a](#a2a) (4 tools) — A2A protocol integration: discover agents, send tasks, check status, export agent card
- [trigger](#trigger) (2 tools) — External agent triggering and cron-based scheduling
- [approval](#approval) (3 tools) — Human-in-the-loop approval: request, resolve, list pending approvals (Factor 7: Contact Humans with Tool Calls)
- [context](#context) (1 tools) — Context window budget monitoring: track token usage per session
- [prefetch](#prefetch) (1 tools) — Deterministic context pre-fetching: inspect registered hooks

---

## management

Always-available discovery and contract tools registered ahead of deferred tool-group loading.

### `ralphglasses_tool_groups`

List available tool groups for deferred loading, or search the live workflow and skill catalog when query/include flags are provided.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_skills` | boolean |  | Include matching skill catalog entries in the response |
| `include_workflows` | boolean |  | Include matching workflow catalog entries in the response |
| `limit` | number |  | Optional per-section result limit for filtered search responses |
| `query` | string |  | Optional search query across tool groups, workflow names, skill names, descriptions, and key tools |
| `tool_group` | string |  | Optional tool-group filter (for example "repo", "fleet", or "management") |

**Example:**

```json
{
  "tool": "ralphglasses_tool_groups"
}
```

### `ralphglasses_load_tool_group`

Load all tools in a named group (core, session, loop, prompt, fleet, repo, roadmap, team, tenant, awesome, advanced, events, feedback, eval, fleet_h, observability, rdcycle, plugin, sweep, rc, autonomy, workflow, docs, recovery, promptdj, a2a, trigger, approval, context, prefetch). Use ralphglasses_tool_groups or ralph:///catalog/tool-groups first if you need discovery.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `group` | string | yes | Tool group name to load |

**Example:**

```json
{
  "arguments": {
    "group": "..."
  },
  "tool": "ralphglasses_load_tool_group"
}
```

### `ralphglasses_skill_export`

Generate SKILL.md documentation from all registered tool groups. Returns markdown or JSON.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `format` | string |  | Output format: "markdown" (default) or "json" |
| `group` | string |  | Filter to a specific tool group (for example "core", "session", or "management") |

**Example:**

```json
{
  "tool": "ralphglasses_skill_export"
}
```

### `ralphglasses_server_health`

Show the active ralphglasses MCP contract shape, including available tool groups, loaded groups, and resource/prompt coverage.

**Example:**

```json
{
  "tool": "ralphglasses_server_health"
}
```

### `ralphglasses_autobuild_ledger_append`

Append an entry to the machine-readable autobuild execution ledger and emit telemetry.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `patch_id` | string | yes | Unique identifier for the autobuild tranche |
| `status` | string | yes | Current status: "planned", "in_progress", "completed", "blocked", "cancelled", "deferred" |
| `acceptance_condition` | string |  | Comma-separated list of conditions for completion |
| `changes` | string |  | Comma-separated list of changes applied |
| `closure_state` | string |  | Final state: "completed", "blocked", "cancelled", "deferred" |
| `closure_summary` | string |  | High-signal outcome summary |
| `next_recommended_patch` | string |  | ID of the next recommended patch |
| `prevented_failure_class` | string |  | Comma-separated failure classes prevented |
| `recommended_entry_surface` | string |  | Resource, doc, command, or tool to start with |
| `remote_main_verified` | boolean |  | Whether the trigger signal was verified against remote main |
| `repo_owned_scope` | string |  | Comma-separated list of items in scope |
| `stop_condition` | string |  | Comma-separated list of boundaries for the tranche |
| `trigger_source` | string |  | Signal source resource or path |
| `trigger_summary` | string |  | Why this tranche was opened |
| `trigger_type` | string |  | Signal type: "adoption", "integrity", "ci", "manual", "other" |

**Example:**

```json
{
  "arguments": {
    "patch_id": "...",
    "status": "..."
  },
  "tool": "ralphglasses_autobuild_ledger_append"
}
```

## core

Essential fleet management: scan, list, start, stop, pause, logs, config

### `ralphglasses_scan`

Scan for ralph-enabled repos and return their current status

**Example:**

```json
{
  "tool": "ralphglasses_scan"
}
```

### `ralphglasses_list`

List all discovered repos with status summary

**Example:**

```json
{
  "tool": "ralphglasses_list"
}
```

### `ralphglasses_status`

Get detailed status for a specific repo including loop status, circuit breaker, progress, and config

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name (basename of directory) |
| `include_config` | boolean |  | Include full config in status response |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_status"
}
```

### `ralphglasses_start`

Start a ralph loop for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name to start loop for |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_start"
}
```

### `ralphglasses_stop`

Stop a running ralph loop for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name to stop loop for |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_stop"
}
```

### `ralphglasses_stop_all`

Stop all running ralph loops

**Example:**

```json
{
  "tool": "ralphglasses_stop_all"
}
```

### `ralphglasses_pause`

Pause or resume a running ralph loop for a `repo`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name to pause/resume |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_pause"
}
```

### `ralphglasses_logs`

Get recent log lines from a repo's ralph log

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `lines` | number |  | Number of lines to return (default 50, max 500) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_logs"
}
```

### `ralphglasses_config`

Get or set .ralphrc config values for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `key` | string |  | Config key to get/set (omit to list all) |
| `value` | string |  | Value to set (omit to get current value) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_config"
}
```

### `ralphglasses_config_bulk`

Get/set .ralphrc `key` values across multiple repos

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | yes | Config key to get/set |
| `repos` | string |  | Comma-separated repo names (default: all) |
| `value` | string |  | Value to set (omit to query) |

**Example:**

```json
{
  "arguments": {
    "key": "..."
  },
  "tool": "ralphglasses_config_bulk"
}
```

### `ralphglasses_doctor`

Run CLI-style environment and workspace readiness checks: binaries, config, state dir, sqlite, scan path, disk, and API keys

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `checks` | string |  | Comma-separated check names to run (e.g. git,scan_path,api_keys) |
| `include_optional` | boolean |  | Include optional provider/API key checks (default: true) |
| `scan_path` | string |  | Override scan root to inspect (defaults to the server scan path) |

**Example:**

```json
{
  "tool": "ralphglasses_doctor"
}
```

### `ralphglasses_validate`

Validate .ralphrc files across one repo or the full scan path

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_clean` | boolean |  | Include OK repos in the response (default: false) |
| `repo` | string |  | Single repo name to validate |
| `repos` | string |  | Comma-separated repo names to validate |
| `scan_path` | string |  | Override scan root to validate (defaults to the server scan path) |
| `strict` | boolean |  | Treat warnings as errors (default: false) |

**Example:**

```json
{
  "tool": "ralphglasses_validate"
}
```

### `ralphglasses_config_schema`

List known config keys with type and constraint metadata

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_constraints` | boolean |  | Include rendered constraints (default: true) |
| `include_defaults` | boolean |  | Include default metadata when available |
| `key` | string |  | Optional single key to inspect |

**Example:**

```json
{
  "tool": "ralphglasses_config_schema"
}
```

### `ralphglasses_debug_bundle`

Build a sanitized debug bundle matching the CLI debug-bundle workflow

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: view (default) or save |
| `name` | string |  | Optional output filename when action=save |
| `repo` | string |  | Optional repo name whose root should anchor the bundle save path |
| `sections` | string |  | Comma-separated bundle sections to include |

**Example:**

```json
{
  "tool": "ralphglasses_debug_bundle"
}
```

### `ralphglasses_theme_export`

Export a named theme in ghostty, starship, or k9s format

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `format` | string | yes | Export format: ghostty, starship, or k9s |
| `theme` | string | yes | Theme name |

**Example:**

```json
{
  "arguments": {
    "format": "...",
    "theme": "..."
  },
  "tool": "ralphglasses_theme_export"
}
```

### `ralphglasses_telemetry_export`

Export local telemetry data as JSON or CSV with optional filtering

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `format` | string |  | Output format: json (default) or csv |
| `limit` | number |  | Maximum events to return |
| `provider` | string |  | Optional provider filter |
| `repo` | string |  | Optional repo filter |
| `since` | string |  | RFC3339 or YYYY-MM-DD lower bound |
| `type` | string |  | Optional telemetry event type filter |
| `until` | string |  | RFC3339 or YYYY-MM-DD upper bound |

**Example:**

```json
{
  "tool": "ralphglasses_telemetry_export"
}
```

### `ralphglasses_firstboot_profile`

Read, update, validate, or mark done the thin-client firstboot profile

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: get (default), set, validate, or mark_done |
| `anthropic_api_key` | string |  | Anthropic API key override |
| `autonomy_level` | number |  | Autonomy level 0-3 |
| `config_dir` | string |  | Optional config directory override (defaults to ~/.ralphglasses when HOME is available; otherwise the XDG config dir) |
| `coordinator_url` | string |  | Fleet coordinator URL |
| `google_api_key` | string |  | Google API key override |
| `hostname` | string |  | Hostname to persist or validate |
| `openai_api_key` | string |  | OpenAI API key override |

**Example:**

```json
{
  "tool": "ralphglasses_firstboot_profile"
}
```

### `ralphglasses_tasks_get`

Get status of an async task by `task_id` — poll for long-running operations (loop_start, fleet_submit, self_improve)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID returned from async tool invocation |

**Example:**

```json
{
  "arguments": {
    "task_id": "..."
  },
  "tool": "ralphglasses_tasks_get"
}
```

### `ralphglasses_tasks_list`

List all async tasks with optional state filter

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `state` | string |  | Filter by state: running, completed, failed, canceled, input_required |

**Example:**

```json
{
  "tool": "ralphglasses_tasks_list"
}
```

### `ralphglasses_tasks_cancel`

Cancel a running async task by `task_id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID to cancel |

**Example:**

```json
{
  "arguments": {
    "task_id": "..."
  },
  "tool": "ralphglasses_tasks_cancel"
}
```

## session

LLM session lifecycle: launch, list, status, resume, stop, budget, retry, output, tail, diff, compare, errors, export

### `ralphglasses_session_launch`

Launch a headless LLM CLI session (claude/gemini/codex) for a repo with a `prompt`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Prompt/task to send |
| `repo` | string | yes | Repo name |
| `agent` | string |  | Agent/subagent name. Native for Claude only. |
| `allowed_tools` | string |  | Comma-separated allowed tools (e.g. Bash,Read,Edit). Supported by Claude and Gemini; unsupported by Codex. |
| `bare` | boolean |  | Skip hooks/plugins for faster scripted startup |
| `budget_usd` | number |  | Budget in USD — maximum spend for this session. Native for Claude; externally enforced by ralphglasses for Codex and Gemini. |
| `effort` | string |  | Thinking effort level: low, medium, high, max |
| `enhance_prompt` | string |  | Auto-enhance the prompt before launch: local (deterministic), llm (Claude API), auto (try LLM, fallback). Omit to skip enhancement |
| `fallback_model` | string |  | Auto-fallback model on overload |
| `max_turns` | number |  | Maximum conversation turns. Native for Claude only. |
| `model` | string |  | Model to use |
| `no_journal` | string |  | Skip improvement journal injection: true/false (default: false) |
| `output_schema` | string |  | JSON schema for structured output validation (Claude: --json-schema, Codex: --output-schema) |
| `provider` | string |  | LLM provider: codex (default), claude, gemini |
| `session_name` | string |  | Human-readable session name |
| `system_prompt` | string |  | Additional system prompt to append. Native for Claude only; use GEMINI.md or AGENTS.md for Gemini/Codex repo instructions. |
| `target_provider` | string |  | Target LLM provider for prompt enhancement: claude, gemini, openai (defaults to session provider) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |
| `worktree` | string |  | Git worktree isolation (true for auto, or branch name). Supported by Claude and Gemini; unsupported for Codex. |

**Example:**

```json
{
  "arguments": {
    "prompt": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_session_launch"
}
```

### `ralphglasses_session_list`

List all tracked LLM sessions with status, cost, and turns

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_ended` | boolean |  | Include historical ended sessions from the persistent store when available |
| `provider` | string |  | Filter by provider: claude, gemini, codex (omit for all) |
| `repo` | string |  | Filter by repo name (omit for all) |
| `status` | string |  | Filter by status: running, completed, errored, stopped |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "tool": "ralphglasses_session_list"
}
```

### `ralphglasses_session_status`

Get detailed status for a session by `id` including output, cost, and turns

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_status"
}
```

### `ralphglasses_session_resume`

Resume a previous LLM CLI session for `repo` using `session_id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `session_id` | string | yes | Provider session ID to resume (from session status) |
| `prompt` | string |  | Follow-up prompt (optional) |
| `provider` | string |  | LLM provider: codex (default), claude, gemini |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "repo": "...",
    "session_id": "..."
  },
  "tool": "ralphglasses_session_resume"
}
```

### `ralphglasses_session_stop`

Stop a running session by `id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID to stop |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_stop"
}
```

### `ralphglasses_session_stop_all`

Stop all running LLM sessions — emergency cost cutoff

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "tool": "ralphglasses_session_stop_all"
}
```

### `ralphglasses_session_budget`

Get cost/budget info for a session by `id`, update the budget, or reset spend tracking

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID |
| `action` | string |  | Action: get (default), set, or reset_spend |
| `budget_usd` | number |  | Budget in USD — new budget to set (omit to just query) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_budget"
}
```

### `ralphglasses_budget_status`

Show aggregate budget status across all sessions, matching the CLI budget status command

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "tool": "ralphglasses_budget_status"
}
```

### `ralphglasses_session_retry`

Re-launch a failed session by `id` with same params, optional overrides

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID to retry |
| `budget_usd` | number |  | Budget in USD — override budget for retry |
| `model` | string |  | Override model |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_retry"
}
```

### `ralphglasses_session_output`

Get recent output from a session by `id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID |
| `lines` | number |  | Number of lines to return (default 20, max 100) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_output"
}
```

### `ralphglasses_session_tail`

Tail session output by `id` with cursor — returns only new lines since last call

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID |
| `cursor` | string |  | Cursor from previous response (omit for latest) |
| `lines` | number |  | Max lines to return (default 30, max 100) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_tail"
}
```

### `ralphglasses_session_diff`

Git changes made during a session's execution window by `id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID |
| `max_lines` | number |  | Truncate diff at N lines (default 200) |
| `stat_only` | string |  | true/false (default: true) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_diff"
}
```

### `ralphglasses_session_compare`

Compare two sessions by `id1` and `id2`: cost, turns, duration, provider efficiency

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id1` | string | yes | First session ID |
| `id2` | string | yes | Second session ID |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "id1": "...",
    "id2": "..."
  },
  "tool": "ralphglasses_session_compare"
}
```

### `ralphglasses_session_replay_diff`

Side-by-side comparison of two session JSONL replay files: event alignment, similarity score

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path_a` | string | yes | Path to first JSONL replay file |
| `path_b` | string | yes | Path to second JSONL replay file |
| `max_events` | number |  | Max events in response (default 100) |

**Example:**

```json
{
  "arguments": {
    "path_a": "...",
    "path_b": "..."
  },
  "tool": "ralphglasses_session_replay_diff"
}
```

### `ralphglasses_session_errors`

Aggregated error view: parse failures, API errors, budget warnings

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | Max errors (default 50) |
| `repo` | string |  | Filter by repo name |
| `severity` | string |  | Filter: critical, warning, info |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "tool": "ralphglasses_session_errors"
}
```

### `ralphglasses_session_export`

Export a recorded session replay as Markdown or JSON — includes timeline, tool calls, inputs/outputs

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | string | yes | Session ID whose replay to export |
| `after` | string |  | Only include events after this RFC3339 timestamp |
| `before` | string |  | Only include events before this RFC3339 timestamp |
| `event_types` | string |  | Comma-separated event types to include: input, output, tool, status (default: all) |
| `format` | string |  | Export format: markdown (default) or json |
| `repo` | string |  | Repo name hint for locating replay file |

**Example:**

```json
{
  "arguments": {
    "session_id": "..."
  },
  "tool": "ralphglasses_session_export"
}
```

### `ralphglasses_session_fork`

Fork a running or completed session — creates a child session with the parent's context and optional new prompt

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Parent session ID to fork from |
| `budget_usd` | number |  | Budget override in USD (inherits parent budget if omitted) |
| `inject_context` | boolean |  | Include parent session output history in fork prompt (default: true) |
| `model` | string |  | Override model for the fork (inherits parent model if omitted) |
| `prompt` | string |  | New prompt for the forked session (inherits parent prompt if omitted) |
| `provider` | string |  | Override provider for the fork (inherits parent provider if omitted) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_fork"
}
```

### `ralphglasses_session_handoff`

Transfer session state to a new session, optionally switching providers. Includes context from observations, cost tracking, and scratchpad.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `source_session_id` | string | yes | Session ID to transfer from |
| `context_lines` | number |  | Number of recent observations to include (default: 5) |
| `handoff_reason` | string |  | Reason for handoff (tracked in handoff record) |
| `include_context` | boolean |  | Include observation context in handoff (default: true) |
| `stop_source` | boolean |  | Stop the source session after handoff (default: false) |
| `target_provider` | string |  | Target provider: claude, gemini, codex (default: same as source) |

**Example:**

```json
{
  "arguments": {
    "source_session_id": "..."
  },
  "tool": "ralphglasses_session_handoff"
}
```

### `ralphglasses_error_context`

Get error context for a session — consecutive errors, escalation status, and formatted recent errors for LLM context injection (12-Factor Agents Factor 9)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | string | yes | Session ID |

**Example:**

```json
{
  "arguments": {
    "session_id": "..."
  },
  "tool": "ralphglasses_error_context"
}
```

## loop

Perpetual development loops: start, status, step, stop, benchmark, baseline, gates, self-test, self-improve

### `ralphglasses_loop_start`

Create a multi-provider planner/worker perpetual development loop for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `budget_usd` | number |  | Budget in USD — total budget split 1/3 planner, 2/3 worker |
| `duration_hours` | number |  | Maximum loop duration in hours (0 = unlimited) |
| `enable_cascade` | boolean |  | Enable cascade routing (cheap-then-expensive provider) |
| `enable_curriculum` | boolean |  | Enable curriculum learning (difficulty-based task sorting) |
| `enable_episodic_memory` | boolean |  | Enable episodic memory (successful trajectory recall) |
| `enable_reflexion` | boolean |  | Enable reflexion loop (failure correction injection) |
| `enable_uncertainty` | boolean |  | Enable uncertainty quantification (confidence scoring) |
| `max_concurrent_workers` | number |  | Maximum concurrent workers (currently only 1 supported) |
| `max_iterations` | number |  | Maximum loop iterations (0 = unlimited) |
| `planner_model` | string |  | Planner model (default: gpt-5.4) |
| `planner_provider` | string |  | Planner provider: claude, gemini, codex (default: codex) |
| `retry_limit` | number |  | Maximum consecutive failed iterations before step is refused |
| `self_improvement` | boolean |  | Enable self-improvement mode with autonomous acceptance gate |
| `trace_level` | string |  | Trace verbosity: none, summary (default), verbose |
| `verifier_model` | string |  | Verifier model metadata (default: gpt-5.4) |
| `verifier_provider` | string |  | Verifier provider: claude, gemini, codex (default: codex) |
| `verify_commands` | string |  | SECURITY: Privileged input. Newline-separated bash commands (default: ./scripts/dev/ci.sh) |
| `worker_model` | string |  | Worker model (default: gpt-5.4) |
| `worker_provider` | string |  | Worker provider: claude, gemini, codex (default: codex) |
| `worktree_policy` | string |  | Worktree isolation policy (default: git) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_loop_start"
}
```

### `ralphglasses_loop_status`

Get status for a perpetual development loop by `id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Loop run ID |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_loop_status"
}
```

### `ralphglasses_loop_step`

Execute one planner/worker/verifier iteration for a loop by `id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Loop run ID |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_loop_step"
}
```

### `ralphglasses_loop_stop`

Stop a perpetual development loop by `id`

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Loop run ID |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_loop_stop"
}
```

### `ralphglasses_loop_benchmark`

P50/P95 metrics from recent loop observations for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `hours` | number |  | Look-back window in hours (default: 48) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_loop_benchmark"
}
```

### `ralphglasses_loop_baseline`

Generate, view, or pin loop performance baseline for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `action` | string |  | Action: view (default), refresh, or pin |
| `hours` | number |  | Window for refresh/pin in hours (default: 48) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_loop_baseline"
}
```

### `ralphglasses_loop_gates`

Evaluate regression gates — returns pass/warn/fail report

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `baseline_path` | string |  | Optional absolute path to a baseline JSON file |
| `hours` | number |  | Look-back window in hours (default: 24) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_loop_gates"
}
```

### `ralphglasses_self_test`

Run recursive self-test iterations against a repository using the ralphglasses loop engine

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Absolute path to the repository to test |
| `budget_usd` | number |  | Budget in USD (default: 5.0) |
| `dry_run` | boolean |  | Validate config without running iterations (default: false) |
| `iterations` | number |  | Number of self-test iterations (default: 3) |
| `use_snapshot` | boolean |  | Restore repo snapshot between iterations (default: true) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_self_test"
}
```

### `ralphglasses_self_improve`

Start a self-improvement loop that autonomously improves a repository — auto-merges safe changes, creates PRs for review-required changes

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `budget_usd` | number |  | Budget in USD (default: 20.0, split 1/4 planner + 3/4 worker) |
| `duration_hours` | number |  | Maximum duration in hours (default: 4) |
| `max_iterations` | number |  | Maximum iterations (default: 5) |
| `planner_provider` | string |  | Planner provider (claude, gemini, codex) |
| `trace_level` | string |  | Trace verbosity: none, summary (default), verbose |
| `worker_provider` | string |  | Worker provider (claude, gemini, codex) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_self_improve"
}
```

### `ralphglasses_loop_prune`

Prune stale loop run files by status and age — removes phantom pending/failed loop state that accumulates over time

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `dry_run` | boolean |  | If true, return count without deleting (default: true) |
| `older_than_hours` | number |  | Only prune loop runs older than this many hours (default: 72) |
| `repo` | string |  | Optional repo name filter — only prune runs for this repo |
| `statuses` | string |  | Comma-separated statuses to prune (default: pending,failed) |

**Example:**

```json
{
  "tool": "ralphglasses_loop_prune"
}
```

### `ralphglasses_loop_await`

Block until a session or loop completes (replaces sleep anti-pattern)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session or loop ID to wait for |
| `type` | string | yes | 'session' or 'loop' |
| `poll_interval_seconds` | number |  | Poll interval in seconds (default 10, min 5) |
| `timeout_seconds` | number |  | Max wait time in seconds (default 300) |

**Example:**

```json
{
  "arguments": {
    "id": "...",
    "type": "..."
  },
  "tool": "ralphglasses_loop_await"
}
```

### `ralphglasses_loop_poll`

Non-blocking single status check for a session or loop

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session or loop ID |
| `type` | string | yes | 'session' or 'loop' |

**Example:**

```json
{
  "arguments": {
    "id": "...",
    "type": "..."
  },
  "tool": "ralphglasses_loop_poll"
}
```

## prompt

Prompt enhancement: analyze, enhance, lint, improve, classify, should_enhance, templates, template_fill

### `ralphglasses_prompt_analyze`

Score a prompt across 10 quality dimensions (clarity, specificity, structure, examples, etc.) with letter grades and actionable suggestions

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt text to analyze |
| `target_provider` | string |  | Target model provider for scoring suggestions: openai (default), claude, gemini |
| `task_type` | string |  | Override auto-detection: code, troubleshooting, analysis, creative, workflow, general |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_prompt_analyze"
}
```

### `ralphglasses_prompt_enhance`

Run the 13-stage prompt enhancement pipeline (specificity, positive reframing, XML structure, context reorder, format enforcement, etc.)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt text to enhance |
| `mode` | string |  | Enhancement mode: local (default, deterministic), llm (Claude/Gemini/OpenAI API), auto (try LLM, fallback to local) |
| `repo` | string |  | Repo name to load .prompt-improver.yaml config from |
| `target_provider` | string |  | Target model provider — controls structure style and scoring: openai (default), claude, gemini |
| `task_type` | string |  | Override auto-detection: code, troubleshooting, analysis, creative, workflow, general |
| `trace_level` | string |  | Trace verbosity: none, summary (default), verbose |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_prompt_enhance"
}
```

### `ralphglasses_prompt_lint`

Deep-lint a prompt for anti-patterns: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt text to lint |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_prompt_lint"
}
```

### `ralphglasses_prompt_improve`

LLM-powered prompt improvement using Claude, Gemini, or OpenAI with domain-specific meta-prompts

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt text to improve |
| `feedback` | string |  | Optional feedback to guide the improvement direction |
| `provider` | string |  | LLM provider for improvement: openai (default, requires OPENAI_API_KEY), claude (requires ANTHROPIC_API_KEY), gemini (requires GOOGLE_API_KEY) |
| `task_type` | string |  | Override auto-detection: code, troubleshooting, analysis, creative, workflow, general |
| `thinking_enabled` | boolean |  | Include thinking scaffolding in the improved prompt |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_prompt_improve"
}
```

### `ralphglasses_prompt_classify`

Classify a prompt's task type (code, troubleshooting, analysis, creative, workflow, general)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt text to classify |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_prompt_classify"
}
```

### `ralphglasses_prompt_should_enhance`

Check whether a prompt would benefit from enhancement

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt text to check |
| `repo` | string |  | Repo name for loading .prompt-improver.yaml config |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_prompt_should_enhance"
}
```

### `ralphglasses_prompt_templates`

List available prompt templates with descriptions and required variables

**Example:**

```json
{
  "tool": "ralphglasses_prompt_templates"
}
```

### `ralphglasses_prompt_template_fill`

Fill a prompt template by `name` with `vars` (JSON key-value pairs)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Template name |
| `vars` | string | yes | JSON object of variable key-value pairs |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "vars": "..."
  },
  "tool": "ralphglasses_prompt_template_fill"
}
```

### `ralphglasses_prompt_ab_test`

A/B test two prompts by scoring them across 10 quality dimensions. Returns winner, score diff, confidence, and per-dimension comparison.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt_a` | string | yes | First prompt to compare |
| `prompt_b` | string | yes | Second prompt to compare |
| `repo` | string |  | Repo name for result storage |
| `target_provider` | string |  | Target provider for scoring: openai (default), claude, gemini |

**Example:**

```json
{
  "arguments": {
    "prompt_a": "...",
    "prompt_b": "..."
  },
  "tool": "ralphglasses_prompt_ab_test"
}
```

## fleet

Fleet operations: fleet_status, analytics, submit, budget, workers, dlq, marathon_dashboard, capacity_plan

### `ralphglasses_fleet_status`

Fleet-wide dashboard: aggregate status, costs, health, and alerts across all repos and sessions in one call

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | Max repos to return in full mode (default 50) |
| `offset` | number |  | Pagination offset for repos (default 0) |
| `repo` | string |  | Filter to a specific repo name |
| `summary_only` | boolean |  | Return compact JSON with just repo names, session counts, and total spend instead of full dump |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_status"
}
```

### `ralphglasses_fleet_analytics`

Cost breakdown by provider/repo/time-period with trend analysis. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider` | string |  | Filter by provider |
| `repo` | string |  | Filter by repo name |
| `window` | string |  | Time window as Go duration (e.g. '1h', '24h'). Default: 1h |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_analytics"
}
```

### `ralphglasses_fleet_submit`

Submit work for `repo` with `prompt` to the distributed fleet queue. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Task prompt |
| `repo` | string | yes | Repo name |
| `budget_usd` | number |  | Budget in USD (default: 5.0) |
| `priority` | number |  | Priority 0-10 (default: 5, higher = first) |
| `provider` | string |  | codex (default), claude, gemini |

**Example:**

```json
{
  "arguments": {
    "prompt": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_fleet_submit"
}
```

### `ralphglasses_fleet_budget`

View or set the fleet-wide budget. Shows spent, remaining, and active work. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | New budget limit in USD (omit to just view current budget) |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_budget"
}
```

### `ralphglasses_fleet_workers`

List registered fleet workers with status, capacity, and spend. Optionally pause, resume, or drain a worker. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Worker action: pause, resume, or drain (omit to list) |
| `worker_id` | string |  | Worker ID (required for pause/resume/drain actions) |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_workers"
}
```

### `ralphglasses_fleet_dlq`

Dead letter queue operations for permanently failed fleet work items. Actions: list, retry, purge, depth. Requires fleet coordinator mode.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: list (default), retry, purge, depth |
| `item_id` | string |  | Work item ID (required for retry action) |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_dlq"
}
```

### `ralphglasses_fleet_schedule`

Build a dependency-ordered schedule plan from tasks with dependencies. Uses topological sort to group tasks into parallelizable batches. Returns batch plan, critical path, and depth.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tasks` | string | yes | JSON array of task objects: [{"id": "build", "name": "Build", "dependencies": ["lint"], "priority": 5}, ...] |

**Example:**

```json
{
  "arguments": {
    "tasks": "..."
  },
  "tool": "ralphglasses_fleet_schedule"
}
```

### `ralphglasses_marathon_dashboard`

Compact marathon status: burn rate, stale sessions, team progress, alerts

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `stale_threshold_min` | number |  | Minutes idle before flagged stale (default 5) |

**Example:**

```json
{
  "tool": "ralphglasses_marathon_dashboard"
}
```

### `ralphglasses_fleet_runtime`

Start, stop, restart, inspect, or auto-discover fleet runtime mode parity for the CLI serve command

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: status (default), discover, start, stop, or restart |
| `automation` | boolean |  | Run repo-local automation supervisors alongside the runtime (default: true) |
| `coordinator_url` | string |  | Coordinator URL for worker mode |
| `fleet_budget` | number |  | Fleet-wide budget ceiling in USD (coordinator mode only) |
| `mode` | string |  | Runtime mode: coordinator or worker (default: worker for start/restart) |
| `port` | number |  | Fleet port (default: 9473) |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_runtime"
}
```

### `ralphglasses_marathon`

Start, resume, inspect, or stop the CLI marathon workflow

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: status (default), start, resume, or stop |
| `budget_usd` | number |  | Maximum budget in USD (default: 10.0) |
| `checkpoint_interval` | string |  | Checkpoint interval (default: 10m) |
| `duration` | string |  | Maximum duration (default: 1h) |
| `repo` | string |  | Target repo name |

**Example:**

```json
{
  "tool": "ralphglasses_marathon"
}
```

### `ralphglasses_fleet_capacity_plan`

Recommend worker count from queue depth and budget. Returns recommended workers, estimated cost, completion time, and cost per worker hour.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `available_budget` | number | yes | Available budget in USD |
| `queue_depth` | number | yes | Number of tasks in the queue |
| `avg_task_cost` | number |  | Average cost per task in USD (auto-estimated from observations if omitted) |
| `avg_task_duration_min` | number |  | Average task duration in minutes (default: 10) |
| `target_completion_hours` | number |  | Target completion time in hours (default: 4) |
| `utilization_factor` | number |  | Worker efficiency factor 0.1-1.0 (default: 0.8 — workers aren't 100% productive) |

**Example:**

```json
{
  "arguments": {
    "available_budget": 1,
    "queue_depth": 1
  },
  "tool": "ralphglasses_fleet_capacity_plan"
}
```

### `ralphglasses_fleet_grafana`

Export a Grafana-compatible JSON dashboard for fleet metrics (session throughput, cost burn rate, provider comparison, error rate, active sessions, budget utilization). Import the output directly into Grafana.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `datasource` | string |  | Prometheus data source name/UID (default: 'prometheus') |
| `metrics` | string |  | Comma-separated metric names to include: session_throughput, cost_burn_rate, provider_comparison, error_rate, active_sessions, budget_utilization. Default: all |
| `title` | string |  | Dashboard title (default: 'Ralphglasses Fleet Metrics') |

**Example:**

```json
{
  "tool": "ralphglasses_fleet_grafana"
}
```

## repo

Repo management: health, optimize, scaffold, claudemd_check, snapshot

### `ralphglasses_repo_health`

Composite health check: circuit breaker, budget, staleness, errors, active sessions

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_repo_health"
}
```

### `ralphglasses_repo_optimize`

Analyze and optimize ralph config files — detect misconfigs, missing settings, stale plans

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root path |
| `dry_run` | string |  | Report only, don't modify: true/false (default: true) |
| `focus` | string |  | Focus area: config, prompt, plan, all (default: all) |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_repo_optimize"
}
```

### `ralphglasses_repo_scaffold`

Create/initialize ralph config files (.ralph/, .ralphrc, PROMPT.md, AGENT.md, fix_plan.md) for a repo

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root path |
| `force` | string |  | Overwrite existing files: true/false (default: false) |
| `minimal` | boolean |  | Generate the minimal .ralphrc variant |
| `project_name` | string |  | Project name override (defaults to directory name) |
| `project_type` | string |  | Project type override (auto-detected from go.mod, package.json, etc.) |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_repo_scaffold"
}
```

### `ralphglasses_repo_surface_audit`

Audit repo instruction/config surfaces used by Codex, Claude, Gemini, and MCP clients

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_repo_surface_audit"
}
```

### `ralphglasses_claudemd_check`

Health-check a repo's CLAUDE.md for common issues (length, inline code, overtrigger language, missing headers)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_claudemd_check"
}
```

### `ralphglasses_snapshot`

Save or list fleet state snapshots

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: save (default) or list |
| `name` | string |  | Snapshot name (auto-generated if omitted) |
| `repo` | string |  | Target repo name for snapshot storage (resolved from CWD if omitted) |

**Example:**

```json
{
  "tool": "ralphglasses_snapshot"
}
```

### `ralphglasses_worktree_create`

Create a new git worktree for a repo under .ralph/worktrees/manual/

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Worktree name (sanitized for filesystem) |
| `repo` | string | yes | Repo name |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_worktree_create"
}
```

### `ralphglasses_worktree_list`

List git worktrees for a repo with optional dirty/stale filtering

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `dirty_only` | boolean |  | Only include dirty worktrees |
| `include_stale` | boolean |  | Annotate stale worktrees using stale_after_hours |
| `stale_after_hours` | number |  | Age threshold in hours for stale detection (default: 24) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_worktree_list"
}
```

### `ralphglasses_worktree_cleanup`

Clean up stale loop worktrees older than a given age — skips locked/active worktrees

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `dry_run` | boolean |  | Preview stale worktrees without deleting them |
| `max_age_hours` | number |  | Max age in hours before cleanup (default 24) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_worktree_cleanup"
}
```

## roadmap

Roadmap automation: parse, analyze, research, expand, export

### `ralphglasses_roadmap_parse`

Parse ROADMAP.md at `path` into structured JSON (phases, sections, tasks, deps, completion stats)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root or direct .md path |
| `file` | string |  | Override filename (default: ROADMAP.md) |
| `max_depth` | number |  | Depth of detail: 0=phases only, 1=phases+sections, 2=full (default 2) |
| `phase` | string |  | Filter to a specific phase name |
| `summary_only` | boolean |  | Return compact summary (phase counts, completion %) instead of full task details |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_roadmap_parse"
}
```

### `ralphglasses_roadmap_analyze`

Compare roadmap vs codebase at `path` — find gaps, stale checkboxes, ready tasks, orphaned code

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root path |
| `category` | string |  | Filter by category: gaps, stale, orphaned, ready |
| `file` | string |  | Override filename (default: ROADMAP.md) |
| `limit` | number |  | Max items per category (default 20) |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_roadmap_analyze"
}
```

### `ralphglasses_roadmap_research`

Search GitHub for relevant repos and tools at `path` that unlock new capabilities

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root path |
| `limit` | number |  | Max results (default 10) |
| `topics` | string |  | Search topics (inferred from go.mod/README if omitted) |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_roadmap_research"
}
```

### `ralphglasses_roadmap_expand`

Generate proposed roadmap expansions at `path` from analysis gaps and research findings

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root path |
| `file` | string |  | Override filename (default: ROADMAP.md) |
| `limit` | number |  | Max proposals to return (default 20) |
| `phase` | string |  | Filter proposals to a specific phase |
| `research` | string |  | Research topics to include (runs research internally) |
| `style` | string |  | Expansion style: conservative, balanced, aggressive (default: balanced) |
| `summary_only` | boolean |  | Drop proposal markdown and return truncated summaries only |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_roadmap_expand"
}
```

### `ralphglasses_roadmap_export`

Export roadmap items at `path` as loop specs or tranche checkpoint summaries for ralph automation

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Repo root path |
| `file` | string |  | Override filename (default: ROADMAP.md) |
| `format` | string |  | Output format: rdcycle, fix_plan, progress, launch_ready, checkpoint (default: rdcycle). checkpoint emits a docs-ready tranche summary with completed work, verification notes, and next-wave follow-ups. launch_ready enriches tasks with difficulty_score, suggested_provider, estimated_budget_usd |
| `max_tasks` | number |  | Max tasks to export (default 20) |
| `phase` | string |  | Filter by phase name (default: all) |
| `respect_deps` | string |  | Skip tasks with unmet deps (default: true) |
| `section` | string |  | Filter by section name (default: all) |
| `status` | string |  | Filter by status: incomplete (default), complete, all |

**Example:**

```json
{
  "arguments": {
    "path": "..."
  },
  "tool": "ralphglasses_roadmap_export"
}
```

### `ralphglasses_roadmap_prioritize`

Score and rank uncompleted roadmap items by impact, effort, and dependency readiness. Detects section-level blocking and adds phase momentum bonus. Returns prioritized list and recommended next sprint.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `phase_filter` | string |  | Filter to phases containing this string (e.g., 'Phase 10') |
| `top_n` | number |  | Number of items to return (default: 20) |
| `weights` | string |  | JSON weights: {"impact": 0.4, "effort": 0.3, "dependency": 0.3} |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_roadmap_prioritize"
}
```

## team

Agent teams and definitions: team_create, team_status, team_delegate, agent_define, agent_list, agent_compose

### `ralphglasses_team_create`

Create an agent team by `name` for `repo`; Codex teams use the structured autonomous runtime with planner/worker orchestration

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Team name |
| `repo` | string | yes | Repo name |
| `tasks` | string | yes | Newline-separated task descriptions |
| `a2a_agent_url` | string |  | Reserved for future A2A support; ignored today |
| `autostart` | boolean |  | Start the background team controller immediately; defaults true for Codex teams |
| `budget_usd` | number |  | Budget in USD — total budget for the team |
| `dry_run` | boolean |  | Return team configuration without launching sessions |
| `execution_backend` | string |  | Worker execution backend: local or fleet |
| `lead_agent` | string |  | Provider-native agent definition name for the lead session. Not supported for Codex teams. |
| `max_concurrency` | number |  | Maximum concurrent worker tasks (default 2) |
| `max_retries` | number |  | Maximum retries per task (default 2) |
| `model` | string |  | Model for lead session |
| `provider` | string |  | LLM provider for lead: codex (default), claude, gemini |
| `target_branch` | string |  | Repo branch to reconcile/promote into (default main) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |
| `worker_model` | string |  | Model override for worker sessions |
| `worker_provider` | string |  | Default LLM provider for worker tasks: codex (default), claude, gemini |
| `worktree_policy` | string |  | Worker isolation mode: per_worker (default for Codex) or shared |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "repo": "...",
    "tasks": "..."
  },
  "tool": "ralphglasses_team_create"
}
```

### `ralphglasses_team_status`

Get team status including lead session, tasks, and progress

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Team name |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "name": "..."
  },
  "tool": "ralphglasses_team_status"
}
```

### `ralphglasses_team_delegate`

Add a new task to an existing team

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Team name |
| `task` | string | yes | Task description to delegate |
| `provider` | string |  | LLM provider override for this task: claude, gemini, codex |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "task": "..."
  },
  "tool": "ralphglasses_team_delegate"
}
```

### `ralphglasses_agent_define`

Create or update an agent definition for a repo (supports all providers)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Agent name |
| `prompt` | string | yes | Agent system prompt / instructions (markdown) |
| `repo` | string | yes | Repo name |
| `description` | string |  | Agent description |
| `max_turns` | number |  | Max turns for this agent |
| `model` | string |  | Model override (sonnet, opus, haiku) |
| `provider` | string |  | Target provider: codex (default, .codex/agents/*.toml), claude (.claude/agents/), gemini (.gemini/agents/*.md) |
| `tools` | string |  | Comma-separated allowed tools |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "prompt": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_agent_define"
}
```

### `ralphglasses_agent_list`

List available agent definitions for a repo (supports all providers)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `provider` | string |  | Filter by provider: codex (default), claude, gemini, or 'all' |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_agent_list"
}
```

### `ralphglasses_agent_compose`

Create a composite agent by layering multiple existing agent definitions

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agents` | string | yes | Comma-separated agent names to compose |
| `name` | string | yes | Name for the composite agent |
| `repo` | string | yes | Repo name |
| `model` | string |  | Override model for composite agent |
| `provider` | string |  | Provider: codex (default), claude, gemini |

**Example:**

```json
{
  "arguments": {
    "agents": "...",
    "name": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_agent_compose"
}
```

## tenant

Workspace tenant administration: list, create, status, rotate trigger token, and batch role leaderboards

### `ralphglasses_tenant_list`

List all configured workspace tenants

**Example:**

```json
{
  "tool": "ralphglasses_tenant_list"
}
```

### `ralphglasses_tenant_create`

Create or update a workspace tenant

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tenant_id` | string | yes | Tenant ID |
| `allowed_repo_roots` | string |  | Comma-separated allowed repo roots |
| `budget_cap_usd` | number |  | Budget cap in USD |
| `display_name` | string |  | Display name |

**Example:**

```json
{
  "arguments": {
    "tenant_id": "..."
  },
  "tool": "ralphglasses_tenant_create"
}
```

### `ralphglasses_tenant_status`

Get tenant details plus current active session/team counts

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tenant_id` | string | yes | Tenant ID |

**Example:**

```json
{
  "arguments": {
    "tenant_id": "..."
  },
  "tool": "ralphglasses_tenant_status"
}
```

### `ralphglasses_tenant_rotate_trigger_token`

Rotate the bearer token used by the trigger HTTP API for one tenant

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `tenant_id` | string | yes | Tenant ID |

**Example:**

```json
{
  "arguments": {
    "tenant_id": "..."
  },
  "tool": "ralphglasses_tenant_rotate_trigger_token"
}
```

### `ralphglasses_tenant_role_leaderboards`

Generate top-role leaderboards for one tenant or all tenants in a batch

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_ended` | boolean |  | Include persisted ended sessions (default: true) |
| `limit` | number |  | Maximum role entries per tenant (default: 10) |
| `tenant_id` | string |  | Optional tenant ID; omit to return all tenants |

**Example:**

```json
{
  "tool": "ralphglasses_tenant_role_leaderboards"
}
```

## awesome

Awesome-list research: fetch, analyze, diff, report, sync

### `ralphglasses_awesome_fetch`

Fetch and parse an awesome-list README into structured entries with categories

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string |  | GitHub repo (default: hesreallyhim/awesome-claude-code) |

**Example:**

```json
{
  "tool": "ralphglasses_awesome_fetch"
}
```

### `ralphglasses_awesome_analyze`

Deep-analyze repos: fetch READMEs, score value/complexity vs ralph capabilities

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `max_workers` | number |  | Concurrent README fetches (default 5) |
| `repo` | string |  | GitHub repo (default: hesreallyhim/awesome-claude-code) |

**Example:**

```json
{
  "tool": "ralphglasses_awesome_analyze"
}
```

### `ralphglasses_awesome_diff`

Compare current awesome-list against previous fetch (new/removed entries)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `save_to` | string | yes | Repo path where previous index is saved |
| `repo` | string |  | GitHub repo (default: hesreallyhim/awesome-claude-code) |

**Example:**

```json
{
  "arguments": {
    "save_to": "..."
  },
  "tool": "ralphglasses_awesome_diff"
}
```

### `ralphglasses_awesome_report`

Generate formatted report from saved analysis results

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `save_to` | string | yes | Repo path where analysis is saved |
| `format` | string |  | Output format: json or markdown (default: markdown) |

**Example:**

```json
{
  "arguments": {
    "save_to": "..."
  },
  "tool": "ralphglasses_awesome_report"
}
```

### `ralphglasses_awesome_sync`

Full pipeline: fetch awesome-list -> diff -> analyze new entries -> report -> save

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `save_to` | string | yes | Repo path to save results |
| `full_rescan` | string |  | Re-analyze all entries, not just new: true/false (default: false) |
| `max_workers` | number |  | Concurrent README fetches (default 5) |
| `repo` | string |  | GitHub repo (default: hesreallyhim/awesome-claude-code) |

**Example:**

```json
{
  "arguments": {
    "save_to": "..."
  },
  "tool": "ralphglasses_awesome_sync"
}
```

## advanced

Advanced: journals, tool benchmark, circuit breaker reset

### `ralphglasses_tool_benchmark`

Per-tool performance benchmarks: latency percentiles, success rates, and regression detection

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `compare` | string |  | Include regression analysis vs previous baseline: true/false (default: false) |
| `hours` | number |  | Time window in hours (default 24) |
| `tool` | string |  | Filter to a specific tool name |

**Example:**

```json
{
  "tool": "ralphglasses_tool_benchmark"
}
```

### `ralphglasses_journal_read`

Read improvement journal entries for a repo with synthesized context

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `limit` | number |  | Max entries to return (default 10) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_journal_read"
}
```

### `ralphglasses_journal_write`

Manually write an improvement note to a repo's journal

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `failed` | string |  | Comma-separated items that failed |
| `session_id` | string |  | Associated session ID (optional) |
| `suggest` | string |  | Comma-separated suggestions |
| `worked` | string |  | Comma-separated items that worked |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_journal_write"
}
```

### `ralphglasses_journal_prune`

Compact improvement journal to prevent unbounded growth

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `dry_run` | string |  | Preview only, don't modify: true/false (default: true) |
| `keep` | number |  | Number of entries to keep (default 100) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_journal_prune"
}
```

### `ralphglasses_circuit_reset`

Reset circuit breaker state for a named service, re-enabling requests after failures. Use 'enhancer' for the LLM prompt enhancer circuit breaker, or a repo name for its file-based circuit breaker.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `service` | string | yes | Service/circuit to reset: 'enhancer' or a repo name |

**Example:**

```json
{
  "arguments": {
    "service": "..."
  },
  "tool": "ralphglasses_circuit_reset"
}
```

## events

Fleet event bus: query and poll for session, cost, loop, and circuit events

### `ralphglasses_event_list`

Query recent fleet events from the event bus

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | Max events to return (default 50) |
| `offset` | number |  | Skip first N results for pagination (default 0) |
| `provider` | string |  | Filter by provider |
| `repo` | string |  | Filter by repo name |
| `session_id` | string |  | Filter by session ID |
| `since` | string |  | ISO timestamp filter |
| `type` | string |  | Filter by event type (e.g. session.started, cost.update) |
| `types` | string |  | Comma-separated event types to filter (OR). Alternative to single 'type' param |
| `until` | string |  | ISO timestamp upper bound (exclusive) |

**Example:**

```json
{
  "tool": "ralphglasses_event_list"
}
```

### `ralphglasses_event_poll`

Poll for new fleet events since last check. Cursor-based for efficient mobile polling.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cursor` | string |  | Cursor from previous response (omit for recent) |
| `limit` | number |  | Max events (default 20, max 50) |
| `type` | string |  | Filter by event type (e.g. session.started, cost.update) |

**Example:**

```json
{
  "tool": "ralphglasses_event_poll"
}
```

## feedback

Provider feedback: performance profiles, recommendations, bandit stats, confidence calibration

### `ralphglasses_feedback_profiles`

View feedback profiles: per-task-type and per-provider performance data from journal analysis. Auto-seeds from observations when empty.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: 'get' (default) returns profiles, 'seed' forces re-seed from observations |
| `type` | string |  | Filter: prompt, provider, or omit for both |

**Example:**

```json
{
  "tool": "ralphglasses_feedback_profiles"
}
```

### `ralphglasses_provider_recommend`

Recommend best provider + model + budget for a task based on feedback profiles and cost normalization

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task` | string | yes | Task description (e.g. 'fix lint errors', 'add search feature') |

**Example:**

```json
{
  "arguments": {
    "task": "..."
  },
  "tool": "ralphglasses_provider_recommend"
}
```

### `ralphglasses_provider_capabilities`

Inspect the provider capability matrix for Claude, Codex, and Gemini, including native, emulated, and install-dependent features

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider` | string |  | Optional provider filter: claude, codex, gemini |

**Example:**

```json
{
  "tool": "ralphglasses_provider_capabilities"
}
```

### `ralphglasses_provider_compare`

Compare Claude, Codex, and Gemini repo/runtime surfaces and optionally include task-specific recommendation data

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task` | string |  | Optional task description to include recommendation and feedback context |

**Example:**

```json
{
  "tool": "ralphglasses_provider_compare"
}
```

### `ralphglasses_bandit_status`

View multi-armed bandit arm statistics for provider selection

**Example:**

```json
{
  "tool": "ralphglasses_bandit_status"
}
```

### `ralphglasses_confidence_calibration`

View calibrated confidence model weights, training status, and feature importances

**Example:**

```json
{
  "tool": "ralphglasses_confidence_calibration"
}
```

## eval

Offline evaluation: counterfactual analysis, Bayesian A/B testing, frequentist significance testing, changepoint detection

### `ralphglasses_eval_counterfactual`

Estimate outcomes under hypothetical policy changes using inverse propensity scoring on loop observations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `policy` | string | yes | Policy type: cascade_threshold or provider_routing |
| `repo` | string | yes | Repo name |
| `hours` | number |  | Observation window in hours (default: 168 = 7 days) |
| `provider` | string |  | Target provider for provider_routing policy |
| `task_type` | string |  | Task type for provider_routing policy |
| `threshold` | number |  | New cascade threshold for cascade_threshold policy (default: 0.6) |

**Example:**

```json
{
  "arguments": {
    "policy": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_eval_counterfactual"
}
```

### `ralphglasses_eval_ab_test`

Bayesian A/B test comparing providers or time periods using Beta-Bernoulli model

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `mode` | string | yes | Comparison mode: providers or periods |
| `repo` | string | yes | Repo name |
| `hours` | number |  | Observation window in hours (default: 168) |
| `provider_a` | string |  | First provider (providers mode) |
| `provider_b` | string |  | Second provider (providers mode) |
| `split_hours_ago` | number |  | Hours ago to split time periods (periods mode) |

**Example:**

```json
{
  "arguments": {
    "mode": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_eval_ab_test"
}
```

### `ralphglasses_eval_changepoints`

Detect performance shifts using CUSUM changepoint detection on loop observations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `hours` | number |  | Observation window in hours (default: 168) |
| `metric` | string |  | Specific metric to analyze (completion_rate, cost, latency, confidence, difficulty). Omit for all. |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_eval_changepoints"
}
```

### `ralphglasses_eval_significance`

Frequentist significance testing (z-test/t-test) with combined Bayesian comparison report

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `mode` | string | yes | Comparison mode: providers, periods, or cost |
| `repo` | string | yes | Repo name |
| `hours` | number |  | Observation window in hours (default: 168) |
| `provider_a` | string |  | First provider (providers/cost mode) |
| `provider_b` | string |  | Second provider (providers/cost mode) |
| `split_hours_ago` | number |  | Hours ago to split time periods (periods mode) |

**Example:**

```json
{
  "arguments": {
    "mode": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_eval_significance"
}
```

### `ralphglasses_anomaly_detect`

Detect anomalies in metric streams using sliding-window z-score analysis

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `metric` | string | yes | Metric to analyze — full names (total_cost_usd, total_latency_ms, confidence, difficulty_score) or shorthands (cost, latency, difficulty) |
| `repo` | string | yes | Repo name |
| `hours` | number |  | Observation window in hours (default: 168) |

**Example:**

```json
{
  "arguments": {
    "metric": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_anomaly_detect"
}
```

### `ralphglasses_eval_define`

Validate and parse an A/B test definition from YAML content. Returns the parsed definition or validation errors.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `yaml_content` | string | yes | YAML content defining the A/B test (name, variants, metrics, sample_size, timeout) |

**Example:**

```json
{
  "arguments": {
    "yaml_content": "..."
  },
  "tool": "ralphglasses_eval_define"
}
```

### `ralphglasses_provider_benchmark`

Compare providers using a standardized task suite (code gen, explanation, debugging, refactoring, test writing). Returns quality scores, cost estimates, and winner recommendation.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `iterations` | number |  | Number of iterations per task (default: 3, max: 10) |
| `providers` | string |  | Comma-separated providers to benchmark (default: codex,gemini,claude) |
| `repo` | string |  | Repo for result storage |

**Example:**

```json
{
  "tool": "ralphglasses_provider_benchmark"
}
```

## fleet_h

Fleet intelligence: blackboard coordination, A2A task delegation, cost forecasting, cost recommendations

### `ralphglasses_blackboard_query`

Query blackboard entries by namespace for fleet worker coordination. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `namespace` | string | yes | Namespace to query |

**Example:**

```json
{
  "arguments": {
    "namespace": "..."
  },
  "tool": "ralphglasses_blackboard_query"
}
```

### `ralphglasses_blackboard_put`

Write an entry to the blackboard for fleet coordination. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `key` | string | yes | Entry key |
| `namespace` | string | yes | Entry namespace |
| `value` | string | yes | JSON object value |
| `ttl_seconds` | number |  | Time-to-live in seconds (0 for no expiry) |
| `writer_id` | string |  | Writer identifier |

**Example:**

```json
{
  "arguments": {
    "key": "...",
    "namespace": "...",
    "value": "..."
  },
  "tool": "ralphglasses_blackboard_put"
}
```

### `ralphglasses_a2a_offers`

List open agent-to-agent task delegation offers. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

**Example:**

```json
{
  "tool": "ralphglasses_a2a_offers"
}
```

### `ralphglasses_cost_forecast`

Cost burn rate, anomaly detection, and budget exhaustion ETA. Requires a fleet coordinator started via `ralphglasses serve --coordinator` and an MCP client configured with `RALPH_FLEET_URL`.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `budget_remaining` | number |  | Remaining budget in USD for exhaustion ETA (default: 0) |

**Example:**

```json
{
  "tool": "ralphglasses_cost_forecast"
}
```

### `ralphglasses_cost_recommend`

Analyze cost history and recommend config changes: provider switches, budget pacing, anomaly responses, cache optimization, model downgrades

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `budget_hours` | number |  | Desired budget runway in hours (default: 8) |
| `budget_remaining` | number |  | Remaining budget in USD for pacing recommendations |
| `concurrency` | number |  | Current session concurrency for pacing recommendations |
| `min_samples` | number |  | Minimum samples per provider for comparison (default: 5) |
| `type` | string |  | Filter by recommendation type: provider_switch, budget_pacing, anomaly_response, cache_optimize, model_downgrade |

**Example:**

```json
{
  "tool": "ralphglasses_cost_recommend"
}
```

## observability

Observation queries, scratchpad automation, loop wait/poll, coverage, cost estimation, merge verification

### `ralphglasses_observation_query`

Filter and paginate loop observations from .ralph/logs/loop_observations.jsonl

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `hours` | number |  | Hours of history to query (default 48) |
| `limit` | number |  | Max results (default 50, max 500) |
| `loop_id` | string |  | Filter by loop ID |
| `provider` | string |  | Filter by provider |
| `since` | string |  | RFC3339 timestamp lower bound (overrides hours if set) |
| `status` | string |  | Filter by status (pass, fail, error) |
| `until` | string |  | RFC3339 timestamp upper bound |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_observation_query"
}
```

### `ralphglasses_observation_summary`

Aggregate observation stats with p50/p95/p99 percentiles via SummarizeObservations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `hours` | number |  | Hours of history (default 48) |
| `loop_id` | string |  | Filter by loop ID |
| `since` | string |  | RFC3339 timestamp lower bound (overrides hours if set) |
| `until` | string |  | RFC3339 timestamp upper bound |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_observation_summary"
}
```

### `ralphglasses_scratchpad_read`

Read a .ralph/{name}_scratchpad.md file

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Scratchpad name (e.g. 'tool_improvement') |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "arguments": {
    "name": "..."
  },
  "tool": "ralphglasses_scratchpad_read"
}
```

### `ralphglasses_scratchpad_append`

Append a markdown note to a scratchpad file

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Markdown content to append |
| `name` | string | yes | Scratchpad name |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |
| `section` | string |  | Optional section header to add before content |

**Example:**

```json
{
  "arguments": {
    "content": "...",
    "name": "..."
  },
  "tool": "ralphglasses_scratchpad_append"
}
```

### `ralphglasses_scratchpad_list`

List all scratchpad files in .ralph/

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "tool": "ralphglasses_scratchpad_list"
}
```

### `ralphglasses_scratchpad_resolve`

Mark a numbered scratchpad item as resolved

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `item_number` | number | yes | Item number to resolve |
| `name` | string | yes | Scratchpad name |
| `resolution` | string | yes | Resolution description |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "arguments": {
    "item_number": 1,
    "name": "...",
    "resolution": "..."
  },
  "tool": "ralphglasses_scratchpad_resolve"
}
```

### `ralphglasses_scratchpad_delete`

Delete a numbered finding from a scratchpad file

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `finding_id` | string | yes | Finding number to delete (e.g. '2') |
| `scratchpad` | string | yes | Scratchpad name |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "arguments": {
    "finding_id": "...",
    "scratchpad": "..."
  },
  "tool": "ralphglasses_scratchpad_delete"
}
```

### `ralphglasses_scratchpad_validate`

Validate scratchpad entries against expected constraints: score inflation, path errors, budget mismatches, no-op iterations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `check` | string | yes | Validation type: scores, paths, budget, noops, all |
| `name` | string | yes | Scratchpad name |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "arguments": {
    "check": "...",
    "name": "..."
  },
  "tool": "ralphglasses_scratchpad_validate"
}
```

### `ralphglasses_scratchpad_context`

Auto-enrich scratchpad with current system context: fleet status, observations, routing, autonomy level

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Scratchpad name |
| `sections` | string | yes | Comma-separated sections: fleet, observations, routing, autonomy, all |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "sections": "..."
  },
  "tool": "ralphglasses_scratchpad_context"
}
```

### `ralphglasses_scratchpad_reason`

Record intermediate reasoning between tool calls: dimension-to-stage mapping, rate cards, prune thresholds, provider selection

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Scratchpad name |
| `topic` | string | yes | Reasoning topic: enhance_stages, rate_cards, prune_thresholds, provider_selection |
| `input` | string |  | JSON string with context data for the reasoning |
| `repo` | string |  | Repo name (auto-detected from CWD; required when multiple repos are scanned) |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "topic": "..."
  },
  "tool": "ralphglasses_scratchpad_reason"
}
```

### `ralphglasses_coverage_report`

Run go test -coverprofile and report per-package coverage vs threshold

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name or absolute path |
| `packages` | string |  | Package pattern (default ./...) |
| `threshold` | number |  | Coverage threshold percentage (default 70) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_coverage_report"
}
```

### `ralphglasses_cost_estimate`

Pre-launch cost estimate for a session or loop

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `provider` | string | yes | Provider: claude, gemini, or codex |
| `iterations` | number |  | Loop iterations for mode=loop (default 3) |
| `mode` | string |  | 'session' or 'loop' (default session) |
| `model` | string |  | Model name (uses provider default if omitted) |
| `output_tokens_per_turn` | number |  | Output tokens per turn (default 2000) |
| `prompt_tokens` | number |  | Prompt length in tokens (default 5000) |
| `repo` | string |  | Repo name for historical calibration |
| `turns` | number |  | Expected conversation turns (default 5) |

**Example:**

```json
{
  "arguments": {
    "provider": "..."
  },
  "tool": "ralphglasses_cost_estimate"
}
```

### `ralphglasses_merge_verify`

Run build+vet+test sequence to verify a merge

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo path (absolute) |
| `coverage` | boolean |  | Include coverage profile |
| `fast` | boolean |  | Use -short flag for faster tests |
| `packages` | string |  | Package pattern to test (default ./...) |
| `race` | boolean |  | Enable -race detector (default true) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_merge_verify"
}
```

### `ralphglasses_observation_correlate`

Link observations to git commits by timestamp proximity

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `hours` | number |  | Hours of history to correlate (default 24) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_observation_correlate"
}
```

## rdcycle

R&D cycle automation — finding-to-task conversion, cycle planning, baselines, merging, and scheduling

### `ralphglasses_finding_to_task`

Convert a scratchpad finding into a structured loop task spec

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `scratchpad_name` | string | yes | Scratchpad filename without path; supports .md or .jsonl (default .md) |
| `finding_id` | string |  | Finding ID (e.g., FINDING-240); if omitted, all findings in the scratchpad are converted |

**Example:**

```json
{
  "arguments": {
    "repo": "...",
    "scratchpad_name": "..."
  },
  "tool": "ralphglasses_finding_to_task"
}
```

### `ralphglasses_cycle_baseline`

Snapshot current repo metrics as a cycle baseline

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_baseline"
}
```

### `ralphglasses_cycle_plan`

Generate a prioritized task plan for the next R&D cycle from unresolved findings

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `budget` | number |  | Budget in USD (default: 5.0) |
| `max_tasks` | number |  | Maximum tasks to include (default: 10) |
| `previous_cycle_id` | string |  | Previous cycle ID for continuity |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_plan"
}
```

### `ralphglasses_cycle_merge`

Merge parallel worktree results with conflict detection

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `worktree_paths` | string | yes | Comma-separated worktree paths to merge |
| `conflict_strategy` | string |  | Conflict resolution: ours, theirs, or manual (default: manual) |

**Example:**

```json
{
  "arguments": {
    "worktree_paths": "..."
  },
  "tool": "ralphglasses_cycle_merge"
}
```

### `ralphglasses_cycle_schedule`

Schedule recurring R&D cycles with cron expressions

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cron_expr` | string | yes | Cron expression (e.g., '0 2 * * 1' for Monday 2am) |
| `repo` | string | yes | Repository name or path |
| `criteria` | string |  | Comma-separated success criteria when cycle_config is omitted |
| `cycle_config` | string |  | JSON cycle configuration |
| `enabled` | boolean |  | Whether the schedule is active (default: true) |
| `job_kind` | string |  | Scheduled job kind (default: cycle) |
| `max_tasks` | number |  | Max cycle tasks when cycle_config is omitted |
| `name` | string |  | Cycle name for cycle schedules |
| `objective` | string |  | Cycle objective when cycle_config is omitted |

**Example:**

```json
{
  "arguments": {
    "cron_expr": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_schedule"
}
```

### `ralphglasses_loop_replay`

Replay a failed loop iteration with modified parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `iteration` | number | yes | Iteration number to replay |
| `loop_id` | string | yes | Loop run ID |
| `overrides` | string |  | JSON overrides: {model, provider, budget} |
| `repo` | string |  | Repo name (auto-detected if omitted) |

**Example:**

```json
{
  "arguments": {
    "iteration": 1,
    "loop_id": "..."
  },
  "tool": "ralphglasses_loop_replay"
}
```

### `ralphglasses_budget_forecast`

Predict cost of N more iterations based on historical cost observations

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `loop_id` | string | yes | Loop run ID to forecast |
| `iterations` | number |  | Number of iterations to forecast (default 10) |
| `repo` | string |  | Repo name (auto-detected if omitted) |

**Example:**

```json
{
  "arguments": {
    "loop_id": "..."
  },
  "tool": "ralphglasses_budget_forecast"
}
```

### `ralphglasses_diff_review`

Auto-review a git diff for quality issues: scope creep, missing tests, TODOs, style

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `checks` | string |  | CSV of checks: scope_creep, missing_tests, style, todos (default: all) |
| `ref` | string |  | Git ref to review (default HEAD) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_diff_review"
}
```

### `ralphglasses_finding_reason`

Analyze scratchpad findings for root causes by category clustering

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Scratchpad name (e.g., 'tool_improvement') |
| `repo` | string |  | Repo name (auto-detected if omitted) |

**Example:**

```json
{
  "arguments": {
    "name": "..."
  },
  "tool": "ralphglasses_finding_reason"
}
```

### `ralphglasses_cycle_create`

Create a new R&D cycle with objective and success criteria

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `objective` | string | yes | Cycle objective |
| `repo` | string | yes | Repository name or path |
| `criteria` | string |  | Comma-separated success criteria |
| `name` | string |  | Cycle name (default: 'cycle') |

**Example:**

```json
{
  "arguments": {
    "objective": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_create"
}
```

### `ralphglasses_cycle_advance`

Advance active cycle to next phase in state machine

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `cycle_id` | string |  | Cycle ID (default: active cycle) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_advance"
}
```

### `ralphglasses_cycle_status`

Get current state of a cycle including tasks, findings, and synthesis

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `cycle_id` | string |  | Cycle ID (default: active cycle) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_status"
}
```

### `ralphglasses_cycle_fail`

Mark a cycle as failed with error message

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `cycle_id` | string |  | Cycle ID (default: active cycle) |
| `error` | string |  | Error message |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_fail"
}
```

### `ralphglasses_cycle_list`

List all R&D cycles for a repo, sorted by most recent

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `limit` | number |  | Max results (default 20) |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_list"
}
```

### `ralphglasses_cycle_synthesize`

Set cycle synthesis and advance to complete

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name or path |
| `summary` | string | yes | Synthesis summary |
| `accomplished` | string |  | Comma-separated list of accomplishments |
| `cycle_id` | string |  | Cycle ID (default: active cycle) |
| `next_objective` | string |  | Objective for next cycle |
| `patterns` | string |  | Comma-separated patterns observed |
| `remaining` | string |  | Comma-separated list of remaining work |

**Example:**

```json
{
  "arguments": {
    "repo": "...",
    "summary": "..."
  },
  "tool": "ralphglasses_cycle_synthesize"
}
```

### `ralphglasses_cycle_run`

Drive a full R&D cycle synchronously through all phases: create, baseline, plan, execute, observe, synthesize, complete

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `objective` | string | yes | Cycle objective |
| `repo` | string | yes | Repository name or path |
| `criteria` | string |  | Comma-separated success criteria |
| `max_tasks` | number |  | Max tasks to plan (default 10) |
| `name` | string |  | Cycle name (default: 'cycle') |
| `repo_path` | string |  | Deprecated alias for repo; retained for backward compatibility |

**Example:**

```json
{
  "arguments": {
    "objective": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_cycle_run"
}
```

## plugin

Plugin management: list, info, enable, disable registered plugins

### `ralphglasses_plugin_list`

List all registered plugins with name, version, status, and type (builtin/yaml/grpc)

**Example:**

```json
{
  "tool": "ralphglasses_plugin_list"
}
```

### `ralphglasses_plugin_info`

Show detailed information for a specific plugin

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Plugin name |

**Example:**

```json
{
  "arguments": {
    "name": "..."
  },
  "tool": "ralphglasses_plugin_info"
}
```

### `ralphglasses_plugin_enable`

Enable a disabled plugin

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Plugin name to enable |

**Example:**

```json
{
  "arguments": {
    "name": "..."
  },
  "tool": "ralphglasses_plugin_enable"
}
```

### `ralphglasses_plugin_disable`

Disable an active plugin

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Plugin name to disable |

**Example:**

```json
{
  "arguments": {
    "name": "..."
  },
  "tool": "ralphglasses_plugin_disable"
}
```

## sweep

Cross-repo audit sweeps: generate optimized prompts, fan-out sessions, monitor, nudge stalled sessions, schedule recurring checks

### `ralphglasses_sweep_generate`

Generate an optimized audit prompt using the 13-stage enhancer pipeline. Returns enhanced prompt text, quality score, and stages applied.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `custom_prompt` | string |  | Custom base prompt instead of built-in template. Will be enhanced through the pipeline. |
| `target_provider` | string |  | Target model provider for structure style: openai (default), claude, gemini |
| `task_type` | string |  | Task type for prompt template: audit (default), fix, review, improve |

**Example:**

```json
{
  "tool": "ralphglasses_sweep_generate"
}
```

### `ralphglasses_sweep_launch`

Launch an enhanced prompt against multiple repos as parallel sessions. Returns a sweep_id for tracking all sessions as a group.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | The prompt to run (use sweep_generate output or provide your own). Use REPO_PLACEHOLDER for repo name substitution. |
| `allowed_tools` | string |  | Comma-separated allowed tools (default: read-only tools for plan mode) |
| `budget_usd` | number |  | Per-session budget in USD (default: 5.0) |
| `effort` | string |  | Effort level: low, medium, high, max |
| `enhance_prompt` | string |  | Enhance each repo's prompt before launch: local (default), llm, auto, none |
| `limit` | number |  | Max repos to launch against (default 10) |
| `max_sweep_budget_usd` | number |  | Total sweep budget cap in USD (default 100). Rejects launch if estimated cost exceeds this. |
| `max_turns` | number |  | Max turns per session (default 50). Prevents runaway sessions. |
| `model` | string |  | Model to use: opus (default), sonnet, haiku |
| `permission_mode` | string |  | Claude permission mode: plan (default, read-only), auto, default |
| `repos` | string |  | JSON array of repo names, or "active" for recently-active repos, or "all". Default: active |
| `session_persistence` | boolean |  | Persist session history to disk (default false for sweeps). |
| `sweep_concurrency` | number |  | Max simultaneous session launches (default 10). Use 1 for serial, higher for large idle machines. |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_sweep_launch"
}
```

### `ralphglasses_sweep_status`

Dashboard for a sweep: per-repo status, total cost, completion percentage, stalled sessions, and optional output tails.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sweep_id` | string | yes | Sweep ID returned by sweep_launch |
| `verbose` | boolean |  | Include last output lines per session (default false) |

**Example:**

```json
{
  "arguments": {
    "sweep_id": "..."
  },
  "tool": "ralphglasses_sweep_status"
}
```

### `ralphglasses_sweep_nudge`

Detect and restart stalled sessions in a sweep. Identifies sessions idle beyond threshold and restarts them with the same prompt.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sweep_id` | string | yes | Sweep ID to nudge |
| `action` | string |  | Action for stalled sessions: restart (default), skip |
| `stale_threshold_min` | number |  | Minutes idle before a session is considered stalled (default 5) |

**Example:**

```json
{
  "arguments": {
    "sweep_id": "..."
  },
  "tool": "ralphglasses_sweep_nudge"
}
```

### `ralphglasses_sweep_schedule`

Set up recurring status checks for a sweep at configurable intervals. Optionally auto-nudges stalled sessions. Returns a task_id for cancellation.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sweep_id` | string | yes | Sweep ID to monitor |
| `auto_nudge` | boolean |  | Automatically restart stalled sessions (default false) |
| `interval_minutes` | number |  | Check interval in minutes (default 5) |
| `max_checks` | number |  | Stop after N checks, 0 for unlimited (default 0) |
| `max_sweep_budget_usd` | number |  | Abort sweep and stop all sessions if total spend exceeds this (default 0 = no cap) |

**Example:**

```json
{
  "arguments": {
    "sweep_id": "..."
  },
  "tool": "ralphglasses_sweep_schedule"
}
```

### `ralphglasses_sweep_report`

Generate a Markdown or JSON summary of a completed sweep: per-repo status, commits, costs, and changes.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sweep_id` | string | yes | Sweep ID to report on |
| `format` | string |  | Output format: markdown (default), json |

**Example:**

```json
{
  "arguments": {
    "sweep_id": "..."
  },
  "tool": "ralphglasses_sweep_report"
}
```

### `ralphglasses_sweep_retry`

Re-launch only failed or errored sessions from a sweep, preserving all original settings.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sweep_id` | string | yes | Sweep ID to retry |
| `budget_usd` | number |  | Override per-session budget for retry (default: use original) |

**Example:**

```json
{
  "arguments": {
    "sweep_id": "..."
  },
  "tool": "ralphglasses_sweep_retry"
}
```

### `ralphglasses_sweep_push`

Push all repos in a sweep that have unpushed commits to their remote.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `sweep_id` | string | yes | Sweep ID whose repos to push |
| `dry_run` | boolean |  | Show what would be pushed without pushing (default false) |

**Example:**

```json
{
  "arguments": {
    "sweep_id": "..."
  },
  "tool": "ralphglasses_sweep_push"
}
```

## rc

Remote control — send prompts, read output, and act on sessions from mobile or scripted contexts

### `ralphglasses_rc_status`

Compact fleet overview for mobile: active sessions, costs, alerts in readable text

**Example:**

```json
{
  "tool": "ralphglasses_rc_status"
}
```

### `ralphglasses_rc_send`

Send prompt to repo -- auto-stops existing session, launches new. The 'input' tool for remote control.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | What to tell the agent |
| `repo` | string | yes | Repo name |
| `budget_usd` | number |  | Budget in USD (default: 5.0) |
| `model` | string |  | Override model |
| `provider` | string |  | codex (default), claude, gemini |
| `resume` | string |  | true to resume last session instead of fresh start |

**Example:**

```json
{
  "arguments": {
    "prompt": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_rc_send"
}
```

### `ralphglasses_rc_read`

Read recent output from most active session. Combines tail + status for mobile.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `cursor` | string |  | Cursor from previous call -- only new output |
| `id` | string |  | Session ID (omit for most recently active) |
| `lines` | number |  | Max lines (default 10, max 30) |

**Example:**

```json
{
  "tool": "ralphglasses_rc_read"
}
```

### `ralphglasses_rc_act`

Quick fleet action: stop, stop_all, pause, resume, retry. Single tool for all control actions.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | Action: stop, stop_all, pause, resume, retry |
| `target` | string |  | Session ID or repo name (required except stop_all) |

**Example:**

```json
{
  "arguments": {
    "action": "..."
  },
  "tool": "ralphglasses_rc_act"
}
```

### `ralphglasses_dispatch`

Unified cross-provider dispatch for mobile RC. Sends prompts, stops, pauses, resumes, or retries sessions with auto provider selection.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | Action: send, stop, pause, resume, retry |
| `repo` | string | yes | Repo name |
| `prompt` | string |  | Prompt text (required for send action) |
| `provider` | string |  | Provider: claude, codex, gemini, or auto (default: auto) |

**Example:**

```json
{
  "arguments": {
    "action": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_dispatch"
}
```

### `ralphglasses_fleet_summary`

One-call fleet overview: total sessions, active/stopped counts, total cost, per-repo status. Compact text for mobile.

**Example:**

```json
{
  "tool": "ralphglasses_fleet_summary"
}
```

## autonomy

Autonomy management — view/set autonomy level, inspect supervisor status, review and override autonomous decisions, track HITL events

### `ralphglasses_autonomy_level`

View or set the autonomy level (0=observe, 1=auto-recover, 2=auto-optimize, 3=full-autonomy). When setting, also starts/stops the autonomous supervisor.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `level` | string |  | New level: 0-3 or name (omit to view current) |
| `repo_path` | string |  | Repo path for the supervisor (used when setting level) |

**Example:**

```json
{
  "tool": "ralphglasses_autonomy_level"
}
```

### `ralphglasses_supervisor_status`

Returns the autonomous supervisor status including whether it's running, tick count, and last cycle launch time.

**Example:**

```json
{
  "tool": "ralphglasses_supervisor_status"
}
```

### `ralphglasses_automation_policy`

View, set, or recommend repo-local subscription-window automation policy for Codex pause/resume pacing.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `action` | string |  | Action: get, set, recommend (default: get) |
| `default_model` | string |  | Default model for queued work |
| `default_task_budget_usd` | number |  | Default budget for queued work items |
| `default_task_max_turns` | number |  | Default max turns for queued work items |
| `enabled` | boolean |  | Enable or disable subscription-window automation |
| `max_concurrent_sessions` | number |  | Max concurrent automated sessions (v1 only supports 1) |
| `provider` | string |  | Provider to automate (default: codex) |
| `reset_anchor` | string |  | RFC3339 anchor timestamp when using fixed reset_window_hours |
| `reset_cron` | string |  | 5-field cron for subscription reset timing |
| `reset_window_hours` | number |  | Window size in hours when using reset_anchor |
| `resume_backoff_minutes` | number |  | Backoff between failed auto-resume attempts |
| `target_utilization_pct` | number |  | Target percent of the synthetic budget to consume before reset |
| `timezone` | string |  | IANA timezone for reset windows |
| `window_budget_usd` | number |  | Synthetic per-window budget used for pacing and forecasting |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_automation_policy"
}
```

### `ralphglasses_automation_queue`

Manage the durable queue for repo-local subscription-window automation.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repo name |
| `action` | string |  | Action: list, enqueue, remove, reprioritize (default: list) |
| `budget_usd` | number |  | Budget override for queued item |
| `criteria` | string |  | Comma-separated success criteria for cycle jobs |
| `id` | string |  | Queue item ID for remove/reprioritize |
| `job_kind` | string |  | Queue job kind: session (default), loop, cycle, research |
| `max_tasks` | number |  | Max tasks for cycle jobs |
| `max_turns` | number |  | Max turns override for queued item |
| `model` | string |  | Model override for queued item |
| `name` | string |  | Cycle name for cycle jobs |
| `objective` | string |  | Cycle or research objective/topic |
| `priority` | number |  | Queue priority (higher runs first) |
| `prompt` | string |  | Prompt to enqueue |
| `provider` | string |  | Provider override for queued item |
| `research_domain` | string |  | Research domain for research jobs |
| `research_model_tier` | string |  | Research queue model tier for research jobs |
| `research_priority_score` | number |  | Research queue priority score for research jobs |
| `source` | string |  | Queue item source label |

**Example:**

```json
{
  "arguments": {
    "repo": "..."
  },
  "tool": "ralphglasses_automation_queue"
}
```

### `ralphglasses_autonomy_decisions`

Recent autonomous decisions with rationale, inputs, and outcomes

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | Max decisions (default: 20) |

**Example:**

```json
{
  "tool": "ralphglasses_autonomy_decisions"
}
```

### `ralphglasses_autonomy_override`

Override/reverse an autonomous decision and record human intervention

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `decision_id` | string | yes | Decision ID to override |
| `details` | string |  | Why this was overridden |

**Example:**

```json
{
  "arguments": {
    "decision_id": "..."
  },
  "tool": "ralphglasses_autonomy_override"
}
```

### `ralphglasses_hitl_score`

Current human-in-the-loop score: manual interventions vs autonomous actions, with trend

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `hours` | number |  | Time window in hours (default: 24) |

**Example:**

```json
{
  "tool": "ralphglasses_hitl_score"
}
```

### `ralphglasses_hitl_history`

Recent HITL events: manual stops, auto-recoveries, config changes, etc.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `hours` | number |  | Time window in hours (default: 24) |
| `limit` | number |  | Max events (default: 50) |

**Example:**

```json
{
  "tool": "ralphglasses_hitl_history"
}
```

## workflow

Workflow automation — define, run, and delete multi-step YAML workflows that sequence agent sessions

### `ralphglasses_workflow_define`

Define a multi-step workflow as YAML

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Workflow name |
| `repo` | string | yes | Repo name |
| `yaml` | string | yes | Workflow YAML definition |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "repo": "...",
    "yaml": "..."
  },
  "tool": "ralphglasses_workflow_define"
}
```

### `ralphglasses_workflow_run`

Execute a defined workflow, launching sessions per step

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Workflow name |
| `repo` | string | yes | Repo name |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_workflow_run"
}
```

### `ralphglasses_workflow_delete`

Delete a workflow definition file

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `name` | string | yes | Workflow name to delete |
| `repo` | string | yes | Repo name |

**Example:**

```json
{
  "arguments": {
    "name": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_workflow_delete"
}
```

## docs

Docs repo integration: search research, check existing, write findings, push changes, meta-roadmap coordination, cross-repo roadmap management

### `ralphglasses_docs_search`

Full-text search across docs/research/ files using ripgrep. Returns matching file paths, line numbers, and previews.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | yes | Search query (regex supported) |
| `domain` | string |  | Filter to research domain: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive |
| `limit` | number |  | Max results (default: 20) |

**Example:**

```json
{
  "arguments": {
    "query": "..."
  },
  "tool": "ralphglasses_docs_search"
}
```

### `ralphglasses_docs_check_existing`

Check if research exists on a topic before starting new work. Searches SEARCH-GUIDE.md and all research files. Returns recommendation: read existing or proceed with new research.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `topic` | string | yes | Topic to check (e.g., 'MCP tool design', 'cascade routing') |

**Example:**

```json
{
  "arguments": {
    "topic": "..."
  },
  "tool": "ralphglasses_docs_check_existing"
}
```

### `ralphglasses_docs_write_finding`

Write a research finding to docs/research/<domain>/<filename>. Validates domain and creates directory if needed.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `content` | string | yes | Markdown content to write |
| `domain` | string | yes | Research domain: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive |
| `filename` | string | yes | Filename (kebab-case.md, e.g., 'fleet-metrics-scaling.md') |

**Example:**

```json
{
  "arguments": {
    "content": "...",
    "domain": "...",
    "filename": "..."
  },
  "tool": "ralphglasses_docs_write_finding"
}
```

### `ralphglasses_docs_push`

Commit and push all pending changes in the docs repo via push-docs.sh

**Example:**

```json
{
  "tool": "ralphglasses_docs_push"
}
```

### `ralphglasses_meta_roadmap_status`

Parse docs/strategy/META-ROADMAP.md and return phase count, task totals, completion percentage, and summary

**Example:**

```json
{
  "tool": "ralphglasses_meta_roadmap_status"
}
```

### `ralphglasses_meta_roadmap_next_task`

Get the next incomplete task from META-ROADMAP.md, optionally filtered by phase name

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `phase` | string |  | Filter to phase containing this string (e.g., 'Wave 1', 'Security') |

**Example:**

```json
{
  "tool": "ralphglasses_meta_roadmap_next_task"
}
```

### `ralphglasses_roadmap_cross_repo`

Compare roadmap progress across all repos using docs/snapshots/roadmaps/. Returns repos sorted by completion (least complete first).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | Max repos to return (default: 10) |

**Example:**

```json
{
  "tool": "ralphglasses_roadmap_cross_repo"
}
```

### `ralphglasses_roadmap_assign_loop`

Create an R&D loop targeting a specific roadmap task. Returns loop_start parameters for the task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string | yes | Repository name |
| `task` | string | yes | Task description from roadmap |
| `budget_usd` | number |  | Budget in USD (default: 3.0) |
| `provider` | string |  | Provider: claude (default), gemini, codex |

**Example:**

```json
{
  "arguments": {
    "repo": "...",
    "task": "..."
  },
  "tool": "ralphglasses_roadmap_assign_loop"
}
```

## recovery

Emergency session recovery: triage killed sessions, salvage partial output, generate recovery plans, batch re-launch, write incident reports, discover orphaned sessions

### `ralphglasses_session_triage`

Triage killed/interrupted sessions across all repos within a time window. Groups by kill reason, repo, provider. Shows cost wasted and recovery potential.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `repo` | string |  | Filter by repo name (omit for all) |
| `since` | string |  | Start of time window (RFC3339 or relative: '2h', '30m', '1d'). Default: 1h |
| `status` | string |  | Comma-separated statuses: interrupted, errored, stopped (default: interrupted,errored) |
| `until` | string |  | End of time window (RFC3339 or relative). Default: now |

**Example:**

```json
{
  "tool": "ralphglasses_session_triage"
}
```

### `ralphglasses_session_salvage`

Extract partial output from a killed session, classify what was accomplished vs lost, and generate a recovery task prompt.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | string | yes | Session ID to salvage |
| `generate_prompt` | boolean |  | Generate a recovery prompt that continues where the session left off (default: true) |
| `save_to_docs` | string |  | Domain to save salvaged findings to docs/research/<domain>/ (omit to skip) |

**Example:**

```json
{
  "arguments": {
    "id": "..."
  },
  "tool": "ralphglasses_session_salvage"
}
```

### `ralphglasses_recovery_plan`

Generate a prioritized recovery plan from killed sessions. Categorizes each: retry (transient error), salvage-and-close (non-recoverable), or escalate (unclear). Respects budget cap.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `budget_cap_usd` | number |  | Max total budget for retry operations (default: 50.0) |
| `session_ids` | string |  | Comma-separated session IDs (omit to auto-discover from time window) |
| `since` | string |  | Time window for auto-discovery (RFC3339 or relative). Default: 1h |
| `strategy` | string |  | Strategy: conservative, aggressive, cost-aware (default: cost-aware) |

**Example:**

```json
{
  "tool": "ralphglasses_recovery_plan"
}
```

### `ralphglasses_recovery_execute`

Execute a recovery plan: batch re-launch retry sessions in parallel, tracked as a sweep with budget cap.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `plan_json` | string | yes | JSON recovery plan (the 'actions' array from recovery_plan output) |
| `budget_cap_usd` | number |  | Total sweep budget cap (default: 50.0) |
| `concurrency` | number |  | Max simultaneous re-launches (default: 5) |
| `model_override` | string |  | Override model for all retries (e.g., downgrade to save cost) |

**Example:**

```json
{
  "arguments": {
    "plan_json": "..."
  },
  "tool": "ralphglasses_recovery_execute"
}
```

### `ralphglasses_incident_report`

Generate a structured incident report in docs/research/incidents/. Includes timeline, affected sessions, salvaged outputs, recovery actions taken, and lessons learned.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `title` | string | yes | Incident title (kebab-case, used as filename) |
| `cause` | string |  | Root cause description (e.g., 'hyprland-hy3-plugin-crash') |
| `recovery_sweep_id` | string |  | Sweep ID from recovery_execute, if recovery was run |
| `session_ids` | string |  | Comma-separated affected session IDs (omit to auto-discover) |
| `since` | string |  | Incident window start (RFC3339 or relative). Default: 1h |

**Example:**

```json
{
  "arguments": {
    "title": "..."
  },
  "tool": "ralphglasses_incident_report"
}
```

### `ralphglasses_session_discover`

Scan all repos' .ralph/ directories and Claude Code project dirs to discover session state beyond the local store. Finds orphaned processes and external session files.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `check_processes` | boolean |  | Check if discovered sessions still have running processes (default: true) |
| `include_claude_projects` | boolean |  | Also scan ~/.claude/projects/ for session metadata (default: true) |
| `scan_path` | string |  | Base directory to scan (default: configured scan path) |

**Example:**

```json
{
  "tool": "ralphglasses_session_discover"
}
```

## promptdj

Prompt DJ: quality-aware prompt routing to optimal providers

### `ralphglasses_promptdj_route`

Route a prompt to the best provider based on quality, task type, and domain. Does NOT launch a session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Prompt text to route |
| `repo` | string |  | Repository name |
| `score` | number |  | Pre-computed quality score 0-100 |
| `task_type` | string |  | Override: code, analysis, troubleshooting, creative, workflow, general |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_promptdj_route"
}
```

### `ralphglasses_promptdj_dispatch`

Route AND launch a session. Optionally enhances prompt if quality is low.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Prompt to route and dispatch |
| `repo` | string | yes | Repository path |
| `budget_usd` | number |  | Max session budget USD |
| `dry_run` | boolean |  | Preview routing without launching |
| `enhance` | string |  | Enhancement: none, local, auto (default: auto) |
| `task_type` | string |  | Override task type |

**Example:**

```json
{
  "arguments": {
    "prompt": "...",
    "repo": "..."
  },
  "tool": "ralphglasses_promptdj_dispatch"
}
```

### `ralphglasses_promptdj_feedback`

Record outcome feedback to improve routing over time.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `decision_id` | string | yes | Decision ID from route/dispatch |
| `success` | boolean | yes | Whether session succeeded |
| `correct_provider` | string |  | Correct provider if DJ chose wrong |
| `cost_usd` | number |  | Actual cost USD |
| `notes` | string |  | Outcome notes |
| `turns` | number |  | Actual turn count |

**Example:**

```json
{
  "arguments": {
    "decision_id": "...",
    "success": true
  },
  "tool": "ralphglasses_promptdj_feedback"
}
```

### `ralphglasses_promptdj_similar`

Find similar high-quality prompts from the registry for few-shot context injection. Uses BM25-lite keyword similarity, Jaccard tag overlap, and MMR diversity re-ranking.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Prompt to find similar examples for |
| `repo` | string |  | Repository context for relevance boosting |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_promptdj_similar"
}
```

### `ralphglasses_promptdj_suggest`

Get routing-aware improvement suggestions for a prompt. Shows where it would route, quality score, and actionable suggestions by category (quality, structure, cost, provider_fit).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `prompt` | string | yes | Prompt to analyze |
| `repo` | string |  | Repository context |

**Example:**

```json
{
  "arguments": {
    "prompt": "..."
  },
  "tool": "ralphglasses_promptdj_suggest"
}
```

### `ralphglasses_promptdj_history`

View routing decision history with optional summary. Filter by repo, provider, task type, status, and time window.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `limit` | number |  | Max results (default 50) |
| `provider` | string |  | Filter by provider |
| `repo` | string |  | Filter by repo |
| `since` | string |  | Time window: RFC3339 or duration ('24h', '7d') |
| `status` | string |  | Filter: routed, dispatched, succeeded, failed, all |
| `summary` | boolean |  | If true, return aggregate stats instead of individual decisions |
| `task_type` | string |  | Filter by task type |

**Example:**

```json
{
  "tool": "ralphglasses_promptdj_history"
}
```

## a2a

A2A protocol integration: discover agents, send tasks, check status, export agent card

### `ralphglasses_a2a_discover`

Discover an A2A agent by fetching its agent card from /.well-known/agent.json. Returns skills, capabilities, and metadata.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | Base URL of the A2A agent |

**Example:**

```json
{
  "arguments": {
    "url": "..."
  },
  "tool": "ralphglasses_a2a_discover"
}
```

### `ralphglasses_a2a_send`

Send a task to a remote A2A agent. Returns task ID and initial state.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `message` | string | yes | Task message to send |
| `url` | string | yes | Base URL of the A2A agent |
| `task_id` | string |  | Optional task ID (auto-generated if empty) |

**Example:**

```json
{
  "arguments": {
    "message": "...",
    "url": "..."
  },
  "tool": "ralphglasses_a2a_send"
}
```

### `ralphglasses_a2a_status`

Check the status of a previously sent A2A task.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `task_id` | string | yes | Task ID to check |
| `url` | string | yes | Base URL of the A2A agent |

**Example:**

```json
{
  "arguments": {
    "task_id": "...",
    "url": "..."
  },
  "tool": "ralphglasses_a2a_status"
}
```

### `ralphglasses_a2a_agent_card`

Generate and return this server's A2A agent card (our capabilities as an A2A agent).

**Example:**

```json
{
  "tool": "ralphglasses_a2a_agent_card"
}
```

## trigger

External agent triggering and cron-based scheduling

### `ralphglasses_trigger_webhook`

Trigger an agent session from an external source (webhook, API, or another agent). Creates a trigger record and optionally launches immediately.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agent_type` | string | yes | Agent type to trigger |
| `prompt` | string | yes | Task prompt for the agent session |
| `budget_usd` | number |  | Budget in USD for the session |
| `launch` | boolean |  | Launch the session immediately (default: false, just queue) |
| `max_turns` | number |  | Maximum conversation turns |
| `model` | string |  | Model override for the session |
| `priority` | number |  | Priority 1-10, higher = more urgent (default: 5) |
| `repo` | string |  | Repo name (required when launch=true) |
| `tenant_id` | string |  | Workspace tenant ID (default: _default) |

**Example:**

```json
{
  "arguments": {
    "agent_type": "...",
    "prompt": "..."
  },
  "tool": "ralphglasses_trigger_webhook"
}
```

### `ralphglasses_schedule_create`

Create, list, enable, or disable cron-based schedules. When repo is provided, schedules are persisted to that repo's .ralph/schedules/*.json; otherwise the legacy ~/.ralph/schedules.json store is used.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string |  | Action: create (default), list, enable, disable |
| `agent_type` | string |  | Agent type: ralph (default), loop, cycle |
| `budget_usd` | number |  | Budget override for repo-local schedules |
| `criteria` | string |  | Comma-separated cycle success criteria |
| `cron_expression` | string |  | Cron expression e.g. '0 */6 * * *' (required for create) |
| `enabled` | boolean |  | Whether the schedule is active (default: true) |
| `id` | string |  | Schedule ID (required for enable/disable) |
| `max_tasks` | number |  | Max planned tasks for cycle schedules |
| `max_turns` | number |  | Max turns override for repo-local schedules |
| `model` | string |  | Model override for repo-local schedules |
| `name` | string |  | Cycle name for cycle schedules |
| `objective` | string |  | Cycle objective override for cycle schedules |
| `priority` | number |  | Queue priority for repo-local schedules |
| `prompt` | string |  | Task prompt (required for create) |
| `provider` | string |  | Provider override for repo-local schedules |
| `repo` | string |  | Repo name or path for canonical repo-local schedules |

**Example:**

```json
{
  "tool": "ralphglasses_schedule_create"
}
```

## approval

Human-in-the-loop approval: request, resolve, list pending approvals (Factor 7: Contact Humans with Tool Calls)

### `ralphglasses_request_approval`

Request human approval for an action — creates a pending record and optionally pauses the session until resolved

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `action` | string | yes | What needs approval (e.g. 'merge PR #42', 'deploy to prod') |
| `context` | string | yes | Why this needs approval — background and rationale |
| `urgency` | string | yes | Urgency level: low, normal, high, critical |
| `session_id` | string |  | Session ID to pause while awaiting approval (omit to skip pause) |

**Example:**

```json
{
  "arguments": {
    "action": "...",
    "context": "...",
    "urgency": "..."
  },
  "tool": "ralphglasses_request_approval"
}
```

### `ralphglasses_resolve_approval`

Resolve a pending approval — approves or rejects the requested action and resumes the paused session if applicable

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `approval_id` | string | yes | Approval ID returned from request_approval |
| `decision` | string | yes | Decision: approved or rejected |
| `reason` | string |  | Reason for the decision |

**Example:**

```json
{
  "arguments": {
    "approval_id": "...",
    "decision": "..."
  },
  "tool": "ralphglasses_resolve_approval"
}
```

### `ralphglasses_list_approvals`

List pending approval requests (set include_resolved=true for all)

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `include_resolved` | boolean |  | Include already-resolved approvals (default: false) |

**Example:**

```json
{
  "tool": "ralphglasses_list_approvals"
}
```

## context

Context window budget monitoring: track token usage per session

### `ralphglasses_context_budget`

Get context window budget status for a session or all sessions. Returns used tokens, limit, utilization percent, and threshold status (ok/warning/critical).

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `session_id` | string |  | Session ID (omit to return all sessions) |

**Example:**

```json
{
  "tool": "ralphglasses_context_budget"
}
```

## prefetch

Deterministic context pre-fetching: inspect registered hooks

### `ralphglasses_prefetch_status`

List registered prefetch hooks that run before session launch to pre-gather context

**Example:**

```json
{
  "tool": "ralphglasses_prefetch_status"
}
```

