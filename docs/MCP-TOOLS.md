# MCP Server & Tools

Ralphglasses is an installable MCP server exposing 84 tools for managing ralph loops, multi-provider LLM sessions, fleet orchestration, and self-improvement subsystems programmatically.

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

## Tools

| Tool | Description |
|------|-------------|
| `ralphglasses_fleet_status` | Fleet dashboard: repos, sessions, teams, costs, alerts |
| `ralphglasses_scan` | Scan for ralph-enabled repos |
| `ralphglasses_list` | List all repos with status summary |
| `ralphglasses_status` | Detailed status for a repo (loop, circuit breaker, progress, config) |
| `ralphglasses_start` | Start a ralph loop |
| `ralphglasses_stop` | Stop a ralph loop |
| `ralphglasses_stop_all` | Stop all managed loops |
| `ralphglasses_pause` | Pause/resume a loop |
| `ralphglasses_logs` | Get recent log lines |
| `ralphglasses_config` | Get/set .ralphrc values |
| `ralphglasses_roadmap_parse` | Parse ROADMAP.md into structured JSON |
| `ralphglasses_roadmap_analyze` | Compare roadmap vs codebase (gaps, stale, ready) |
| `ralphglasses_roadmap_research` | Search GitHub for relevant repos/tools |
| `ralphglasses_roadmap_expand` | Generate proposed roadmap expansions |
| `ralphglasses_roadmap_export` | Export tasks as rdcycle/fix_plan/progress specs |
| `ralphglasses_repo_scaffold` | Create/init ralph config files for a repo |
| `ralphglasses_repo_optimize` | Analyze and optimize ralph config files |
| `ralphglasses_session_launch` | Launch a headless LLM session (`provider`: claude/gemini/codex) |
| `ralphglasses_session_list` | List sessions (filterable by `provider`, repo, status) |
| `ralphglasses_session_status` | Detailed session info (provider, output, cost, turns, model) |
| `ralphglasses_session_resume` | Resume a previous session (with `provider` param) |
| `ralphglasses_session_stop` | Stop a running session |
| `ralphglasses_session_budget` | Get/update budget for a session |
| `ralphglasses_team_create` | Create agent team with `provider` for lead session |
| `ralphglasses_team_status` | Get team status (tasks, lead session, progress) |
| `ralphglasses_team_delegate` | Add a task to an existing team |
| `ralphglasses_agent_define` | Create/update .claude/agents/*.md agent definitions |
| `ralphglasses_agent_list` | List available agent definitions for a repo |
| `ralphglasses_journal_read` | Read improvement journal entries with synthesized context |
| `ralphglasses_journal_write` | Manually write an improvement note to a repo's journal |
| `ralphglasses_journal_prune` | Compact improvement journal to prevent unbounded growth |
| `ralphglasses_event_list` | Query recent fleet events (by type, repo, time range) |
| `ralphglasses_fleet_analytics` | Cost breakdown by provider/repo/time-period |
| `ralphglasses_session_compare` | Compare two sessions (cost, turns, duration, efficiency) |
| `ralphglasses_session_output` | Get recent output from a running session |
| `ralphglasses_repo_health` | Composite health score (0-100) for a repo |
| `ralphglasses_session_retry` | Re-launch a failed session with same params |
| `ralphglasses_config_bulk` | Get/set .ralphrc values across multiple repos |
| `ralphglasses_agent_compose` | Create composite agent by layering existing agents |
| `ralphglasses_workflow_define` | Define multi-step YAML workflows |
| `ralphglasses_workflow_run` | Execute workflows with dependency ordering |
| `ralphglasses_snapshot` | Save/list fleet state snapshots |
| `ralphglasses_session_stop_all` | Stop all running LLM sessions (emergency cost cutoff) |
| `ralphglasses_awesome_fetch` | Fetch + parse awesome-list README into structured entries |
| `ralphglasses_awesome_analyze` | Deep-analyze repos: fetch READMEs, score value/complexity vs ralph capabilities |
| `ralphglasses_awesome_diff` | Compare current list against previous fetch (new/removed entries) |
| `ralphglasses_awesome_report` | Generate formatted report from analysis results (json/markdown) |
| `ralphglasses_awesome_sync` | Full pipeline: fetch → diff → analyze new → report → save |
| `ralphglasses_prompt_analyze` | Score a prompt across 10 quality dimensions with letter grades and suggestions |
| `ralphglasses_prompt_enhance` | Run the 13-stage prompt enhancement pipeline |
| `ralphglasses_prompt_lint` | Deep-lint a prompt for anti-patterns (injection risks, etc.) |
| `ralphglasses_prompt_improve` | LLM-powered prompt improvement via Claude, Gemini, or OpenAI |
| `ralphglasses_prompt_templates` | List available prompt templates with descriptions and required variables |
| `ralphglasses_prompt_template_fill` | Fill a prompt template with variable values |
| `ralphglasses_claudemd_check` | Health-check a repo's CLAUDE.md for common issues |
| `ralphglasses_prompt_classify` | Classify a prompt's task type (code, troubleshooting, analysis, etc.) |
| `ralphglasses_prompt_should_enhance` | Check whether a prompt would benefit from enhancement |
| `ralphglasses_session_tail` | Tail session output with cursor — returns only new lines since last call |
| `ralphglasses_session_diff` | Git changes made during a session's execution window |
| `ralphglasses_marathon_dashboard` | Compact marathon status: burn rate, stale sessions, team progress, alerts |
| `ralphglasses_session_errors` | Aggregated error view: parse failures, API errors, budget warnings |
| `ralphglasses_rc_status` | Compact fleet overview for mobile remote control (text output) |
| `ralphglasses_rc_send` | Send prompt to repo — auto-stops existing session, launches new |
| `ralphglasses_rc_read` | Read recent output from most active session with cursor |
| `ralphglasses_event_poll` | Cursor-based event polling for efficient mobile updates |
| `ralphglasses_rc_act` | Quick fleet actions: stop, stop_all, pause, resume, retry |
| `ralphglasses_fleet_submit` | Submit work to the distributed fleet queue for execution on any worker |
| `ralphglasses_fleet_budget` | View or set the fleet-wide budget (spent, remaining, active work) |
| `ralphglasses_fleet_workers` | List registered fleet workers with status, capacity, and spend |
| `ralphglasses_hitl_score` | Current human-in-the-loop score: manual interventions vs autonomous actions |
| `ralphglasses_hitl_history` | Recent HITL events: manual stops, auto-recoveries, config changes |
| `ralphglasses_autonomy_level` | View or set the autonomy level (0-3) |
| `ralphglasses_autonomy_decisions` | Recent autonomous decisions with rationale, inputs, and outcomes |
| `ralphglasses_autonomy_override` | Override/reverse an autonomous decision and record human intervention |
| `ralphglasses_feedback_profiles` | Per-task-type and per-provider performance data from journal analysis |
| `ralphglasses_provider_recommend` | Recommend best provider + model + budget for a task based on feedback profiles |
| `ralphglasses_tool_benchmark` | Per-tool performance benchmarks: latency percentiles, success rates |
| `ralphglasses_loop_benchmark` | P50/P95 metrics from recent loop observations for a repo |
| `ralphglasses_loop_baseline` | Generate, view, or pin loop performance baseline for a repo |
| `ralphglasses_loop_gates` | Evaluate regression gates — returns pass/warn/fail report |
