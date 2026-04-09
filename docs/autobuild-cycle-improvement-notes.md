# Autobuild Cycle Improvement Notes

Date: 2026-04-08
Status: active roadmap companion

This file is the repo-owned note lane for improving the `ralphglasses` autobuild loop over time.

Use it for:

- tranche selection lessons
- false-positive reduction rules
- evidence and closure discipline
- patch queue tuning notes
- recurring failure patterns that should become gates, tooling, or machine-readable policy

Keep `ROADMAP.md` strategic and long-horizon. Put cycle-level execution lessons here so future autobuild patches have a stable operational memory.

## Current Operating Assumptions

- The broad repo-owned parity and filesystem-hardening sweeps are largely complete.
- The highest-value remaining work is narrow, integrity-led, and triggered by live signal.
- Autobuild capacity should prefer repo-owned source fixes over workstation or user-home cleanup.
- A red signal is not actionable until it reproduces against current remote `main`.

## Current Selection Rules

1. Prefer integrity failures over speculative roadmap breadth.
2. Prefer source-backed evidence over branch-local or dirty-worktree noise.
3. Prefer small gates that prevent future regressions over one-off cleanup.
4. Prefer machine-readable queue inputs over prose-only instructions.
5. After every tranche, record both closure evidence and the next recommended patch id.

## Lessons Learned

### 2026-04-08: Pair prose with machine-readable artifacts

A roadmap note alone is not enough for autobuild selection.

What worked:

- markdown checkpoint or queue for human review
- JSON companion for automation
- JSON schema so later sessions can validate structure instead of guessing

Rule:

Every new autobuild-facing planning artifact should ship as a three-part set when practical:

- human-readable markdown
- machine-readable JSON
- schema for validation

### 2026-04-08: Stable pointers matter more than dated files alone

Dated checkpoint files are good historical records but weak entrypoints for automation.

What worked:

- date-stamped files for immutable checkpoints
- stable `latest-*` manifests for current consumers

Rule:

When a dated checkpoint becomes the current source of truth, add or update a stable pointer so future sessions do not need to discover the newest file by convention.

### 2026-04-08: Separate repo-owned debt from environment debt

The wider parity program is now dominated by source-vs-overlay distinction.

Rule:

Do not let user-home, workstation, or harness-local drift consume repo-owned autobuild capacity unless the tranche is explicitly an environment-maintenance pass.

### 2026-04-08: Remote-main verification is mandatory before repair

A local red state can be stale, branch-local, or caused by a dirty worktree.

Rule:

Any red signal entering the autobuild queue must carry one of:

- remote `main` reproduction evidence
- current CI evidence
- a clearly source-backed integrity failure path

Without that evidence, the patch should not be ranked above known source-backed work.

### 2026-04-08: Small integrity gates have compounding value

The highest-leverage near-term work is not another broad sweep.

Examples:

- tracked temp artifact gate
- layout-harness drift gate
- generated-surface drift gate
- shared path-helper bypass audit

Rule:

When choosing between a broad cleanup tranche and a small gate that prevents recurrence, prefer the gate unless the cleanup directly blocks shipping.

### 2026-04-08: Reproduced commit-gate reds outrank new breadth

Signal:
- `go test -short -count=1 ./...` failed on clean remote `main`, not just in a dirty local worktree

What mattered:

- the failures were source-backed and reproducible
- the breakage sat on the normal commit path
- the fixes were narrow contract repairs, not speculative cleanup

Rule:

If a red commit gate reproduces against current remote `main`, promote that repair ahead of new roadmap breadth until the publish lane is green again.

### 2026-04-08: Artifact gates should start with path contracts, not content heuristics

Signal:
- the next integrity tranche targeted tracked temp artifacts and placeholder outputs

What mattered:

- filename and path-segment checks are deterministic
- content greps would have false positives across docs, fixtures, and examples
- an explicit allowlist keeps exceptional cases reviewable instead of silently tolerated

Rule:

When guarding against tracked temp artifacts, start with tracked-path and filename rules plus an explicit allowlist before adding any content-level heuristic.

## Autobuild Patch Note Template

Use this shape when closing or reprioritizing a tranche:

```md
## YYYY-MM-DD: <patch-id>

Signal:
- <what triggered this tranche>

Scope:
- <repo-owned scope only>

What changed:
- <artifact, gate, helper, or policy added>

Evidence:
- <commit / file / CI / verification path>

What this should prevent next time:
- <failure class>

Next recommended patch:
- <patch-id>
```

## Current Recommended Sequence

1. `layout_harness_drift_gate`
2. `shared_path_bypass_audit`
3. `adoption_led_tranche_selector`
4. `remote_main_red_signal_filter`
5. `generated_surface_drift_gate`
6. `telemetry_to_patch_feedback`

## Backlog For Future Productization

These notes should eventually become repo-owned automation instead of remaining prose:

- machine-readable latest-queue pointer
- execution ledger for closed autobuild tranches
- queue item validator against schema
- remote-main evidence requirement in patch intake
- tranche closure writer that records evidence and next patch id

## First Standing Notes

## 2026-04-08: planning artifacts need explicit contracts

Signal:
- new autobuild queue and parity checkpoint artifacts were added for current-main planning

Scope:
- planning surface only

What changed:
- added human-readable queue and checkpoint files
- added JSON companions
- added schemas
- added a stable latest-checkpoint manifest on the docs side

Evidence:
- `docs/autobuild-patch-queue.md`
- `docs/autobuild-patch-queue.json`
- `docs/autobuild-patch-queue.schema.json`
- `hairglasses-studio/docs/projects/agent-parity/2026-04-08-next-tranche-checkpoint.md`
- `hairglasses-studio/docs/projects/agent-parity/2026-04-08-next-tranche-checkpoint.json`
- `hairglasses-studio/docs/projects/agent-parity/2026-04-08-next-tranche-checkpoint.schema.json`
- `hairglasses-studio/docs/projects/agent-parity/latest-checkpoint.json`
- `hairglasses-studio/docs/projects/agent-parity/latest-checkpoint.schema.json`

What this should prevent next time:
- orphan planning notes
- automation scraping markdown structure ad hoc
- ambiguity about which dated checkpoint is current

Next recommended patch:
- `autobuild_execution_ledger_template`

## 2026-04-08: integrity_temp_artifact_gate

Signal:
- autobuild queue ranked tracked temp artifacts and placeholder outputs as the first narrow integrity gate after the planning bundle and the short-suite repair

Scope:
- repo-owned verification only

What changed:
- added a tracked artifact gate that scans `git ls-files`
- added an explicit repo-owned allowlist for justified exceptions
- added fixture tests for `.DS_Store`, temp directories, placeholder filenames, and allowlist behavior
- wired the gate into `scripts/dev/ci.sh` and `scripts/dev/pre-commit`

Evidence:
- `scripts/dev/check-tracked-artifacts.sh`
- `scripts/dev/test_tracked_artifacts.sh`
- `.tracked-artifact-allowlist`

What this should prevent next time:
- stray tracked temp outputs reaching `main`
- placeholder output files shipping as real repo artifacts
- long CI runs masking an early source-tree integrity failure

Next recommended patch:
- `layout_harness_drift_gate`
