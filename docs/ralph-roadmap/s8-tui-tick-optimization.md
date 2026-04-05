# S8 — TUI Tick Optimization

**Date:** 2026-04-04
**Scope:** `internal/tui/` — tick handler, event bus integration, per-view conditional rendering
**Status:** Design (pre-implementation)

---

## 1. Current Tick Handler Code Flow

### Tick setup

`tickCmd()` is defined in `internal/tui/app_init.go:275`:

```go
func (m Model) tickCmd() tea.Cmd {
    return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

It is started in `Init()` (`app_init.go:266`) alongside `scanRepos()`, `Spinner.Tick`, and the process exit watcher. At the end of every `tickMsg` handler the tick re-arms itself (`cmds = append(cmds, m.tickCmd())`, `app_update.go:82`), so it fires unconditionally every 2 seconds for the lifetime of the process.

### What executes on every tick

The `tickMsg` case in `Update()` (`app_update.go:55–87`) runs this sequence unconditionally:

| Step | Call | Cost |
|------|------|------|
| 1 | `m.refreshAllRepos()` | Synchronous disk I/O — reads 4 files per repo (`.ralph/status.json`, `.ralph/progress.json`, `.ralph/.circuit_breaker_state`, `.ralphrc`). On 69 repos that is up to **276 file reads per tick**. |
| 2 | `m.SessMgr.LoadExternalSessions()` | Reads session state persisted by the MCP server (disk I/O). |
| 3 | `m.refreshObsCache()` | TTL-gated at 10 s — reads all per-repo observation files when TTL expires (`helpers.go:26`). |
| 4 | `m.refreshGateCache()` | TTL-gated at 30 s — evaluates regression gates when TTL expires (`helpers.go:47`). |
| 5 | `m.drainRegressionEvents()` | Reads last 5 events from bus history (`helpers.go:143`). |
| 6 | `m.refreshLoopView()` | Iterates all loops, locks each one, builds a string (`app_init.go:330`). |
| 7 | `m.refreshLoopControlData()` | Iterates all loops to build loop control snapshot (`app_init.go:364`). |
| 8 | `m.loopListCmd()` | Snapshots all loop runs for `LoopListMsg`. |
| 9 | `m.updateTable()` | Calls `views.ReposToRows()` and rebuilds all status bar fields — iterates all sessions, all loops, calls `session.CheckProviderHealth()` for each of 3 providers (`app_init.go:375`). |
| 10 | `m.updateSessionTable()` | Calls `views.SessionsToRows()` — iterates all sessions (`app_init.go:486`). |
| 11 | `m.updateTeamTable()` | Calls `views.TeamsToRows()` — iterates all teams (`app_init.go:495`). |

**All 11 steps execute on every tick regardless of which view is active.**

The only existing view-conditional logic is the log tail at `app_update.go:84–86`, which fires `process.TailLog` only when in `ViewLogs`. The three table updates (steps 9–11) and all repo I/O (step 1) run unconditionally even when the user is viewing a detail, diff, or log view where none of those tables are visible.

### RefreshRepo cost breakdown

`model.RefreshRepo` (`internal/model/status.go:76`) performs four sequential `os.ReadFile` calls:
- `.ralph/status.json`
- `.ralph/.circuit_breaker_state`
- `.ralph/progress.json`
- `.ralphrc`

Each is checked for `os.ErrNotExist` (missing files are not errors). On a fast local filesystem with 69 repos this is ~276 syscalls at ~50–200 µs each — roughly **14–55 ms of blocking I/O in the BubbleTea Update goroutine every 2 seconds**. BubbleTea's `Update` runs on the Elm event loop; synchronous I/O here blocks the entire TUI render pipeline.

### What already works reactively

`process.FileChangedMsg` (from fsnotify) already triggers a single-repo refresh and `updateTable()` on status file changes (`app_update.go:172`). The watcher covers `.ralph/` status files and degrades gracefully to polling-only on repeated failures (`WatcherDisabled`). This means repo status updates for running loops already arrive in sub-second time via fsnotify — the tick's `refreshAllRepos` is a redundant safety net that scans everything.

---

## 2. Event Bus Integration Design

### What the bus already has

`internal/events/Bus` (`internal/events/bus.go`) is a fully-featured in-process pub/sub bus with:
- `SubscribeFiltered(id string, types ...EventType)` — delivers only named event types
- Non-blocking channel sends with a 100-event buffer
- Pluggable transport (in-memory or NATS)
- Ring buffer history, JSONL persistence, async writes

The bus is already wired into the TUI `Model` as `m.EventBus` (`app_init.go:157`) and populated in `cmd/root.go:158`. The session runner already publishes `SessionStarted`, `SessionEnded`, `SessionStopped`, `CostUpdate`, `BudgetAlert`, `BudgetExceeded`, `JournalWritten`, `RecordingStarted`, `RecordingEnded`. The supervisor publishes `LoopStarted`, `LoopStopped`, `LoopRestarted`, `LoopIterated`.

### Bridging the bus into BubbleTea

BubbleTea is a single-goroutine Elm loop. The standard pattern for feeding external goroutine events into the loop is a `tea.Cmd` that blocks on a channel read and returns the received value as a `tea.Msg`. This is exactly how `process.WaitForProcessExit` works today.

Add a new command `watchEventBus(ch <-chan events.Event) tea.Cmd` in `app_init.go`:

```go
// EventBusMsg wraps an event bus event for delivery to the TUI Update loop.
type EventBusMsg struct{ events.Event }

