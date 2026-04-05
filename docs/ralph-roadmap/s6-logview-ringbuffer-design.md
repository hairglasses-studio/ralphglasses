# S6 LogView Ring Buffer Design — ralphglasses

**Date**: 2026-04-04
**Author**: Claude Sonnet 4.6 (automated design)
**Scope**: `internal/tui/views/logstream.go`, `internal/process/logstream.go`, `internal/tui/app_update.go`
**Status**: Design — ready for implementation

---

## 1. Problem: Unbounded `Lines []string`

### Where it lives

`internal/tui/views/logstream.go`, lines 13–18:

```go
type LogView struct {
    vp     viewport.Model
    Lines  []string   // grows without bound
    Follow bool
    Search string
    Width  int
    Height int
}
```

### How lines enter

Two distinct ingestion paths both call into `LogView`:

| Path | Source | Handler in `app_update.go` | LogView method |
|------|--------|---------------------------|----------------|
| Initial load | `loadLogCmd` → `process.ReadFullLog` | `LogLoadedMsg` (line 198–204) | `SetLines` |
| Tail polling | `process.TailLog` (every 2-second tick) | `process.LogLinesMsg` (line 206–209) | `AppendLines` |
| Session stream | `streamSessionOutput` | `SessionOutputMsg` (line 229–233) | `AppendLines` on `Stream.OutputView` |

`AppendLines` (logstream.go line 45–48) is the hot path:

```go
func (lv *LogView) AppendLines(lines []string) {
    lv.Lines = append(lv.Lines, lines...)  // no cap check, grows forever
    lv.rebuildContent()
}
```

After every `AppendLines`, `rebuildContent` (line 56–72) iterates the full `Lines` slice, builds one large string, and hands it to `viewport.SetContent`. That means every render is O(N) in the total number of lines accumulated since the view was opened.

### Growth rate estimate

A busy Ralph loop emitting ~10 lines/second (JSON output, tool calls) produces:
- 600 lines/min
- 36K lines/hour
- ~432K lines in a 12-hour session

At ~200 bytes average per line, that is ~86 MB of retained strings — plus the equivalent-sized rendered content string the viewport holds. Both grow simultaneously. After a few hours of heavy use `LogView` can easily hold hundreds of megabytes.

---

## 2. No Existing Ring Buffer

A codebase-wide search for `ring`, `circular`, `RingBuffer`, `ringbuf` found no matching implementations in production Go files. The term appears only in non-TUI packages as part of unrelated patterns (circuit breaker, routing). There is nothing to reuse; the ring buffer must be written fresh.

---

## 3. Design: `lineRing` — a Fixed-Capacity String Ring Buffer

### 3.1 Implementation sketch

```go
// lineRing is a fixed-capacity circular buffer of strings.
// When full, new writes overwrite the oldest entries.
// All methods are not safe for concurrent use; callers must hold LogView's lock
// (or rely on the single-goroutine Bubble Tea model).
type lineRing struct {
    buf  []string
    cap  int
    head int // index of the oldest entry
    size int // number of valid entries (0..cap)
}

// newLineRing allocates a ring with the given capacity.
// Panics if cap < 1.
func newLineRing(cap int) *lineRing {
    if cap < 1 {
        panic("lineRing: capacity must be >= 1")
    }
    return &lineRing{buf: make([]string, cap), cap: cap}
}

// push adds a line. When full, the oldest line is silently dropped.
func (r *lineRing) push(line string) {
    if r.size < r.cap {
        r.buf[(r.head+r.size)%r.cap] = line
        r.size++
    } else {
        // Overwrite the oldest slot and advance head.
        r.buf[r.head] = line
        r.head = (r.head + 1) % r.cap
    }
}

// pushAll appends a batch of lines efficiently.
func (r *lineRing) pushAll(lines []string) {
    for _, l := range lines {
        r.push(l)
    }
}

// reset clears all entries without reallocating the backing slice.
func (r *lineRing) reset() {
    r.head = 0
    r.size = 0
}

// len returns the number of valid entries.
func (r *lineRing) len() int { return r.size }

// isFull reports whether the buffer has reached capacity.
func (r *lineRing) isFull() bool { return r.size == r.cap }

// get returns the i-th oldest entry (0 = oldest, len-1 = newest).
// Panics if i is out of range.
func (r *lineRing) get(i int) string {
    if i < 0 || i >= r.size {
        panic("lineRing: index out of range")
    }
    return r.buf[(r.head+i)%r.cap]
}

// slice returns all valid entries in order from oldest to newest.
// The returned slice is a copy — safe to hold across mutations.
func (r *lineRing) slice() []string {
    out := make([]string, r.size)
    for i := range out {
        out[i] = r.buf[(r.head+i)%r.cap]
    }
    return out
}
```

