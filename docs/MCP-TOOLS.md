# MCP Server & Tools

Ralphglasses is an installable MCP server exposing 110 tools (108 namespace tools + 2 meta-tools) in 13 namespaces for managing ralph loops, multi-provider LLM sessions, fleet orchestration, and self-improvement subsystems programmatically.

## Install

```bash
# Via claude CLI (recommended)
claude mcp add ralphglasses -- go run . mcp

# Or with custom scan path
claude mcp add ralphglasses -e RALPHGLASSES_SCAN_PATH=~/hairglasses-studio -- go run . mcp

# Or via the Cobra subcommand
go run . mcp --scan-path ~/hairglasses-studio
```

A `.mcp.json` is also included in the repo root for automatic local discovery.

## Deferred Loading

To minimize startup latency and memory usage, only core tools are loaded upfront. The remaining tools are organized into 13 namespaces and loaded on demand via meta-tools:

| Namespace | Tools | Description |
|-----------|-------|-------------|
| `core` | 10 | Scan, list, status, start, stop, stop_all, pause, logs, config, config_bulk (always loaded) |
| `session` | 13 | Session lifecycle: launch, list, status, resume, stop, stop_all, budget, retry, output, tail, diff, compare, errors |
| `loop` | 9 | Perpetual dev loops: start, status, step, stop, benchmark, baseline, gates, self_test, self_improve |
| `prompt` | 8 | Prompt enhancement: analyze, enhance, lint, improve, classify, should_enhance, templates, template_fill |
| `fleet` | 6 | Fleet ops: fleet_status, analytics, submit, budget, workers, marathon_dashboard |
| `repo` | 5 | Repo management: health, optimize, scaffold, claudemd_check, snapshot |
| `roadmap` | 5 | Roadmap automation: parse, analyze, research, expand, export |
| `team` | 6 | Agent teams: team_create, team_status, team_delegate, agent_define, agent_list, agent_compose |
| `awesome` | 5 | Awesome-list research: fetch, analyze, diff, report, sync |
| `advanced` | 22 | RC tools, events, HITL, autonomy, feedback, journals, workflows, bandit, circuit breaker |
| `eval` | 4 | Offline evaluation: counterfactual, A/B test, changepoints, anomaly detection |
| `fleet_h` | 4 | Fleet intelligence: blackboard coordination, A2A offers, cost forecasting |
| `observability` | 11 | Observations, scratchpad, loop wait/poll, coverage, cost estimation, merge verification |

Use the meta-tools below to discover and load tool groups at runtime.

## Meta-Tools

| Tool | Description |
|------|-------------|
| `ralphglasses_tool_groups` | List available tool groups for deferred loading |
| `ralphglasses_load_tool_group` | Load a tool namespace on demand (e.g., `{"group": "session"}`) |

## Tools

### core (10 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_scan` | Scan for ralph-enabled repos and return their current status |
| `ralphglasses_list` | List all discovered repos with status summary |
| `ralphglasses_status` | Get detailed status for a specific repo including loop status, circuit breaker, progress, and config |
| `ralphglasses_start` | Start a ralph loop for a repo |
| `ralphglasses_stop` | Stop a running ralph loop for a repo |
| `ralphglasses_stop_all` | Stop all running ralph loops |
| `ralphglasses_pause` | Pause or resume a running ralph loop |
| `ralphglasses_logs` | Get recent log lines from a repo's ralph log |
| `ralphglasses_config` | Get or set .ralphrc config values for a repo |
| `ralphglasses_config_bulk` | Get/set .ralphrc values across multiple repos |

### session (13 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_session_launch` | Launch a headless LLM CLI session (claude/gemini/codex) for a repo |
| `ralphglasses_session_list` | List all tracked LLM sessions with status, cost, and turns |
| `ralphglasses_session_status` | Get detailed status for a session including output, cost, and turns |
| `ralphglasses_session_resume` | Resume a previous LLM CLI session |
| `ralphglasses_session_stop` | Stop a running session |
| `ralphglasses_session_stop_all` | Stop all running LLM sessions (emergency cost cutoff) |
| `ralphglasses_session_budget` | Get cost/budget info for a session, or update budget |
| `ralphglasses_session_retry` | Re-launch a failed session with same params, optional overrides |
| `ralphglasses_session_output` | Get recent output from a session's output history |
| `ralphglasses_session_tail` | Tail session output with cursor — returns only new lines since last call |
| `ralphglasses_session_diff` | Git changes made during a session's execution window |
| `ralphglasses_session_compare` | Compare two sessions by ID: cost, turns, duration, provider efficiency |
| `ralphglasses_session_errors` | Aggregated error view: parse failures, API errors, budget warnings |

