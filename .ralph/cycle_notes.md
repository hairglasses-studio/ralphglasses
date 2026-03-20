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
