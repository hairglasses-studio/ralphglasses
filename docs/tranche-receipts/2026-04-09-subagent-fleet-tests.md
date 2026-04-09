# Tranche Receipt: Subagent Fleet Tests

Date: 2026-04-09
Status: landed on main

## Signal

- The canonical role projection generator and the Gemini native-agent migration path were live, but the repo still lacked focused regression coverage for provider overrides, drift detection behavior, and native-vs-legacy Gemini discovery.
- The active Codex session still could not launch a shell in the repo cwd, even after global full-access intent was declared, so execution-lane selection needed to account for the live harness rather than config intent.

## Scope

- add narrow, source-backed regression tests for the subagent fleet migration surface
- record the live-harness capability mismatch so later autobuild loops do not assume local write access without probing it first

## What Changed

- Added `scripts/dev/test_sync_provider_roles.sh`.
  - Builds a temporary fixture repo around `scripts/sync-provider-roles.py`.
  - Verifies provider override surfaces for Codex and Gemini.
  - Verifies default Claude projection behavior.
  - Verifies `--check` drift detection and useful error output.
- Added `internal/session/agents_gemini_test.go`.
  - Verifies Gemini discovery prefers native `.gemini/agents/*.md` over duplicate legacy `.gemini/commands/*.toml` definitions.
  - Verifies legacy-only Gemini command definitions are still discovered during migration.
  - Verifies `WriteAgent` now targets `.gemini/agents/*.md` instead of writing legacy command TOML.
- Execution note.
  - This tranche was implemented without local shell access to the repo workspace.
  - Repo writes still succeeded through the GitHub connector after explicit user confirmation.

## Evidence

- `scripts/dev/test_sync_provider_roles.sh`
- `internal/session/agents_gemini_test.go`
- commits `4ee392e9394e50790fa782238b5c7632d2062c0c` and `c64af6052a8b2b68e4e6ef21109d374594261e7d`

## What This Should Prevent Next Time

- silent regression from Gemini native agents back to commands-only behavior
- projection drift checks that verify checked-in outputs but not provider override semantics
- wasted tranches caused by assuming global config intent is equivalent to live harness capability

## Next Recommended Patch

- wire `scripts/dev/test_sync_provider_roles.sh` into `scripts/dev/ci.sh`
- patch the remaining repo docs that still describe Gemini as commands-only
- add provider-aware team templates and write-scope enforcement tests
