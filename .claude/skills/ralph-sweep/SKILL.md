---
name: ralph-sweep
description: Run recurring external ecosystem sweeps for Codex subagents, skills, automations, and ralph-loop-adjacent repos, then write back indexed links and roadmap opportunities.
---

# Ralph Sweep

Use this skill when the task is to refresh external ecosystem intelligence for ralphglasses or the shared docs repo.

## Default loop

1. Check shared research first:
   - `~/hairglasses-studio/docs/`
   - `ralphglasses_docs_check_existing`
   - `ralphglasses_docs_search`
2. Capture the link set and normalize it into durable indexes:
   - local project index under `docs/ralph-roadmap/`
   - mirrored shared-docs index under `~/hairglasses-studio/docs/projects/ralphglasses/`
3. Extract reusable patterns from the sources:
   - subagents and role packs
   - skills and progressive disclosure layouts
   - automations, review/fix loops, and completion contracts
   - state directories, checkpointing, and recovery patterns
4. Convert findings into concrete repo work:
   - roadmap opportunities grouped into `now`, `next`, `later`
   - Codex/Claude/Gemini surface additions where the pattern is ready to ship
   - shared docs writeback for reusable research
5. Keep the tranche auditable:
   - record review date
   - distinguish shipped work from backlog proposals
   - prefer primary sources and repo READMEs over commentary

## Focus areas

- Codex subagent catalogs
- Codex skills and plugin-adjacent packaging
- Ralph-loop derivatives and completion promises
- review/fixer/verifier workflows
- automation packs for CI, docs, release, and dependency hygiene

## Guardrails

- Check `~/hairglasses-studio/docs/` before starting net-new research.
- Update link indexes and research memos together so the findings stay discoverable.
- When a pattern is small enough to ship immediately, land the repo surface and then document it.
- When a pattern is not yet safe to ship, convert it into explicit roadmap opportunities instead of leaving it implicit.
