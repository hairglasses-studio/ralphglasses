# Getting Started with Ralphglasses

Ralphglasses is a command-and-control TUI for managing parallel LLM agent loops across multiple repositories. It supports Claude Code, Gemini CLI, and OpenAI Codex CLI as session providers.

## Prerequisites

- **Go 1.21+** (Go 1.26 recommended — `go version`)
- **git** (`git --version`)
- **At least one provider CLI** (see below)
- **direnv** (optional but recommended for API key management)

### Provider CLIs

```bash
# Claude Code (primary — install via Anthropic)
# https://docs.anthropic.com/en/docs/claude-code/overview

# Gemini CLI (optional)
npm install -g @google/gemini-cli

# OpenAI Codex CLI (optional)
npm install -g @openai/codex-cli
```

### API Keys

Create a `.env` file in the project root:

```bash
ANTHROPIC_API_KEY=sk-ant-...    # Claude (skip if using OAuth)
GOOGLE_API_KEY=AIza...          # Gemini
OPENAI_API_KEY=sk-...           # Codex
```

> **Note for Claude Code OAuth users:** Do NOT set `ANTHROPIC_API_KEY` if you authenticated via `claude login`. Setting it causes conflicts.

If you use direnv, add a `.envrc` that loads `.env`:

```bash
# .envrc
dotenv
```

Then run `direnv allow`.

---

## Installation

### Build from source

```bash
git clone https://github.com/hairglasses-studio/ralphglasses
cd ralphglasses
go build -o ralphglasses .
# Or install to GOBIN:
go install .
```

### Bootstrap toolchain (recommended)

The bootstrap script checks prerequisites and installs optional dev tools:

```bash
./scripts/bootstrap-toolchain.sh
```

To verify your environment:

```bash
./scripts/dev/doctor.sh
```

---

## First Run

```bash
ralphglasses --scan-path ~/projects
```

Ralphglasses scans the given directory for repos that have `.ralph/` or `.ralphrc` files, then opens the TUI.

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--scan-path` | `~/hairglasses-studio` | Root directory to scan |
| `--theme` | `k9s` | Color theme (`k9s`, `dracula`, `gruvbox`, `nord`, or path to YAML) |
| `--notify` | false | Enable desktop notifications for critical alerts |

---

## Creating Your First Ralph-Enabled Repo

A ralph-enabled repo needs a `.ralphrc` file and a `.ralph/` directory. The fastest way to create both:

### Via MCP tool

```bash
# From Claude Code with the MCP server running:
ralphglasses_repo_scaffold {"repo_path": "/path/to/your/repo"}
```

### Manually

```bash
cd /path/to/your/repo
mkdir -p .ralph/logs

