# Multi-Session Claude Code Tools: Comparison

Research conducted March 2026. Evaluates tools for managing multiple Claude Code sessions on Ubuntu/WSL.

## The Two cmux Projects

### cmux (manaflow-ai) — macOS Only
- Native Ghostty-based macOS terminal app (Swift/AppKit)
- Vertical tabs, split panes, notifications, integrated browser
- GPU-accelerated rendering via libghostty
- **Does NOT work on Linux/WSL** — macOS exclusive

### cmux (craigsc) — Cross-Platform
- Pure bash script using Git worktrees
- Each agent gets its own isolated directory sharing the same `.git` database
- Commands: `cmux new <branch>`, `cmux start`, `cmux merge`, `cmux ls`
- **Works on Linux, macOS, WSL** — requires only Git + Claude CLI
- Tab completion for bash/zsh

## Terminal Multiplexer + Worktree Solutions

### cc-workflow
- **Repo**: [jrimmer/cc-workflow](https://github.com/jrimmer/cc-workflow)
- tmux + Git worktrees + Claude Code
- Parallel sessions with instant switching (Alt+]/[)
- Multi-pane layouts: dev, wide, minimal
- **Ubuntu 22.04+ provision script** included
- Remote-optimized with OSC 52 clipboard support over SSH

### zenportal
- **Repo**: [kgang/zenportal](https://github.com/kgang/zenportal)
- Python TUI built with Textual
- Manages Claude Code, OpenAI Codex, Google Gemini CLI, and shell sessions
- Vim-style navigation (j/k), session persistence
- Git worktree isolation per session
- Install: `uv tool install zen-portal`

### claude-tools
- **Repo**: [oreoriorosu/claude-tools](https://github.com/oreoriorosu/claude-tools)
- Shell scripts for tmux session management
- Project-based sessions with switching, archival, restore
- **Built for WSL** — default base directory is `/mnt/c/git`
- Dependencies: bash, tmux, jq, Claude Code CLI

### claude-code-tmux-hud
- **Repo**: [sokojh/claude-code-tmux-hud](https://github.com/sokojh/claude-code-tmux-hud)
- Real-time tmux HUD: context usage, quota tracking, task management, git status
- Statusline showing model, context %, plan type
- Supports Linux with GNU coreutils compatibility
- Launch with `ct` command

## Web-Based Dashboards

### cc-hub
- **Repo**: [m0a/cc-hub](https://github.com/m0a/cc-hub)
- Web UI for multi-session management
- Multi-pane terminals, file viewer with syntax highlighting, diff tracking
- Usage dashboard with cost estimates
- **Linux x64 binary** available
- Requires tmux 3.0+, Tailscale

### Hive
- **Repo**: [latagore/hive](https://github.com/latagore/hive)
- Mobile-first PWA dashboard for Claude Code tmux fleet
- Task queue with auto-dispatch to idle sessions
- Permission detection and one-tap approvals
- Multi-computer fleet support via remote workers
- Telegram bot integration
- Node.js 18+ / tmux 3.2+

### SplitMind
- **Repo**: [webdevtodayjason/splitmind](https://github.com/webdevtodayjason/splitmind)
- Multi-agent platform with React dashboard
- Wave-based task generation and planning
- Agent-to-agent coordination via Redis (A2AMCP protocol)
- File locking for conflict prevention

## Desktop Apps

### OpenClaudgents
- **Repo**: [MagnusPladsen/OpenClaudgents](https://github.com/MagnusPladsen/OpenClaudgents)
- Tauri v2 desktop GUI (React + Rust backend)
- Agent team visualization as interactive graphs
- Git worktree isolation, context budget tracking
- Monaco diff viewer, 11 themes
- **macOS + Linux** (not Windows native)

## CLI / Framework

### agent-runner
- **Repo**: [zsyu9779/agent-runner](https://github.com/zsyu9779/agent-runner)
- Go CLI for long-running multi-session agents
- Stateful sessions with resume, auto-commits
- Multiple agent types: initializer, coding, summary
- **macOS + Linux**

### code-on-incus
- **Repo**: [mensfeld/code-on-incus](https://github.com/mensfeld/code-on-incus)
- Go-based, uses Incus system containers
- Workspace persistence at `/workspace`
- Multi-slot parallel agents with isolated home directories
- SSH agent forwarding without exposing keys
- Protected paths (`.git/hooks`, `.git/config`) mounted read-only
- Real-time threat detection (reverse shells, C2, data exfiltration, DNS tunneling)
- Three network modes: Restricted, Allowlist, Open

### claude-multi.nvim
- **Repo**: [mb6611/claude-multi.nvim](https://github.com/mb6611/claude-multi.nvim)
- Neovim plugin for multiple Claude Code terminals
- Tab navigation, worktree support, floating/sidebar modes

## Recommendations for WSL

1. **cc-workflow** — Most complete tmux+worktree setup with Ubuntu provision script
2. **claude-tools** — Literally built for WSL (`/mnt/c/` default paths)
3. **cc-hub** — Web UI accessible from any device, Linux binary
4. **zenportal** — If using multiple AI CLIs (Claude + Codex + Gemini)
5. **craigsc/cmux** — Pure bash, cross-platform, minimal dependencies
6. **code-on-incus** — Strongest isolation (Go-based, threat detection)
