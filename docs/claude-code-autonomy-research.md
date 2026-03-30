# Claude Code Autonomy Research for Marathon Operations

**Date:** 2026-03-29
**Purpose:** Comprehensive reference for Claude Code features that enable autonomous marathon R&D execution in ralphglasses.

---

## 1. `/loop` — Session-Scoped Recurring Scheduling

**Syntax:**
```bash
/loop 5m <prompt>           # explicit interval
/loop <prompt> every 10m    # trailing "every" clause
/loop <prompt>              # defaults to 10m
```

**Interval suffixes:** `s` (seconds, rounded to nearest minute), `m` (minutes), `h` (hours), `d` (days)

**Underlying tools:** `CronCreate` (cron expression + prompt + recurring bool), `CronList`, `CronDelete` (8-char IDs)

**Constraints:**
- Session-scoped — dies when the Claude Code session exits
- Max 50 tasks per session
- 3-day auto-expiry for recurring tasks (fires one final time, then self-deletes)
- No catch-up: if Claude is busy during fire time, fires once when idle
- Up to 10% jitter on recurring, up to 90s early for one-shots
- All times in local timezone

**Marathon integration:**
- Currently used to poll marathon loop status every 5 minutes
- Limited by session lifetime — not suitable for durable multi-hour marathons without a persistent session
- Best for: in-session monitoring, status polling, health checks

---

## 2. `/batch` — Parallel Agent Decomposition

**Syntax:**
```bash
/batch "your instruction describing the operation"
```

**How it works:**
1. Orchestrator enters plan mode, researches files/patterns/call sites
2. Decomposes work into 5-30 units based on codebase + change complexity
3. Spawns one background agent per unit in isolated git worktrees
4. Each agent gets independent branch; failure in one doesn't affect others
5. Output as separate PRs with deterministic merging

**Key constraints:**
- Requires git worktree support
- Works best with specific scope ("only in `auth/` module") not vague ("fix everything")
- Each agent context is fresh — no inter-agent communication mid-execution

**Marathon integration:**
- Natural replacement for our manual parallel worktree agent pattern (5 agents in WS1-5)
- Sprint-level parallelism: 5 ROADMAP items → 5 batch units
- Integrates with existing `internal/session/worktree.go` cleanup

---

## 3. `/schedule` & Cloud/Desktop Scheduled Tasks

Three scheduling tiers with different persistence and capabilities:

| Feature | Cloud Scheduled Tasks | Desktop Scheduled Tasks | `/loop` |
|---------|----------------------|------------------------|---------|
| Runs on | Anthropic cloud | Your machine | Your machine |
| Requires computer on | No | Yes | Yes |
| Requires open session | No | No | Yes |
| Persistent across restarts | Yes | Yes | No |
| Local file access | Limited (fresh clone) | Full | Full |
| MCP servers | Connectors per-task | Full config | Inherited |
| Min interval | 1 hour | 1 minute | 1 minute |

**Cloud tasks (via `/schedule`):**
- Run even when your computer is off — true durable autonomy
- Inherits MCP servers from your web account
- Create from web UI, CLI, or settings.json
- Management: `/schedule list`, `/schedule update`, `/schedule run`

**Desktop tasks:**
- Configure in Claude Desktop app UI
- Direct local file access, tools, permissions
- Survives machine restarts

**Cron expressions (standard 5-field):**
```
*/5 * * * *     → every 5 minutes
0 9 * * *       → daily at 9am local
0 9 * * 1-5     → weekdays at 9am
0 */6 * * *     → every 6 hours
```

**Marathon integration:**
- Cloud scheduled tasks could replace `marathon.sh` entirely
- Schedule daily R&D marathon: `/schedule "0 2 * * *"` (runs at 2am)
- No need for tmux sessions or process supervision
- Trade-off: cloud tasks can't access local MCP servers directly

---

## 4. Hooks — Pre/Post Tool Automation

**Handler types:**
- `command` — shell script via `exec.CommandContext`
- `http` — POST to URL with JSON body
- `prompt` — send to Claude for LLM-powered evaluation
- `agent` — spawn subagent for complex verification

**Hook events:**
- `PreToolUse` / `PostToolUse` — before/after tool execution (can block)
- `PermissionRequest` — when permission dialog appears (can deny)
- `Stop` — can block stop to force continuation
- `SessionStart` — setup on session creation
- `FileChanged` — react to file modifications

**Exit codes:**
- `0` = success (parse JSON for decisions)
- `2` = blocking error (execution stops, stderr fed to Claude)
- Other codes = non-blocking error

