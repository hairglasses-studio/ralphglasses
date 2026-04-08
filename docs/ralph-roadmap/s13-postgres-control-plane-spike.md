# S13: PostgreSQL Control-Plane Research Spike

Date: 2026-04-08  
Scope: Optional PostgreSQL backend for distributed coordinator state in `ralphglasses`

## Summary

`ralphglasses` should keep SQLite as the default local-first persistence layer and treat
PostgreSQL as an optional backend for distributed coordinator state only.

The current repo and prior research already point in that direction:

- SQLite is the intended embedded default for sessions, loops, checkpoints, and local
  control-plane state.
- Shared SQLite in WAL mode is acceptable for one machine with multiple processes, but it
  is not a correct cross-host coordination primitive.
- Multi-node fleet work in Phase 10.5.6 needs coordinator-grade concurrency, leader/lease
  semantics, and change signaling that are awkward to bolt onto shared SQLite files.

Recommendation:

1. Stabilize the current SQLite path first.
2. Add a coordinator-scoped PostgreSQL spike behind the existing store boundary.
3. Do not plan a big-bang replacement of all SQLite usage.

## 1. Current State In Repo

### Existing storage shape

- [`internal/session/store.go`](../../internal/session/store.go)
  already defines a store interface, which is the correct seam for future backend work.
- [`internal/session/store_sqlite.go`](../../internal/session/store_sqlite.go)
  is the main persistent store for sessions, loop runs, cost ledger, recovery ops, and
  tenants.
- [`internal/session/shared_state.go`](../../internal/session/shared_state.go)
  already shows the safer SQLite DSN pattern: WAL + `busy_timeout` + `SetMaxOpenConns(1)`.
- [`internal/marathon/checkpoint_sqlite.go`](../../internal/marathon/checkpoint_sqlite.go)
  uses SQLite for local checkpoint persistence.
- [`docs/ARCHITECTURE.md`](../ARCHITECTURE.md)
  describes fleet distribution with a coordinator/worker model, which is the part most
  likely to outgrow shared-file coordination.

### Current pressure points

- The live runtime logs show repeated `SQLITE_BUSY` writes and bootstrap fallback to
  in-memory state.
- The repo's own research already warns that shared SQLite is appropriate for local
  multi-process coordination, but not for cross-machine coordination.
- The roadmap's Phase 10.5.6 "Multi-Node Marathon Distribution" explicitly introduces
  cross-node supervisor state sync and leader election.

### Immediate conclusion

The storage problem is currently two different problems:

1. SQLite is not yet hardened enough for the repo's local multi-process case.
2. SQLite is the wrong sole primitive for a future multi-host control plane.

Those should not be solved by the same migration.

## 2. External Constraints

### SQLite constraints

- SQLite's WAL documentation says WAL allows readers and writers to proceed concurrently,
  but there is still only one writer at a time.
- The same documentation also states that all processes using a WAL database must be on
  the same host and that WAL does not work over a network filesystem.
- SQLite's application-file-format guidance still makes it the right embedded default for
  local state because it is zero-ops, single-file, and easy to ship inside a CLI/TUI.

Sources:

- SQLite WAL: <https://sqlite.org/wal.html>
- SQLite application file format: <https://sqlite.org/appfileformat.html>
- SQLite `busy_timeout` pragma reference:
  <https://system.data.sqlite.org/home/doc/914417fc18aae0fb/Doc/Extra/Core/pragma.html>

### PostgreSQL capabilities relevant to ralphglasses

- PostgreSQL explicit locking provides advisory locks with application-defined meaning,
  which fit leader election, work leasing, and coordinator fencing better than file locks.
- PostgreSQL `NOTIFY` / `LISTEN` provides built-in interprocess signaling, which is a
  better match for coordinator-to-worker change notification than file polling.
- PostgreSQL's concurrency model is designed for multi-process access to shared state in a
  way SQLite is not when hosts are involved.

Sources:

- PostgreSQL explicit locking / advisory locks:
  <https://www.postgresql.org/docs/current/explicit-locking.html>
- PostgreSQL `NOTIFY` / `LISTEN`:
  <https://www.postgresql.org/docs/current/sql-notify.html>

## 3. Candidate Architectures

### Option A: SQLite only

Keep SQLite everywhere and try to extend it into multi-node coordination.

Assessment:

- Good for: single machine, local TUI, local MCP server, per-node checkpointing.
- Bad for: coordinator HA, shared leases across hosts, network filesystem deployment,
  remote workers, control-plane signaling.

Recommendation: reject as the long-term multi-node answer.

### Option B: Full PostgreSQL replacement

Replace all current SQLite persistence with PostgreSQL.

Assessment:

- Good for: one database story, unified concurrency semantics, shared fleet state.
- Bad for: local-first UX, embedded deployment, zero-config startup, checkpoint/replay
  artifacts, one-machine installs, migration risk.

Recommendation: reject for now. Too much churn for too little immediate value.

### Option C: Split backend model

Keep SQLite for local artifacts and add PostgreSQL only for shared control-plane state.

Assessment:

- Good for: preserving local-first ergonomics while giving multi-node coordinator logic a
  real shared database.
