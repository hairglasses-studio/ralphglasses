# Autonomy Tranche Contract — 2026-04-09

## Context

This addendum records the operating contract for the current Codex-led autonomous development cycle in `ralphglasses`.

Two earlier tranche pairs have already landed on `main`:

- cross-provider subagent fleet starter assets
- shared docs-side cross-provider subagent research addenda

The next open tranche stack remains the ecosystem sweep pair plus follow-on autonomy wiring and roadmap reconciliation.

## Tranche Contract

Every tranche should follow this order:

1. Select the narrowest backlog slice that materially improves the active control plane.
2. Prefer additive files and low-churn surfaces when local workspace patching is degraded.
3. Open a branch and PR for the tranche.
4. Land the tranche to `main` before starting the next one.
5. Record a dated autonomy note immediately after merge.
6. Feed the note back into future tranche selection.

## Current Sequencing Rule

Until local repo patching is healthy again, prioritize work in this order:

1. additive docs, skill, role, and roadmap artifacts that can be landed safely through GitHub-native mutations
2. branch-local fixes that unblock already-open PRs
3. in-place code or config edits only when a safe GitHub-side patch path is available
4. larger refactors only after the connector and local patch lanes both support verification and recovery

## Current Integration Order

The working integration order for the live backlog is:

1. reconcile and land the ecosystem sweep pair
2. reconcile roadmap truth around shipped rdcycle capabilities versus stale roadmap notes
3. wire shipped rdcycle tools into the live autonomous cycle path
4. close the remaining batch sprint and merge-path gaps
5. close the scheduled marathon and cost-aware autonomy gaps

## Findings For The Auto-Build System

### 1. Additive tranches are the most reliable recovery lane

When the local writable mount is degraded, the fastest safe path is not to stall. It is to keep shipping additive slices that preserve forward motion and write down the missing in-place edits as explicit next-tranche debt.

Implication:

- The auto-build system should rank additive tranches above in-place refactors whenever local patch capability is unhealthy.

### 2. The roadmap must distinguish shipped surface from unwired runtime

The rdcycle tool surface already exists in the MCP layer, but the roadmap still describes some of it as missing source. That creates bad tranche selection and duplicated work.

Implication:

- Future loops should classify items as `missing`, `shipped-but-unwired`, `shipped-but-underdocumented`, or `complete` before choosing implementation work.

### 3. Merge-order notes are part of the product surface

Once multiple roadmap PRs are open at the same time, the merge order becomes a real operational dependency and should be recorded durably.

Implication:

- Each tranche note should name the next dependent PRs or branches so later loops do not re-derive the same sequencing logic.

## Auto-Build Patch Hooks

1. Add a capability probe that records whether local repo patching, GitHub branch mutation, PR mutation, and direct verification are currently available.
2. Add tranche labels for `additive`, `in-place`, `merge-unblock`, and `runtime-wiring` so the selector can avoid overcommitting when one lane is degraded.
3. Persist PR-stack sequencing notes in a machine-readable block or sidecar file so future loops can sort open work by dependency.
4. Add a roadmap reconciliation pass that compares roadmap claims against registered MCP tools, handlers, tests, and shipped docs before selecting new work.
5. Add a post-merge note template that captures what remained blocked and why.

## Next Tranche

The next tranche after this note should be the ecosystem sweep reconciliation tranche:

- close the review-fixer sandbox gap
- reconcile provider-mirror claims
- land the ecosystem sweep artifacts on `main`
- write the follow-on note before moving to rdcycle runtime wiring
