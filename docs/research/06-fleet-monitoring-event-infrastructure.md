# Phase 6 Research: Fleet Monitoring & Event Infrastructure

Covers ROADMAP Phase 2.75 (complete), items 6.4 (Analytics & Observability), 6.5 (External Notifications), and 6.7 (Replay/Audit Trail).

---

## 1. Executive Summary

- **Phase 2.75 event bus is production-ready** (95.1% test coverage, ring buffer, cursor-based polling, 12 event types) and serves as the foundation for all three target items.
- **6.4 (Analytics)** requires a SQLite persistence layer, OpenTelemetry integration, and a Prometheus `/metrics` endpoint; the in-memory `fleet_analytics` MCP handler already computes per-provider/repo stats but has no historical storage.
- **6.5 (External Notifications)** needs a webhook dispatcher, Discord/Slack formatters, rate-limiting, and templates; the existing `internal/notify/` package (39 lines) handles only desktop OS notifications and has no retry, dedup, or HTTP capability.
- **6.7 (Replay/Audit Trail)** is blocked by 6.4; it requires session recording at the tool-call level, a TUI replay viewer, Markdown/JSON export, and retention policies -- none of which exist today.
- **Immediate high-value wins**: (1) add SQLite event sink to the bus (unlocks 6.4 and 6.7), (2) build webhook dispatcher with backoff/dedup (6.5.1 + 6.5.5), (3) wire Prometheus counters into the existing 2.11 HTTP stub.

---

## 2. Current State Analysis

### 2.1 What Exists

| File | Lines | Tests | Coverage | Status |
|------|------:|-------|----------|--------|
| `internal/events/bus.go` | 192 | `bus_test.go` (219 lines, 9 tests) | 95.1% | Solid; ring buffer + cursor API |
| `internal/hooks/hooks.go` | 147 | `hooks_test.go` (101 lines, 4 tests) | 82.7% | Working; sync/async dispatch, YAML config |
| `internal/notify/notify.go` | 39 | none | 0% | Minimal; OS-only, no dedup/retry |
| `internal/session/manager.go` | 846 | `manager_test.go` | 64.5% (package) | Publishes 3 event types (SessionStarted, TeamCreated, session persistence) |
| `internal/session/runner.go` | 380 | `runner_test.go` | (included in session 64.5%) | Publishes SessionEnded, CostUpdate, BudgetExceeded, JournalWritten |
| `internal/session/loop.go` | 870 | `loop_test.go` | (included in session 64.5%) | Publishes PromptEnhanced |
| `internal/session/budget.go` | 131 | `budget_test.go` | (included in session 64.5%) | BudgetEnforcer + LedgerEntry, no event-driven alerts |
| `internal/process/manager.go` | 356 | none directly | untested for events | Publishes LoopStarted, LoopStopped |
| `internal/mcpserver/tools.go` | 3992 | `tools_test.go` | partial | handleEventList, handleEventPoll, handleFleetAnalytics, handleSessionCompare, summarizeEvent |
| `internal/tui/components/notification.go` | 47 | `notification_test.go` (48 lines) | full | Toast-style TUI notifications |
| `internal/tui/fleet_builder.go` | 480 | none | untested | Desktop notify for critical alerts via `notify.Send()` |

### 2.2 What Works Well

- **Event bus design** (`internal/events/bus.go:50-57`): The `Bus` struct uses a simple, correct pattern -- `sync.RWMutex`, non-blocking publish with dropped overflow, ring buffer with configurable max history. The cursor-based `HistoryAfterCursor()` (line 149) enables efficient incremental polling (used by `event_poll` MCP tool).
- **Event type taxonomy** (`internal/events/bus.go:11-37`): 12 well-defined event types covering session lifecycle, cost, loops, teams, journal, config, scan, and prompt enhancement. Covers the full system lifecycle.
- **Hook system** (`internal/hooks/hooks.go:30-36`): The `Executor` cleanly subscribes to the bus and dispatches to per-repo hooks defined in YAML. Supports both sync (blocking) and async execution with configurable timeouts. Environment variables expose event context to shell commands.
- **Cursor-based polling** (`internal/mcpserver/tools.go:3816-3880`): The `handleEventPoll` handler returns a cursor string + `has_more` flag, enabling efficient mobile/remote polling without duplicate delivery.
- **Fleet analytics** (`internal/mcpserver/tools.go:2071-2130`): Per-provider and per-repo cost aggregation computed on the fly from in-memory sessions. Good foundation for persistent analytics.
- **Desktop notification integration** (`internal/tui/fleet_builder.go:234-241`): Critical alerts trigger `notify.Send()` asynchronously. Simple, low-overhead.

