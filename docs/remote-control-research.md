# Remote Control via Claude Mobile App — Research

## Architecture

Claude Code supports remote control from the Claude mobile app (Android/iOS). The flow:

```
Phone (Claude app)
  → Anthropic cloud
    → WebSocket to local Claude Code session (WSL2)
      → MCP tools (ralphglasses)
        → session manager, event bus, process manager
```

The user types `/rc` in the Claude Code terminal to display a QR code. Scanning it with the Claude mobile app establishes a persistent connection. All conversation flows through Anthropic's cloud relay to the local CLI session, which has full MCP tool access.

## Session I/O Constraints

Key constraint from `runner.go:110`: **stdin is closed after the initial write.** This means:

1. You cannot inject follow-up prompts into a running LLM session
2. The only way to "send another message" is to stop the current session and launch a new one
3. `--resume` allows continuing a previous session's conversation context (Claude and Gemini only)
4. `--continue` resumes the most recent session (Claude only)

This shapes the `rc_send` tool design: it must stop any existing session before launching a new one, unless `resume=true` is specified.

## Event Bus Architecture

The `internal/events/bus.go` provides in-process pub/sub with ring buffer history:

- **11 event types**: session.started, session.ended, session.stopped, cost.update, budget.exceeded, loop.started, loop.stopped, team.created, journal.written, config.changed, scan.complete
- **Ring buffer**: configurable max size (default 1000), drops oldest on overflow
- **Pub/sub**: non-blocking send to subscribers, dropped if subscriber is slow
- **History access**: `History(type, limit)` and `HistorySince(time)` — both return chronological order

### Gap: No cursor-based access

The existing `History()` and `HistorySince()` methods work for one-shot queries but don't support the "give me everything since my last call" pattern needed for efficient polling. The `Session` struct already solves this with `TotalOutputCount` (monotonic counter) + `OutputHistory` (bounded buffer), used by `handleSessionTail`.

We add `HistoryAfterCursor(cursor, limit)` to the Bus using the same pattern.

## Existing Ecosystem

### Related projects (from awesome-claude-code research)

- **Hive PWA** — web dashboard for monitoring Claude Code sessions. Uses WebSocket for real-time updates. Good UI patterns but requires a separate web server.
- **cc-hub** — centralized Claude Code management. Desktop-focused, not mobile-optimized.
- **Claude Squad** — tmux-based multi-session management. Terminal UI, not suitable for mobile.

### Why MCP tools are the right approach

- No additional infrastructure (no web server, no WebSocket server)
- Works through the existing Claude mobile app connection
- Claude naturally formats MCP tool results as conversational text
- Tool responses can be pre-formatted text (not JSON that needs re-rendering)

## Design Principles for Mobile-Optimized MCP Tools

### 1. Text over JSON for human-facing tools

RC tools use `textResult()` instead of `jsonResult()`. Claude on mobile doesn't need to parse JSON — it renders tool results as conversation text. Pre-formatted text:
- Reduces token count (no JSON overhead)
- Reduces latency (Claude doesn't re-render)
- Reads naturally in chat bubbles

### 2. One call per action

Desktop tools often require 2-3 calls (list sessions → find ID → act on ID). Mobile tools should collapse this: `rc_send repo=X prompt=Y` does find-repo + stop-existing + launch in one call.

### 3. Minimal required params

Most RC tools need 0-1 required parameters. `rc_status` needs none. `rc_read` auto-selects the most active session.

### 4. Cursor-based polling

For `rc_read` and `event_poll`, return a cursor string that the caller passes back to get only new data. This avoids re-sending the same output lines on each poll.

### 5. Compact output

Phone screens are small. Status lines use abbreviations: `$1.23`, `15t` (turns), `2m` (idle time), `[running]` prefix.

## Tool Inventory

| Tool | Purpose | Returns |
|------|---------|---------|
| `rc_status` | Fleet overview | text |
| `rc_send` | Send prompt to repo | text |
| `rc_read` | Read session output | text |
| `event_poll` | Poll fleet events | JSON (data tool) |
| `rc_act` | Control actions | text |

## Security Considerations

- `rc_send` validates repo names via `ValidateRepoName()` and prompts via `SanitizeString()`
- Budget defaults to $5 to prevent runaway spending from mobile
- `rc_act stop_all` is the emergency kill switch — no target required
- All tools go through the same MCP permission model as desktop tools
