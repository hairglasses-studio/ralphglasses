# ralphglasses TUI Audit

**Date:** 2026-04-04
**Scope:** `internal/tui/` — app, views, components, styles
**Analyst:** Claude Code (Sonnet 4.6)

---

## Summary

The TUI layer is a well-structured BubbleTea v2 application with 21 named ViewModes, 38 view files across 4 directories, and a complete component library. The architecture is transitioning from a monolithic switch-based dispatch to a registration pattern (`viewDispatch` map + `views.Registry`). The keymap system is context-aware and formally specified. Core scalability risks center on the 2-second tick refreshing all repos in-process and the fleet dashboard doing a full-render string rebuild on every update. Mouse support is shallow. Several planned views from the ROADMAP are not yet implemented.

---

## 1. View Stack Architecture

### How navigation works

Navigation state is stored in `NavigationState` on the root `Model`:

```go
type NavigationState struct {
    CurrentView ViewMode
    ViewStack   []ViewMode
    Breadcrumb  components.Breadcrumb
    ActiveTab   int
}
```

`CurrentView` holds the active view as a typed integer (`ViewMode iota`). `ViewStack` is a `[]ViewMode` slice acting as a push-down stack. `Breadcrumb` holds the display path (a `[]string` of labels), kept in sync with the stack.

**Push:** `m.pushView(v ViewMode, name string)` appends `CurrentView` to `ViewStack`, sets `CurrentView = v`, pushes the label to `Breadcrumb`, and calls `m.Keys.SetViewContext(v)` to activate the correct keybindings for the new view.

**Pop:** `m.popView()` pops the top of `ViewStack`, restores `CurrentView`, pops the `Breadcrumb`, and calls `m.Keys.SetViewContext(prev)`. At the stack root it is a no-op (not an error).

**Tab switching** via `m.switchTab(tab, view, name)` resets the entire stack: it sets `CurrentView`, clears `ViewStack` to `nil`, resets the breadcrumb to a single root label, and clears the filter. Tab switching does not preserve cross-tab view history.

**User navigation paths:**
- `1–4` — jump to root tab views (Repos, Sessions, Teams, Fleet) — clears stack
- `Enter` — drill into selected row (push detail view)
- `Esc` — pop back (handled by `handleEscape`, which also handles modal/selection dismissal first)
- Letter keys like `d`, `t`, `h`, `o`, `l`, `R` — push specific side views from context

### Stack depth

The stack is an unbounded slice. In practice, the deepest natural path is 3–4 levels deep (e.g., Fleet → Session → Diff). There is no depth cap or cycle detection. An operator using `:goto` commands in command mode could construct deeper paths, but the breadcrumb renders all labels inline and would eventually overflow the title line on a narrow terminal.

### Breadcrumb rendering

The breadcrumb is rendered inline on the title bar row alongside the title: `b.WriteString(m.Nav.Breadcrumb.View())`. It uses `" › "` as a separator. There is no overflow truncation; on a narrow terminal this silently wraps or clips.

---

## 2. Update Cycle

### Tick interval and what happens on every tick

The tick command fires every **2 seconds** via `tea.Tick(2*time.Second, ...)`. On each `tickMsg`:

1. `m.refreshAllRepos()` — calls `model.RefreshRepo()` for every repo in `m.Repos`. This is synchronous I/O (file reads of `.ralph/status.json`, `progress.json`, circuit breaker, `.ralphrc`) for every repo discovered. Errors emit `RefreshErrorMsg` as follow-up commands.
2. `m.SessMgr.LoadExternalSessions()` — reads session JSON files from disk for sessions created by the MCP server.
3. `m.refreshObsCache()` — loads observations from disk per repo (TTL-gated at 10s).
4. `m.refreshGateCache()` — evaluates regression gates (TTL-gated at 30s).
5. `m.drainRegressionEvents()` — checks event bus history.
6. `m.refreshLoopView()` / `m.refreshLoopControlData()` — reads from `SessMgr` (in-memory, fast).
7. `m.loopListCmd()` — reads from `SessMgr` (in-memory, fast).
8. `m.updateTable()` — rebuilds all overview rows, aggregates session cost/velocity/loop stats. Calls `m.SessMgr.List("")` and locks each session inside a loop.
9. `m.updateSessionTable()` / `m.updateTeamTable()` — rebuilds session and team table rows.
10. `m.LastRefresh = components.NowFunc()`
11. Re-arms the tick command.