### 2.3 What Doesn't Work

- **No persistent event storage** -- all event history is lost on process restart. The ring buffer (1000 events max) is in-memory only. Blocks 6.4.1, 6.7.1.
- **No external notification channels** -- `internal/notify/` only handles OS desktop notifications. No HTTP webhooks, Discord, Slack, or email. Blocks all of 6.5.
- **No notification dedup/throttle** -- `fleet_builder.go:234-241` fires desktop notifications for every critical alert with no dedup window. Relates to ROADMAP 2.6.4.
- **No OpenTelemetry or Prometheus** -- no structured traces, no metrics endpoint. Blocks 6.4.3, 6.4.4.
- **No session recording at tool-call granularity** -- `runner.go` captures cost/turn/output but not individual tool calls or LLM responses. Blocks 6.7.1.
- **Hooks don't capture output or errors** -- `hooks.go:139` discards `cmd.Run()` error. No way to know if a hook failed or what it produced.
- **Notify package has no tests** -- 0% coverage on `internal/notify/notify.go`. AppleScript injection possible via `escapeOSA()` edge cases.
- **Session package coverage at 64.5%** -- event publishing paths in `runner.go` and `loop.go` are partially covered but manager event paths (Launch, LaunchTeam) lack targeted event assertion tests.

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap |
|-------------|--------|---------------|-----|
| 6.4.1 | SQLite historical data model | In-memory only (`events.Bus.history`) | No persistence layer |
| 6.4.2 | TUI analytics view | `fleet_builder.go` computes live stats | No historical time-series, no dedicated view |
| 6.4.3 | OpenTelemetry traces | None | No tracing infrastructure |
| 6.4.4 | Prometheus `/metrics` | 2.11.4 has `/metrics` stub planned | No HTTP server, no metrics |
| 6.4.5 | Grafana dashboard JSON | None | Depends on 6.4.4 |
| 6.5.1 | Webhook dispatcher | None | No HTTP POST capability |
| 6.5.2 | Discord integration | None | No Discord webhook formatting |
| 6.5.3 | Slack integration | None | No Slack blocks formatting |
| 6.5.4 | Notification templates | None | No template system |
| 6.5.5 | Rate limiting and retry | None | No dedup, no backoff |
| 6.7.1 | Session recording | `runner.go` captures aggregates only | No tool-call-level recording |
| 6.7.2 | Replay viewer | None | No TUI view |
| 6.7.3 | Export to Markdown/JSON | None | No export |
| 6.7.4 | Diff view (A/B) | `session_compare` MCP tool exists | Compare is live-only, no replay |
| 6.7.5 | Retention policy | `LoadExternalSessions()` cleans up 24h+ sessions | No configurable retention |

### 3.2 Missing Capabilities

From peer projects and industry patterns:

| Capability | Where It Exists | Gap in ralphglasses |
|-----------|----------------|---------------------|
| Structured event logging (JSONL) | ROADMAP 2.12 (telemetry), `shielddd` audit logs | No append-only event log file |
| Webhook with HMAC signing | Discord/GitHub webhooks | No request signing for security |
| Event filtering per subscriber | NATS, Redis Pub/Sub | Bus broadcasts all events to all subscribers |
| Backpressure / flow control | Go channel patterns | Overflow silently drops (by design, but no metrics on drops) |
| Event schema versioning | Protobuf, CloudEvents | No schema version field on `Event` struct |
| Distributed event bus | NATS, Redis Streams | In-process only (single binary) |
| Metric cardinality control | Prometheus best practices | N/A (no metrics yet) |

### 3.3 Technical Debt Inventory

| Location | Severity | Issue | Resolution |
|----------|----------|-------|------------|
| `hooks/hooks.go:139` | Medium | `_ = cmd.Run()` discards hook execution errors | Capture error, log via slog, publish `hook.failed` event |
| `notify/notify.go:13` | Low | String concatenation in AppleScript could break on edge cases | Use `fmt.Sprintf` with proper escaping or switch to `terminal-notifier` |
| `notify/notify.go` | Medium | No tests at all (0% coverage) | Add unit tests with mock exec |
| `events/bus.go:95-97` | Low | Dropped events on subscriber overflow are silent | Add `droppedCount` metric per subscriber |
| `events/bus.go:39-48` | Low | `Event.Data` is `map[string]any` -- no type safety | Consider typed event payloads or at minimum document expected keys |
| `fleet_builder.go:236-238` | Medium | Desktop notifications fire without dedup | Add throttle map: `event_type+session_id -> last_sent` |
| `mcpserver/tools.go:2071-2130` | Medium | `handleFleetAnalytics` recomputes from scratch each call | Cache or persist aggregates |
| `session/runner.go:210-220` | Low | Journal write is fire-and-forget with goroutine leak risk | Add timeout context or track goroutine |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

