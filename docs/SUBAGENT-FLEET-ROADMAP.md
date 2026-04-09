# Subagent Fleet Roadmap

Status: 2026-04-08.

This roadmap addendum tracks the work needed to turn the current research pass into durable cross-provider fleet infrastructure.

## Phase 1: Canonical Role Surface

- [x] Define `.agents/roles/*.json` as the canonical reusable role format.
- [x] Add a starter fleet role catalog for exploration, review, research, coordination, distribution, synthesis, and bootstrap.
- [ ] Wire projection generation into CI drift checks.

## Phase 2: Native Provider Surfaces

- [x] Add checked-in provider-native projections for Codex, Claude, and Gemini.
- [ ] Reconcile all existing repo docs that still describe Gemini as commands-only.
- [ ] Update runtime capability output so Gemini agent paths and subagent support match reality.

## Phase 3: Fleet Runtime

- [ ] Add provider-aware team templates that separate control-plane roles from worker roles.
- [ ] Add explicit ownership and write-scope enforcement for parallel workers.
- [ ] Add remote-agent routing for Gemini A2A workers and non-local execution.
- [ ] Add synthesis and verification lanes as first-class team roles.

## Phase 4: Docs And Knowledge Base

- [x] Capture Codex and Gemini subagent research in durable repo docs.
- [x] Add a raw intake ledger for the researched links.
- [ ] Merge the same knowledge into the shared `hairglasses-studio/docs` canonical indexes when in-place index updates are available.