### Bottleneck at 100+ sessions

**RefreshRepo** is the primary bottleneck. With 69 repos (the hairglasses-studio ecosystem size), each tick performs 69 synchronous file reads. At 100+ repos the tick duration grows linearly.

**updateTable** iterates all sessions twice — once in `m.SessMgr.List("")` (locks each session) and again building the status bar. At 100 sessions with the inner lock acquisitions, this is O(n) per tick but with mutex contention.

**CostHistory aggregation** in `updateTable` accumulates `s.CostHistory` slices from all sessions and then trims to the last 20 samples. At 100 sessions this creates a large intermediate slice every 2 seconds.

The tick does not run in a goroutine — it is fully synchronous in the BubbleTea `Update` function. BubbleTea guarantees single-threaded model updates, so there is no concurrency risk, but a slow tick blocks the render loop and makes the UI feel frozen.

**No virtual tick scheduling exists.** The `ROADMAP.md` acknowledges this as a future improvement: "Event-driven TUI updates (replace 2s polling tick with bus subscription)".

### Messages that drive updates

Message types that trigger model changes:
- `tickMsg` — main 2-second clock
- `process.FileChangedMsg` — fsnotify reactive update (single repo, fast path)
- `scanResultMsg` — initial scan completion
- `process.ProcessExitMsg` — process exit notification
- `LogLoadedMsg` / `process.LogLinesMsg` — log data
- `LoopListMsg`, `LoopStepResultMsg`, `LoopToggleResultMsg`, `LoopPauseResultMsg` — loop events
- `components.ConfirmResultMsg`, `components.ActionResultMsg`, `components.LaunchResultMsg` — modal results
- `SessionOutputMsg` / `SessionOutputDoneMsg` — streaming output
- `tea.MouseMsg`, `tea.KeyMsg` — user input
- `tea.WindowSizeMsg` — terminal resize

---

## 3. Memory Model

### View instances

All view objects are pre-allocated in `NewModel` and stored as pointer fields on the root `Model`. There is **no per-render allocation** of view objects. The pattern is:

1. On each render, `SetData(...)` and `SetDimensions(width, height)` are called on the relevant view.
2. Each view's `SetData`/`SetDimensions` calls `regenerate()` which calls the render function and stores the result in a `ViewportView.content` string.
3. `Render()` returns the viewport's `vp.View()`.

This means every view's `regenerate()` is called on every tick + resize, even for views that are not currently displayed, because `SetData` is called inside the `View()` switch cases. Specifically: views like `RepoDetailView`, `SessionDetailView`, `TeamDetailView`, `FleetView`, `DiffViewport`, etc. are only rendered when `CurrentView` matches, so `SetData` is only called in the `View()` path when that view is active. This is fine.

However, `updateTable()`, `updateSessionTable()`, and `updateTeamTable()` are called on **every tick regardless of the current view**, rebuilding all rows for all three tables even when the user is looking at a log view. This is unnecessary work but bounded.

### Unbounded growth risks

**LogView.Lines:** `AppendLines()` appends lines without any bound. If a session runs for hours emitting continuous output, `m.LogView.Lines` grows unboundedly. The view truncates display to `height - 3` lines but keeps all lines in memory. A long-running session with verbose output could exhaust available memory. No max-lines cap is implemented.

**Cache.Obs (observations):** `refreshObsCache` loads the last 24 hours of observations per repo. Each observation is a struct. With 69 repos each having hundreds of observations, this is manageable but untested at scale.

**Cache.Gate (gate reports):** One `GateCacheEntry` per repo path, bounded by repo count.

**EventBus.History:** The event bus history is bounded by the consumer (`History("", 200)` and `History("", 10)` calls), but the bus itself may have its own limit. Not verified in this audit.

**StatusBar.CostHistory:** Explicitly trimmed to the last 20 samples in `updateTable`. Bounded.

**LoopRun.Iterations:** Each `LoopRun` accumulates `Iteration` objects indefinitely. With 500+ iterations per loop (long-running campaigns), this creates significant per-loop memory. The `SnapshotLoopControl` function iterates all iterations to compute average duration on every tick. This is O(n) per loop per 2s tick.

