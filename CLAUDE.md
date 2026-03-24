# Ralphglasses

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

Supports **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI** as session providers. Claude serves as the primary orchestrator; Gemini and Codex are available as worker providers for cost optimization and task specialization.

## Two Deliverables

1. **`ralphglasses` Go binary** — Cross-platform Unix TUI (k9s-style, Charmbracelet). Manages multi-session, multi-provider LLM loops from any terminal.
2. **Bootable Linux thin client** — Ubuntu 24.04 + i3, boots into ralphglasses TUI. 7-monitor, dual-NVIDIA-GPU.

See ROADMAP.md for full plan. See docs/ for research.

## Build & Run

```bash
go build ./...
go run . --scan-path ~/hairglasses-studio
```

## MCP Server (84 tools)

```bash
claude mcp add ralphglasses -- go run . mcp
```

A `.mcp.json` is also included in the repo root for automatic local discovery. See [docs/MCP-TOOLS.md](docs/MCP-TOOLS.md) for the full tool table.

## Prompt Enhancement

The `internal/enhancer/` package provides multi-provider prompt improvement and scoring:

- **13-stage deterministic pipeline**: specificity, positive reframing, tone downgrade (Claude-only), XML/markdown structure (provider-aware), context reorder, format enforcement, self-check injection
- **10-dimensional quality scoring**: clarity, specificity, structure, examples, tone, etc. with letter grades (A-F) and provider-specific suggestions
- **11+ lint rules**: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering
- **Multi-provider LLM improvement**: Claude, Gemini, and OpenAI API clients with provider-specific meta-prompts, circuit breaker, and caching

### Provider-Aware Behavior

| Concept | `LLM.Provider` / `provider` | `TargetProvider` / `target_provider` |
|---------|----------------------------|--------------------------------------|
| Controls | Which API to call for improvement | Which model family the enhanced prompt targets |
| Example | Use Claude API to improve a prompt | That will be sent to Gemini |

Pipeline stages that adapt per target: `tone_downgrade` and `overtrigger_rewrite` (Claude-only), `structure` (XML for Claude, markdown headers for Gemini/OpenAI).

The `Manager.Enhancer` field enables automatic prompt enhancement in `StepLoop`. The `ralphglasses_session_launch` tool supports `enhance_prompt` parameter. The TUI launcher shows a real-time prompt quality score.

## Key Patterns

- **Styles are in their own package** (`internal/tui/styles/`) to avoid import cycles. Components and views import styles, not the tui package.
- **View stack**: `CurrentView` + `ViewStack` for breadcrumb navigation (push/pop).
- **Reactive updates**: fsnotify watches `.ralph/` dirs; falls back to 2s polling via `tea.Tick`.
- **Process management**: `os/exec` with process groups (`Setpgid`), SIGTERM/SIGSTOP/SIGCONT.

## Distro / Thin Client

The `distro/` directory contains configs for a bootable Linux thin client (Ubuntu 24.04, i3, RTX 4090). Key files: `distro/hardware/proart-x870e.md` (hardware manifest), `distro/scripts/hw-detect.sh` (first-boot detection, testable with `--dry-run`), `distro/systemd/` (service units).

## Related Repos (same org)

- **mcpkit**: Go MCP framework — ralph loop engine, finops, sampling, workflow, gateway
- **hg-mcp**: Go MCP server with modular tool pattern (500+ tools)
- **claudekit**: Go MCP with rdcycle perpetual loop, budget profiles
- **shielddd**: Go + pure SQLite (modernc.org/sqlite) + MCP, audit logs
- **mesmer**: Go MCP server with ralph integration

## See Also

- [docs/PROVIDER-SETUP.md](docs/PROVIDER-SETUP.md) — Multi-provider prerequisites, env vars, orchestration pattern
- [docs/MCP-TOOLS.md](docs/MCP-TOOLS.md) — Full 84-tool table with descriptions
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Package layout, provider dispatch, middleware, fleet, file schemas
- [docs/MARATHON.md](docs/MARATHON.md) — Marathon supervisor, RC tools, environment setup
- [docs/AUTONOMY.md](docs/AUTONOMY.md) — Autonomy levels, self-improvement subsystems
