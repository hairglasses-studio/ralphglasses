---
name: ralphglasses project overview
description: Ralphglasses is a Go BubbleTea TUI + bootable thin client for managing multi-LLM agent fleets across the hairglasses-studio ecosystem
type: project
---

Ralphglasses: command-and-control TUI + bootable Linux thin client for parallel multi-LLM agent fleets.

**Why:** The hairglasses-studio ecosystem has 9+ repos using ralph loops for autonomous AI development. Ralphglasses provides unified monitoring, control, and orchestration from one TUI.

**How to apply:**
- k9s-style vim navigation, sortable tables, command mode (`:` prefix)
- BubbleTea Model/Update/View pattern with view stack (push/pop/breadcrumb)
- Styles in `internal/tui/styles/` (import-cycle isolation)
- Components in `internal/tui/components/`, views in `internal/tui/views/`
- Process management via os/exec with process groups (SIGTERM/SIGSTOP/SIGCONT)
- Reactive updates: fsnotify watches `.ralph/` dirs, 2s tick polling fallback
- MCP control plane with deferred-loaded tool groups and live catalog discovery
- Supports Claude Code, Gemini CLI, OpenAI Codex as session providers
- 13-stage prompt enhancement pipeline with provider-aware behavior
