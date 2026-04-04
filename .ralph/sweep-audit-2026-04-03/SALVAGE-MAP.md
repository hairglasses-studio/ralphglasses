# Salvage Map — Sweep Audit Session 2026-04-03

## What happened

1. Generated an A-grade XML-structured audit prompt (380 words, 13-stage enhancer pipeline)
2. Launched 10 parallel `claude -p` Opus 4.6 sessions via tmux against top repos
3. Built 5 new MCP tools (`sweep` group) and `/audit-sweep` skill to codify the workflow
4. Extracted full detailed audit reports from session JSONL plan files
5. Discovered cost accounting issues: 3 sessions appended to pre-existing histories

## Preserved artifacts

### Audit reports (full structured findings with file paths, line numbers, fixes)
| File | Repo | Lines | Key finding |
|------|------|-------|-------------|
| `claudekit-audit.md` | claudekit | 125 | Goroutine leak on loop restart |
| `cr8-cli-audit.md` | cr8-cli | 127 | `.env` with live credentials in git |
| `crabravee-audit.md` | crabravee | 104 | Zero test coverage on 6,700 LOC |
| `dotfiles-audit.md` | dotfiles | 71 | Shader pipeline lacks error recovery |
| `hg-mcp-audit.md` | hg-mcp | 94 | Non-deferred Body.Close() calls |
| `hgmux-audit.md` | hgmux | 115 | WebSocket connection memory leak |
| `jobb-audit.md` | jobb | 122 | Goroutine leaks in timeout middleware |
| `mcpkit-audit.md` | mcpkit | 107 | Silent audit event loss |
| `mesmer-audit.md` | mesmer | 180 | OAuth token in URL query string |
| `ralphglasses-audit.md` | ralphglasses | 611 | Race on Coordinator.autoscaler |

### Other artifacts
| File | Description |
|------|-------------|
| `audit-prompt-template.txt` | The XML prompt template used (with REPO_PLACEHOLDER) |
| `session-manifest.json` | Session IDs, JSONL paths, turn counts for all sessions |
| `cost-analysis.md` | Full token-level cost breakdown per session |

### Code delivered (pushed to origin/main)
| Commit | Description |
|--------|-------------|
| `2aab2c4` | `feat: add sweep tool group for cross-repo audit sessions` (13 files, 961 insertions) |
| `763e469` | `feat: add /audit-sweep skill for cross-repo audit workflow` |

### New files created
- `internal/mcpserver/handler_sweep.go` — 5 handler functions (~600 lines)
- `internal/mcpserver/handler_sweep_test.go` — 11 tests
- `internal/mcpserver/tools_builders_sweep.go` — tool definitions
- `.claude/skills/audit-sweep.md` — invocable skill

### Session histories (full JSONL with all tool calls, reads, plans)
Preserved at their original paths — see `session-manifest.json` for exact locations.

## Crises identified

1. **Cost blowup risk**: No per-session budget caps. 3 sessions appended to existing histories.
2. **No `--no-session-persistence`**: Audit sessions mixed with pre-existing project sessions.
3. **No `--permission-mode` support**: Had to add it to LaunchOptions/buildClaudeCmd.
4. **tmux output truncation**: `--output-format text` only captures final summary, not the detailed plan.
5. **No sweep_id tracking**: Sessions couldn't be grouped until SweepID was added.
6. **Shell escaping**: First tmux launch attempt failed due to single-quote escaping in prompts.
7. **`cp`/`mv` aliased to interactive**: Required `command cp -f` to bypass.

## Lessons learned

1. Always set `--max-budget-usd` on batch sessions
2. Use `--no-session-persistence` for ephemeral audit runs
3. Reports live in plan files inside session JSONL, not in `-p` text output
4. Fresh sessions via `--session-id` prevent cost accumulation in existing sessions
5. Cache creation ($18.75/M) dominates cost, not cache reads ($1.50/M)
6. Average audit cost: ~$8/repo — acceptable for quarterly sweeps