All operations are O(1) except `pushAll` (O(k) for k new lines) and `slice` (O(N) for N stored lines). Memory is allocated once at construction.

### 3.2 Capacity and configurability

Default capacity: **10,000 lines**.

This should be configurable. The natural hook is `NewLogView`. Add an optional constructor and a package-level default:

```go
const DefaultLogCapacity = 10_000

// NewLogView creates a log view with the default line capacity.
func NewLogView() *LogView {
    return NewLogViewWithCapacity(DefaultLogCapacity)
}

// NewLogViewWithCapacity creates a log view with a custom line capacity.
// A capacity of 0 uses DefaultLogCapacity.
func NewLogViewWithCapacity(cap int) *LogView {
    if cap <= 0 {
        cap = DefaultLogCapacity
    }
    vp := viewport.New()
    vp.KeyMap = viewport.KeyMap{}
    return &LogView{
        vp:     vp,
        ring:   newLineRing(cap),
        Follow: true,
    }
}
```

Exposing capacity through ralphglasses config (`.ralph/config.yaml`) is a logical follow-on. For now, callers that need a different size (e.g. `Stream.OutputView` for session streaming) use `NewLogViewWithCapacity`.

---

## 4. Integration with Existing View Rendering

### 4.1 `LogView` struct change

Replace `Lines []string` with `ring *lineRing`:

```go
type LogView struct {
    vp     viewport.Model
    ring   *lineRing  // was: Lines []string
    Follow bool
    Search string
    Width  int
    Height int
}
```

`Lines` is a public field accessed in tests. The cleanest migration is:

1. Make `ring` the source of truth.
2. Add a read-only `Lines()` method for tests and any external callers.
3. Update tests to use the method.

```go
// Lines returns a snapshot of all stored lines, oldest first.
// Callers that previously read lv.Lines directly should use this instead.
func (lv *LogView) Lines() []string {
    return lv.ring.slice()
}

// Len returns the number of stored lines.
func (lv *LogView) Len() int {
    return lv.ring.len()
}
```

### 4.2 `AppendLines` and `SetLines`

```go
func (lv *LogView) AppendLines(lines []string) {
    lv.ring.pushAll(lines)
    lv.rebuildContent()
}

func (lv *LogView) SetLines(lines []string) {
    lv.ring.reset()
    lv.ring.pushAll(lines)
    lv.rebuildContent()
}
```

`SetLines` is called from `LogLoadedMsg` with the full log file content read by `process.ReadFullLog`. If the file has more than `cap` lines, only the most recent `cap` will be stored (the `pushAll` loop naturally evicts from the front). This is the correct behavior — users want to see recent output.

### 4.3 `filteredLines` and `rebuildContent`

`filteredLines` currently ranges over `lv.Lines`. After the change it ranges over `lv.ring.slice()`:

```go
func (lv *LogView) filteredLines() []string {
    all := lv.ring.slice()
    if lv.Search == "" {
        return all
    }
    needle := strings.ToLower(lv.Search)
    var filtered []string
    for _, line := range all {
        if strings.Contains(strings.ToLower(line), needle) {
            filtered = append(filtered, line)
        }
    }
    return filtered
}
```

`rebuildContent` is unchanged structurally — it calls `filteredLines()` already.

### 4.4 Header display

The `View()` method currently shows `len(lines)` (line 169):

```go
header := fmt.Sprintf("  Lines: %d  Scroll: %.0f%%  %s",
    len(lines), ...)
```

After the ring buffer change, `lines` is `lv.filteredLines()` — this already reflects the bounded count. No header change needed. Optionally add a "cap" indicator when the ring is full:

```go
lineCount := lv.ring.len()
capSuffix := ""
if lv.ring.isFull() {
    capSuffix = fmt.Sprintf("/%d (capped)", lv.ring.cap)
}
header := fmt.Sprintf("  Lines: %d%s  Scroll: %.0f%%  %s",
    lineCount, capSuffix, lv.vp.ScrollPercent()*100, followIndicator)
```

This tells the user that older lines have been dropped.

---

