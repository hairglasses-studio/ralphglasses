# MCP Server & Tools

Ralphglasses is an installable MCP server for repo orchestration, session control, fleet runtime management, roadmap work, recovery, prompts, docs, and self-improvement workflows.

The live contract currently exposes:

- `222` total tools
- `218` grouped tools
- `4` management tools
- `30` deferred-loaded tool groups
- `3` static catalog resources
- `3` templated repo resources
- `5` prompts

Use the live catalog instead of hard-coding assumptions:

- `ralph:///catalog/server`
- `ralph:///catalog/tool-groups`
- `ralph:///catalog/workflows`
- `ralphglasses_server_health`
- `ralphglasses_tool_groups`

## Install

```bash
# Codex-first repo-local MCP/client setup is already configured via .codex/config.toml and .mcp.json.
go run . mcp --scan-path ~/hairglasses-studio
```

A `.mcp.json` and `.codex/config.toml` are included in the repo root for automatic local discovery. Other MCP clients can register `go run . mcp` directly and set `RALPHGLASSES_SCAN_PATH=~/hairglasses-studio` if they need explicit client-level configuration.

## Management Tools

| Tool | Purpose |
|------|---------|
| `ralphglasses_tool_groups` | Discover deferred-loaded tool groups and their counts |
| `ralphglasses_load_tool_group` | Load one named tool group on demand |
| `ralphglasses_skill_export` | Export the current tool contract as skill-friendly markdown or JSON |
| `ralphglasses_server_health` | Return live counts, version/build metadata, resources, prompts, and group summaries |

## Tool Groups

| Group | Count |
|-------|-------|
| `core` | 20 |
| `session` | 19 |
| `loop` | 12 |
| `prompt` | 9 |
| `fleet` | 12 |
| `repo` | 9 |
| `roadmap` | 6 |
| `team` | 6 |
| `tenant` | 5 |
| `awesome` | 5 |
| `advanced` | 5 |
| `events` | 2 |
| `feedback` | 6 |
| `eval` | 7 |
| `fleet_h` | 5 |
| `observability` | 14 |
| `rdcycle` | 16 |
| `plugin` | 4 |
| `sweep` | 8 |
| `rc` | 6 |
| `autonomy` | 8 |
| `workflow` | 3 |
| `docs` | 8 |
| `recovery` | 6 |
| `promptdj` | 6 |
| `a2a` | 4 |
| `trigger` | 2 |
| `approval` | 3 |
| `context` | 1 |
| `prefetch` | 1 |

## CLI Parity Additions

These MCP tools were added or extended to close the remaining non-interactive CLI gaps:

- `ralphglasses_doctor`
- `ralphglasses_validate`
- `ralphglasses_config_schema`
- `ralphglasses_debug_bundle`
- `ralphglasses_theme_export`
- `ralphglasses_telemetry_export`
- `ralphglasses_firstboot_profile`
- `ralphglasses_budget_status`
- `ralphglasses_fleet_runtime`
- `ralphglasses_marathon`
- `ralphglasses_repo_surface_audit`
- `ralphglasses_worktree_list`
- `ralphglasses_session_budget` with `action=get|set|reset_spend`
- `ralphglasses_loop_gates` with `baseline_path`
- `ralphglasses_repo_scaffold` with `minimal`
- `ralphglasses_worktree_cleanup` with `dry_run`

See [CLI-PARITY.md](CLI-PARITY.md) for the full command mapping, including skill-backed and command-only exceptions.

## Resources And Prompts

Discovery resources:

- `ralph:///catalog/server`
- `ralph:///catalog/tool-groups`
- `ralph:///catalog/workflows`

Repo-scoped templates:

- `ralph:///{repo}/status`
- `ralph:///{repo}/progress`
- `ralph:///{repo}/logs`

Prompt surfaces:

- `self-improvement-planner`
- `code-review`
- `test-generation`
- `bootstrap-firstboot`
- `provider-parity-audit`

## Restarting After Code Changes

The MCP server is a long-lived process compiled and run via `go run . mcp`. After making code changes to ralphglasses, restart the server or reconnect your MCP client so it re-reads the updated contract.
