# Phase 7 Research: TUI Enhancement & UI Polish

**ROADMAP Items:** 1.3 (TUI polish), 1.10 (TUI bounds safety), 2.4 (Fleet dashboard TUI view)
**Date:** 2026-03-22
**Scope:** `internal/tui/` tree (app, handlers, keymap, filter, fleet_builder, command) + `internal/tui/styles/` + `internal/tui/components/` + `internal/tui/views/`

---

## 1. Executive Summary

- **Confirm dialog (1.3.1, 1.3.2) is already implemented** -- `components/confirm.go` exists with full HandleKey/View cycle, and `handlers.go` wires it to stop, stopAll, and stopSession actions. These ROADMAP items can be marked complete.
- **Bounds safety (1.10) is the highest-priority gap**: `table.go:200` does not clamp `SortCol`, `LogView` has no search UI (1.10.2), empty-slice and zero-height guards are incomplete, and there are no fuzz tests. These are crash-path risks in production marathon sessions.
- **Fleet dashboard (2.4) is already implemented** in full: `ViewFleet` exists in the view stack, `fleet.go` renders aggregate session tables with sorting, live-update via ticks, inline actions, and a fleet summary bar with cost sparklines. All 2.4.x sub-items are done.
- **Signal handling (1.3.3) is partially done**: `q`/`Ctrl+C` stops all processes, but there is no `os.Signal` handler for SIGTERM/SIGINT from outside the TUI (e.g., systemd stop). This is a reliability gap for the thin-client use case.
- **Test coverage is critically low**: `tui` package at 23%, `styles` at 13.3%. The target from ROADMAP 1.6 is 70% for tui. Achieving this requires focused test investment alongside the polish work.

---

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Has Tests | Coverage | Status |
|------|------:|-----------|----------|--------|
| `tui/app.go` | 716 | Yes (app_test.go, 403 lines) | 23.0% (pkg) | Active, core model |
| `tui/handlers.go` | 829 | Via app_test.go | (included above) | Active, all view handlers |
| `tui/keymap.go` | 350 | No | - | Active, 11 view contexts |
| `tui/fleet_builder.go` | 480 | No | - | Active, fleet data aggregation |
| `tui/command.go` | 21 | Yes (71 lines) | - | Active, `:` command parser |
| `tui/filter.go` | 25 | Yes (53 lines) | - | Active, `/` filter state |
| `styles/styles.go` | 181 | Yes (25 lines) | 13.3% | Active, 30+ style vars |
| `styles/theme.go` | 142 | Via styles_test.go | (included above) | Active, 5 built-in themes |
| `styles/icons.go` | 108 | No | - | Active, 25+ Nerd Font icons |
| `components/table.go` | 372 | Yes (138 lines) | 65.3% (pkg) | Active, sortable/filterable |
| `components/statusbar.go` | 94 | Yes (38 lines) | (included above) | Active, 12+ data fields |
| `components/breadcrumb.go` | 43 | Yes (42 lines) | (included above) | Active, push/pop nav |
| `components/notification.go` | 47 | Yes (48 lines) | (included above) | Active, timed toast |
| `components/confirm.go` | 108 | Yes (76 lines) | (included above) | Active, y/n/cancel modal |
| `components/actionmenu.go` | 133 | Yes (88 lines) | (included above) | Active, shortcut-key menu |
| `components/launcher.go` | 214 | Yes (99 lines) | (included above) | Active, session launch form |
| `components/gauge.go` | 127 | No | - | Active, sparklines/dots |
| `components/tabbar.go` | 27 | No | - | Active, tab rendering |
| `components/width.go` | 26 | No | - | Active, ANSI-aware width |
| `views/overview.go` | 110 | Yes (72 lines) | 58.5% (pkg) | Active, repo table builder |
| `views/repodetail.go` | 155 | Yes (89 lines) | (included above) | Active, full detail render |
| `views/logstream.go` | 186 | Yes (164 lines) | (included above) | Active, viewport-based |
| `views/configeditor.go` | 148 | Yes (131 lines) | (included above) | Active, key-value editor |
| `views/fleet.go` | 373 | Yes (85 lines) | (included above) | Active, full dashboard |
| `views/sessiondetail.go` | 205 | No | - | Active, session deep-view |
| `views/sessions.go` | 114 | No | - | Active, session table builder |
| `views/teams.go` | 73 | No | - | Active, team table builder |
| `views/teamdetail.go` | 118 | No | - | Active, team deep-view |
| `views/diffview.go` | 100 | No | - | Active, git diff rendering |
| `views/timeline.go` | 154 | No | - | Active, session Gantt chart |
| `views/help.go` | 71 | Yes (72 lines) | (included above) | Active, help overlay |

