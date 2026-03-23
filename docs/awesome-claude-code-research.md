# Awesome-Claude-Code Ecosystem Research

Analysis of [awesome-claude-code](https://github.com/hesreallyhim/awesome-claude-code) (181 repos, 30K+ stars) scored against ralphglasses capabilities.

## Methodology

Three-wave analysis: fetch README + metadata for all 181 repos, extract features, match against ralph capability keywords (MCP, TUI, session, provider, loop, budget, workflow, hook, team, fleet, multi-agent, orchestration, worktree, etc.), rate by stars × language × matches.

## HIGH VALUE — 23 repos

### Orchestrators & Session Management

| Repo | Lang | Stars | Why | Complexity |
|------|------|-------|-----|------------|
| `smtg-ai/claude-squad` | Go | 5K+ | Closest cousin — worktree isolation, profile system, multi-provider TUI. Same Bubbletea stack. | moderate |
| `mikeyobrien/ralph-orchestrator` | Rust | 1K+ | 7 AI backends, Hat System personas, Telegram HITL, MCP server | moderate |
| `dtormoen/tsk` | Rust | 500+ | Container-sandboxed agents, parallel worker pool, multi-agent comparison | moderate |
| `eyaltoledano/claude-task-master` | Node | 15K+ | Task dependency graphs, main/research/fallback model concept | moderate |
| `sudocode-ai/sudocode` | Node | 500+ | Git-native agent memory, topological workflow execution | moderate |
| `ryoppippi/ccusage` | Node | 300+ | Cost tracking standard — parse JSONL for token/cost data. Port into budget system | drop-in |

**Integration notes:**
- `claude-squad`: Study worktree isolation pattern for `session_launch --worktree`. Their profile system maps to our agent definitions.
- `ccusage`: JSONL parsing logic directly ports to `session/budget.go` for real Claude Code cost data.

### Skills, Workflows & Hooks

| Repo | Lang | Stars | Why | Complexity |
|------|------|-------|-----|------------|
| `aannoo/hcom` | Rust/Py | 200+ | Multi-agent comms — agents in separate terminals message each other, detect file collisions | drop-in |
| `vaporif/parry` | Rust/Py | 500+ | Prompt injection scanner (DeBERTa v3), 7-layer security, exfil/secret detection | drop-in |
| `GowayLee/cchooks` | Python | 100+ | Typed hook SDK — budget enforcement hooks, stop hooks, session lifecycle | drop-in |
| `NeoLabHQ/context-engineering-kit` | Mixed | 300+ | SDD plugin, 8 specialized agents, MAKER pattern, mentions ralph-loop | moderate |
| `automazeio/ccpm` | Mixed | 100+ | Spec-driven PM, parallel agent execution, PRD→Epic→Task→Issue pipeline | moderate |
| `avifenesh/agentsys` | Mixed | 200+ | 19 plugins, 47 agents, 39 skills, phase-gated pipelines, drift detection | moderate |
| `obra/superpowers` | Mixed | 500+ | Auto skill triggering, subagent-driven-development, mandatory TDD, worktree isolation | drop-in |
| `trailofbits/skills` | Mixed | 300+ | Security skills — `second-opinion` skill invokes other LLM CLIs. Validates multi-provider concept | drop-in |
| `EveryInc/compound-engineering-plugin` | Mixed | 100+ | Cross-platform plugin converter syncing configs between Claude/Gemini/Codex | moderate |

**Integration notes:**
- `hcom`: File collision detection directly applicable to team_delegate concurrent sessions.
- `parry`: Hook integration for pre-session prompt safety scanning.
- `cchooks`: Budget enforcement hook pattern maps to session budget.go guardrails.
- `superpowers`: Worktree isolation + mandatory TDD = blueprint for loop verification step.

### Config, UX & Ecosystem

| Repo | Lang | Stars | Why | Complexity |
|------|------|-------|-----|------------|
| `phiat/claude-esp` | Go | 100+ | Same Go/Bubbletea/Lipgloss stack. JSONL session monitoring TUI with fsnotify, tree view | moderate |
| `avifenesh/agnix` | Rust | 100+ | 342-rule config linter for CLAUDE.md/hooks/MCP. Add to repo_health scoring | drop-in |
| `carlrannaberg/claudekit` | Node | 200+ | Hook perf profiling, git checkpoints, 6-agent parallel code review | moderate |
| `hagan/claudia-statusline` | Go | 50+ | Go-native statusline with SQLite cost tracking, burn rate math | drop-in |
| `Haleclipse/CCometixLine` | Rust | 50+ | Context warning disabler for unattended operation, CC binary patching | drop-in |
| `dyoshikawa/rulesync` | Node | 100+ | Generates configs for 20+ AI tools from unified source (Claude/Gemini/Codex) | moderate |
| `Piebald-AI/tweakcc` | Node | 100+ | MCP startup optimization (~50% faster), auto-accept plan mode | drop-in |

**Integration notes:**
- `claude-esp`: Port fsnotify+JSONL monitoring patterns to TUI session output view.
- `agnix`: Run as pre-session hook or integrate into `repo_health` scoring.
- `claudia-statusline`: SQLite cost schema directly applicable to fleet_analytics.

## MEDIUM VALUE — 42 repos (selective adoption)

Notable repos worth studying:

| Repo | Why |
|------|-----|
| `claude-code-flow` | Swarm topologies (star, mesh, pipeline) — team_create orchestration patterns |
| `claude-swarm` | 12-event hook system — richer than current hooks/lifecycle |
| `cc-sessions` | DAIC pattern (Decompose-Assign-Iterate-Compose) |
| `claude-tmux` | Pane status detection via tmux — relevant for marathon.sh monitoring |
| `better-ccflare` | Multi-account API key load balancing — cost optimization |
| `basic-memory` | Knowledge graph for persistent agent memory — journal improvement |
| `ccstatusline` | Widget architecture for status display |
| `claude-code-power-pack` | Pre-built hooks and skills collection |

## LOW/NONE — 116 repos

Domain-specific (IDE plugins, single-language tools), superseded by existing functionality, or insufficient capability overlap.

---

## Repeatable Analysis

The `internal/awesome/` package and 5 MCP tools (`awesome_fetch`, `awesome_analyze`, `awesome_diff`, `awesome_report`, `awesome_sync`) allow re-running this analysis as the awesome-list updates. Results are persisted to `.ralph/awesome/` with automatic index rotation for incremental diffing.