**NotificationManager:** Only one notification at a time (single `*Notification` pointer). Bounded.

---

## 4. Reactive Update Path

### fsnotify → tea.Msg → view update chain

1. `process.WatchStatusFiles(paths)` returns a `tea.Cmd` that starts an fsnotify watcher goroutine.
2. When a `.ralph/` status file changes, the goroutine sends `process.FileChangedMsg{RepoPath: path}` to the BubbleTea runtime via `program.Send`.
3. In `app_update.go`, `case process.FileChangedMsg:` handles the message:
   - Resets `m.WatcherFails` to 0.
   - Calls `model.RefreshRepo(m.Ctx, r)` for the changed repo (single-repo fast path).
   - Calls `m.updateTable()` to rebuild table rows.
   - Re-arms the watcher with a new `process.WatchStatusFiles(paths)` call.

The re-arm pattern means the watcher is always one-shot per file change. This is safe (no duplicate events) but means there is a small gap between the event and re-arm where a rapid second change could be missed and caught only on the next 2s tick.

### Race conditions

The fsnotify goroutine runs concurrently with the BubbleTea update loop. BubbleTea channels all messages through its internal queue, so `FileChangedMsg` is processed serially within `Update`. There is **no direct race** between the watcher and model updates.

However, the watcher and the 2s tick can both trigger `RefreshRepo` for the same repo within the same tick window. Since BubbleTea processes messages serially, these will be sequential, not concurrent. The risk is double-refresh, not a race — benign but wasteful.

### Watcher failure handling

Failure handling is explicit and well-implemented:

- `process.WatcherErrorMsg` increments `m.WatcherFails`.
- After 5 consecutive failures, the watcher is disabled (`m.WatcherDisabled = true`) and the user is notified with a toast.
- Backoff is exponential: `1 << (fails-1)` seconds, capped at 30s.
- `watcherBackoffMsg` re-arms the watcher after the delay (unless disabled).

In polling-only mode (`WatcherDisabled = true`), the 2s tick handles all refreshes. The UX difference is that reactive single-file updates are replaced by batch polling. The user sees a notification and the refresh interval effectively doubles (worst case: 4s between a file change and display update).

---

## 5. Component Quality

### Components inventory

| Component | Pattern | Notes |
|-----------|---------|-------|
| `Table` | Custom scroll+select | Clean. Flex-width layout system. ANSI-aware padding. Mouse click support. Multi-select. |
| `StatusBar` | Priority-collapse segments | Well-designed. 7 segments, collapses from lowest priority to fit terminal width. |
| `Breadcrumb` | Simple slice-backed | Minimal, correct. No overflow handling. |
| `NotificationManager` | Single-slot toast | No queue; rapid notifications overwrite each other. |
| `SearchInput` | Active input with query field | Part of global search flow. |
| `TabBar` | Mouse-clickable tabs | Handles click-to-switch correctly. |
| `Gauge`, `InlineGauge`, `GaugeWithLabel` | String-rendered bar | Consistent style, used across all views. |
| `Sparkline`, `InlineSparkline` | Delegates to ntcharts | Used for cost trends, status bar. |
| `ConfirmDialog` | Blocking modal | Has mouse support. Implements `Modal` interface. |
| `ActionMenu` | Floating menu | Has mouse support. Implements `Modal` interface. |
| `SessionLauncher` | Form modal | Implements `Modal` interface. Has prompt quality scoring. |
| `ModalStack` | Stack of `Modal` | Defined but not wired into main model (model uses `ModalState` struct instead). |
| `Keybindings` | Display-only | For help rendering. |

### Consistency assessment

**Consistent:** All views follow the same `SetData / SetDimensions / Render` pattern. All viewport-backed views use `ViewportView` with identical scroll bindings disabled at construction. Style usage is consistent — all components import `styles.*` rather than defining own colors.

**Inconsistency — Modal wiring:** A `ModalStack` interface is defined in `components/modal.go` and both `ActionMenu` and `SessionLauncher` implement the `Modal` interface. However, the root model uses `ModalState` struct with three concrete pointer fields rather than the `ModalStack`. The `ModalStack` is never instantiated in the live app. This creates dead code and a split approach.