## 5. Scroll Position Handling

The current scroll semantics are already correct and survive the ring buffer change without modification.

### Follow mode (`lv.Follow == true`)

`rebuildContent` calls `lv.vp.GotoBottom()` when `Follow` is true (logstream.go line 69–71). When new lines arrive and are pushed into the ring, `rebuildContent` is called, and the viewport jumps to the new bottom. This behavior is unchanged.

### User scrolled up (`lv.Follow == false`)

`ScrollUp()` sets `Follow = false`. The viewport position is managed by `viewport.Model` internally as a pixel/line offset into the rendered content string. When new lines arrive, `rebuildContent` calls `SetContent` with the new string. The `viewport.Model` from charmbracelet preserves the absolute offset, which means:

- If the user is scrolled up by N pixels, new lines appended at the bottom do not move the viewport.
- **When the ring wraps and oldest lines are dropped**, the content string shrinks by the number of evicted lines. This shifts the user's position by exactly one rendered line per evicted line, which causes drift.

### Handling wrap-induced drift

When `Follow` is false and new lines cause a wrap (eviction), the viewport needs to be nudged up by the number of evicted lines to maintain the user's apparent position.

Track evictions in `lineRing`:

```go
type lineRing struct {
    buf      []string
    cap      int
    head     int
    size     int
    evicted  int // total lines dropped since last reset
}

func (r *lineRing) push(line string) {
    if r.size < r.cap {
        r.buf[(r.head+r.size)%r.cap] = line
        r.size++
    } else {
        r.buf[r.head] = line
        r.head = (r.head + 1) % r.cap
        r.evicted++
    }
}
```

In `AppendLines`, after `ring.pushAll`, if the user is not following and evictions occurred:

```go
func (lv *LogView) AppendLines(lines []string) {
    before := lv.ring.evicted
    lv.ring.pushAll(lines)
    evictedThisBatch := lv.ring.evicted - before
    lv.rebuildContent()
    if !lv.Follow && evictedThisBatch > 0 {
        // Scroll up by evictedThisBatch lines to compensate for dropped content.
        // This prevents the viewport from sliding forward into new content
        // when the user has intentionally scrolled back.
        lv.vp.ScrollUp(evictedThisBatch)
    }
}
```

This is a best-effort compensation. At high eviction rates (e.g. 1,000 lines/second) the adjustment may over-correct slightly, but for typical log rates (10–50 lines/second) the correction is precise.

An alternative simpler approach: when the ring first becomes full and the user is not following, show a fixed notification rather than adjusting the offset. This trades position accuracy for simplicity. The scroll-compensation approach above is preferred.

---

## 6. Memory Impact

### Before (unbounded)

| Scenario | Lines accumulated | String memory | Viewport content string |
|----------|------------------|---------------|------------------------|
| 1 hour at 10 lines/sec | 36,000 | ~7 MB | ~7 MB |
| 8 hours at 10 lines/sec | 288,000 | ~58 MB | ~58 MB |
| 8 hours at 50 lines/sec | 1,440,000 | ~288 MB | ~288 MB |
| 12 hours at 10 lines/sec | 432,000 | ~86 MB | ~86 MB |

Both the `[]string` slice and the viewport content string grow together. After ~12 hours the TUI can hold 150–600 MB just from one log view. Sessions open multiple `LogView` instances (`LogView` for the global log and `Stream.OutputView` per session).

### After (ring buffer, 10K cap)

| Component | Size |
|-----------|------|
| `lineRing.buf` backing array | 10,000 × ~200 bytes = ~2 MB |
| `[]string` slice headers | 10,000 × 16 bytes = 160 KB |
| Viewport content string (worst case, all filtered) | ~2 MB |
| Total per LogView instance | ~4 MB |

**Upper bound is constant and independent of session duration.**

For 5 concurrent session output views (a typical fleet scenario), worst-case memory from log views is ~20 MB — down from potentially gigabytes in a long sweep run.

### Calculation basis

- Average log line: ~200 bytes (timestamp + level + message)
- Go string header: 16 bytes (pointer + length)
- Viewport content string: approximately 1× the stored lines text (colorize adds ANSI escapes, roughly +20%)
- Buffer allocated once at construction; no GC pressure from append growth

---

## 7. Test Updates

The existing tests in `logstream_test.go` access `lv.Lines` directly:

```go
// logstream_test.go line 19
if len(lv.Lines) != 2 {
```

