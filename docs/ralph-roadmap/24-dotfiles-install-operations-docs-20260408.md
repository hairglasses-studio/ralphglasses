# Dotfiles Install and Operations Docs Tranche — 2026-04-08

## Context

This note records the fourth additive tranche taken against `hairglasses-studio/dotfiles` in the same autonomous cycle.

## Tranche Completed

Delivered artifact:

- `dotfiles/docs/INSTALL-AND-OPERATIONS.md`

What it clarifies:

- `install.sh` is the user-scoped bootstrap entrypoint
- machine-global config belongs to dedicated deploy helpers such as `scripts/etc-deploy.sh`
- `scripts/hg-agent-docs.sh` is the right parity generator for compatibility mirrors
- `scripts/hg-workflow-sync.sh` is retired and informational only
- the recommended operator checks now map to the actual script boundaries

## Cycle Findings

### 1. Public-doc alignment can be shipped as additive guidance even when root docs are hard to edit

The root README was not mutated during this cycle, but the operational surface still became clearer because the missing guidance was added as a standalone document.

Implication:

- When existing top-level docs are expensive to patch, land the missing operational truth in an additive guide first, then re-thread index links later.

### 2. Entry-point ambiguity is a recurring automation tax

Without an explicit guide, loops have to rediscover whether a change belongs in `install.sh`, a deploy helper, `scripts/hg`, or a parity generator.

Implication:

- Treat install and deployment boundaries as first-class documentation targets in shell-heavy repos.

### 3. Deprecation paths need stable docs, not just comments in code

Retired-but-still-present scripts like `hg-workflow-sync.sh` remain part of the operator surface. A dedicated guide makes the no-op contract visible outside the script body.

Implication:

- Auto-build systems should emit documentation tranches for deprecation surfaces, not only tests.

## Auto-Build System Patch Hooks

1. Add an entrypoint-classifier that labels scripts as bootstrap, deploy, parity, informational, or runtime.
2. Add a doc-tranche template that synthesizes an install-and-operations guide from the current script inventory.
3. Add a guardrail that prefers additive docs when the next roadmap item is public-doc alignment.
4. Detect retired scripts automatically and require either a smoke test or a public-facing note describing the retired contract.
5. Track unlinked additive docs so a later tranche can re-thread them into root README or site navigation once in-place edits are available.

## Next Candidate Tranche

The next additive tranche for `dotfiles` should turn the new docs into a stronger discovery surface by adding or expanding an explicit docs navigation layer once existing-file edits are available again.