| Project | URL | Relevance | Key Pattern |
|---------|-----|-----------|-------------|
| **mcpkit** (sibling) | `github.com/hairglasses-studio/mcpkit` | High -- same org, has `observability` package with OpenTelemetry, `finops` package with cost tracking | Port OTel span creation from `mcpkit/observability`; reuse cost model patterns |
| **shielddd** (sibling) | `github.com/hairglasses-studio/shielddd` | High -- pure SQLite (`modernc.org/sqlite`), audit log pattern | Adopt `modernc.org/sqlite` (no CGo) for event persistence; mirror audit log table schema |
| **Grafana Agent / Alloy** | `github.com/grafana/alloy` | Medium -- Go-native Prometheus + OTel pipeline | Study `prometheus/client_golang` integration pattern for Go metrics endpoint |
| **Watermill** | `github.com/ThreeDotsLabs/watermill` | Medium -- Go pub/sub with pluggable backends (NATS, Kafka, in-process) | Fan-out pattern, middleware (dedup, throttle, retry), subscriber groups |
| **CloudEvents Go SDK** | `github.com/cloudevents/sdk-go` | Medium -- standardized event envelope | Event schema with `specversion`, `source`, `type`, `id` fields; industry-standard interop |
| **ntfy** | `github.com/binwiederhier/ntfy` | Medium -- HTTP-based push notifications to phones | Simple webhook + topic model; mobile push without app store dependency |

### 4.2 Patterns Worth Adopting

**1. SQLite event sink (from shielddd)**
- Write all events to a `fleet_events` table with the same schema as `events.Event` plus auto-increment ID, indexed by `type` and `timestamp`.
- Use `modernc.org/sqlite` (pure Go, no CGo) to avoid build complexity.
- Target: new `internal/events/store.go` with `EventStore` interface.
- Ref: `internal/events/bus.go:71` (Publish) -- add `store.Write(event)` call after history append.

**2. Webhook dispatcher with exponential backoff (from Watermill)**
- `internal/webhook/dispatcher.go`: subscribe to event bus, POST to configured URLs.
- Use `net/http.Client` with per-URL retry queue, exponential backoff (1s, 2s, 4s, 8s, max 60s).
- Dedup: track `(event_type, session_id, 60s_window)` to suppress duplicates.
- Ref: `internal/hooks/hooks.go:68-87` (Start/Subscribe pattern) -- webhook dispatcher follows same lifecycle.

**3. Prometheus client_golang counters (from Grafana/alloy)**
- `internal/metrics/prometheus.go`: register counters (`ralphglasses_events_total`, `ralphglasses_sessions_active`, `ralphglasses_cost_usd_total`).
- Wire into existing 2.11 HTTP server plan.
- Subscribe to event bus, increment counters on each event.

**4. CloudEvents envelope (from CloudEvents SDK)**
- Add `specversion`, `id` (UUID), `source` fields to `events.Event`.
- Enables interop with external systems expecting CloudEvents format.
- Can be a wrapper layer for webhook payloads without changing internal Event struct.

### 4.3 Anti-Patterns to Avoid

- **Fat event bus**: Do not add filtering, routing, or transformation logic to the bus itself. Keep `Bus` as a simple broadcast mechanism; put intelligence in subscribers.
- **Synchronous persistence in Publish()**: SQLite writes should be buffered/batched, not inline in the Publish hot path. Use a dedicated goroutine consuming from a channel.
- **Unbounded webhook queues**: Cap retry queues per URL (e.g., 1000 pending). Drop oldest if full, log the drop.
- **High-cardinality Prometheus labels**: Do not use `session_id` as a label (unbounded). Use `provider`, `repo`, `status` (bounded).
- **Recording everything for replay**: Do not store full LLM response bodies in SQLite (too large). Store tool call names, costs, timing, and truncated summaries. Full output stays in session JSON files.

### 4.4 Academic & Industry References