cat > .ralphrc << 'EOF'
PROJECT_NAME="my-project"
MAX_CALLS_PER_HOUR=80
CLAUDE_TIMEOUT_MINUTES=20
EOF
```

The scaffold creates four files:
- `.ralphrc` — project configuration (see `docs/examples/ralphrc-full` for all options)
- `.ralph/PROMPT.md` — system prompt for the agent
- `.ralph/AGENT.md` — build/test/run instructions
- `.ralph/fix_plan.md` — task list the agent works through

---

## Understanding the TUI

The TUI has four tabs, navigated with number keys `1`–`4`.

### Tab 1: Repos

A sortable table of all discovered ralph-enabled repos. Columns:

| Column | Description |
|--------|-------------|
| Name | Repository directory name |
| Status | Loop status from `.ralph/status.json` |
| Circuit | Circuit breaker state (`CLOSED`/`HALF_OPEN`/`OPEN`) |
| Calls | Calls made this hour vs max |
| Updated | Time since last status update |

### Tab 2: Sessions

Active and recent LLM sessions launched via ralphglasses. Columns include provider, model, cost, duration, and status.

### Tab 3: Teams

Multi-provider agent teams created via `ralphglasses_team_create`. Shows lead/worker sessions and task progress.

### Tab 4: Fleet Dashboard

Aggregate view: total cost by provider, active loops, circuit breaker summary, alerts.

### Keybindings

#### Global

| Key | Action |
|-----|--------|
| `q` / `Ctrl+C` | Quit |
| `?` | Toggle help |
| `Esc` | Back / cancel |
| `r` | Refresh |
| `:` | Command mode |
| `/` | Filter mode |
| `1` `2` `3` `4` | Switch tab |

#### Table Navigation

| Key | Action |
|-----|--------|
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `Enter` | Drill into item |
| `s` | Cycle sort column |
| `Space` | Toggle selection |

#### Repo Actions

| Key | Action |
|-----|--------|
| `S` | Start loop |
| `X` | Stop loop / session |
| `P` | Pause / resume loop |
| `a` | Actions menu |
| `L` | Launch session |

#### Repo Detail

| Key | Action |
|-----|--------|
| `e` | Edit config |
| `w` | Save config |
| `d` | View git diff |

#### Log Viewer

| Key | Action |
|-----|--------|
| `f` | Toggle follow mode |
| `G` | Jump to end |
| `g` | Jump to start |
| `Ctrl+U` | Page up |
| `Ctrl+D` | Page down |

#### Session Detail

| Key | Action |
|-----|--------|
| `o` | Session output |
| `t` | Session timeline |
| `X` | Stop session |

---

## Running Your First Loop

1. Press `1` to open the Repos tab.
2. Select a ralph-enabled repo with `j`/`k`.
3. Press `S` to start the loop.

Ralphglasses launches the loop as a background process. If a `ralph_loop.sh` exists in the repo directory, it runs that; otherwise, it uses the native Go loop. The PID is written to `.ralph/ralphglasses.pid`.

To watch live logs: press `Enter` on the repo to open repo detail, then `Enter` again on the log entry to open the log viewer. Press `f` to follow new output.

To stop: press `X` in any repo row or in repo detail.

---

## Monitoring with the Fleet Dashboard

Press `4` to open the Fleet dashboard. It shows:

- Active repos and their loop states
- Per-provider session costs
- Circuit breaker summary
- Recent alerts from the event bus

The fleet data refreshes via fsnotify (file system events from `.ralph/` directories), with a 2-second polling fallback when fsnotify is unavailable.

---

## Using the MCP Server

Ralphglasses exposes all its functionality as an MCP server, letting Claude (or other LLM clients) orchestrate it programmatically.

### Install for Claude Code

```bash
# Recommended — use the wrapper script
claude mcp add ralphglasses -- ./scripts/dev/run-mcp.sh --scan-path ~/hairglasses-studio

# Or directly
claude mcp add ralphglasses -- go run . mcp --scan-path ~/hairglasses-studio
```

A `.mcp.json` is included in the repo root for automatic local discovery when you open Claude Code from the ralphglasses directory.

### Start the MCP server manually

```bash
go run . mcp --scan-path ~/projects
```

The server uses stdio transport (JSON-RPC over stdin/stdout). It exposes 110 tools — see [CLAUDE.md](../CLAUDE.md) for the full tool list.

### Example: launch a session via MCP

```
ralphglasses_session_launch {
  "repo_path": "/path/to/repo",
  "provider": "claude",
  "prompt": "Fix the failing tests in internal/session/"
}
```

### Example: check fleet status

```
ralphglasses_fleet_status {}
```

---

## Troubleshooting

### "no ralph-enabled repos found"

- Ensure your repos have `.ralphrc` or `.ralph/` directories.
- Check that `--scan-path` points to the right directory.
- Use `ralphglasses_scan` (MCP) or run `ralphglasses_repo_scaffold` to initialize a repo.

### Loop won't start

- Ensure the repo has a `.ralph/` directory and `.ralphrc` config.
- If using a custom `ralph_loop.sh`, verify it exists in the repo root.
- For the native Go loop, no external binary is needed.

### Circuit breaker is OPEN

The circuit breaker opens when the agent repeatedly fails without progress. Check the reason in the repo detail view:
- **consecutive_no_progress** — agent is looping without completing tasks
- **consecutive_same_error** — same error keeps occurring; fix the root cause
- **consecutive_permission_denials** — agent lacks permissions; check `ALLOWED_TOOLS` in `.ralphrc`

The breaker auto-resets after `CB_COOLDOWN_MINUTES` (default: 15) if `CB_AUTO_RESET=true`.

### MCP server won't connect

```bash
# Run directly to see startup errors
go run . mcp --scan-path ~/projects
```

Check that `go run .` works without errors, and that the `--scan-path` directory exists.

### direnv not loading

```bash
direnv allow    # Run from the repo root
```

### Claude Code: "nested session detected"

Unset `CLAUDECODE` before launching headless sessions:

```bash
unset CLAUDECODE
```

This variable is set by Claude Code to prevent nested invocations. Ralph handles this automatically in marathon mode.

### Budget / cost tracking shows $0.00

The `session_spend_usd` field in `.ralph/status.json` is populated by the loop script. If it shows zero, the loop script may not be reporting costs. Check `.ralph/logs/` for the active session log.