// watchEventBus returns a Cmd that blocks until the next event from ch.
// After delivery, Update must re-arm it with another watchEventBus call.
func watchEventBus(ch <-chan events.Event) tea.Cmd {
    return func() tea.Msg {
        e, ok := <-ch
        if !ok {
            return nil // bus closed
        }
        return EventBusMsg{e}
    }
}
```

Subscribe in `Init()`:

```go
func (m Model) Init() tea.Cmd {
    cmds := []tea.Cmd{
        m.scanRepos(),
        m.tickCmd(),
        m.Spinner.Tick,
        process.WaitForProcessExit(m.ProcMgr.ExitChan()),
    }
    if m.EventBus != nil {
        ch := m.EventBus.SubscribeFiltered("tui",
            events.SessionStarted, events.SessionEnded, events.SessionStopped,
            events.CostUpdate, events.BudgetAlert, events.BudgetExceeded,
            events.LoopStarted, events.LoopStopped, events.LoopRestarted, events.LoopIterated,
            events.LoopRegression,
            events.TeamCreated,
            events.AnomalyDetected, events.EmergencyStop, events.EmergencyResume,
        )
        m.EventBusCh = ch
        cmds = append(cmds, watchEventBus(ch))
    }
    return tea.Batch(cmds...)
}
```

`m.EventBusCh` is a new `<-chan events.Event` field on `Model`. Handle `EventBusMsg` in `Update()` and re-arm:

```go
case EventBusMsg:
    m.handleEventBusMsg(msg.Event)
    if m.EventBusCh != nil {
        return m, watchEventBus(m.EventBusCh)
    }
    return m, nil