### loop (9 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_loop_start` | Create a multi-provider planner/worker perpetual development loop for a repo |
| `ralphglasses_loop_status` | Get status for a perpetual development loop |
| `ralphglasses_loop_step` | Execute one planner/worker/verifier iteration for a loop |
| `ralphglasses_loop_stop` | Stop a perpetual development loop |
| `ralphglasses_loop_benchmark` | P50/P95 metrics from recent loop observations for a repo |
| `ralphglasses_loop_baseline` | Generate, view, or pin loop performance baseline for a repo |
| `ralphglasses_loop_gates` | Evaluate regression gates — returns pass/warn/fail report |
| `ralphglasses_self_test` | Run recursive self-test iterations against a repository using the loop engine |
| `ralphglasses_self_improve` | Start a self-improvement loop that autonomously improves a repository |

### prompt (8 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_prompt_analyze` | Score a prompt across 10 quality dimensions with letter grades and suggestions |
| `ralphglasses_prompt_enhance` | Run the 13-stage prompt enhancement pipeline |
| `ralphglasses_prompt_lint` | Deep-lint a prompt for anti-patterns: unmotivated rules, negative framing, injection risks, etc. |
| `ralphglasses_prompt_improve` | LLM-powered prompt improvement using Claude, Gemini, or OpenAI |
| `ralphglasses_prompt_classify` | Classify a prompt's task type (code, troubleshooting, analysis, creative, workflow, general) |
| `ralphglasses_prompt_should_enhance` | Check whether a prompt would benefit from enhancement |
| `ralphglasses_prompt_templates` | List available prompt templates with descriptions and required variables |
| `ralphglasses_prompt_template_fill` | Fill a prompt template with variable values |

### fleet (6 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_fleet_status` | Fleet-wide dashboard: aggregate status, costs, health, and alerts across all repos |
| `ralphglasses_fleet_analytics` | Cost breakdown by provider/repo/time-period with trend analysis |
| `ralphglasses_fleet_submit` | Submit work to the distributed fleet queue for execution on any worker |
| `ralphglasses_fleet_budget` | View or set the fleet-wide budget (spent, remaining, active work) |
| `ralphglasses_fleet_workers` | List registered fleet workers with status, capacity, and spend |
| `ralphglasses_marathon_dashboard` | Compact marathon status: burn rate, stale sessions, team progress, alerts |

### repo (5 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_repo_health` | Composite health check: circuit breaker, budget, staleness, errors, active sessions |
| `ralphglasses_repo_optimize` | Analyze and optimize ralph config files — detect misconfigs, missing settings, stale plans |
| `ralphglasses_repo_scaffold` | Create/initialize ralph config files for a repo |
| `ralphglasses_claudemd_check` | Health-check a repo's CLAUDE.md for common issues |
| `ralphglasses_snapshot` | Save or list fleet state snapshots |

### roadmap (5 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_roadmap_parse` | Parse ROADMAP.md into structured JSON (phases, sections, tasks, deps, completion stats) |
| `ralphglasses_roadmap_analyze` | Compare roadmap vs codebase — find gaps, stale checkboxes, ready tasks, orphaned code |
| `ralphglasses_roadmap_research` | Search GitHub for relevant repos and tools that unlock new capabilities |
| `ralphglasses_roadmap_expand` | Generate proposed roadmap expansions from analysis gaps and research findings |
| `ralphglasses_roadmap_export` | Export roadmap items as structured task specs for ralph loop consumption |

### team (6 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_team_create` | Create an agent team with a lead session that delegates tasks to teammates |
| `ralphglasses_team_status` | Get team status including lead session, tasks, and progress |
| `ralphglasses_team_delegate` | Add a new task to an existing team |
| `ralphglasses_agent_define` | Create or update an agent definition for a repo (supports all providers) |
| `ralphglasses_agent_list` | List available agent definitions for a repo (supports all providers) |
| `ralphglasses_agent_compose` | Create a composite agent by layering multiple existing agent definitions |

### awesome (5 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_awesome_fetch` | Fetch and parse an awesome-list README into structured entries with categories |
| `ralphglasses_awesome_analyze` | Deep-analyze repos: fetch READMEs, score value/complexity vs ralph capabilities |
| `ralphglasses_awesome_diff` | Compare current awesome-list against previous fetch (new/removed entries) |
| `ralphglasses_awesome_report` | Generate formatted report from saved analysis results |
| `ralphglasses_awesome_sync` | Full pipeline: fetch awesome-list, diff, analyze new entries, report, save |

