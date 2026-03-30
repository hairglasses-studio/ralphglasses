# Architecture

For the full architecture reference, see [ARCHITECTURE.md](ARCHITECTURE.md).

This page provides visual overviews of the ralphglasses system.

## High-Level Architecture

```
+------------------+     +------------------+     +------------------+
|   TUI (BubbleTea)|     |   MCP Server     |     |   CLI Commands   |
|   4 tabs, views  |     |   126 tools      |     |   mcp, scan, ... |
+--------+---------+     +--------+---------+     +--------+---------+
         |                        |                        |
         v                        v                        v
+----------------------------------------------------------------------+
|                        Session Manager                                |
|  - Provider dispatch (Claude, Gemini, Codex)                         |
|  - Process groups (SIGTERM/SIGSTOP/SIGCONT)                          |
|  - Step loop with prompt enhancement                                 |
+----------------------------------------------------------------------+
         |                        |                        |
         v                        v                        v
+------------------+     +------------------+     +------------------+
|  Claude Code     |     |  Gemini CLI      |     |  Codex CLI       |
|  (primary)       |     |  (worker)        |     |  (worker)        |
+------------------+     +------------------+     +------------------+
```

## Package Layout

```
ralphglasses/
  cmd/              CLI entry points (root, mcp, scan)
  internal/
    config/         Configuration loading, .ralphrc parsing
    enhancer/       13-stage prompt enhancement pipeline
    events/         Event bus for reactive updates
    fleet/          Fleet orchestration, tiered routing
    mcpserver/      MCP tool definitions (14 namespaces)
    process/        OS process management, PID tracking
    scanner/        Repo discovery, .ralph/ detection
    session/        Session manager, providers, step loop
    tui/            BubbleTea views, components, styles
  distro/           Bootable thin client configs
  docs/             Documentation (you are here)
```

## Session Lifecycle

```
Launch -> Provider Dispatch -> Process Start -> Step Loop -> Completion
  |            |                    |              |             |
  |            v                    v              v             v
  |       Claude/Gemini/      PID tracking    Prompt        Status
  |       Codex selection     Process groups  Enhancement   .ralph/status.json
  |                                           (13 stages)
  v
Event Bus -> TUI Update -> Fleet Dashboard
```

## Prompt Enhancement Pipeline

The 13-stage deterministic pipeline runs in order:

1. Specificity injection
2. Positive reframing
3. Tone downgrade (Claude-only)
4. Overtrigger rewrite (Claude-only)
5. XML/markdown structure (provider-aware)
6. Context reorder
7. Format enforcement
8. Self-check injection
9. Cache-friendly ordering
10. Example injection
11. Constraint tightening
12. Output format specification
13. Final lint pass

## Autonomy Levels

| Level | Name | Description |
|-------|------|-------------|
| 0 | Manual | Human approves every action |
| 1 | Assisted | Auto-start loops, human approves stops |
| 2 | Supervised | Supervisor manages cycles, human handles escalations |
| 3 | Autonomous | Full self-direction with budget guardrails |
