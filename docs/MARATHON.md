# Marathon Supervisor

`marathon.sh` is a supervisor (not a thin wrapper) that runs ralph in the background and enforces guardrails:

```bash
# Requires: ANTHROPIC_API_KEY in environment (direnv loads .env automatically)
bash marathon.sh --dry-run                          # Preview
bash marathon.sh --verbose -p ~/hairglasses-studio/<project>  # Real run
bash marathon.sh -b 50 -d 6 -c 60                  # Custom budget/duration
```

## What it enforces

- **Duration limit**: Hard wallclock kill after N hours (default: 12)
- **Budget limit**: Reads `session_spend_usd` from `.ralph/status.json`, stops at 90% of budget ceiling (default: $100 × 0.90 = $90)
- **Checkpoints**: Git tag + commit every N hours (default: 3)
- **Signal handling**: SIGINT/SIGTERM → graceful SIGTERM to ralph → 30s window → SIGKILL
- **Logging**: All supervisor events → `.ralph/logs/marathon-*.log`

## Flags ralph actually reads from .ralphrc

Only `MAX_CALLS_PER_HOUR` and `CLAUDE_TIMEOUT_MINUTES` are used by ralph_loop.sh. Other marathon-specific keys (MARATHON_DURATION_HOURS, RALPH_SESSION_BUDGET, etc.) are only for documentation/reference — the supervisor enforces them externally.

## Environment setup

Uses direnv (`.envrc` → `dotenv` → `.env`). The `.env` holds API keys for all providers:
- `ANTHROPIC_API_KEY` — Claude Code
- `GOOGLE_API_KEY` — Gemini CLI
- `OPENAI_API_KEY` — Codex CLI

Both `.env` and `.envrc` are gitignored.

## Incompatibilities

`--monitor` is incompatible with the supervisor (tmux fork breaks PID tracking). Use `--verbose` or `--live` instead.

## Remote Control (RC) Tools

Tools prefixed `rc_` are optimized for mobile remote-control via Claude Android/iOS app. Design principles:

- **Text responses** (`textResult`), not JSON — reads naturally in mobile chat bubbles
- **Minimal params** — most tools need 0-1 required parameters
- **One call per action** — `rc_send` does find-repo + stop-existing + launch in one call
- **Cursor-based polling** — `rc_read` and `event_poll` return cursors for incremental updates
- **Default budget $5** — prevents runaway spending from mobile

Key constraint: stdin closes after initial write (`runner.go:110`), so `rc_send` handles follow-up by stop-and-relaunch (or `resume=true` for session continuity).

See `docs/remote-control-research.md` for full architecture details.