- **Event Sourcing pattern** (Martin Fowler): Store state changes as immutable event log. Relevant for 6.7 replay -- rebuild session state from events rather than snapshotting.
- **CQRS (Command Query Responsibility Segregation)**: Separate write path (event bus) from read path (analytics queries). The existing `handleFleetAnalytics` already follows this implicitly.
- **Prometheus naming conventions** (`https://prometheus.io/docs/practices/naming/`): Use `_total` suffix for counters, `_seconds` for durations, `_bytes` for sizes.
- **CloudEvents specification v1.0** (`https://cloudevents.io/`): Industry standard for event metadata. Adopting the envelope format improves interoperability.
- **SQLite as application file format** (`https://www.sqlite.org/appfileformat.html`): SQLite is ideal for local-first analytics storage; WAL mode supports concurrent readers.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions (Week 1-2)

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|--------------|
| 1 | Create `internal/events/store.go` -- SQLite event persistence with `modernc.org/sqlite` | `internal/events/store.go`, `internal/events/store_test.go` | M | High -- unlocks 6.4 and 6.7 | 6.4.1 |
| 2 | Wire event store into `Bus.Publish()` as async sink (buffered channel + writer goroutine) | `internal/events/bus.go:71-99` | S | High -- all events automatically persisted | 6.4.1 |
| 3 | Create `internal/webhook/dispatcher.go` -- HTTP POST with backoff, dedup, HMAC signing | `internal/webhook/dispatcher.go`, `internal/webhook/dispatcher_test.go` | M | High -- core of 6.5 | 6.5.1, 6.5.5 |
| 4 | Add notification dedup to `fleet_builder.go` -- throttle map with 60s window | `internal/tui/fleet_builder.go:234-241` | S | Medium -- fixes existing spam issue | 2.6.4 |
| 5 | Add tests for `internal/notify/notify.go` using exec mock | `internal/notify/notify_test.go` | S | Low -- closes 0% coverage gap | 2.75.1 |
| 6 | Capture hook execution errors in `hooks.go:139` | `internal/hooks/hooks.go:128-146` | S | Medium -- enables hook failure alerting | 2.75.2 |

### 5.2 Near-Term Actions (Week 3-6)

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|--------------|
| 7 | Discord webhook formatter -- embed builder for session events | `internal/webhook/discord.go`, `internal/webhook/discord_test.go` | S | Medium -- first external integration | 6.5.2 |
| 8 | Slack webhook formatter -- Block Kit builder for session events | `internal/webhook/slack.go`, `internal/webhook/slack_test.go` | S | Medium -- enterprise integration | 6.5.3 |
| 9 | Notification templates -- Go `text/template` with event context vars | `internal/webhook/templates.go`, `internal/webhook/templates_test.go` | M | Medium -- user-customizable messages | 6.5.4 |
| 10 | Prometheus metrics endpoint -- `client_golang` counters and gauges | `internal/metrics/prometheus.go`, `internal/metrics/prometheus_test.go` | M | High -- standard observability | 6.4.4 |
| 11 | Wire Prometheus into 2.11 HTTP server (create if needed) | `internal/http/server.go` (new), `cmd/root.go` (add `--http-addr`) | M | High -- exposes metrics to Grafana | 6.4.4, 2.11 |
| 12 | TUI analytics view -- historical cost chart using ntcharts | `internal/tui/views/analytics.go` | L | Medium -- visual cost tracking | 6.4.2 |
| 13 | SQLite query layer for analytics -- time-bucketed aggregations | `internal/events/store.go` (extend) | M | Medium -- powers TUI analytics | 6.4.1, 6.4.2 |

### 5.3 Strategic Actions (Week 7-12)

| # | Action | Files | Effort | Impact | ROADMAP Item |
|---|--------|-------|--------|--------|--------------|
| 14 | OpenTelemetry traces -- port from `mcpkit/observability`, span per session/task | `internal/telemetry/otel.go`, `internal/telemetry/otel_test.go` | L | Medium -- distributed tracing | 6.4.3 |
| 15 | Grafana dashboard JSON -- pre-built panels for session metrics | `distro/grafana/fleet-dashboard.json` | M | Medium -- out-of-box monitoring | 6.4.5 |
| 16 | Session recording -- capture tool calls, state transitions in event store | `internal/session/recorder.go`, `internal/session/recorder_test.go` | L | High -- enables replay | 6.7.1 |
| 17 | Replay viewer -- TUI view with forward/backward/seek through session events | `internal/tui/views/replay.go` | XL | Medium -- post-mortem analysis | 6.7.2 |
| 18 | Session export -- Markdown/JSON report generator from recorded events | `internal/session/export.go`, `internal/session/export_test.go` | M | Medium -- shareable reports | 6.7.3 |
| 19 | Replay diff view -- side-by-side comparison of two session recordings | `internal/tui/views/replay_diff.go` | L | Low -- A/B model testing UX | 6.7.4 |
| 20 | Retention policy -- auto-archive/delete events and sessions older than N days | `internal/events/store.go` (extend), `.ralphrc` config key | S | Low -- storage management | 6.7.5 |

