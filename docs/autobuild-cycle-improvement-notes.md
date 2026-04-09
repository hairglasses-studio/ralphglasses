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

### 2026-04-09: Probe harness capability before promising local execution

Signal:
- local shell access and `apply_patch` were silently blocked by the harness, failing only on execution

What mattered:
- assuming global intended config matched live capability caused wasted turns
- a connector fallback path had to be synthesized mid-flight

Rule:

When planning a tranche, explicitly probe live harness capability (e.g., local shell cwd, git write access) before choosing local-vs-connector execution and before promising local `make ci`.

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

### 2026-04-08: Generated-surface drift should be repaired before the queue advances

Signal:
- full `scripts/dev/ci.sh` on current `main` reproduced provider-role projection drift immediately after the artifact gate tranche landed

What mattered:

- the drift was already source-backed because the existing gate reproduced it on current `main`
- the fix was a pure regeneration wave with no source-manifest edits
- leaving the drift in place would keep the publish lane red and invalidate later tranche evidence

Rule:

If a generated-surface drift gate reproduces on current `main`, land the generated sync as its own narrow tranche before returning to the broader ranked queue.

### 2026-04-08: Nested layout gates need width budgets and deterministic content order

Signal:
- the ranked queue selected `layout_harness_drift_gate` because nested viewport-backed views still lacked a dedicated top-level harness gate

What mattered:

- overview and help snapshots did not exercise nested Repo Detail or Fleet layout paths
- explicit width-budget assertions are easier to diagnose than broad visual diffs alone
- repo detail snapshot stability depended on sorting config keys instead of iterating a map directly

Rule:

When adding a nested TUI regression gate, snapshot at least one detail view and one dashboard view through the top-level harness, and pair the snapshots with explicit visible-width assertions.

### 2026-04-08: Path convergence should centralize legacy compatibility instead of scattering string joins

Signal:
- the next ranked tranche was a shared-path bypass audit after the broad filesystem-hardening wave had already landed

What mattered:

- the remaining bypasses were a small set of active callers, not another repo-wide sweep
- some of those callers still needed legacy scan-root session discovery for compatibility
- the real regression risk was hidden path policy living in repeated inline joins instead of one helper layer

Rule:

When the remaining path debt is a narrow bypass set, move the contract into shared helpers first, then point the callers at those helpers and keep any legacy fallback explicit and localized.

### 2026-04-08: Adoption telemetry needs a patch-id translation layer

Signal:
- discovery-adoption and adoption-priority fronts already existed, but autobuild still required a human to map those workflow and surface gaps back to concrete patch ids

What mattered:

- inactive workflows and surfaces are useful signals, but they are not a tranche plan by themselves
- the missing step was a machine-readable selector that names the actual patch candidate, entry surface, and confidence
- once that selector exists, the next queue handoff can be driven from the live adoption front instead of another static roadmap pass

Rule:

When adoption telemetry is meant to drive autobuild, expose one machine-readable selector that translates workflow and surface gaps into concrete patch ids with confidence and a recommended entry surface.

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

1. `remote_main_red_signal_filter`
2. `generated_surface_drift_gate`
3. `telemetry_to_patch_feedback`
4. `shared_path_bypass_cleanup_followups_if_new_callers_appear`
5. `adoption_led_tranche_selector_followups_if_confidence_mapping_needs_tuning`

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

## 2026-04-08: provider_role_projection_sync

Signal:
- full `scripts/dev/ci.sh` reproduced `scripts/sync-provider-roles.py --check` drift on current `main`

Scope:
- generated provider-role projections only

What changed:
- regenerated the affected `.claude/agents/*.md` and `.gemini/agents/*.md` projections from their role manifests
- verified the sync gate directly
- reran the full commit gate after the regeneration

Evidence:
- `.claude/agents/codebase-mapper.md`
- `.claude/agents/fleet-bootstrap.md`
- `.claude/agents/knowledge-synthesizer.md`
- `.claude/agents/multi-agent-coordinator.md`
- `.claude/agents/task-distributor.md`
- `.gemini/agents/codebase-mapper.md`
- `.gemini/agents/fleet-bootstrap.md`
- `.gemini/agents/knowledge-synthesizer.md`
- `.gemini/agents/multi-agent-coordinator.md`
- `.gemini/agents/task-distributor.md`