- Good for: incremental migration behind the existing store interface.
- Good for: aligning with Phase 10.5.6 without rewriting replay logs, observations, and
  local checkpoint stores.

Recommendation: preferred direction.

## 4. Data Ownership Split

### Keep local SQLite

These are naturally local to a repo, node, or operator workstation:

- observations and local analytics snapshots
- replay logs and session output archives
- marathon checkpoints
- per-node caches, prompt caches, and scratch artifacts
- offline research/knowledge stores already embedded in SQLite

### Candidate shared control-plane state for PostgreSQL

These are coordinator-managed and likely to need cross-host correctness:

- tenants
- sessions and session leases
- loop runs and loop iteration metadata
- cost ledger summaries used for shared budget enforcement
- recovery operations and recovery actions
- fleet queue, work assignment, and worker lease records
- leader-election, coordinator fencing, and worker heartbeats

### Non-goal

Do not move append-heavy or repo-local artifacts into PostgreSQL unless a concrete
cross-host query or consistency need exists.

## 5. Recommended Migration Plan

### Phase 0: Harden SQLite first

Before any PostgreSQL work:

- align [`internal/session/store_sqlite.go`](../../internal/session/store_sqlite.go)
  with the safer DSN pattern already used in
  [`internal/session/shared_state.go`](../../internal/session/shared_state.go)
- add `busy_timeout`, `SetMaxOpenConns(1)`, and lock-contention tests
- remove fallback-to-memory behavior for normal lock contention paths where retry is
  viable

Reason: this fixes the current single-host failure mode without committing to a backend
swap.

### Phase 1: Separate local state from shared control-plane state

- keep the current `Store` interface as the primary seam for sessions/loops/ledger
- add a coordinator-specific persistence boundary for:
  - queue items
  - worker heartbeats
  - leader/lease state
  - shared recovery orchestration
- make explicit which APIs require durable cross-host visibility and which do not

Reason: a good migration does not start by writing SQL for everything.

### Phase 2: Add optional PostgreSQL adapter

- use direct SQL, not an ORM
- prefer `pgx/v5` for the PostgreSQL adapter
- implement an opt-in coordinator backend, not a mandatory global backend
- gate it behind explicit config such as:
  - `RALPH_CONTROL_PLANE_BACKEND=postgres`
  - `RALPH_DATABASE_URL=postgres://...`

Reason: this keeps local installs unchanged while enabling a real control plane for
distributed fleets.

### Phase 3: Shadow mode and cutover

- dual-write coordinator state to SQLite and PostgreSQL in staging
- compare row counts, lease decisions, and queue transitions
- cut reads over to PostgreSQL only after shadow parity is stable
- keep rollback simple: coordinator reads can fall back to SQLite until confidence is
  earned

Reason: the repo's own research already warns against big-bang backend migrations.

## 6. Coordination Primitives To Prototype

### Advisory locks

Use PostgreSQL advisory locks for:

- leader election
- repo-level lease ownership
- queue item claim/reclaim
- migration fencing

This is a better fit than shared file locks once workers span hosts.

### LISTEN / NOTIFY

Use `LISTEN` / `NOTIFY` for:

- queue wakeups
- worker lease updates
- session/loop status changes relevant to coordinators
- config change broadcasts

Still keep polling as a fallback path for simplicity and reconnect recovery.

### Row-backed leases

Use normal tables for the source of truth:

- workers
- queue items
- lease expirations
- coordinator generation / fencing token

Notifications should only hint that something changed; tables remain authoritative.

## 7. Benchmark Plan

The spike should produce benchmark data for these scenarios:

1. Single coordinator, 50 concurrent session updates/sec.
2. Three coordinators competing for advisory-lock leadership.
3. 200 queued work items with workers claiming and renewing leases.
4. Burst notification fan-out for 100 queue wakeups.
5. Recovery after coordinator crash with leased work reassignment.

Success targets:

- lease acquisition p95 under 50 ms
- queue claim + state transition p95 under 100 ms
- no duplicate work claims under forced failover
- coordinator recovery without manual DB repair
- local single-node mode remains SQLite and requires no Postgres service

## 8. Operational Cost And Risk

### Benefits of optional PostgreSQL

- better concurrency for shared coordinator state
- proper cross-host durability
- advisory locks and notifications remove custom lock/signaling glue
- easier future HA coordinator work in Phase 10.5.6 and beyond

### Costs

- external service dependency
- credentials and bootstrap complexity
- more operational burden for users who only want local fleet management
- split-brain risk if local and shared state boundaries are not explicit

### Main migration risks

- over-migrating local artifacts into the shared database
- coupling the TUI startup path to network DB availability
- introducing a mandatory database dependency before distributed deployment exists

## 9. Recommendation

Recommended roadmap outcome:

1. Keep SQLite as the default and first-class local store.
2. Add a PostgreSQL spike only for distributed coordinator state.
3. Make any future PostgreSQL backend optional and coordinator-scoped.
4. Defer implementation until SQLite lock handling is fixed and the control-plane state
   boundary is explicit.

This gives `ralphglasses` a credible path to a real distributed control plane without
breaking the embedded, local-first model that the current product still needs.
