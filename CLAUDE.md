# Ralphglasses

This repo uses [AGENTS.md](AGENTS.md) as the canonical instruction file.

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

Supports **Claude Code**, **Gemini CLI**, and **OpenAI Codex CLI** as session providers. Codex now serves as the default command-and-control runtime; Claude remains available as the higher-cost reasoning and fallback lane when a task genuinely needs it.

## Two Deliverables

1. **`ralphglasses` Go binary** — Cross-platform Unix TUI (k9s-style, Charmbracelet). Manages multi-session, multi-provider LLM loops from any terminal.
2. **Bootable Linux thin client** — Manjaro Linux + Sway (Wayland), boots into ralphglasses TUI. 7-monitor, dual-NVIDIA-GPU. Legacy fallback: Ubuntu 24.04 + i3.

See ROADMAP.md for full plan. See docs/ for research.

## Build & Run

```bash
go build ./...
go run . --scan-path ~/hairglasses-studio
```

## MCP Server (200 total tools, 29 deferred-load groups)

```bash
claude mcp add ralphglasses -- go run . mcp
```

A `.mcp.json` is also included in the repo root for automatic local discovery. See [docs/MCP-TOOLS.md](docs/MCP-TOOLS.md) for the full tool table.

Tools use **deferred loading** — only the `core` group is loaded at startup. Use the `ralphglasses_tool_groups` meta-tool to list groups and `ralphglasses_load_tool_group` to load them on demand.

> **Note**: Standalone MCP server repos (dotfiles-mcp, hg-mcp, systemd-mcp, tmux-mcp, process-mcp) no longer exist as separate repos. All MCP tools are consolidated in `dotfiles/mcp/` (7 Go modules + 3 JS servers via `go.work`).

## Prompt Enhancement

The `internal/enhancer/` package provides multi-provider prompt improvement and scoring:

- **13-stage deterministic pipeline**: specificity, positive reframing, tone downgrade (Claude-only), XML/markdown structure (provider-aware), context reorder, format enforcement, self-check injection
- **10-dimensional quality scoring**: clarity, specificity, structure, examples, tone, etc. with letter grades (A-F) and provider-specific suggestions
- **11+ lint rules**: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering
- **Multi-provider LLM improvement**: Claude, Gemini, and OpenAI API clients with provider-specific meta-prompts, circuit breaker, and caching
- **Prompt caching**: All 3 providers support 80-90% input cost savings on repeated prefixes (Claude cache_control, Gemini cachedContents, OpenAI prefix caching)

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

The `distro/` directory contains configs for a bootable Linux thin client. Primary: Manjaro Linux + Sway (Wayland) + RTX 4090. Alternative: Manjaro + Hyprland. Legacy: Ubuntu 24.04 + i3 (X11). Key files: `distro/sway/` (Sway configs), `distro/hyprland/` (Hyprland configs), `distro/i3/` (legacy i3 configs), `distro/scripts/compositor-detect.sh` + `compositor-cmd.sh` (compositor abstraction layer), `distro/scripts/hw-detect.sh` (first-boot detection, writes both Sway+Hyprland configs), `distro/systemd/` (service units). The Go layer supports all three compositors via `internal/wm/` with IPC clients for Sway, Hyprland, and i3.

## Related Repos (same org, 14 total)

- **mcpkit**: Go MCP framework — ralph loop engine, finops, sampling, workflow, gateway
- **dotfiles**: Desktop config + ALL MCP servers (1,300+ tools across 7 Go + 3 JS modules in `dotfiles/mcp/`). Standalone MCP server repos (dotfiles-mcp, hg-mcp, systemd-mcp, tmux-mcp, process-mcp) no longer exist as separate repos — they live in `dotfiles/mcp/`.
- **claudekit**: Go MCP with rdcycle perpetual loop, budget profiles
- **docs**: Research knowledge base, 1,734+ files, infrastructure docs
- **mesmer**: Multi-cloud ops platform (Go, 432K LOC, potential SaaS)

## See Also

- [docs/PROVIDER-SETUP.md](docs/PROVIDER-SETUP.md) — Multi-provider prerequisites, env vars, orchestration pattern
- [docs/MCP-TOOLS.md](docs/MCP-TOOLS.md) — Live-inventory workflow and deferred-loading MCP overview
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Package layout, provider dispatch, middleware, fleet, tiered routing, prompt caching
- [docs/MARATHON.md](docs/MARATHON.md) — Marathon supervisor, RC tools, environment setup
- [docs/AUTONOMY.md](docs/AUTONOMY.md) — Autonomy levels, self-improvement subsystems
