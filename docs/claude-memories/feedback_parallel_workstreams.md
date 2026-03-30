---
name: Parallel workstream execution pattern
description: User prefers 5-8 parallel worktree agents for multi-file improvements — merge carefully, always run full test suite after
type: feedback
---

When the user requests improvement batches, they want: analysis → task inventory → independent workstream definitions → parallel worktree agent execution → merge to main → build/test verification → commit.

**Why:** User runs ambitious multi-file improvement sprints (Sprints 1-7 used 8 parallel agents each). Speed through parallelism is the priority.

**How to apply:**
- Use worktree-isolated agents (`isolation: "worktree"`) for independent workstreams
- Each agent gets non-overlapping file ownership to avoid merge conflicts
- CRITICAL: Use `git -C <worktree> status --short` before cleanup — `git diff --name-only HEAD` does NOT show untracked (NEW) files
- Use `/bin/cp -f` for overwrites when merging (macOS cp prompts by default)
- Merge order matters when agents touch overlapping packages
- Always run `go build ./... && go test ./... -count=1` after merge
- Fix naming conflicts (redeclared test functions) before final test run