**Inconsistency — `fleet_dashboard.go` vs `fleet.go`:** `FleetDashboardModel` in `fleet_dashboard.go` is a standalone `tea.Model` with its own `Init/Update/View` that takes a `[]FleetSession` slice. The actual fleet view used by the app is `FleetView` in `fleet.go`, which wraps `RenderFleetDashboard` in a `ViewportView` and takes a `FleetData` struct. The `FleetDashboardModel` is not wired into the main model and appears to be a legacy or standalone demo that was never removed.

**Error handling:** All `SetData` methods guard against `nil` session/repo pointers (checking `if v.session == nil` etc.). The `View()` functions on views like `SessionDetailView` delegate to `RenderSessionDetail(v.session, ...)` which itself does a nil check at line 18. Nil safety is present throughout.

**tea.Model implementation:** Views do not implement the full `tea.Model` interface (no `Init`/`Update` methods). This is intentional — they are pure render objects, not nested models. The app delegates input via handler functions rather than sub-model `Update` calls. This is a valid BubbleTea pattern but means views cannot own their own async commands. Only `FleetDashboardModel` and `AnalyticsView` have partial `Update`-style key handling.

---

## 6. View Completeness

### Implemented views (21 named ViewMode constants)

| ViewMode | View Object / Renderer | Status |
|----------|----------------------|--------|
| `ViewOverview` | `components.Table` (overview) | Complete |
| `ViewRepoDetail` | `views.RepoDetailView` | Complete |
| `ViewLogs` | `views.LogView` | Complete |
| `ViewConfigEditor` | `views.ConfigEditor` | Complete |
| `ViewHelp` | `views.HelpView` | Complete |
| `ViewSessions` | `components.Table` (sessions) | Complete |
| `ViewSessionDetail` | `views.SessionDetailView` | Complete |
| `ViewTeams` | `components.Table` (teams) | Complete |
| `ViewTeamDetail` | `views.TeamDetailView` | Complete |
| `ViewFleet` | `views.FleetView` | Complete |
| `ViewDiff` | `views.DiffViewport` | Complete |
| `ViewTimeline` | `views.TimelineViewport` | Complete |
| `ViewLoopHealth` | `views.LoopHealthView` | Complete |
| `ViewLoopList` | `components.Table` (loops) | Complete |
| `ViewLoopDetail` | `views.LoopDetailView` | Complete |
| `ViewLoopControl` | `views.LoopControlView` | Complete |
| `ViewObservation` | `views.ObservationViewport` | Complete |
| `ViewEventLog` | `views.EventLogView` | Complete |
| `ViewRDCycle` | `views.RDCycleView` | Complete |
| `ViewTeamOrchestration` | `views.TeamOrchestrationView` | Complete |
| `ViewSearch` | `views.SearchView` | Defined but has no handler in the `handleKey` switch |

In addition, the `views/` directory contains views not wired into `ViewMode`:
- `analytics_view.go` — `AnalyticsView` (standalone, no `ViewMode` constant)
- `fleet_dashboard.go` — `FleetDashboardModel` (standalone `tea.Model`, unused in main app)
- `forecast_view.go` — `ForecastView` (not wired)
- `firstboot.go` — `FirstBootView` (not wired)
- `rdcycle.go` — present, wired as `ViewRDCycle`
- `replay_viewer.go` — `ReplayViewer` (not wired)
- `prompt_editor.go` — `PromptEditor` (not wired to a `ViewMode`)
- `launcher_budget.go` — budget sub-form for launcher (not a view, a component extension)

### Planned but missing views (from ROADMAP)

| ROADMAP Item | Description |
|-------------|-------------|
| `5.5.3` | Budget dashboard view |
| `6.1.3` | DAG visualization (task graph) |
| `6.8.4` | TUI A/B test view |
| `8.2.5` | TUI prompt editor (view exists as `prompt_editor.go` but no `ViewMode`) |
| `8.4.5` | Review dashboard |
| `8.6.4` | TUI graph view |
| `CP-6` | Flame graph `ProfileView` |
| `CQ-5` | Quality dashboards (trends, outliers, provider comparison) |
| `DS-3` | Gantt chart for task timelines and dependencies |
| `LO-3` | Token usage dashboards |
| `MR-8` | Repo health dashboard (aggregate across repos) |
| `RE-9` | Cross-provider leaderboard with time-series |