**Configuration hierarchy (highest priority first):**
1. `~/.claude/settings.json` — user-wide
2. `.claude/settings.json` — project (committed to git)
3. `.claude/settings.local.json` — project (local only)

**Example: auto-vet after file changes:**
```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [{"type": "command", "command": "go vet ./..."}]
      }
    ]
  }
}
```

**Marathon integration:**
- `PostToolUse` on `Write`/`Edit` → auto-run `go vet` after every change
- `PreToolUse` on `Bash` → enforce safety rules (no `rm -rf`, no force push)
- `Stop` hook → force continuation when context compaction triggers
- Already partially implemented in `internal/hooks/hooks.go` for internal event bus

---

## 5. Headless Mode — `claude -p`

**Key flags:**
```bash
claude -p "prompt"                    # non-interactive mode
claude --bare -p "prompt"             # skip hooks, CLAUDE.md, MCP discovery
claude -p "prompt" --output-format json   # structured output with metadata
claude -p "prompt" --output-format stream-json  # NDJSON for real-time
claude -p "prompt" --allowedTools "Bash,Read,Edit"  # auto-approve tools
claude -p "prompt" --json-schema schema.json   # structured output validation
claude -p "prompt" --append-system-prompt "extra instructions"
```

**`--bare` mode:**
- Skips: hooks, plugins, MCP servers, CLAUDE.md, auto memory, keychain reads
- Must pass all context via flags
- Reproducible across machines

**Piping:**
```bash
git diff main --name-only | claude -p "Review for security issues"
tail -200 app.log | claude -p "Slack me if you see anomalies"
```

**Exit codes:** 0 = success, 1 = execution error, 2 = API/permission error (retryable)

**Marathon integration:**
- Core primitive for headless marathon sessions
- `claude --bare -p "Execute sprint 5" --allowedTools "Bash,Read,Edit,Grep"` is the building block
- Combined with `--resume` for multi-sprint chains

---

## 6. `--continue` and `--resume`

**Session continuation:**
```bash
claude --continue -p "What was the last issue?"     # most recent session
claude --resume <session-id> -p "Continue that work" # specific session
claude --resume                                       # interactive picker
```

**State preserved:** full message history, tool call results, compaction state, checkpoint data

**Session ID capture:**
```bash
session_id=$(claude -p "Start review" --output-format json | jq -r '.session_id')
claude -p "Focus on database queries" --resume "$session_id"
```

**Marathon integration:**
- Enables multi-invocation marathon chains: Sprint 1 → capture ID → Sprint 2 resumes with full context
- Already partially implemented via `--resume` flag on our `marathon` command
- `supervisor_state.json` stores session lineage for cross-invocation continuity

---

## 7. Agent SDK (Python & TypeScript)

| Language | Package | Status |
|----------|---------|--------|
| Python | `anthropic` (built-in) | Official, bundled CLI |
| TypeScript | `@anthropic-ai/claude-agent-sdk` | Official, npm |
| Go | Via MCP + API | No first-class SDK |

**Capabilities:**
- Full agent loop control (iterate, inspect tool calls, custom logic)
- Structured output validation (JSON Schema enforcement)
- Custom tools via in-process MCP servers
- Permission callbacks for tool approvals
- Model selection per agent

