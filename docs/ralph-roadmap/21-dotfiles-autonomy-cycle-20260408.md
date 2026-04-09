# Dotfiles Autonomy Cycle Note — 2026-04-08

## Context

This note records a live autonomous development tranche run against `hairglasses-studio/dotfiles`, with the goal of turning roadmap recommendations into independently shippable increments.

## Tranche Completed

The completed tranche addressed the dotfiles roadmap item for a searchable architecture and provenance split.

Delivered artifacts:

- `dotfiles/docs/ARCHITECTURE-PROVENANCE.md`
- `dotfiles/docs/README.md`

Outcome:

- The repo now has an explicit orientation note separating installer/bootstrap flow, workstation runtime config, and the bundled `mcp/` subtree.
- The docs surface has a local index so future automation can discover that note without depending on root-file edits.

## Cycle Findings

### 1. Prefer additive tranches when the local patch path is degraded

During this cycle, local workspace patching was blocked by a sandbox cwd failure, but GitHub connector writes to `main` remained available. Additive roadmap work still shipped cleanly because the tranche was chosen to require only new files.

Implication for future loops:

- When local file mutation is unreliable, prefer tranches that can land as additive docs, tests, or workflows without rebasing existing files.

### 2. Architecture notes are a high-leverage first tranche

Cross-surface repos like `dotfiles` lose time when future loops must rediscover ownership boundaries. A compact provenance note reduces that rediscovery cost and improves later tranche selection.

Implication for future loops:

- When a repo mixes installer, workstation, and MCP surfaces, land an orientation note before deeper automation changes.

### 3. Roadmap notes should live in a machine-discoverable directory

`docs/ralph-roadmap/` is a better sink for cross-repo cycle notes than only appending top-level roadmap prose. It is easier for auto-build and research passes to enumerate.

Implication for future loops:

- Prefer dated note files under `docs/ralph-roadmap/` for autonomy-cycle artifacts.

## Auto-Build System Patch Hooks

These are the follow-on improvements worth automating in `ralphglasses`.

1. Add a tranche selector that favors additive files when the active environment cannot safely patch existing files.
2. Add a connector-native roadmap-note writer for `docs/ralph-roadmap/` so cycles can persist findings without hand-built file creation.
3. Add a repo-surface classifier that scores whether the next tranche should target docs, tests, workflows, or code.
4. Teach the auto-build loop to recognize unchecked roadmap items that can be satisfied by standalone docs or CI assets.
5. Record environment blockers like sandbox write-path failures as first-class loop observations so tranche planning can adapt automatically.

## Next Dotfiles Tranche

The next high-value additive tranche for `dotfiles` is verification coverage:

- smoke tests for the repo launcher (`scripts/hg`)
- smoke tests for `install.sh` entrypoints
- smoke tests for `scripts/hg-workflow-sync.sh`
- a dedicated CI workflow that runs those smoke checks independently of the broader shell test suite