**Total: 48 files, ~10,492 lines** across the TUI tree (including tests).

### 2.2 What Works Well

1. **View stack navigation** (`app.go:460-477`): Clean push/pop with breadcrumb sync. No stack corruption paths observed.
2. **Modal overlay layering** (`app.go:356-365`, `app.go:667-679`): ConfirmDialog, ActionMenu, and Launcher correctly take priority over normal keys.
3. **Theme system** (`styles/theme.go`): 5 built-in themes, YAML loadable, `ApplyTheme()` rebuilds all styles. Ready for Phase 3.5 expansion.
4. **ANSI-aware rendering** (`components/width.go`): Delegates to `charmbracelet/x/ansi` for correct CJK/emoji width handling.
5. **Fleet dashboard richness** (`views/fleet.go`): Stat boxes, cost sparkline (ntcharts), provider breakdown, budget gauges, event feed, alerts, 3-panel repo/session/team list with selection markers.
6. **Component library** is well-factored: each component is self-contained, imports only `styles`, and has a clear `View()` + `HandleKey()` contract.
7. **Reactive updates** (`app.go:296-320`): fsnotify watcher with exponential backoff and polling fallback is production-hardened.
8. **Prompt quality scoring** in launcher (`components/launcher.go:81-94`): Real-time `enhancer.Analyze()` score after prompt editing -- a standout UX feature.

### 2.3 What Doesn't Work

1. **SortCol out-of-bounds** (`components/table.go:200`): `CycleSort()` modulo wraps on `len(t.Columns)` but never validates that `SortCol` is still valid after a `SetRows()` call with different column count. Panics possible when switching between overview/session/team tables. **[ROADMAP 1.10.1]**

2. **No search UI in LogView** (`views/logstream.go`): The `Search` field exists (line 16) and `filteredLines()` works (line 136-148), but there is no key binding to enter search mode, no `/` handler, and no `n`/`N` next/prev match navigation. **[ROADMAP 1.10.2]**

3. **Empty-slice panics not fully audited** (`components/table.go:293`): `t.filtered[vi]` can panic if `filtered` is nil and `end > 0` due to race between `SetRows` and `View`. `sortRows()` at line 217 indexes `t.Rows[i][col]` without checking row length vs column count. **[ROADMAP 1.10.3]**

4. **No fuzz tests** for table rendering. **[ROADMAP 1.10.4]**

5. **Zero-height terminal not handled** (`components/table.go:189-193`): `visibleRows()` returns 5 when `Height <= 3`, but the caller at line 287 (`end = t.Offset + visible`) can produce out-of-range access when `filtered` has fewer than 5 rows and offset is nonzero. **[ROADMAP 1.10.5]**

6. **No SIGINT/SIGTERM handler** (`app.go`): Only `tea.KeyMsg` for `q`/`Ctrl+C` triggers cleanup. If the process receives `SIGTERM` from systemd or a marathon supervisor, managed processes are orphaned. **[ROADMAP 1.3.3]**

7. **Scroll bounds audit incomplete** (`views/logstream.go:36-38`): `SetDimensions` clamps `vpHeight` to minimum 1, but does not re-validate scroll position. On terminal shrink, viewport may reference content offsets beyond bounds. **[ROADMAP 1.3.4]**

8. **Diff view is not scrollable** (`views/diffview.go`): Truncates to `height - 15` lines and has no viewport/scroll. Long diffs are cut off with a "truncated" message.

9. **Timeline view is not scrollable** (`views/timeline.go`): Truncates to `height - 8` entries. No cursor or scroll navigation.