What this should prevent next time:
- current-main publish failures caused by stale generated provider surfaces
- later tranches inheriting a red lane from already-detected generated drift

Next recommended patch:
- `layout_harness_drift_gate`

## 2026-04-08: layout_harness_drift_gate

Signal:
- the ranked queue selected the known nested TUI layout harness as the next narrow integrity gate

Scope:
- repo-owned TUI harness coverage only

What changed:
- added top-level teatest golden snapshots for nested Repo Detail and Fleet views
- added explicit visible-width assertions for those nested views
- added a resize-path width-budget regression test
- sorted repo detail config keys before rendering so the nested snapshot is deterministic

Evidence:
- `internal/tui/app_teatest_test.go`
- `internal/tui/views/repodetail.go`
- `internal/tui/testdata/TestTeatest_RepoDetailView.golden`
- `internal/tui/testdata/TestTeatest_FleetView.golden`

What this should prevent next time:
- nested layout width regressions only showing up after a larger tranche lands
- nondeterministic nested snapshots caused by map iteration order
- top-level TUI coverage drifting back to overview-only snapshots

Next recommended patch:
- `shared_path_bypass_audit`

## 2026-04-08: shared_path_bypass_audit

Signal:
- the ranked queue selected the remaining shared-path bypass set after the layout harness gate closed

Scope:
- repo-owned path helper convergence for active session, state, and config-adjacent callers

What changed:
- added shared `ralphpath` helpers for cost events, command history, and external session search paths
- moved session, budget, and tenant command discovery onto the shared search helper while keeping legacy scan-root session state readable
- removed the marathon command's direct scan-root session-state override so new runtime state stays on the canonical shared path
- added regression coverage for helper resolution and shared-plus-legacy external session discovery

Evidence:
- `internal/ralphpath/paths.go`
- `cmd/store.go`
- `cmd/store_test.go`
- `internal/session/cost_events.go`
- `internal/tui/command_history.go`

What this should prevent next time:
- active path contracts drifting through repeated inline string joins
- command surfaces seeing only legacy scan-root sessions or only shared sessions depending on call site
- future path-hardening work reopening a broad sweep just to find a few remaining bypasses

Next recommended patch:
- `adoption_led_tranche_selector`

## 2026-04-08: adoption_led_tranche_selector

Signal:
- the ranked queue selected the missing selector layer between existing adoption telemetry and actual autobuild patch choice

Scope:
- repo-owned autobuild tranche selection from live discovery and adoption fronts

What changed:
- added a machine-readable autobuild tranche summary that ranks concrete patch ids from the live adoption fronts
- exposed that selector through `ralph:///catalog/autobuild-tranches`
- wired the selector into the catalog server and runtime health summaries for read-first discovery
- added focused regression coverage for the selector summary, new resource, and updated resource counts

Evidence:
- `internal/mcpserver/autobuild_tranches.go`
- `internal/mcpserver/resources.go`
- `internal/mcpserver/resources_test.go`
- `internal/mcpserver/tools_dispatch.go`

What this should prevent next time:
- autobuild still needing a human to translate workflow gaps into concrete patch ids
- adoption telemetry existing without a direct tranche-selection consumer
- future queue handoffs falling back to static roadmap ordering despite live repo signals

Next recommended patch:
- `generated_surface_drift_gate`

---

## Tranche 5: Integrity Gates (Red Signal Filter)

Target repo: `ralphglasses`

Scope:
- Filter autobuild intake to actionable red signals that reproduce on remote main.

What changed:
- Added `RemoteMainVerified` metadata to `AutobuildTriggerSignal`.
- Added `activeRedSignalCandidates` parser to find `EventCrash` failures in the telemetry log.
- Enforced verification: branch-local failures and dirty-worktree crashes are ignored by default.
- Injected verified red signals into the patch queue with priority P0.

Evidence:
- `5666929`

What this should prevent next time:
- Wasted autobuild cycles chasing local dirty-worktree bugs or unresolved branch state.
- False alarms dominating the repo-owned queue.