```

### Per-event reactive actions

```go
func (m *Model) handleEventBusMsg(e events.Event) {
    switch e.Type {
    case events.SessionStarted, events.SessionEnded, events.SessionStopped:
        // Session list changed — update session and team tables only if visible
        m.updateSessionTable()
        m.updateTeamTable()
        // Refresh status bar session counts (already part of updateTable)
        // but avoid the full repo I/O — only update session-derived fields
        m.refreshSessionStatusBar()

    case events.CostUpdate, events.BudgetAlert, events.BudgetExceeded:
        // Cost changed — update status bar cost fields only
        m.refreshCostStatusBar()
        if e.Type == events.BudgetAlert || e.Type == events.BudgetExceeded {
            label := e.Data["label"]
            m.Notify.Show(fmt.Sprintf("Budget alert: %v (session %s)", label, truncateID(e.SessionID)), 5*time.Second)
        }

    case events.LoopStarted, events.LoopStopped, events.LoopRestarted, events.LoopIterated:
        // Loop state changed — update loop-related displays
        m.refreshLoopView()
        m.refreshLoopControlData()
        // Update repo row for the affected repo by path
        if e.RepoPath != "" {
            for _, r := range m.Repos {
                if r.Path == e.RepoPath {
                    model.RefreshRepo(m.Ctx, r) // single-repo refresh
                    break
                }
            }
        }
        m.updateTable() // re-render overview rows

    case events.LoopRegression:
        m.drainRegressionEvents() // shows notification

    case events.TeamCreated:
        m.updateTeamTable()

    case events.AnomalyDetected:
        m.Notify.Show(fmt.Sprintf("Anomaly: %v", e.Data["message"]), 8*time.Second)

    case events.EmergencyStop, events.EmergencyResume:
        sev := "EMERGENCY STOP engaged"
        if e.Type == events.EmergencyResume {
            sev = "Emergency stop lifted"
        }
        m.Notify.Show(sev, 10*time.Second)
    }
}
```

`refreshSessionStatusBar()` and `refreshCostStatusBar()` are new thin helpers that update only the status bar fields derived from sessions (counts, spend, velocity) without calling `views.ReposToRows()` or doing any disk I/O. They extract the relevant loops from `updateTable()` into standalone functions.

---

## 3. Per-View Conditional Rendering

### Table update suppression

The three table update calls in the tick handler should be conditioned on view relevance:

```go
// In tickMsg handler, replace:
m.updateTable()
m.updateSessionTable()
m.updateTeamTable()

// With:
activeViews := m.activeViewSet()
if activeViews.needsRepoTable() {
    m.updateTable()
}
if activeViews.needsSessionTable() {
    m.updateSessionTable()
}
if activeViews.needsTeamTable() {
    m.updateTeamTable()
}
```

Where `activeViewSet()` captures the current view and any rendered sub-components:

```go
func (m *Model) needsRepoTable() bool {
    switch m.Nav.CurrentView {
    case ViewOverview, ViewRepoDetail, ViewLoopHealth, ViewDiff, ViewTimeline, ViewObservation:
        return true
    }
    return false
}

func (m *Model) needsSessionTable() bool {
    switch m.Nav.CurrentView {
    case ViewSessions, ViewSessionDetail, ViewFleet:
        return true
    }
    return false
}

func (m *Model) needsTeamTable() bool {
    switch m.Nav.CurrentView {
    case ViewTeams, ViewTeamDetail, ViewTeamOrchestration, ViewFleet:
        return true
    }
    return false
}
```

The status bar always needs its core counts updated regardless of view (they appear on every screen), so `refreshSessionStatusBar()` — which only updates `StatusBar` fields without calling `SetRows` — runs unconditionally on the reduced tick.

### Loop view suppression

`refreshLoopView()` and `refreshLoopControlData()` run on every tick to maintain `m.LoopView` and `m.LoopControlData`. These should be gated:

```go
if m.ShowLoopPanel {
    m.refreshLoopView()
}
if m.Nav.CurrentView == ViewLoopControl {
    m.refreshLoopControlData()
}
```

When a `LoopStarted`/`LoopStopped`/`LoopIterated` event arrives via the bus, the handler calls these unconditionally regardless of view — so freshness is preserved reactively.

### Log tail (already conditional)

The log tail (`process.TailLog`) is already gated on `ViewLogs` at `app_update.go:84`. No change needed. The reduced tick means this fires less frequently for repos not actively changing — an improvement for the log view when no new lines are being written.

---

## 4. Fallback Tick Strategy

Remove the 2-second unconditional tick. Replace it with a two-speed strategy:

### Fast tick: 5 seconds (heartbeat)

The fast fallback tick fires every 5 seconds and runs only the operations that cannot be covered by events:

- `refreshAllRepos()` — fsnotify may have missed files not under `.ralph/` (e.g. `.ralphrc` config edits). This catches any drift.
- `m.SessMgr.LoadExternalSessions()` — MCP server writes sessions to disk; the watcher covers `.ralph/` but external session files may be in a different path.
- Status bar uptime field (`time.Since(m.StartedAt)`) — not event-driven.

```go
func (m Model) tickCmd() tea.Cmd {
    return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
        return tickMsg(t)
    })
}
```

The tick handler strips out session/loop/team table updates that are now covered by the event bus:

```go
case tickMsg:
    // Context shutdown check
    if m.Ctx != nil {
        select {
        case <-m.Ctx.Done():
            return m, tea.Quit
        default:
        }
    }
    m.TickFrame++
    var cmds []tea.Cmd

    // Drift correction: re-read all repos (fsnotify may miss some files)
    cmds = append(cmds, m.refreshAllRepos()...)
    if m.SessMgr != nil {
        m.SessMgr.LoadExternalSessions()
    }

    // TTL-gated caches (unchanged)
    m.refreshObsCache()
    m.refreshGateCache()
    m.drainRegressionEvents()

    // Loop views: only refresh if panel or loop views are active
    if m.ShowLoopPanel {
        m.refreshLoopView()
    }
    if m.Nav.CurrentView == ViewLoopControl {
        m.refreshLoopControlData()
    }
    cmds = append(cmds, m.loopListCmd())

    // Conditional table updates (event bus handles reactive updates;
    // tick provides drift correction for stale data only)
    if m.needsRepoTable() {
        m.updateTable()
    } else {
        m.refreshSessionStatusBar() // status bar counts still need refreshing
    }
    if m.needsSessionTable() {
        m.updateSessionTable()
    }
    if m.needsTeamTable() {
        m.updateTeamTable()
    }

    m.LastRefresh = components.NowFunc()
    cmds = append(cmds, m.tickCmd())

    if m.Nav.CurrentView == ViewLogs && m.Sel.RepoIdx < len(m.Repos) {
        cmds = append(cmds, process.TailLog(m.Repos[m.Sel.RepoIdx].Path, &m.LogOffset))
    }
    return m, tea.Batch(cmds...)