**Repos:**
- [anthropics/claude-agent-sdk-typescript](https://github.com/anthropics/claude-agent-sdk-typescript)
- [anthropics/claude-agent-sdk-python](https://github.com/anthropics/claude-agent-sdk-python)
- [anthropics/claude-agent-sdk-demos](https://github.com/anthropics/claude-agent-sdk-demos)

**Marathon integration:**
- Our Go approach via MCP is equally valid — no need to add Python/TS dependency
- Could use Python SDK for external orchestration layer if needed
- Agent SDK patterns inform our `internal/session/runner.go` design

---

## 8. Subagents & Agent Definitions

**Definition format** (`.claude/agents/<name>.md`):
```yaml
---
name: sprint-executor
description: Executes sprint items from ROADMAP autonomously
tools: Read, Write, Edit, Bash, Grep, Glob
model: sonnet
permissionMode: dontAsk
maxTurns: 50
memory: project
isolation: worktree
effort: high
---

You are a sprint executor. Read ROADMAP.md, implement unchecked items...
```

**Frontmatter fields:**
- `name` — unique ID (lowercase + hyphens)
- `description` — when Claude should delegate to this agent
- `tools` / `disallowedTools` — tool allowlist/denylist
- `model` — `sonnet`, `opus`, `haiku`, or full model ID
- `permissionMode` — `default`, `acceptEdits`, `dontAsk`, `bypassPermissions`, `plan`
- `maxTurns` — max agentic iterations
- `memory` — `user`, `project`, or `local` (persistent learning)
- `isolation` — `worktree` (isolated git branch per agent)
- `hooks` — lifecycle hooks (PreToolUse, PostToolUse, SubagentStart, SubagentStop)
- `mcpServers` — MCP servers scoped to this agent
- `background` — run concurrently vs blocking
- `effort` — `low`, `medium`, `high`, `max`

**Invocation:**
```bash
claude "Use the sprint-executor to handle ROADMAP items"
@sprint-executor implement the next 5 items
claude --agent sprint-executor
```

**Constraints:**
- Subagents cannot spawn other subagents (1-level deep)
- For parallel: use `/batch` or launch multiple agents from main conversation
- Each agent gets independent context window

**Existing agents** (`.claude/agents/`):
- `fleet-optimizer.md`, `cost-auditor.md`, `quality-checker.md`
- `migration-evaluator.md`, `stale-loop-cleaner.md`, `remediation-squad.md`
- `cost-optimizer.md`, `audit-squad.md`

**Marathon integration:**
- Create `sprint-executor.md` with `isolation: worktree` and `permissionMode: dontAsk`
- Create `marathon-monitor.md` with `model: haiku` for cheap status polling
- Agent persistent memory enables cross-session learning

---

## 9. Permission Modes & Auto Mode

**Five modes:**

| Mode | Behavior | Use Case |
|------|----------|----------|
| Normal | Prompts on dangerous operations | Dev with guardrails |
| Auto-Accept Edit | Auto-approves edits, prompts on other tools | Safe file editing |
| Plan Mode | Read-only, no modifications | Analysis & research |
| Don't Ask | Pre-approved tools only via `allowedTools` | Selective autonomy |
| Bypass (`--dangerously-skip-permissions`) | No prompts, all tools | High-trust isolated envs |

**Auto Mode (March 2026 research preview):**
- Model-based classifier decides approval per tool call
- Safe actions auto-approved, risky actions blocked
- Blocks dangerous allow rules on activation
- Middle ground between manual and bypass

**Marathon integration:**
- Auto Mode is ideal for supervised marathon runs
- Safer than `--dangerously-skip-permissions`
- Less friction than manual approval
- Map to autonomy levels: L0→Plan, L1→Normal+hooks, L2→Auto, L3→Bypass (isolated only)

---

## 10. Context Window Management

**Auto-compaction:**
- Triggers at ~95% capacity (or 25% remaining)
- Summarizes conversation, preloads summary as new context
- No user intervention needed

**Manual compaction:**
```bash
/compact                        # general compaction
/compact focus on the API changes  # focused compaction
/clear                          # complete wipe
```

**Compact Instructions** (add to CLAUDE.md):
```markdown
## Compact Instructions
When compacting, preserve: current sprint number, ROADMAP progress, active cycle state,
budget spent, and any unresolved findings. Discard: individual file diffs, tool output,
intermediate search results.
```

**Marathon integration:**
- Critical for multi-hour marathons that exceed context window
- Our supervisor should trigger `/compact` between sprints
- `LoopProfile.CompactionEnabled` already exists but needs Compact Instructions in CLAUDE.md
- `CompactionThreshold` configures when compaction activates

---

## 11. Cost Control

**Built-in tools:**
- `/cost` — token usage (API users)
- `/stats` — usage patterns (Pro/Max subscribers)
- Token counting API — free, rate-limited pre-check before sending

**Cost optimization strategies:**
- **Prompt caching**: 80-90% savings on stable preambles (all 3 providers)
  - Claude: `cache_control` breakpoints
  - Gemini: `cachedContents` API with TTL
  - OpenAI: automatic prefix caching
- **Batch API**: 50% discount for non-interactive workloads (combinable with caching)
- **Model routing**: Haiku for exploration ($0.80/1M), Sonnet for coding ($3/15M), Opus for planning ($15/75M)
- **`--effort` flag**: low = cheaper model, max = full reasoning

**Cost benchmarks:**
- Average: $6/developer/day, <$12/day for 90% of users
- Marathon 1h with Sonnet: ~$5-15 depending on task density
- Marathon 1h with Opus planner + Sonnet worker: ~$15-75

**Marathon integration:**
- `BudgetEnvelope` in `supervisor_budget.go` tracks real-time spend
- Token counting API integration for accurate pre-cycle forecasting
- Batch API for non-interactive marathon workloads (additional 50% savings)
- Prompt caching for stable CLAUDE.md/system prompts across iterations

---

## 12. Self-Improvement Patterns

**Implemented in ralphglasses (validated by research):**

| Pattern | File | Description |
|---------|------|-------------|
| Reflexion | `reflexion.go` | Verbal self-critique stored in episodic buffer; inject corrections on retry |
| Episodic Memory | `episodic.go` | Persistent log of successful trajectories; retrieval via Jaccard/embedding similarity |
| Curriculum Learning | `curriculum.go` | Order tasks by difficulty; 5 scoring signals (type, success rate, complexity, episodic, keywords) |
| Cascade Routing | `cascade_routing.go` | Try cheap provider first, escalate if needed; 4-tier stack |
| Bandit Selection | `internal/bandit/` | UCB1 multi-armed bandit for dynamic provider selection after ≥10 samples |

**Research-validated enhancements (from academic papers):**
- **LATS** (ICML 2024): Parallel candidate approaches for high-value tasks, prune by value estimate
- **DSPy/OPRO**: Optimizer loop for prompt improvement: generate N candidates, score, keep best
- **FrugalGPT/RouteLLM**: Learned classifier for task→provider routing (2-4x cost reduction)
- **CrewAI memory taxonomy**: Upgrade flat KV to structured memory (short-term, long-term, entity, contextual)

---

## Architecture: How It All Fits Together

```
┌─────────────────────────────────────────────────────────┐
│                    Entry Points                          │
│  /schedule (cloud)  │  marathon.sh  │  claude -p        │
│  (durable, hourly)  │  (local, 12h) │  (headless)       │
└─────────┬───────────┴───────┬───────┴────────┬──────────┘
          │                   │                │
          ▼                   ▼                ▼
┌─────────────────────────────────────────────────────────┐
│              Session Layer (--resume)                     │
│  Session ID tracking  │  Context compaction  │  Hooks    │
└─────────────────────────┬───────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│              Supervisor (60s tick)                        │
│  Health Monitor → Decision Gate → Cycle Chainer          │
│  Budget Envelope │ Stall Detection │ Sprint Planner      │
└─────────────────────────┬───────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│              Execution Layer                              │
│  /batch (parallel)  │  Subagents  │  Worktree isolation  │
│  Auto Mode perms    │  Model routing │  Cascade router   │
└─────────────────────────┬───────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────┐
│              Learning Layer                               │
│  Reflexion  │  Episodic Memory  │  Curriculum  │  Bandit │
│  Feedback Profiles │ Cost Predictor │ Auto-Optimize      │
└─────────────────────────────────────────────────────────┘
```

---

## Sources

### Official Claude Code Documentation
- [Run prompts on a schedule](https://code.claude.com/docs/en/scheduled-tasks)
- [Hooks reference](https://code.claude.com/docs/en/hooks)
- [Run Claude Code programmatically](https://code.claude.com/docs/en/headless)
- [Common workflows (resume/continue)](https://code.claude.com/docs/en/common-workflows)
- [Create custom subagents](https://code.claude.com/docs/en/sub-agents)
- [Configure permissions](https://code.claude.com/docs/en/permissions)
- [Manage costs effectively](https://code.claude.com/docs/en/costs)
- [MCP integration](https://code.claude.com/docs/en/mcp)

### Anthropic API Documentation
- [Agent SDK overview](https://platform.claude.com/docs/en/agent-sdk/overview)
- [Prompt caching](https://platform.claude.com/docs/en/build-with-claude/prompt-caching)
- [Batch processing](https://platform.claude.com/docs/en/build-with-claude/batch-processing)
- [Extended thinking](https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking)
- [Token counting](https://platform.claude.com/docs/en/build-with-claude/token-counting)

### Agent SDK Repositories
- [claude-agent-sdk-typescript](https://github.com/anthropics/claude-agent-sdk-typescript)
- [claude-agent-sdk-python](https://github.com/anthropics/claude-agent-sdk-python)
- [claude-agent-sdk-demos](https://github.com/anthropics/claude-agent-sdk-demos)

### Related ralphglasses Documentation
- [docs/MARATHON.md](MARATHON.md) — Marathon supervisor, RC tools, environment setup
- [docs/AUTONOMY.md](AUTONOMY.md) — Autonomy levels, self-improvement subsystems
- [docs/ARCHITECTURE.md](ARCHITECTURE.md) — Package layout, provider dispatch, middleware
