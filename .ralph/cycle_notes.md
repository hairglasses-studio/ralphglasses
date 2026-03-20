# Cycle Notes

Per-cycle log of Ralph's autonomous development work. Append-only — each cycle adds a new entry below.

Machine-readable companion: `improvement_notes.jsonl`

---

<!-- Template for new entries (copy below the line):

## Cycle N — YYYY-MM-DD

**Tasks:** 0.5.X (subtasks worked)
**Files modified:** list files changed
**`make ci`:** PASS / FAIL (if fail, what broke)

### What worked
- ...

### Unexpected issues
- ...

### Decisions made
- ...

### Next
- Task group to work on next cycle

-->

## Cycle 1 — 2026-03-20

**Tasks:** 0.5.1 (0.5.1.1–0.5.1.4)
**Files modified:** internal/model/status.go, internal/model/repo.go, internal/model/status_test.go, internal/tui/app.go, internal/tui/styles/styles.go, internal/tui/views/repodetail.go
**`make ci`:** PASS

### What worked
- Clean separation: `RefreshRepo` returns `[]error`, stores on `Repo.RefreshErrors`, TUI reads from there
- Missing files (os.ErrNotExist) excluded from errors — only parse failures surface

### Unexpected issues
- None

### Decisions made
- Polling path (`tickMsg`) stores errors on repo but doesn't emit notifications (would spam every 2s). Reactive path (`FileChangedMsg`) emits `RefreshErrorMsg` for one-time notifications.
- Added `WarningStyle` (yellow) to styles package for parse error display

### Next
- Task group 0.5.2 (Watcher error handling)

## Cycle 2 — 2026-03-20

**Tasks:** 0.5.2 (0.5.2.1–0.5.2.4)
**Files modified:** internal/process/watcher.go, internal/process/watcher_test.go, internal/tui/app.go
**`make ci`:** PASS

### What worked
- Clean error propagation: all watcher failure paths now return `WatcherErrorMsg` instead of `nil`
- Exponential backoff (1s→2s→4s→8s→16s→30s cap) prevents hammering on persistent failures
- After 5 consecutive failures, `WatcherDisabled` flag stops re-watch attempts; TUI continues with 2s tick polling

### Unexpected issues
- None

### Decisions made
- `watcher.Add()` errors for individual paths are tolerated (partial watch); only fails if ALL paths fail to watch
- Backoff counter resets on successful `FileChangedMsg`, so transient errors self-heal
- `WatcherDisabled` is one-way per session — requires TUI restart to re-enable watcher (polling is reliable enough)

### Next
- Task group 0.5.3 (Process reaper exit status)
