# ADR 004: Blackboard Pattern for Inter-Session State

## Status

Accepted

## Context

Ralphglasses orchestrates multiple concurrent LLM sessions that need to share state -- cost observations, task progress, quality signals, and coordination data. We evaluated two approaches:

1. **Message passing** -- Sessions communicate via typed messages on an event bus. This provides strong decoupling but requires defining message schemas for every interaction and makes it hard to query "current state" without replaying history.

2. **Blackboard pattern** -- A shared key-value store where any subsystem can write observations and any other can read them. This is simpler for the "publish observations, read when needed" pattern that dominates our use cases.

We already have an event bus (`internal/events.Bus`) for real-time notifications. The question was whether inter-session state coordination should also flow through events or use a complementary mechanism.

## Decision

We implemented the blackboard pattern at two levels:

### Session-level blackboard (`internal/session/blackboard.go`)

- Simple key-value store for intra-manager subsystem communication
- `Put(key, value, source)` / `Get(key)` / `Snapshot()` API
- Source tracking identifies which subsystem wrote each entry
- JSON file persistence in the session's `.ralph/` state directory
- Used by the self-learning subsystem to share observations between autonomy, cost tracking, loop benchmarking, and HITL scoring

### Fleet-level blackboard (`internal/blackboard/blackboard.go`)

- Namespaced key-value store for cross-session coordination
- Entries have `Namespace`, `Key`, `Value`, `WriterID`, `Version`, and `TTL` fields
- Optimistic concurrency via CAS (compare-and-swap) on monotonic version numbers -- `ErrVersionConflict` on mismatch
- Watcher notifications (`Watch(fn)`) for reactive updates
- TTL-based garbage collection (`GC()`) for ephemeral data
- JSONL append-only persistence with periodic compaction
- Exposed via MCP tools: `ralphglasses_blackboard_put` and `ralphglasses_blackboard_query`

Both blackboards are wired into the `Server` struct (`internal/mcpserver/tools.go`) and accessible to fleet handlers (`internal/mcpserver/handler_fleet_h.go`).

## Consequences

**Positive:**

- Subsystems are decoupled: writers and readers share only key conventions, not interfaces
- CAS versioning prevents lost updates in concurrent fleet scenarios
- TTL + GC keeps the blackboard from growing unboundedly
- Complements the event bus: events for real-time notifications, blackboard for queryable state
- JSONL persistence survives process restarts without a database dependency

**Negative:**

- No schema enforcement on values -- keys and value shapes are conventions, not contracts
- Two blackboard implementations (session-level and fleet-level) could cause confusion
- Watcher notifications are best-effort; slow watchers could delay Put returns

**Mitigations:**

- Key naming conventions are documented in handler code
- The fleet-level blackboard's namespace field provides logical partitioning
- Watchers are called outside the lock to avoid holding up concurrent operations