```

### Slow tick: 30 seconds (deep refresh)

Optionally add a second, slower `tea.Tick` at 30 seconds that forces a full `updateTable()` + `updateSessionTable()` + `updateTeamTable()` regardless of active view. This ensures tables never drift more than 30 seconds even for views not currently active. This replaces the current behavior for non-active views:

```go
type slowTickMsg time.Time

func (m Model) slowTickCmd() tea.Cmd {
    return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
        return slowTickMsg(t)
    })
}

// In Update():
case slowTickMsg:
    m.updateTable()
    m.updateSessionTable()
    m.updateTeamTable()
    return m, m.slowTickCmd()
```

The slow tick is started in `Init()` alongside the fast tick.

---

## 5. Expected Performance Improvement

### Disk I/O reduction

| Scenario | Current (2 s tick) | After optimization |
|----------|-------------------|-------------------|
| 69 repos, all idle, user viewing logs | 276 file reads/tick × 30 ticks/min = **8,280 reads/min** | 276 reads/5 s × 12 ticks/min = **3,312 reads/min** (-60%) |
| Session state changes | Detected at next tick (avg 1 s lag) | Detected on event publish (**< 1 ms lag**) |
| Budget alert | Detected at next tick (avg 1 s lag) | Delivered immediately via event bus |
| Loop start/stop | Detected at next tick | Detected immediately via event bus |

### BubbleTea Update goroutine blocking

`refreshAllRepos()` is synchronous I/O in the Update goroutine. At 69 repos × 4 files × ~100 µs per file = ~27.6 ms per tick. At 2-second intervals this is **~1.4% of Update goroutine time spent in disk I/O**. Extending the tick to 5 seconds reduces this to **~0.55%**, improving input responsiveness (key presses, mouse events) during the I/O burst.

### Table render reduction

`views.ReposToRows()` iterates all repos and builds lipgloss-styled strings. `updateTable()` also iterates all sessions twice and all loops twice. When viewing logs or a diff, none of this output is rendered — it is computed and immediately discarded. Conditional rendering eliminates this dead work:

- **Viewing logs** (most common during active loops): skips `updateTable()`, `updateSessionTable()`, `updateTeamTable()` on the fast tick. Savings: ~3 full table rebuilds per 5 seconds.
- **Viewing a detail view** (RepoDetail, SessionDetail): skips `updateSessionTable()` and `updateTeamTable()` on the fast tick.

---

## 6. Estimated Effort and Risk

### Effort

| Task | Estimated lines | Notes |
|------|----------------|-------|
| Add `EventBusMsg` type and `watchEventBus` cmd | ~15 | New message type + blocking cmd |
| Wire subscription in `Init()`, add `EventBusCh` to `Model` | ~20 | One new field, Init change |
| Add `EventBusMsg` case in `Update()` | ~5 | Just re-arm and dispatch |
| Implement `handleEventBusMsg()` | ~60 | Per-event routing logic |
| Extract `refreshSessionStatusBar()` and `refreshCostStatusBar()` from `updateTable()` | ~40 | Refactor existing code |
| Add `needsRepoTable()`, `needsSessionTable()`, `needsTeamTable()` helpers | ~25 | Simple switch helpers |
| Change tick interval 2s → 5s, add conditional guards in tick handler | ~20 | Modify existing tick case |
| Add `slowTickMsg` + `slowTickCmd()` + case in `Update()` | ~20 | New slow tick |
| Update `Init()` to start slow tick | ~5 | One-liner |
| Tests: update `TestTickMsg`, add `TestEventBusMsg*` | ~80 | Required for coverage |
| **Total** | **~290 lines** | Spread across `app_init.go`, `app_update.go`, new `app_eventbus.go` |

**Estimated developer time: 1–2 days** for implementation, 0.5 days for test updates.

### Risk: Low

**Bus channel backpressure.** The `SubscribeFiltered` channel has a 100-event buffer. High-frequency events (`CostUpdate`, `LoopIterated`) could fill this under heavy fleet load. The bus already does non-blocking sends and logs dropped events — so the TUI would miss events and fall back to the 5-second tick for correction. This is acceptable behavior. If needed, the buffer size can be increased at subscribe time via a future `SubscribeFilteredBuf(id, size, types...)` variant.

**Race on `EventBusCh` close.** The bus `Unsubscribe` closes the channel. The `watchEventBus` cmd checks `ok` on receive and returns `nil` (no further re-arm) when the channel is closed. BubbleTea ignores `nil` messages. Safe.

**WatcherDisabled fallback.** When `WatcherDisabled == true`, the system relies entirely on polling for file state. The event bus handles session/loop events regardless (they are published in-process, not from file watchers). The `WatcherDisabled` path is unchanged and still benefits from the 5-second tick.

**TickFrame for spinner animation.** `TickFrame` is incremented on each `tickMsg` and passed to `views.ReposToRows()` for spinner character cycling (`app_init.go:377, 382`). At 5 seconds the spinner in the repo table will be visually sluggish when a loop is running. Mitigate by driving the spinner animation from `spinner.TickMsg` (already in the update loop via `m.Spinner.Tick`) and passing `m.Spinner.View()` to the row builder instead of `TickFrame`. This is a minor follow-up refactor.

**Test surface.** `TestTickMsg` at `app_test.go:409` tests that a tick produces a follow-up command. It will pass unchanged. New tests needed: `TestEventBusMsgSessionStarted`, `TestEventBusMsgCostUpdate`, `TestNeedsRepoTable`, and a table-driven test for `handleEventBusMsg`. These are straightforward unit tests using the existing `NewModel` test helper.

### Risk: Medium (one edge case)

**Events published before TUI subscribes.** The bus subscription in `Init()` misses events published between process start and `Init()` execution (typically < 100 ms). For `SessionStarted` this is acceptable — any sessions launched before the TUI initialized will be loaded by the initial `scanRepos()` → `updateTable()` path. For `CostUpdate` the initial table build reads `s.SpentUSD` directly from the session manager, so the initial state is correct. No data loss.

---

## Implementation Order

1. Extract `refreshSessionStatusBar()` from `updateTable()` (pure refactor, no behavior change).
2. Add `needsRepoTable()` / `needsSessionTable()` / `needsTeamTable()` helpers and apply in tick handler.
3. Change tick interval to 5 seconds.
4. Add `EventBusCh` field, `watchEventBus` cmd, `EventBusMsg` type, wire in `Init()` and `Update()`.
5. Implement `handleEventBusMsg()`.
6. Add slow tick at 30 seconds.
7. Add/update tests.

Steps 1–3 are independently releasable with no new dependencies. Steps 4–6 build on top of the stable tick change.
