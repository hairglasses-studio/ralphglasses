# ralphglasses

**Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.**

ralphglasses is a k9s-style terminal UI built with Go and [Charmbracelet](https://charm.sh/) that manages multi-session, multi-provider LLM loops from any terminal. It supports **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI** as session providers, with Codex now serving as the default command-and-control runtime.

---

## Features

- **Multi-provider fleet control** — Launch and manage parallel LLM sessions across Claude Code, Gemini CLI, and OpenAI Codex CLI from a single TUI.
- **126 MCP tools across 14 namespaces** — Deferred-loading tool system with core, session, loop, fleet, cycle, prompt, autonomy, and more.
- **Prompt enhancement pipeline** — 13-stage deterministic pipeline with 10-dimensional quality scoring, 11+ lint rules, and multi-provider LLM improvement with prompt caching.
- **Autonomous supervision** — Marathon supervisor with health monitoring, cycle chaining, and configurable autonomy levels.
- **R&D cycle management** — State-machine-driven research and development cycles with observation feedback loops, baselining, and synthesis.
- **Fleet orchestration** — Tiered routing, budget management, dead-letter queues, and worker analytics.
- **Process management** — OS-level process groups with SIGTERM/SIGSTOP/SIGCONT for pause, resume, and graceful shutdown.
- **Reactive updates** — fsnotify file watching with polling fallback for real-time status display.
- **Bootable thin client** — Ubuntu 24.04 + i3 configuration for a dedicated 7-monitor, dual-GPU workstation.

## Quick install

```bash
# From source
go install github.com/hairglasses-studio/ralphglasses@latest

# Or clone and build
git clone https://github.com/hairglasses-studio/ralphglasses.git
cd ralphglasses
go build ./...
```

## Quick start

```bash
# Launch the TUI, scanning a workspace for repos
ralphglasses --scan-path ~/hairglasses-studio

# Register as an MCP server for Claude Code
claude mcp add ralphglasses -- go run . mcp
```

## Documentation

| Page | Description |
|------|-------------|
| [Getting Started](getting-started.md) | Installation, prerequisites, first run, configuration |
| [Architecture](ARCHITECTURE.md) | Package layout, provider dispatch, middleware, fleet, tiered routing |
| [MCP Tools](MCP-TOOLS.md) | Full 126-tool table with descriptions and deferred loading |
| [Provider Setup](PROVIDER-SETUP.md) | Multi-provider prerequisites, env vars, orchestration pattern |
| [Codex Reference](CODEX-REFERENCE.md) | Codex-first defaults, pinned docs, Claude cache guardrails |
| [Marathon](MARATHON.md) | Marathon supervisor, RC tools, environment setup |
| [Autonomy](AUTONOMY.md) | Autonomy levels, self-improvement subsystems |
| [Self-Learning](SELF-LEARNING.md) | 9 self-learning subsystems, task discovery, cascade routing |
| [Multi-Session](MULTI-SESSION.md) | Parallel session management and coordination |
| [Loop Defaults](LOOP-DEFAULTS.md) | Default loop configuration and tuning |
| [Contributing Tools](CONTRIBUTING-TOOLS.md) | How to add new MCP tools |

## Related projects

ralphglasses is part of the **hairglasses-studio** ecosystem:

- **mcpkit** — Go MCP framework with ralph loop engine, finops, sampling, workflow, gateway
- **hg-mcp** — Go MCP server with modular tool pattern (500+ tools)
- **claudekit** — Go MCP with rdcycle perpetual loop, budget profiles
- **shielddd** — Go + pure SQLite + MCP, audit logs
- **mesmer** — Go MCP server with ralph integration
