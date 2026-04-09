---
paths:
  - "internal/session/**"
---

Session management patterns:
- Provider dispatch via `providers.go` — each provider has a cmd builder and event normalizer
- Provider enum: `codex` (default, gpt-5.4), `claude` (sonnet), `gemini` (gemini-3.1-pro), `antigravity` (external interactive handoff)
- Budget tracking in `budget.go` — per-provider cost enforcement; Codex, Gemini, and Antigravity budgets are tracked externally
- LaunchOptions carries all session config; TeamConfig for multi-session coordination
- Session lifecycle: launch → stream → terminate; use SIGTERM/SIGSTOP/SIGCONT for process control
- Resume support: Claude, Gemini, Codex, and Cline support resume; Antigravity is launch-only through ralphglasses
- Always return `codedError()` from handlers, never `(nil, error)`
