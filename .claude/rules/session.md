---
paths:
  - "internal/session/**"
---

Session management patterns:
- Provider dispatch via `providers.go` — each provider has a cmd builder and event normalizer
- Provider enum: `codex` (default, gpt-5.4), `claude` (sonnet), `gemini` (gemini-3.1-pro)
- Budget tracking in `budget.go` — per-provider cost enforcement; Codex has no built-in budget so track externally
- LaunchOptions carries all session config; TeamConfig for multi-session coordination
- Session lifecycle: launch → stream → terminate; use SIGTERM/SIGSTOP/SIGCONT for process control
- Resume support: all providers support `--resume` flag
- Always return `codedError()` from handlers, never `(nil, error)`
