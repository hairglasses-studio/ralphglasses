# Fleet Role Catalog

This directory is the provider-neutral source of truth for reusable fleet roles.

## Canonical Rules

- One JSON manifest per reusable role.
- Keep workflows and runbooks in `.agents/skills/`.
- Project native role files into:
  - `.codex/agents/`
  - `.claude/agents/`
  - `.gemini/agents/`
- Treat `.gemini/commands/` as prompt shortcuts only.

## Starter Roles In This Branch

- `codebase-mapper`
- `reviewer`
- `docs-researcher`
- `multi-agent-coordinator`
- `task-distributor`
- `knowledge-synthesizer`
- `fleet-bootstrap`
