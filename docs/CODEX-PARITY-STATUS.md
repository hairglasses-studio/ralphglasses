# Codex Parity Status

Status as of 2026-04-07: the current shipped ralphglasses control-plane workflows are at Codex parity for the scope tracked in roadmap section `3.5.5`.

Broader three-provider parity now lives in [PROVIDER-PARITY-OBJECTIVES.md](PROVIDER-PARITY-OBJECTIVES.md). This document stays focused on the Codex-primary migration closeout.

## What is complete

- Codex is the default provider when callers omit `provider` across session launch/resume, RC control, teams, fleet worker discovery, loops, and sweep/self-improve entrypoints.
- Codex resume is supported when the installed CLI exposes `codex exec resume`; repo code probes support instead of hard-rejecting Codex.
- Codex-native repo surfaces exist:
  - `AGENTS.md`
  - `.codex/config.toml`
  - `.codex/agents/*.toml`
  - `.agents/skills/ralphglasses/SKILL.md`
  - `plugins/ralphglasses/.codex-plugin/plugin.json`
- Prompt improvement and enhancer defaults are OpenAI/Codex-first when callers omit `provider` or `target_provider`.
- Claude prompt-cache regressions are guarded operationally:
  - resumed-Claude sessions with cache writes but zero cache reads are marked anomalous
  - cache read/write health is exposed in session status and fleet analytics
  - repeated resumed-Claude cache anomalies reroute implicit long-running orchestration back to Codex

## What is not a Codex parity blocker

These items remain valid product work, but they are not blockers for concluding the Codex-parity migration:

- `ROADMAP 10.5.1` — token counting API for tighter pre-cycle budget forecasts
- `ROADMAP 10.5.2` — Batch API support for non-interactive marathon workloads
- `ROADMAP 1792` — fleet-wide shared prompt caching across sessions
- `ROADMAP SH-8` — orphan process reaper on startup
- `ROADMAP UO-4` — graceful degradation when a provider is unavailable
- `ROADMAP AE-5` — systematic cross-provider quality/cost comparison

## Future-session rules

- Do not reopen `3.5.5` unless a new workflow regresses to Claude-first behavior or a new shipped surface lacks Codex support.
- If a future task genuinely requires Claude Code to unblock work, do not switch ad hoc. Write a focused Claude Code prompt, copy it to the paste buffer, and record the reason in repo docs or roadmap notes.
- When changing provider defaults, verify both runtime code and operator surfaces:
  - `AGENTS.md`
  - `CLAUDE.md`
  - `GEMINI.md`
  - `README.md`
  - `docs/CODEX-REFERENCE.md`
  - MCP tool descriptions in `internal/mcpserver/tools_builders_*.go`

## Safe-to-conclude criteria

A session focused on Codex parity is safe to conclude when all of the following are true:

- `ROADMAP 3.5.5.*` items are complete and their notes match reality
- `ROADMAP 10.5.3.*` cache-safety items are complete and their notes match reality
- repo docs describe Codex as the default command-and-control provider
- no known shipped workflow still defaults implicitly to Claude when `provider` is omitted
- remaining local changes, if any, are unrelated user work or transient artifacts rather than parity work