10. **Config map iteration is non-deterministic** (`views/repodetail.go:103`): `for k, v := range r.Config.Values` renders config keys in random order per frame. The config editor (`configeditor.go:26-29`) correctly sorts keys.

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap |
|-------------|--------|---------------|-----|
| 1.3.1 | ConfirmDialog component | **DONE** -- `components/confirm.go` (108 lines) | None |
| 1.3.2 | Wire confirm to destructive actions | **DONE** -- wired to stop, stopAll, stopSession in `handlers.go` | None |
| 1.3.3 | SIGINT/SIGTERM shutdown handler | **NOT DONE** -- only `tea.KeyMsg` quit, no `os.Signal` | Full gap |
| 1.3.4 | Audit scroll bounds | **PARTIAL** -- logstream has min-height clamp, no resize re-validation | Moderate gap |
| 1.10.1 | SortCol bounds clamping | **NOT DONE** -- modulo wrap only, no post-SetRows validation | Full gap |
| 1.10.2 | LogView search UI | **NOT DONE** -- backend filtering exists, no UI | Full gap |
| 1.10.3 | Empty-slice panic audit | **NOT DONE** -- several unguarded slice accesses found | Full gap |
| 1.10.4 | Fuzz tests for table | **NOT DONE** -- no fuzz tests exist | Full gap |
| 1.10.5 | Zero-height terminal handling | **NOT DONE** -- `visibleRows()` returns hardcoded 5 on small terminals | Full gap |
| 2.4.1 | ViewFleet in view stack | **DONE** -- `ViewFleet` constant, `switchTab(3, ViewFleet, "Fleet")` | None |
| 2.4.2 | Sortable aggregate session table | **DONE** -- `fleet.go` renders sessions with provider/repo/status/spend | None |
| 2.4.3 | Live-update via watcher ticks | **DONE** -- `tickMsg` refreshes all tables including fleet data | None |
| 2.4.4 | Inline actions from fleet view | **DONE** -- `handleFleetKey` supports stop, diff, timeline, enter | None |
| 2.4.5 | Fleet summary bar | **DONE** -- stat boxes with repos/loops/sessions/spend/turns/circuits | None |

### 3.2 Missing Capabilities

1. **Signal handler for graceful shutdown**: Required for systemd integration (thin client) and marathon supervisor. Must call `ProcMgr.StopAll()` and `SessMgr.StopAll()` before exit.
2. **LogView search UI**: Filter backend exists but has no entry point. Needs `/` key binding, search mode indicator, `n`/`N` navigation, and match highlighting.
3. **Scrollable diff view**: Current truncation is inadequate for real diffs (hundreds of lines). Needs viewport backing like LogView.
4. **Scrollable timeline view**: Same truncation issue. Needs viewport or cursor navigation.
5. **"Terminal too small" guard**: No minimum-size enforcement. Should display a centered message when terminal is below e.g. 40x10.
6. **Config key ordering in repo detail**: Non-deterministic map iteration causes key flicker on refresh.

### 3.3 Technical Debt Inventory

| Debt Item | Location | Severity | Effort |
|-----------|----------|----------|--------|
| `SortCol` not clamped after column changes | `components/table.go:200` | High (crash) | S |
| `sortRows()` no row-length guard | `components/table.go:217` | High (crash) | S |
| `filtered` nil check missing in View | `components/table.go:293` | Medium (crash) | S |
| Config map random order in detail view | `views/repodetail.go:103` | Low (visual flicker) | S |
| DiffView not scrollable | `views/diffview.go` | Medium (truncation) | M |
| Timeline not scrollable | `views/timeline.go` | Medium (truncation) | M |
| No `os.Signal` shutdown handler | `tui/app.go` | High (orphans) | S |
| Test coverage 23% (tui pkg) | `tui/app_test.go` | High (regression risk) | L |
| Test coverage 13.3% (styles pkg) | `styles/styles_test.go` | Medium | S |
| `fleet_builder.go` untested (480 lines) | `tui/fleet_builder.go` | High (regression risk) | M |
| `keymap.go` untested (350 lines) | `tui/keymap.go` | Medium | S |
| `gauge.go` untested (127 lines) | `components/gauge.go` | Medium | S |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

| Project | Language | Key UI Patterns | Relevance |
|---------|----------|----------------|-----------|
| **k9s** | Go (tview) | Skins system (YAML themes), plugin system (YAML commands), resource aliases, consistent panel layout, xray drill-down, pulse view (sparklines), bench command | Direct inspiration. ralphglasses already mirrors skins (theme.go) and panel layout. k9s's plugin.yml pattern is targeted in ROADMAP 3.5.2. |
| **lazygit** | Go (gocui) | Fixed panel layout with always-visible state, tab-within-panel, undo stack, interactive rebase, diff staging, consistent keybinds across panels | lazygit's "state -> action -> new state" loop and spatial consistency principle directly applies. The undo buffer concept maps to ROADMAP 1.5.4. |
| **lazydocker** | Go (gocui) | Container/image/volume panels, log streaming with follow, resource graphs, custom commands, bulk operations | Log streaming with follow is already implemented. lazydocker's container lifecycle maps well to session lifecycle. |
| **btop++** | C++ | Responsive layout (adapts to terminal size), GPU monitoring, mouse support, multiple themes, process tree view | Responsive layout is the key lesson -- btop++ gracefully degrades sections when width/height shrinks. ralphglasses does not do this. |
| **Ratatui** (Rust) | Rust | Constraint-based layout, flex layout, widget trait system, built-in scrollbar widget, responsive breakpoints | Ratatui's constraint layout system is more sophisticated than BubbleTea's string-concat approach. The scrollbar widget pattern would benefit LogView. |
| **phiat/claude-esp** | Go (BubbleTea) | Real-time Claude Code session monitoring via JSONL parsing, tree-view session hierarchy, event streaming with follow, terminal-native session inspector | Directly comparable architecture -- Go + BubbleTea for Claude Code monitoring. claude-esp's JSONL parsing approach for live session output is more robust than ralphglasses' line-based tail. Tree view for session hierarchy is a pattern worth evaluating for team detail views. |
| **NimbleMarkets/ntcharts** | Go | Sparkline, bar chart, heatmap, line chart, streaming line, time series, candlestick, waveline, scatter. All implement `bubbletea.Model` with `Update()`/`View()`. Canvas-based rendering with braille/block character sets. | Already used in ralphglasses (`views/sessiondetail.go`, `views/fleet.go` for cost sparkline). Additional chart types (heatmap for session activity, time series for cost over time, bar chart for provider comparison) could enrich the fleet dashboard. |
| **Ghostty** | Zig | Theme system: `ghostty +list-themes` command, `~/.config/ghostty/themes/` directory for custom themes, system light/dark auto-switching, theme preview in terminal | Ghostty's theme discovery pattern (CLI command to list available themes + directory for user themes) maps directly to a `ralphglasses themes list` subcommand and `~/.ralphglasses/themes/` directory. System light/dark switching is relevant for the thin client auto-boot scenario. |