### advanced (22 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_rc_status` | Compact fleet overview for mobile: active sessions, costs, alerts |
| `ralphglasses_rc_send` | Send prompt to repo — auto-stops existing session, launches new |
| `ralphglasses_rc_read` | Read recent output from most active session with cursor |
| `ralphglasses_rc_act` | Quick fleet action: stop, stop_all, pause, resume, retry |
| `ralphglasses_event_list` | Query recent fleet events from the event bus |
| `ralphglasses_event_poll` | Poll for new fleet events since last check (cursor-based) |
| `ralphglasses_hitl_score` | Current human-in-the-loop score: manual interventions vs autonomous actions |
| `ralphglasses_hitl_history` | Recent HITL events: manual stops, auto-recoveries, config changes |
| `ralphglasses_autonomy_level` | View or set the autonomy level (0-3) |
| `ralphglasses_autonomy_decisions` | Recent autonomous decisions with rationale, inputs, and outcomes |
| `ralphglasses_autonomy_override` | Override/reverse an autonomous decision and record human intervention |
| `ralphglasses_feedback_profiles` | Per-task-type and per-provider performance data from journal analysis |
| `ralphglasses_provider_recommend` | Recommend best provider + model + budget for a task based on feedback profiles |
| `ralphglasses_tool_benchmark` | Per-tool performance benchmarks: latency percentiles, success rates |
| `ralphglasses_journal_read` | Read improvement journal entries with synthesized context |
| `ralphglasses_journal_write` | Manually write an improvement note to a repo's journal |
| `ralphglasses_journal_prune` | Compact improvement journal to prevent unbounded growth |
| `ralphglasses_workflow_define` | Define a multi-step workflow as YAML |
| `ralphglasses_workflow_run` | Execute a defined workflow, launching sessions per step |
| `ralphglasses_bandit_status` | View multi-armed bandit arm statistics for provider selection |
| `ralphglasses_confidence_calibration` | View calibrated confidence model weights, training status, and feature importances |
| `ralphglasses_circuit_reset` | Reset circuit breaker state for a named service |

### eval (4 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_eval_counterfactual` | Estimate outcomes under hypothetical policy changes using inverse propensity scoring |
| `ralphglasses_eval_ab_test` | Bayesian A/B test comparing providers or time periods using Beta-Bernoulli model |
| `ralphglasses_eval_changepoints` | Detect performance shifts using CUSUM changepoint detection on loop observations |
| `ralphglasses_anomaly_detect` | Detect anomalies in metric streams using sliding-window z-score analysis |

### fleet_h (4 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_blackboard_query` | Query blackboard entries by namespace for fleet worker coordination |
| `ralphglasses_blackboard_put` | Write an entry to the blackboard for fleet coordination |
| `ralphglasses_a2a_offers` | List open agent-to-agent task delegation offers |
| `ralphglasses_cost_forecast` | Cost burn rate, anomaly detection, and budget exhaustion ETA |

### observability (11 tools)

| Tool | Description |
|------|-------------|
| `ralphglasses_observation_query` | Filter and paginate loop observations from .ralph/logs/loop_observations.jsonl |
| `ralphglasses_observation_summary` | Aggregate observation stats with p50/p95/p99 percentiles |
| `ralphglasses_scratchpad_read` | Read a .ralph/{name}_scratchpad.md file |
| `ralphglasses_scratchpad_append` | Append a markdown note to a scratchpad file |
| `ralphglasses_scratchpad_list` | List all scratchpad files in .ralph/ |
| `ralphglasses_scratchpad_resolve` | Mark a numbered scratchpad item as resolved |
| `ralphglasses_loop_await` | Block until a session or loop completes (replaces sleep anti-pattern) |
| `ralphglasses_loop_poll` | Non-blocking single status check for a session or loop |
| `ralphglasses_coverage_report` | Run go test -coverprofile and report per-package coverage vs threshold |
| `ralphglasses_cost_estimate` | Pre-launch cost estimate for a session or loop |
| `ralphglasses_merge_verify` | Run build+vet+test sequence to verify a merge |

## Restarting After Code Changes

The MCP server is a long-lived process compiled and run via `go run . mcp`. After making code changes to ralphglasses, you must restart the server for them to take effect:

```bash
claude mcp remove ralphglasses && claude mcp add ralphglasses -- go run . mcp
```

If using a `.mcp.json`-based setup, simply restart your MCP client (e.g., re-open Claude Code) to pick up the changes.
