# Dotfiles MCP Mirror Parity Tranche — 2026-04-08

## Context

This note records the third additive tranche taken against `hairglasses-studio/dotfiles` during the same autonomous development cycle.

## Tranche Completed

Delivered artifact:

- `dotfiles/docs/MCP-MIRROR-PARITY.md`

What it captures:

- which bundled `dotfiles/mcp/*` modules are canonical source trees
- which standalone repositories are publish mirrors
- the sync and check scripts that enforce parity
- the excluded mirror-only metadata that should remain repo-local
- the recommended verification commands and change order

## Cycle Findings

### 1. Cross-repo parity is documentation debt until it is explicit

The mirror relationships were already encoded in scripts and the workspace manifest, but they were not easy to discover from docs alone. That creates repeated rediscovery cost for autonomous loops.

Implication:

- When a repo acts as both a bundle and a feeder for standalone publish repos, write the parity contract down in a searchable note.

### 2. Manifest-backed docs are better than ad hoc repo memory

The parity mapping in this tranche came from `workspace/manifest.json` and the existing sync scripts, not from assumptions. That keeps the note grounded in the actual automation surface.

Implication:

- Future auto-build flows should synthesize parity docs from manifest data whenever possible.

### 3. Additive documentation tranches can unlock safer later code tranches

Once ownership and parity rules are explicit, later loops can make stronger automation decisions about where code changes should originate and how they should be promoted.

Implication:

- Use documentation tranches to reduce the risk envelope before multi-repo code or release automation work.

## Auto-Build System Patch Hooks

1. Add a manifest-to-doc generator for mirror parity notes.
2. Teach the tranche planner to detect bundled-module plus standalone-mirror topologies automatically.
3. Add a parity-verification recommender that proposes `sync-standalone-mcp-repos.sh check` whenever a canonical mirrored module changes.
4. Persist a canonical-vs-mirror map as structured metadata so later loops can route edits to the correct repo without human arbitration.
5. Add a guardrail that warns when a loop proposes editing a standalone mirror directly for a mirror-managed surface.

## Next Candidate Tranche

The next additive tranche for `dotfiles` should tighten public-facing install and recovery docs so they line up with the actual scripts, especially around:

- Linux-only `install.sh` behavior
- `scripts/hg-workflow-sync.sh` retirement semantics
- `scripts/etc-deploy.sh` and machine-global deployment boundaries
