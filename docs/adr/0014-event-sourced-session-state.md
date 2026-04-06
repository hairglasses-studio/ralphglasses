# ADR 0014: Event-Sourced Session State

## Status

Accepted

## Context

Current session state in `internal/session/types.go` is mutable: fields on `Session` are updated in-place by the runner, stream parser, and supervisor. While this is simple and fast, it has several drawbacks:

- **No audit trail** -- once a field is overwritten, the previous value is lost. Debugging "what happened at turn 7?" requires correlating log files, replay recordings, and cost history arrays separately.
- **No replay** -- the existing `Recorder` writes JSONL to disk for post-hoc viewing, but there is no in-memory event log that can be replayed to reconstruct session state at an arbitrary point.
- **No forking** -- session forking (already tracked via `ParentID` / `ChildIDs` / `ForkPoint`) currently duplicates mutable state. A shared, immutable event prefix would make forks cheaper and more correct.
- **12-Factor Agents Factor 5** -- the 12-factor-agents framework recommends unifying execution state and business state via append-only event logs. This positions ralphglasses for horizontal scaling where multiple supervisors can consume the same event stream.

## Decision

Introduce an append-only `SessionEventLog` alongside the existing mutable `Session` state. This is a phased approach:

- **Phase 1 (this ADR):** Add the `SessionEventLog` type, `SessionEvent` struct, and `SessionEventType` enum. The log lives in-memory, bounded by a configurable `maxSize` (default 10,000 events). When the limit is exceeded, the oldest events are dropped (ring-buffer semantics). This phase does not modify existing code paths -- the log is additive.
- **Phase 2:** Wire the event log into the runner and supervisor so that state mutations also append events. Derive read-only state projections from the log for debugging and the TUI.
- **Phase 3:** Add replay (reconstruct `Session` state from events) and fork (create a new log sharing a prefix with the parent).

## Consequences

**Positive:**

- Every state transition is recorded with a timestamp, enabling post-hoc debugging without external log correlation
- `FormatForContext` provides a structured XML summary that can be injected into LLM context windows for self-aware agents
- `Fork` enables cheap, correct session forking by sharing an immutable event prefix
- Ring-buffer bounding prevents unbounded memory growth for long-running sessions
- Thread-safe design (`sync.RWMutex`) matches the existing session concurrency model

**Negative:**

- Increased memory usage per session (mitigated by the ring buffer with configurable `maxSize`)
- Dual state (mutable `Session` + append-only log) during Phase 1-2 transition creates a consistency risk
- Additional allocations on the hot path when events are appended every turn

**Mitigations:**

- Default `maxSize` of 10,000 keeps memory bounded (~2-4 MB per session assuming ~200-400 bytes per event)
- Phase 2 will add consistency checks between mutable state and event log projections
- `Events()` and filter methods return copies to prevent callers from mutating the log
