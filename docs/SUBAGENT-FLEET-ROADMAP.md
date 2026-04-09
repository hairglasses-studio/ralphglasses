# Subagent Fleet Roadmap

Status: 2026-04-09.

This roadmap addendum tracks the work needed to turn the current research pass into durable cross-provider fleet infrastructure.

## Phase 1: Canonical Role Surface

- [x] Define `.agents/roles/*.json` as the canonical reusable role format.
- [x] Add a starter fleet role catalog for exploration, review, research, coordination, distribution, synthesis, and bootstrap.
- [x] Wire projection generation into CI drift checks.
- [x] Allow provider override surfaces and provider-tuned projections from the canonical role manifests.

## Phase 2: Native Provider Surfaces

- [x] Add checked-in provider-native projections for Codex, Claude, and Gemini.
- [ ] Reconcile all existing repo docs that still describe Gemini as commands-only.
- [x] Update runtime capability output so Gemini agent paths and subagent support match reality.
- [x] Dual-read Gemini native `.gemini/agents/*.md` and legacy `.gemini/commands/*.toml` during migration.

## Phase 3: Fleet Runtime

- [ ] Add provider-aware team templates that separate control-plane roles from worker roles.
- [ ] Add explicit ownership and write-scope enforcement for parallel workers.
- [ ] Add remote-agent routing for Gemini A2A workers and non-local execution.
- [ ] Add synthesis and verification lanes as first-class team roles.

## Phase 4: Docs And Knowledge Base

- [x] Capture Codex and Gemini subagent research in durable repo docs.
- [x] Add a raw intake ledger for the researched links.
- [ ] Merge the same knowledge into the shared `hairglasses-studio/docs` canonical indexes when in-place index updates are available.

## Autonomous Cycle Notes

- Prefer compatibility tranches: add native provider surfaces first, dual-read legacy paths second, and remove legacy writers only after discovery and docs are aligned.
- Preserve tuned provider-native prompts, models, and sandbox defaults through manifest-level overrides when folding older mature agents into the canonical role catalog.
- Add deterministic UTF-8 and LF generation before broadening the role catalog so CI can catch drift cheaply.
- Try to fast-forward `main` after each tranche, but when connector safety blocks direct branch updates keep landing work on the tranche branch and record the blocker explicitly instead of pausing the program.

## Immediate Next Tranches

- Patch the remaining runtime and docs files that still describe Gemini as commands-only.
- Add provider-aware team templates for planner, worker, reviewer, and synthesizer lanes backed by the new canonical role catalog.
- Add explicit write-scope enforcement tests for structured team owned-path claims and drift handling.
