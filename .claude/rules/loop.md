---
paths:
  - "internal/loop/**"
  - "internal/process/**"
  - ".ralph/**"
---

Loop and process lifecycle:
- `.ralph/status.json` — LoopStatus (current loop state, iteration count, budget remaining)
- `.ralph/.circuit_breaker_state` — CircuitBreakerState: CLOSED (healthy), HALF_OPEN (testing), OPEN (stopped)
- `.ralph/progress.json` — Progress tracking per iteration
- `.ralph/improvement_journal.jsonl` — append-only JSONL, one entry per session
- `.ralphrc` — shell-style KEY="value" config (sourced by scripts)
- Process management: `os/exec` with process groups (`Setpgid`), SIGTERM for graceful stop, SIGSTOP/SIGCONT for pause/resume
- Reactive updates: fsnotify watches `.ralph/` dirs; falls back to 2s polling if fsnotify unavailable