### 4.2 Theme Ecosystem Analysis

The research identified three mature theme specification systems relevant to ralphglasses:

**Catppuccin**: 4 flavors (Mocha, Frappe, Macchiato, Latte), 26 named colors per flavor, 300+ application ports. The specification defines semantic color roles (base, surface, overlay, text, subtext, rosewater, flamingo, pink, mauve, red, maroon, peach, yellow, green, teal, sapphire, blue, lavender) that map well to ralphglasses' 8-color Theme struct. Adding Catppuccin support would require expanding Theme to ~14 colors or mapping Catppuccin's 26 to the existing 8 semantically.

**Tokyo Night**: 3 variants (Storm, Night, Day) with specific hex palettes. Simpler than Catppuccin -- maps almost 1:1 to the existing Theme struct (primary, accent, green, yellow, red, gray, dark_bg, bright_fg).

**k9s skins**: Hierarchical YAML with per-resource-type style overrides (e.g., `k9s.body.fgColor`, `k9s.frame.title.fgColor`, `k9s.views.table.header.fgColor`). More granular than ralphglasses' flat 8-color model. k9s supports env-specific themes (different colors per Kubernetes context) and live file watching for hot-reload. The hierarchical approach is overkill for ralphglasses Phase 1 but worth considering for Phase 3.5.

**Current ralphglasses theme gaps**: The existing 5 built-in themes (k9s, dracula, gruvbox, nord, snazzy) use a mix of ANSI-256 color numbers and hex strings. The `snazzy` theme uses hex (`#57c7ff`) while others use ANSI numbers (`"39"`). Lipgloss handles both, but consistency would improve maintainability. Adding Catppuccin Mocha and Tokyo Night Storm as built-in themes requires only new entries in `DefaultThemes()`.

### 4.3 Charmbracelet Ecosystem Findings

**Lipgloss v2 compositing API**: Lipgloss v2 introduces a compositing/layering system with X, Y, Z positioning for rendering overlays. This would simplify modal rendering in `app.go` -- instead of concatenating the confirm dialog or action menu as a string override, use `lipgloss.Place()` with Z-layering. However, lipgloss v2 also changes `lipgloss.Color` to `color.Color` (standard library type), which would require updating all color constants in `styles/styles.go`. This is a breaking migration that should be deferred until Phase 3.5.

**Bubbles display components**: The `charmbracelet/bubbles` library provides several components not yet used by ralphglasses:
- `table` -- official table widget with header/row styling, but ralphglasses' custom `components/table.go` is more feature-rich (sorting, filtering, multi-select, ANSI-aware padding). No migration recommended.
- `list` -- filterable list with fuzzy matching. Could replace the fleet dashboard's session/repo/team panels for more consistent filtering UX.
- `viewport` -- already used in `views/logstream.go`. Could be applied to DiffView and Timeline to make them scrollable.
- `progress` -- already used in `views/sessiondetail.go` for budget progress bar.
- `spinner` -- could replace the custom `ActivityDot` braille spinner in `components/gauge.go` for more animation options.
- `textinput` -- could replace the manual `EditBuf` + cursor rendering in `components/launcher.go` for proper cursor movement, selection, and clipboard support.