The ROADMAP reports 19 TUI views exist (matching the 21 `ViewMode` constants minus 2 unimplemented) and 11% migrated to the Phase 2 `ViewHandler` interface.

---

## 7. Fleet Dashboard Scalability

### Rendering strategy

The fleet dashboard (`fleet.go`) is a **full-render string builder** with no virtualization. `RenderFleetDashboard` renders all repos, all sessions, and all teams in a single `strings.Builder` call. The output is then set as the content of a `ViewportView`, which provides scrolling.

The three-column panel layout at the bottom uses `lipgloss.JoinHorizontal` on three independently-built string builders. Each of `data.Repos`, `data.Sessions`, and `data.Teams` is fully iterated.

### Scalability assessment at 100+ sessions

**Direct iteration:** The session list in `buildFleetData` iterates all sessions with `s.Lock()` inside the loop. At 100 sessions this is 100 mutex acquisitions per fleet view render. Since `View()` → `buildFleetData()` is called on every render frame (not just on data change), this means 100 lock acquisitions per frame for as long as the fleet view is displayed.

**Alert aggregation:** `buildFleetData` generates one alert per errored session. At 100 sessions this could produce a 100-item alerts list that is fully rendered.

**TopExpensive sessions:** The `data.TopExpensive` list is built for all sessions and then trimmed to 5. This is O(n) allocation and sort per render.

**Cost history** is rebuilt each render from the event bus history (up to 200 events). The `buildFleetCostHistory` function iterates all cost-update events and maintains a per-session map. At 100 sessions each emitting frequent cost updates, the history could be large.

**No pagination or virtual scroll:** The fleet view relies entirely on the `ViewportView` scroll mechanism. Rendering 100 sessions produces a large string buffer. There is no row virtualization — all rows are always rendered into the content string. This is functionally correct but computationally O(n) per render.

**Recommendation:** For fleets beyond ~50 sessions, consider: (a) moving `buildFleetData` into a background goroutine that sends a `FleetDataMsg`; (b) capping session list rendering with a configurable `MaxDisplayed`; (c) adopting the Lip Gloss v2 `table` package (noted as a ROADMAP item `1.5.10.4`).

---

## 8. Mouse Support

### Current state

Mouse support is implemented in `handlers_mouse.go` and routes to three targets:

1. **Tab bar** — clicking on `mouse.Y == 1` (the tab bar row) calls `m.TabBar.HandleMouse()`. This switches tabs correctly.
2. **Table views** — clicking in the content area (`mouse.Y >= 3`) calls `tbl.HandleMouse(x, relY, button, action)`, which sets the cursor to the clicked row. Only left-click press (`button == 1, action == 0`) is handled.
3. **Modal overlays** — `ConfirmDialog.HandleMouse` and `handleActionMenuMouse` handle yes/no clicks and menu item clicks.

### What is not supported

**Scroll events are not handled.** Mouse wheel events (`tea.MouseWheelUp`, `tea.MouseWheelDown` in BubbleTea v2) are not dispatched anywhere in `handleMouse`. The viewport-backed views (`LogView`, `SessionDetailView`, `FleetView`, etc.) can only be scrolled via keyboard.

**Click-to-select on non-table views.** The detail views (session detail, repo detail, fleet dashboard stats, loop control panel) have no mouse targets. Clicking within these views has no effect.

**Drag is not implemented.** No `tea.MouseMotionMsg` handling.

**Modal launcher has no mouse support.** `SessionLauncher.ModalView` and the confirm dialog only partially cover mouse interactions (confirm dialog has `HandleMouse`, launcher does not).

**Inconsistency:** Some modal types (confirm dialog) have `HandleMouse`. Others (action menu — relies on `handleActionMenuMouse`) have ad-hoc coordinate math. The `SessionLauncher` modal has no mouse support at all.

**MouseMode is set to `tea.MouseModeCellMotion`** in the `View()` return, which enables cell-level precision. This is appropriate but is wasted on the current narrow mouse target set.

---

## 9. Keyboard Model

### Keymap context switching

The keymap system uses a two-tier model defined in `keymap_context.go`:

1. **Global bindings** (15 keys) are always enabled: `q`, `:`, `/`, `?`, `Esc`, `r`, `1–4`, `j/k`, `Enter`, `s`, `Ctrl+F`.
2. **View-specific bindings** are enabled per-view via `SetViewContext(view)`. The full set is defined in `viewBindings map[ViewMode][]func(*KeyMap)*key.Binding`.
3. **Global overrides** suppress specific global bindings in certain views: `r` (Refresh) is disabled in `ViewLoopDetail` and `ViewConfigEditor`. `s` (Sort) is disabled in `ViewConfigEditor`.

`SetViewContext` uses `unsafe.Pointer` arithmetic to deduplicate binding accessors by struct field offset, building `allViewBindings` at init time. This is an efficient but unusual technique.

### Key conflicts between views

Context switching prevents most conflicts from firing simultaneously. Within a view the following **same-key bindings exist for different actions** and are resolved by context:

| Key | Action A (view) | Action B (view) |
|-----|----------------|----------------|
| `r` | Refresh (global) | ConfigRename (`ViewConfigEditor`), LoopDetailToggle (`ViewLoopDetail`) |
| `s` | Sort (global) | LoopListStart (`ViewLoopList`), LoopDetailStep (`ViewLoopDetail`), LoopCtrlStep (`ViewLoopControl`) |
| `d` | ConfigDelete (`ViewConfigEditor`) | DiffView (`ViewRepoDetail`, `ViewSessionDetail`, etc.) |
| `o` | ObservationView (global from `ViewRepoDetail`) | OutputView (`ViewSessionDetail`) | OrchestrationView (`ViewTeamDetail`) |
| `p` | LoopListPause (`ViewLoopList`) | LoopDetailPause (`ViewLoopDetail`), LoopCtrlPause (`ViewLoopControl`) |
| `e` | EditConfig (`ViewRepoDetail`) | EventLogView (global) |

These are **resolved correctly** by the enable/disable mechanism — `SetViewContext` disables the conflicting binding in each view. However, the system is maintained manually (adding a new view requires updating `viewBindings`), and the `ViewLoopHealth` view enables almost every binding (29 accessors), making it a potential source of conflicts if bindings are added globally.

### Customization support

**Plugin-defined keybindings** are supported via `~/.config/ralphglasses/plugins.yml` (Phase 3.5.2, marked complete). The `Aliases` system (Phase 3.5.3) adds command-mode shortcuts.

**Runtime rebinding is not supported.** The `KeyMap` struct is initialized once with `DefaultKeyMap()` and there is no `:keybind` command.

### `KeyDispatch` table

Global key routing is a `[]KeyDispatchEntry` slice (not a map), preserving first-match priority. 18 global entries. View-specific keys are handled by per-view handler functions (`handleOverviewKey`, etc.) in the fallback switch. This is the area under active migration to `viewDispatch`.

---

## 10. Accessibility and Theming

### Styles package separation

The `internal/tui/styles/` package exports all style variables as package-level `var` declarations. This is the deliberate architecture choice documented in `CLAUDE.md`: "Styles are in their own package to avoid import cycles." All components, views, and the main tui package import `styles.*`.

The separation is clean — there are no circular imports. The trade-off is that `styles` variables are mutable globals, which creates a thread-safety requirement. This is handled by `ThemeMu sync.RWMutex` in `theme.go` and `ApplyTheme` taking a write lock. The `ApplyTheme` function rebuilds all style variables atomically under the lock.

### Theme system

8 built-in themes are defined in `DefaultThemes()`: k9s, dracula, gruvbox, nord, catppuccin-macchiato, catppuccin-mocha, tokyo-night, snazzy. Themes are YAML files loaded via `LoadTheme`. The `snazzy` theme matches the user's personal terminal palette.

**Hot-swap:** `ThemeWatcher` (in `hotswap.go`) watches a YAML theme file with fsnotify and 100ms debounce, calling `ApplyTheme` and sending `ThemeChangedMsg` to the BubbleTea program. This enables live theme editing without restart.

**External themes:** `LoadExternalThemes` scans `~/.config/ralphglasses/themes/` for `.yaml`/`.yml` files.

### Color scheme

