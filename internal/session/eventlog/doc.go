// Package eventlog provides the append-only event log, event types,
// journal, replay, and snapshot infrastructure for session state.
//
// See ADR 0014 (Event-Sourced Session State) for design rationale.
// The event log records every session state transition with timestamps,
// enabling post-hoc debugging, session forking via shared immutable
// prefixes, and replay-based state reconstruction. Events are bounded
// by a configurable ring buffer to prevent unbounded memory growth.
package eventlog
