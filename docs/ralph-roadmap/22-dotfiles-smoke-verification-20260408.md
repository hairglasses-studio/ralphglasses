# Dotfiles Smoke Verification Tranche — 2026-04-08

## Context

This note records the verification-focused tranche that followed the additive documentation tranche for `hairglasses-studio/dotfiles`.

## Tranche Completed

Delivered artifacts:

- `dotfiles/tests/repo_smoke.bats`
- `dotfiles/.github/workflows/ci-smoke.yml`

What this tranche covers:

- `install.sh --print-link-specs`
- installer argument rejection behavior
- `scripts/hg --help`
- `scripts/hg-workflow-sync.sh --help`
- `scripts/hg-workflow-sync.sh --dry-run`
- shell syntax validation for the installer and key launcher entrypoints

Why this matters:

- It adds runnable smoke coverage for the exact entrypoints called out by the dotfiles roadmap.
- It keeps the verification surface lightweight and additive, which made it shippable even while local workspace patching remained degraded.

## Cycle Findings

### 1. Smoke tests are the safest second tranche in mixed shell/config repos

After the architecture note landed, the next highest-value change was not a deep refactor. It was a narrow verification layer around the public entrypoints users and automation actually touch.

Implication:

- For repos heavy in shell and config, add syntax checks and help/dry-run smoke coverage before attempting broader behavioral refactors.

### 2. Standalone workflows are a strong fallback when existing CI files are expensive to edit

Because this tranche could be expressed as a new workflow file, it avoided rebasing existing CI definitions while still improving enforcement on `main`.

Implication:

- When environment or tooling friction makes in-place workflow edits costly, prefer a new focused workflow with a narrow trigger set.

### 3. Help and dry-run paths are part of the product surface

The retired `hg-workflow-sync.sh` path is still user-facing. Treating `--help` and `--dry-run` as smoke-test targets catches regressions in operational messaging, not just code paths.

Implication:

- Auto-build loops should include informational and no-op paths in smoke selection, especially for migration or deprecation scripts.

## Auto-Build System Patch Hooks

1. Add a smoke-tranche template that can emit `tests/*_smoke.bats` plus a dedicated CI workflow in one shot.
2. Teach tranche selection to identify help/dry-run entrypoints automatically from shell scripts.
3. Add a policy that mixed shell/config repos should prefer additive smoke coverage before editing existing umbrella CI workflows.
4. Persist a verification-surface inventory so future loops know which install, launcher, and sync entrypoints already have smoke coverage.
5. Add a local-vs-remote capability detector so the loop can choose between workspace patching and connector-native additive commits without manual intervention.

## Next Dotfiles Tranche

The next additive tranche worth taking in `dotfiles` is mirror-parity documentation for the `mcp/` subtree:

- identify which bundled `mcp/` surfaces correspond to standalone publish mirrors
- document the canonical source-of-truth rule for each one
- define the parity verification command set alongside the ownership note
