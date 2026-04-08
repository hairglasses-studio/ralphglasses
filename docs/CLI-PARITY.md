# CLI Parity

This matrix is the source of truth for CLI parity work. The goal is:

- Non-interactive, automatable workflows are MCP-native.
- Interactive terminal workflows are skill-backed.
- Pure transport or shell ergonomics remain command-only by design.

## Current Status

Full parity is in place for the meaningful non-interactive CLI surfaces. The remaining CLI-only surfaces are either interactive terminal affordances or transport helpers that do not represent distinct business capabilities.

Current coverage snapshot from the live parity matrix:

- `24` tracked CLI surfaces
- `21` bespoke-covered surfaces (`87.5%` of all tracked surfaces)
- `100%` coverage of business surfaces after excluding the `3` command-only-by-design transport/shell affordances

## Usage Telemetry

The live `ralph:///catalog/cli-parity` resource now includes a rolling usage snapshot sourced from `<scan-path>/.ralph/tool_benchmarks.jsonl`.

The broader discovery front doors now have their own companion resource:

- `ralph:///catalog/discovery-adoption` for rolling resource, prompt, and focused-skill front-door adoption telemetry
- `ralph:///catalog/adoption-priorities` for ranked next-work candidates derived from inactive CLI parity and discovery surfaces

- Default window: last `30` days
- Source of truth: MCP tool-call recorder output, not hand-maintained counters
- Purpose: measure which parity-covered CLI surfaces are actually being exercised through bespoke MCP flows

Current observability caveat:

- Pure skill-backed terminal surfaces like the root TUI and `tmux` flows are intentionally excluded from the usage denominator because the benchmark file only records MCP tool invocations.
- Shared MCP tools can light up multiple CLI surfaces. For example, `ralphglasses_repo_scaffold` contributes usage to both `ralphglasses init` and `ralphglasses config init`.

## Command Matrix

| CLI Surface | Status | MCP Tool / Skill | Notes |
|-------------|--------|------------------|-------|
| `ralphglasses` root TUI | Skill-backed | `ralphglasses-operator` | Interactive terminal UI; not a stable MCP business primitive |
| `ralphglasses mcp` | Command-only by design | N/A | Transport entrypoint for stdio MCP serving |
| `ralphglasses mcp-call` | Command-only by design | N/A | Local debugging and direct invocation helper |
| `ralphglasses completion` | Command-only by design | N/A | Shell completion generation is transport/shell-specific |
| `ralphglasses tmux list/attach/detach` | Skill-backed | `ralphglasses-operator` | Terminal multiplexing remains interactive/operator-focused |
| `ralphglasses firstboot` | Hybrid parity | `ralphglasses_firstboot_profile`, `ralphglasses-operator` | Profile/config flows are MCP-native; wizard remains skill-backed |
| `ralphglasses doctor` | MCP-native | `ralphglasses_doctor` | Workspace and provider readiness checks |
| `ralphglasses validate` | MCP-native | `ralphglasses_validate` | `.ralphrc` validation across scan path or selected repos |
| `ralphglasses debug-bundle` | MCP-native | `ralphglasses_debug_bundle` | View or save deterministic debug bundles |
| `ralphglasses theme-export` | MCP-native | `ralphglasses_theme_export` | Export snippets for downstream tools |
| `ralphglasses telemetry export` | MCP-native | `ralphglasses_telemetry_export` | JSON/CSV export with filters |
| `ralphglasses config list-keys` | MCP-native | `ralphglasses_config_schema` | Structured schema, defaults, and constraints |
| `ralphglasses config init` | MCP-native | `ralphglasses_repo_scaffold` | Alias behavior covered through scaffold flows |
| `ralphglasses init` | MCP-native | `ralphglasses_repo_scaffold` | Supports full scaffold and `minimal` mode |
| `ralphglasses worktree list` | MCP-native | `ralphglasses_worktree_list` | Dirty/stale filtering parity |
| `ralphglasses worktree create` | MCP-native | `ralphglasses_worktree_create` | Existing parity retained |
| `ralphglasses worktree clean` | MCP-native | `ralphglasses_worktree_cleanup` | Now supports `dry_run` parity |
| `ralphglasses gate-check` | MCP-native | `ralphglasses_loop_gates` | Supports explicit `baseline_path` override |
| `ralphglasses budget status` | MCP-native | `ralphglasses_budget_status` | Aggregate session budget view |
| `ralphglasses budget set/reset` | MCP-native | `ralphglasses_session_budget` | `action=set|get|reset_spend` parity |
| `ralphglasses session list/status/stop` | MCP-native | Existing session tools | Existing parity retained |
| `ralphglasses tenant *` | MCP-native | Existing tenant tools | Existing parity retained |
| `ralphglasses serve` | MCP-native | `ralphglasses_fleet_runtime` | Coordinator/worker runtime lifecycle and discovery |
| `ralphglasses marathon` | MCP-native | `ralphglasses_marathon` | Start, resume, status, and stop |

## Existing Tool Extensions

These existing tools were extended rather than duplicated:

- `ralphglasses_repo_scaffold`: added `minimal`
- `ralphglasses_session_budget`: added `action=get|set|reset_spend`
- `ralphglasses_loop_gates`: added `baseline_path`
- `ralphglasses_worktree_cleanup`: added `dry_run`
- `ralphglasses_server_health`: now reports version, commit, build date, and group summary

## Skills

- `ralphglasses-discovery`: catalog-first discovery, deferred group loading, and contract export
- `ralphglasses-session-ops`: launch, resume, budget, output, compare, export, and handoff flows
- `ralphglasses-repo-admin`: doctor, validate, scaffold, worktree, config, telemetry, and debug-bundle flows
- `ralphglasses-bootstrap`: first-time setup, profile application, runtime control, and bootstrap readiness checks
- `ralphglasses-recovery-observability`: logs, runtime health, recovery planning, session salvage, and marathon diagnosis
- `ralphglasses-operator`: interactive TUI, tmux, firstboot wizard, and operator troubleshooting
- `ralphglasses-self-dev`: repo surface audit, roadmap analysis, loop/marathon execution, merge verification, and docs writeback

## Live Contract

Use the server’s discovery surfaces instead of hard-coding counts:

- `ralph:///catalog/server`
- `ralph:///catalog/tool-groups`
- `ralph:///catalog/workflows`
- `ralph:///catalog/skills`
- `ralph:///catalog/cli-parity`
- `ralph:///catalog/discovery-adoption`
- `ralph:///catalog/adoption-priorities`
- `ralph:///bootstrap/checklist`
- `ralph:///runtime/health`
- `ralphglasses_server_health`
- `ralphglasses_tool_groups`
- `ralphglasses_skill_export`