Colors use ANSI-256 palette references for the default theme (integers like `"196"` for red) and hex strings for named themes (e.g., `"#ff5faf"` for dracula accent). The `ApplyTheme` function switches from ANSI-256 to hex when a named theme is applied. This is slightly inconsistent — the default theme uses ANSI-256 while all named themes use hex. The ROADMAP item `3.5.1.1` (switch from ANSI-256 to hex) is marked complete, but the default `styles.go` constants still use `"196"`, `"42"`, etc. The named themes all use hex, so the inconsistency exists only when no theme is explicitly selected.

### Terminal size handling

`tea.WindowSizeMsg` is handled in `app_update.go` and explicitly sets dimensions on all 15 registered views plus the 4 tables and the status bar. All views implement `SetDimensions(width, height int)`. The minimum terminal check in `View()` returns a fallback message if `Width < 3 || Height < 3`.

The table component uses a flexible column width system (`effectiveWidths`) with `MinWidth`, `MaxWidth`, and `Flex` weight properties. Column widths adapt to terminal width. The fleet dashboard three-column layout computes `panelWidth := width/3 - 2` with a minimum of 24. Most views use `width/2 - 4` for sparkline widths with bounds.

**No minimum width enforcement exists beyond the 3-column minimum check.** On narrow terminals (< 80 columns), the multi-column stat boxes in the fleet dashboard (rendered with `lipgloss.JoinHorizontal`) will wrap or overflow rather than collapse gracefully. The status bar has priority-based segment collapse which does handle narrow terminals correctly.

### Responsive layout

The layout is responsive in the sense that views receive width/height from `SetDimensions` and respect those bounds for line truncation and sparkline sizing. However:

- The fleet dashboard three-column layout has no responsive breakpoint — it is always three columns.
- The session detail view renders content at full width with line-length truncation.
- No view implements a single-column fallback for very narrow terminals (< 60 columns).

---

## Critical Issues

1. **LogView unbounded growth** — `AppendLines` has no line cap. Long-running sessions with verbose output will grow `m.LogView.Lines` without bound. Add a `MaxLines int` field (default 10,000) with a ring-buffer eviction.

2. **LoopRun.Iterations O(n) per tick** — `SnapshotLoopControl` iterates all iterations to compute average duration, called on every 2s tick. For loops with 500+ iterations this is noticeable. Cache the average in the `LoopRun` struct and update incrementally.

3. **`ViewSearch` has no handler** — `ViewMode = ViewSearch` is defined in `app_init.go` but has no case in the `handleKey` switch and no entry in `viewDispatch`. Navigating to it silently breaks keyboard input. The `views.SearchView` object is allocated but `m.Nav.CurrentView = ViewSearch` is never set in the app.

4. **`FleetDashboardModel` dead code** — The standalone `FleetDashboardModel` in `fleet_dashboard.go` implements `tea.Model` but is not wired into the main app. It coexists with the actual `FleetView` wrapper. This creates confusion and duplicates the fleet rendering logic.

5. **`ModalStack` unused** — `components.ModalStack` is defined, `Modal` is implemented by `ActionMenu` and `SessionLauncher`, but the root model uses a concrete `ModalState` struct with three pointer fields. The stack is never instantiated. Either complete the migration or remove the dead interface.

6. **No mouse scroll support** — The `handleMouse` function does not dispatch wheel events. Viewport-backed views (fleet dashboard, session detail, log streams) cannot be scrolled with the mouse wheel.

7. **`RefreshAllRepos` is synchronous on the UI tick** — With 69+ repos, synchronous file I/O on every 2s tick creates latency spikes. The ROADMAP correctly identifies event-driven updates as a fix, but no timeline exists.

---

## Positive Highlights

- The keymap context-switching system is cleanly designed. The `unsafe.Pointer`-based deduplication is unusual but correct and well-documented.
- The `ViewportView` abstraction is consistent across all scrollable views. Disabling built-in key bindings at construction prevents double-handling.
- The `StatusBar` priority-collapse system is well-engineered and handles narrow terminals gracefully.
- The `styles.ThemeMu` mutex correctly serializes hot-swap theme updates.
- Watcher error handling with exponential backoff and polling fallback is robust.
- The `viewDispatch` migration pattern (new views go into the dispatch map, old views stay in the switch) is a clean incremental approach.
- `Table.effectiveWidths` with flex-weight column sizing is a solid responsive layout primitive.