---

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| SQLite write contention during high event throughput | Medium | High -- blocks event publishing | Use WAL mode + `PRAGMA busy_timeout=5000`; buffer writes in channel with batch inserts (100ms flush interval) |
| Webhook failures causing event backlog OOM | Low | High -- memory exhaustion | Cap per-URL retry queue at 1000 events; drop oldest with `dropped_webhook_events` counter |
| Prometheus high-cardinality explosion | Medium | Medium -- Grafana/Prometheus OOM | Use only bounded labels (provider, repo_name, event_type, status); never use session_id as label |
| OpenTelemetry SDK overhead on hot path | Low | Medium -- latency increase | Use async span exporter; benchmark before/after; make OTel opt-in via flag |
| Session recording disk usage growth | Medium | Medium -- fills disk on marathon runs | Implement 6.7.5 retention policy early; default 7-day retention; monitor disk in fleet dashboard |
| CGo dependency from sqlite3 driver | Low | High -- breaks cross-compilation | Use `modernc.org/sqlite` (pure Go) exclusively; add CI cross-compile check |
| Breaking change to Event struct | Medium | Medium -- downstream consumers break | Add `Version int` field now; document migration path; use optional fields |
| Discord/Slack API rate limiting | Medium | Low -- notifications delayed | Implement client-side rate limiter (30 req/min Discord, 1 req/sec Slack); queue excess |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

```
6.4.1 (SQLite store) --> 6.4.2 (TUI analytics) --> 6.7.1 (session recording) --> 6.7.2 (replay viewer)
                    \--> 6.4.4 (Prometheus)    --> 6.4.5 (Grafana dashboard)
```

The SQLite event store is the single most important enabler. It unblocks both the analytics stack (6.4) and the replay/audit trail (6.7). Prometheus is a parallel track that shares the 2.11 HTTP server dependency.

### 7.2 Recommended Sequence

**Sprint 1 (Week 1-2): Storage + Webhook Foundation**
1. `internal/events/store.go` -- SQLite event persistence (6.4.1)
2. Wire store into `Bus.Publish()` as async sink (6.4.1)
3. `internal/webhook/dispatcher.go` -- HTTP POST + backoff + dedup (6.5.1, 6.5.5)
4. Fix hook error handling in `hooks.go:139` (tech debt)
5. Add notify tests (tech debt)
6. Add dedup to desktop notifications (2.6.4)

**Sprint 2 (Week 3-4): External Integrations**
7. Discord formatter (6.5.2)
8. Slack formatter (6.5.3)
9. Notification templates (6.5.4)
10. Webhook config in `.ralphrc` (6.5.1)

**Sprint 3 (Week 5-6): Observability**
11. Prometheus metrics (6.4.4)
12. HTTP server + `--http-addr` flag (2.11.1-2.11.4)
13. SQLite time-bucketed queries (6.4.1)
14. TUI analytics view (6.4.2)

**Sprint 4 (Week 7-8): Traces + Grafana**
15. OpenTelemetry traces (6.4.3)
16. Grafana dashboard JSON (6.4.5)

**Sprint 5 (Week 9-12): Replay**
17. Session recording infrastructure (6.7.1)
18. Session export (6.7.3)
19. Retention policy (6.7.5)
20. Replay viewer (6.7.2)
21. Replay diff view (6.7.4)

### 7.3 Parallelization Opportunities

```
Track A (Analytics):     6.4.1 --> 6.4.2 --> 6.4.3
Track B (Notifications): 6.5.1 --> 6.5.2 + 6.5.3 (parallel) --> 6.5.4 + 6.5.5
Track C (Observability): 2.11 --> 6.4.4 --> 6.4.5
Track D (Replay):        [blocked by 6.4.1] --> 6.7.1 --> 6.7.2 + 6.7.3 (parallel) --> 6.7.4
```

- **Tracks A and B** are fully independent and can be worked in parallel from day one.
- **Track C** depends only on the 2.11 HTTP server, not on Track A's SQLite store.
- **Track D** depends on Track A's SQLite store (6.4.1) being complete.
- Within 6.5, Discord (6.5.2) and Slack (6.5.3) formatters are independent of each other.
- Within 6.7, export (6.7.3) and replay viewer (6.7.2) can proceed in parallel once recording (6.7.1) is done.
- Tech debt items (hook error handling, notify tests, notification dedup) can be interleaved into any sprint as filler tasks.