**BubbleTea high-performance rendering**: For views with heavy ANSI content (fleet dashboard sparklines, colored tables), BubbleTea supports `tea.WithAltScreen()` and `tea.WithMouseCellMotion()`. The key optimization is to use `viewport` for any content that exceeds the terminal height, avoiding full-screen re-renders. The `logstream.go` already does this correctly; `fleet.go` does not and re-renders the entire dashboard string on every tick.

### 4.4 Patterns Worth Adopting

1. **Minimum terminal size guard** (btop++, lazygit): Display a centered "Terminal too small (need 80x24, have WxH)" message. Prevents layout corruption and panics. Directly addresses ROADMAP 1.10.5.

2. **Responsive section collapsing** (btop++): When terminal width drops below thresholds, hide lower-priority sections (e.g., fleet dashboard drops provider breakdown, then budget gauges, then events). Prevents text wrapping and overflow.

3. **Search in scrollable views** (k9s, lazygit): `/` enters search mode, matches are highlighted with a distinct background, `n`/`N` cycle through matches. Directly addresses ROADMAP 1.10.2.

4. **YAML-driven custom commands** (k9s plugins.yml): User-defined shortcuts that run shell commands with variable substitution. ROADMAP 3.5.2 already plans this. Worth noting that k9s plugins scope to specific resource types -- ralphglasses should scope to view types (repos/sessions/teams/fleet).

5. **Undo/redo in editors** (lazygit): Single-level undo for config edits. ROADMAP 1.5.4 plans this.

6. **Scrollbar indicator** (Ratatui, VS Code terminal): A visual scrollbar on the right edge of scrollable views (LogView, DiffView). Provides spatial awareness in long content.

7. **Theme discovery CLI** (Ghostty): A `ralphglasses themes list` subcommand that prints available themes with color previews. Combined with `~/.ralphglasses/themes/` directory for user-defined YAML themes and `--theme` flag for startup selection.

8. **JSONL session streaming** (claude-esp): Parse Claude Code's JSONL output directly rather than line-based log tailing. Provides structured access to tool calls, costs, model switches, and errors. This would improve the accuracy of session detail views.

9. **Heatmap visualization** (ntcharts): Use ntcharts heatmap for session activity over time (x-axis = hours, y-axis = repos, color = cost or API call density). Would add a high-information-density view to the fleet dashboard.

10. **bubbles/textinput for form fields** (Bubbles): Replace the manual `EditBuf` + cursor rendering in `components/launcher.go` with `bubbles/textinput` for proper cursor movement (Home/End/arrow keys), clipboard support (Ctrl+V), and word-level operations (Ctrl+W, Alt+Backspace).

### 4.5 Anti-Patterns to Avoid

1. **Mouse-first design in TUI**: Some TUIs prioritize mouse interaction over keyboard. For a marathon-mode fleet controller, keyboard-first is correct. Mouse support can be added later but should never be required.

2. **Deep modal nesting**: More than 2 levels of modal overlays (e.g., confirm inside action menu inside launcher) creates confusion. ralphglasses correctly prevents this by dismissing modals before opening new ones.

3. **Blocking I/O in View()**: Never call `exec.Command` in View(). `diffview.go:31-53` calls `git diff` synchronously inside `RenderDiffView()`. This blocks the TUI event loop. Should be moved to a `tea.Cmd` that sends the result as a message.

4. **Unbounded string building**: The `View()` method in `app.go` concatenates all sections without height budgeting. On very small terminals, the output exceeds the terminal height and causes scrolling artifacts.

5. **Global mutable style variables**: `styles/styles.go` uses package-level `var` for all styles. While `ApplyTheme()` is thread-safe during init, applying a theme at runtime while `View()` is rendering could produce inconsistent styling. Should be atomic or applied between frames.

6. **Compiled Go plugins (.so)**: ROADMAP 2.13.2 plans scanning for Go plugin `.so` files. Go's `plugin` package is notoriously brittle -- plugins must be compiled with the exact same Go version, toolchain, and dependency set as the host binary. k9s abandoned compiled plugins for YAML-driven command plugins. HashiCorp's `go-plugin` (RPC over stdin/stdout) is more robust but heavier. For ralphglasses, a YAML command plugin system (like k9s) is recommended for Phase 3.5, with compiled plugins deferred or dropped.

### 4.6 Accessibility Considerations

Terminal accessibility is an emerging concern for production TUIs:

- **Screen reader support**: BubbleTea does not currently emit ARIA-like attributes. The Charmbracelet team has an [open discussion](https://github.com/charmbracelet/bubbletea/issues/780) about accessibility. For now, ensure all views have text-only representations (no information conveyed solely by color).
- **High contrast mode**: The existing theme system can support a "high-contrast" theme (bright foreground on black, no dim/gray text). This is a trivial addition to `DefaultThemes()`.
- **Keyboard-only navigation**: ralphglasses is already keyboard-first. Ensure all interactive elements are reachable via tab/arrow keys without mouse -- currently satisfied.
- **Color-blind safety**: Avoid red/green as the sole distinguisher. ralphglasses uses both color AND text/icons for status (e.g., `StatusIcon()` returns distinct Unicode characters per state), which is correct.

### 4.7 Academic & Industry References

1. **The Elm Architecture (TEA)**: BubbleTea's foundation. Key principle: Update is the only place state changes, View is a pure function of state. ralphglasses follows this correctly.

2. **"Don't Make Me Think" (Krug, 2014)**: Discoverability principle -- every view should show its available actions in a help footer. ralphglasses does this in all views (e.g., `repodetail.go:152`, `logstream.go:183`).

3. **Synchronized Output (Mode 2027)**: BubbleTea v2 supports Mode 2027 (DEC private mode for synchronized rendering). Reduces flicker on terminals like Ghostty, iTerm2, and Windows Terminal. Already handled by the framework.

4. **Nielsen's 10 Usability Heuristics**: Relevant heuristics: Visibility of system status (status bar covers this), Error prevention (confirm dialogs cover this), Recognition over recall (help overlay covers this).

5. **Lazygit: 5 Years Retrospective** (Jesse Duffield, 2025): Key takeaways -- (a) spatial consistency matters more than feature density, (b) the undo stack is the most underrated feature, (c) keyboard shortcuts should be discoverable in-context not in a separate help page. ralphglasses already follows (c) via help footers in each view.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions (This Sprint)

| # | Action | Target File(s) | Effort | Impact | ROADMAP |
|---|--------|----------------|--------|--------|---------|
| 1 | Clamp `SortCol` in `SetRows()` and `CycleSort()` to `0..len(Columns)-1` | `components/table.go:50-56, 196-211` | S | High -- prevents crash | 1.10.1 |
| 2 | Guard `sortRows()` against row-length mismatch | `components/table.go:213-224` | S | High -- prevents crash | 1.10.3 |
| 3 | Add nil-check for `filtered` slice in `View()` | `components/table.go:286-295` | S | High -- prevents crash | 1.10.3 |
| 4 | Add "terminal too small" guard in `app.go:View()` | `tui/app.go:601` | S | High -- prevents panic | 1.10.5 |
| 5 | Add `os.Signal` handler (SIGTERM/SIGINT) with graceful shutdown | `tui/app.go` or `cmd/root.go` | S | High -- prevents orphans | 1.3.3 |
| 6 | Sort config keys in `RenderRepoDetail` | `views/repodetail.go:103` | S | Low -- fixes flicker | 1.3.4 |
| 7 | Mark ROADMAP 1.3.1, 1.3.2, 2.4.1-2.4.5 as complete | `ROADMAP.md` | S | - | 1.3, 2.4 |

### 5.2 Near-Term Actions (Next 2 Sprints)

| # | Action | Target File(s) | Effort | Impact | ROADMAP |
|---|--------|----------------|--------|--------|---------|
| 8 | Implement LogView search UI: `/` key binding, `n`/`N` navigation, match highlighting | `views/logstream.go`, `tui/handlers.go`, `tui/keymap.go` | M | High -- key UX feature | 1.10.2 |
| 9 | Make DiffView scrollable (viewport-backed, like LogView) | `views/diffview.go` | M | Medium -- usability | 1.3.4 |
| 10 | Make Timeline view scrollable with cursor navigation | `views/timeline.go`, `tui/handlers.go` | M | Medium -- usability | 1.3.4 |
| 11 | Move `git diff` exec from `RenderDiffView` to a `tea.Cmd` | `views/diffview.go`, `tui/handlers.go` | M | Medium -- unblocks event loop | 1.3.4 |
| 12 | Add fuzz tests for table rendering | `components/table_test.go` | M | Medium -- crash prevention | 1.10.4 |
| 13 | Add resize re-validation in LogView.SetDimensions | `views/logstream.go:33-42` | S | Medium -- prevents OOB | 1.3.4 |
| 14 | Audit all remaining slice accesses in components/views | All TUI files | M | High -- crash prevention | 1.10.3 |
| 15 | Add tests for `fleet_builder.go` (480 untested lines) | `tui/fleet_builder_test.go` (new) | M | High -- regression risk | 1.6.1 |
| 16 | Add tests for `keymap.go` (350 untested lines) | `tui/keymap_test.go` (new) | S | Medium -- regression risk | 1.6.1 |

### 5.3 Strategic Actions (Next Quarter)

| # | Action | Target File(s) | Effort | Impact | ROADMAP |
|---|--------|----------------|--------|--------|---------|
| 17 | Responsive layout: collapse fleet sections on narrow terminals | `views/fleet.go` | L | Medium -- multi-monitor UX | 3.1-3.2 |
| 18 | Add scrollbar indicators to all scrollable views | `components/` (new scrollbar component) | M | Low -- visual polish | 3.5.1 |
| 19 | Add `--theme` CLI flag, `:theme` command, `~/.ralphglasses/themes/` discovery | `cmd/root.go`, `tui/command.go`, `styles/theme.go` | M | Medium -- personalization | 3.5.1.6-7 |
| 20 | Add Catppuccin Mocha and Tokyo Night Storm as built-in themes | `styles/theme.go` (new entries in `DefaultThemes()`) | S | Low -- community themes | 3.5.1 |
| 21 | YAML command plugin system (not compiled .so -- see anti-patterns) | `internal/plugins/` (new pkg) | L | Medium -- extensibility | 2.13 |
| 22 | Resource aliases (`:rp`, `:ss`, `:tm`, `:fl`) | `tui/command.go` | S | Low -- convenience | 3.5.3 |
| 23 | Height budgeting in `View()`: allocate terminal rows to sections | `tui/app.go:601-716` | L | Medium -- prevents overflow | 1.3.4 |
| 24 | Raise tui package test coverage from 23% to 70% | `tui/app_test.go`, new test files | XL | High -- ROADMAP target | 1.6.1 |
| 25 | Replace `launcher.go` EditBuf with `bubbles/textinput` | `components/launcher.go` | M | Medium -- proper cursor/clipboard | 1.3 |
| 26 | Add high-contrast accessibility theme to `DefaultThemes()` | `styles/theme.go` | S | Low -- accessibility | 3.5.1 |
| 27 | Add ntcharts heatmap for session activity in fleet dashboard | `views/fleet.go` | M | Medium -- information density | 2.4 |
| 28 | Wrap fleet dashboard in viewport for scroll on small terminals | `views/fleet.go`, `tui/app.go` | M | Medium -- prevents clipping | 1.3.4 |
| 29 | Mouse support for clickable tabs and table rows | `tui/app.go`, `components/table.go` | L | Low -- secondary input | - |

---

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|-------------|--------|------------|
| Table panic on empty data during marathon | High | High -- TUI crash, orphaned processes | Fix items 1-3 (SortCol clamp, row guard, nil check) immediately |
| SIGTERM kills TUI without process cleanup | High | High -- orphaned ralph/claude processes consume API budget | Fix item 5 (os.Signal handler) immediately |
| Zero-height terminal crash on i3 workspace switch | Medium | High -- TUI restart needed | Fix item 4 (min-size guard) |
| DiffView blocks event loop on large repos | Medium | Medium -- TUI freezes for seconds | Fix item 11 (move to tea.Cmd) |
| Theme switching during View() causes inconsistent render | Low | Low -- visual glitch for one frame | Defer; apply themes between frames only |
| Fleet dashboard performance with 50+ sessions | Medium | Medium -- slow render, high CPU | Profile and cache `buildFleetData()` result; skip rebuild if no tick changes |
| Nerd Font icons render as boxes on non-patched terminals | Medium | Low -- functional but ugly | Add `--ascii` flag to fall back to ASCII icons |
| Config key flicker in repo detail | High | Low -- visual annoyance | Fix item 6 (sort keys) |
| Lipgloss v2 migration breaks all color constants | Medium | Medium -- significant refactor | Defer to Phase 3.5; all colors in `styles/styles.go` need `lipgloss.Color` -> `color.Color` |
| Compiled Go plugins (.so) fragile across versions | High | Medium -- broken plugin ecosystem | Use YAML command plugins (k9s pattern) instead per recommendation #21 |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

The critical path focuses on crash prevention and process safety:

```
1.10.1 (SortCol clamp) ──┐
1.10.3 (slice guards)  ──┼── All S effort, can be done in 1 PR
1.10.5 (min-size guard) ──┘
     │
     ▼
1.3.3 (SIGTERM handler) ── S effort, independent PR
     │
     ▼
1.10.2 (LogView search) ── M effort, depends on stable view system
     │
     ▼
1.3.4 (scroll bounds audit + DiffView/Timeline scrollable) ── M effort
     │
     ▼
1.10.4 (fuzz tests) ── M effort, validates all above fixes
```

### 7.2 Recommended Sequence

**Week 1: Crash Prevention (items 1-6)**
- PR 1: Table bounds safety (`SortCol` clamp, row-length guard, nil-filtered check)
- PR 2: Terminal minimum-size guard + config key sort fix
- PR 3: `os.Signal` shutdown handler

**Week 2: Scrollable Views (items 8-11, 13)**
- PR 4: LogView search UI (`/`, `n`/`N`, highlighting)
- PR 5: DiffView -> viewport-backed + async git diff via `tea.Cmd`
- PR 6: Timeline -> cursor navigation + scrollable

**Week 3: Testing (items 12, 14-16)**
- PR 7: Fuzz tests for table
- PR 8: `fleet_builder_test.go` + `keymap_test.go`
- PR 9: Slice access audit across all components/views

**Week 4+: Polish (items 17-24)**
- Theme CLI flag, responsive layout, scrollbar component
- Plugin system, aliases
- Coverage push toward 70%

### 7.3 Parallelization Opportunities

The following workstreams can proceed in parallel:

| Stream A (Bounds Safety) | Stream B (Scroll/Search) | Stream C (Testing) |
|--------------------------|--------------------------|---------------------|
| 1.10.1 SortCol clamp | 1.10.2 LogView search | 1.10.4 Fuzz tests |
| 1.10.3 Slice guards | 1.3.4 DiffView scrollable | fleet_builder tests |
| 1.10.5 Min-size guard | 1.3.4 Timeline scrollable | keymap tests |
| 1.3.3 Signal handler | Async git diff cmd | gauge tests |

All three streams are independent -- they touch different files and different concerns. A three-developer team could complete weeks 1-3 simultaneously.

**ROADMAP items to mark DONE:**
- 1.3.1 -- ConfirmDialog component exists at `components/confirm.go`
- 1.3.2 -- Wired to stop/stopAll/stopSession in `handlers.go:582-612, 626-703`
- 2.4.1 -- `ViewFleet` exists in view stack at `app.go:36`
- 2.4.2 -- Aggregate session table in `views/fleet.go:242-311`
- 2.4.3 -- Live-update via `tickMsg` handler at `app.go:226-243`
- 2.4.4 -- Inline actions in `handlers.go:473-501` and `fleet_builder.go:301-411`
- 2.4.5 -- Fleet summary bar at `views/fleet.go:78-93`

---

*Sources consulted:*
- [Charmbracelet BubbleTea](https://github.com/charmbracelet/bubbletea) -- Elm-architecture TUI framework
- [Charmbracelet Bubbles](https://github.com/charmbracelet/bubbles) -- Display/input component library (table, list, viewport, progress, spinner, textinput)
- [Charmbracelet Lipgloss](https://github.com/charmbracelet/lipgloss) -- Terminal styling; v2 compositing API analysis
- [NimbleMarkets/ntcharts](https://github.com/NimbleMarkets/ntcharts) -- Terminal charts (sparkline, bar, heatmap, line, streaming, time series, candlestick, waveline, scatter)
- [phiat/claude-esp](https://github.com/phiat/claude-esp) -- Go BubbleTea TUI for Claude Code session monitoring via JSONL
- [k9s skins documentation](https://k9scli.io/topics/skins/) -- YAML theme system with hierarchical style structure, env/context/global priority, live file watching
- [lazygit themes](https://github.com/jesseduffield/lazygit/blob/master/docs/Config.md) -- Config-based theme with `selectedLineBgColor`, file merging
- [Lazygit: 5 Years Retrospective](https://jesseduffield.com/Lazygit-5-Years-On/) -- Spatial consistency, undo stack, discoverability
- [Ghostty theme system](https://ghostty.org/docs/config) -- `ghostty +list-themes`, `~/.config/ghostty/themes/`, system light/dark switching
- [Catppuccin theme specification](https://github.com/catppuccin/catppuccin) -- 4 flavors, 26 colors, 300+ ports
- [Tokyo Night theme](https://github.com/enkia/tokyo-night-vscode-theme) -- Storm/Night/Day variants
- [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) -- RPC-based plugin system (alternative to Go .so plugins)
- [BubbleTea accessibility discussion](https://github.com/charmbracelet/bubbletea/issues/780) -- Screen reader support, ARIA-like attributes
- [BubbleTea State Machine Pattern](https://zackproser.com/blog/bubbletea-state-machine)
- [TUI Frameworks: BubbleTea vs Ratatui](https://www.glukhov.org/post/2026/02/tui-frameworks-bubbletea-go-vs-ratatui-rust/)
- [Build TUI Apps with Go and BubbleTea (2026)](https://dasroot.net/posts/2026/03/build-tui-apps-go-bubbletea/)
- [awesome-tuis](https://github.com/rothgar/awesome-tuis) -- TUI project catalog
