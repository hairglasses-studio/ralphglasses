# Ralphglasses Autobuild Patch Queue

Date: 2026-04-09
Scope: roadmap-derived queue for repo-owned automated patch selection

## Source Inputs

This queue is derived from the current shipped repo surfaces plus the latest
docs control-plane evidence:

- `ROADMAP.md`
- `README.md`
- `docs/CODEX-PARITY-STATUS.md`
- `hairglasses-studio/docs/projects/agent-parity/active-fleet-investigation-2026-04-09.md`
- `hairglasses-studio/docs/agent-parity/workspace-health-matrix.json`

## Current Stance

`ralphglasses` already has the broad repo-owned fronts that the parity program was chasing:

- runtime and operator-facing read-first fronts
- provider-parity discovery and adoption resources
- multiple filesystem-hardening follow-ups already shipped
- generated documentation and parity surfaces under active maintenance

That means the next autobuild work should not reopen another broad sweep. The next useful queue is narrow, integrity-led, and driven by real adoption or verification signal.

## Patch Selection Rules

- Choose repo-owned work from adoption telemetry and integrity signal before static roadmap breadth.
- Require a current `main` reproduction before spending autobuild capacity on a red signal.
- Keep user-home or workstation-only cleanup out of the repo-owned patch queue.
- Prefer small gates and narrow helper convergence over another blanket filesystem pass.
- Record why a tranche was opened, which signal triggered it, and what evidence closed it.

## Ranked Patch Queue

### 1. `shared_path_bypass_audit`

Priority: `P1`

Why now:

- The broad path-hardening tranche is mostly complete.
- Remaining wins should come from the few repo-owned callers that still bypass shared path helpers.

Acceptance:

- Active config, state, cache, session, and coordination paths resolve through shared helpers or are explicitly justified.
- The queue stops after the remaining repo-owned bypass set is exhausted.

### 2. `adoption_led_tranche_selector`

Priority: `P1`

Why now:

- The repo already exposes discovery, adoption, and priority fronts.
- Autobuild should pick the next `ralphglasses` tranche from those signals instead of a static roadmap scan.

Acceptance:

- One machine-readable summary ranks the next repo-owned patch candidates.
- The summary includes source signal, recommended entry surface, and confidence.

### 3. `remote_main_red_signal_filter`

Priority: `P1`

Why now:

- The broader parity program is already seeing stale local red state masquerade as repo debt.
- The refreshed workspace matrix now distinguishes real failures from `dirty`
  and `not_main` worktrees, so autobuild should consume that signal directly.
- `ralphglasses` should not burn autobuild cycles on failures that do not reproduce on remote `main`.

Acceptance:

- Automated patch selection requires remote-main verification metadata for a
  red signal.
- Dirty-worktree or branch-local failures do not enter the repo-owned patch queue by default.

### 4. `generated_surface_drift_gate`

Priority: `P2`

Why now:

- The repo has multiple generated docs and parity-facing surfaces.
- Generated-output drift is now more likely than missing baseline surface.

Acceptance:

- Generated runtime, parity, or docs surfaces fail verification when the source contract changes without regeneration.
- Drift is recorded as a source-backed integrity failure, not as generic docs cleanup.

### 5. `telemetry_to_patch_feedback`

Priority: `P2`

Why now:

- Adoption-led selection becomes much more useful when the system can see which signals actually produced worthwhile patches.
- This keeps future queue ranking from being purely speculative.

Acceptance:

- Each autobuild tranche records trigger signal, chosen patch id, evidence path, and closure status.
- Future ranking can prefer signals that historically produced real repo-owned fixes.

## Explicitly Out Of Scope

- another blanket filesystem-hardening sweep
- user-home overlay cleanup
- repo-external workstation maintenance
- broad roadmap expansion without a live triggering signal

## Autobuild Handoff Format

Every queued patch should carry the same minimum metadata:

- patch id
- trigger signal
- repo-owned scope
- recommended entry surface
- acceptance condition
- stop condition
- evidence path written after closure

## First Recommended Execution Order

1. `shared_path_bypass_audit`
2. `adoption_led_tranche_selector`
3. `remote_main_red_signal_filter`
4. `generated_surface_drift_gate`

That order starts with small integrity gates, then narrows remaining path debt, then lets telemetry choose the next larger tranche.