These must be updated to use the new `Lines()` method:

```go
if len(lv.Lines()) != 2 {
```

New tests to add in the same file:

```go
func TestLineRing_Capacity(t *testing.T) {
    r := newLineRing(3)
    r.push("a"); r.push("b"); r.push("c")
    if !r.isFull() {
        t.Fatal("ring should be full after 3 pushes into cap-3 ring")
    }
    r.push("d") // evicts "a"
    got := r.slice()
    want := []string{"b", "c", "d"}
    if !reflect.DeepEqual(got, want) {
        t.Errorf("got %v, want %v", got, want)
    }
}

func TestLogView_BoundedByCapacity(t *testing.T) {
    lv := NewLogViewWithCapacity(5)
    lv.SetDimensions(80, 20)
    for i := 0; i < 20; i++ {
        lv.AppendLines([]string{fmt.Sprintf("line %d", i)})
    }
    if lv.Len() != 5 {
        t.Errorf("expected 5 lines (capped), got %d", lv.Len())
    }
    lines := lv.Lines()
    if lines[0] != "line 15" {
        t.Errorf("expected oldest retained line to be 'line 15', got %q", lines[0])
    }
}

func TestLogView_SetLines_Truncates(t *testing.T) {
    lv := NewLogViewWithCapacity(3)
    lv.SetDimensions(80, 10)
    lv.SetLines([]string{"a", "b", "c", "d", "e"})
    if lv.Len() != 3 {
        t.Errorf("SetLines with more than cap should truncate, got %d", lv.Len())
    }
    lines := lv.Lines()
    if lines[0] != "c" || lines[2] != "e" {
        t.Errorf("expected [c d e], got %v", lines)
    }
}

func TestLogView_EvictionCount(t *testing.T) {
    r := newLineRing(3)
    r.push("a"); r.push("b"); r.push("c"); r.push("d")
    if r.evicted != 1 {
        t.Errorf("expected 1 eviction, got %d", r.evicted)
    }
}
```

---

## 8. Files to Change

| File | Change |
|------|--------|
| `internal/tui/views/logstream.go` | Replace `Lines []string` with `ring *lineRing`; add `Lines()`, `Len()`, `NewLogViewWithCapacity`; update `AppendLines`, `SetLines`, `filteredLines`, `View` |
| `internal/tui/views/logstream.go` | Add `lineRing` type and methods (can live in same file or a new `internal/tui/views/ringbuffer.go`) |
| `internal/tui/views/logstream_test.go` | Update `lv.Lines` field accesses to `lv.Lines()` method calls |
| `internal/tui/views/logstream_test.go` | Add `TestLineRing_*` and `TestLogView_BoundedByCapacity` tests |
| `internal/tui/views/logstream_extra_test.go` | No changes needed (only tests scroll behavior, not Lines length) |
| `internal/tui/app_init.go` | No changes needed (no direct `lv.Lines` access) |
| `internal/tui/app_update.go` | No changes needed (calls `AppendLines`/`SetLines` only) |

---

## 9. Estimated Effort

| Task | Effort |
|------|--------|
| Implement `lineRing` + unit tests | 1–2 hours |
| Update `LogView` struct, constructors, and methods | 1 hour |
| Update existing tests (field → method calls) | 30 minutes |
| Add new ring-specific tests | 1 hour |
| Verify build + race detector clean | 30 minutes |
| **Total** | **4–5 hours** |

This is a contained, low-risk refactor. The ring buffer is self-contained with no external dependencies. The `viewport.Model` API is unchanged — the ring only affects what string is handed to `SetContent`. The scroll-compensation heuristic for eviction-while-scrolled is the only non-trivial behavioral decision, and it degrades gracefully (worst case: viewport drifts slightly during a burst; the user can press `G` to re-anchor).

---

## 10. Deferred Decisions

1. **Config integration**: expose `log_capacity` in `.ralph/config.yaml` so power users can raise or lower the cap. Low priority — 10K is a reasonable default for all practical use cases.
2. **Per-session vs shared cap**: `Stream.OutputView` instances live for the duration of one session output stream and are typically short-lived. They could use a smaller cap (e.g. 2K) to reduce memory further.
3. **`LogSearchModel`**: `internal/tui/views/logstream_search.go` has its own `[]LogLine lines` field, which has the same unbounded-growth risk when populated from a full log. That model is a separate tea.Model used from a different code path and should be addressed in a follow-on task (S7 or equivalent).
