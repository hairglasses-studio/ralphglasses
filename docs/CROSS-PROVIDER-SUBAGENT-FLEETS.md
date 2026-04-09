# Cross-Provider Subagent Fleets

Status: 2026-04-08.

This addendum captures the current native provider surfaces for reusable subagents and fleet roles in `ralphglasses`.

## Provider Primitives

- Codex: explicit delegation, project `.codex/agents/*.toml`, built-in worker/explorer patterns, native MCP server mode.
- Gemini: native local `.gemini/agents/*.md` subagents, automatic or explicit `@agent-name` delegation, remote A2A agents, extensions, and agent skills.
- Claude: project `.claude/agents/*.md` agents plus native skills and hooks.

## Repo Decisions

- `.agents/skills/` remains the canonical workflow and skill surface.
- `.agents/roles/*.json` is the canonical reusable fleet-role surface.
- Provider-native role projections live in:
  - `.codex/agents/*.toml`
  - `.claude/agents/*.md`
  - `.gemini/agents/*.md`
- `.gemini/commands/` remains available for prompt shortcuts and compatibility shims, not as the reusable role source of truth.

## Operating Model

- Use Codex as the default control-plane lead when explicit delegation, shallow recursion, or MCP-server exposure matters.
- Use Gemini local subagents for native low-cost sidecar work and remote A2A agents when the worker is genuinely remote or provider-managed.
- Keep one owner per write scope. Multi-agent coordination should split responsibilities by files, modules, or artifact types.
- Preserve a dedicated synthesis lane. Exploration, execution, review, and synthesis should not all share the same role prompt.
- Prefer provider-native primitives over synthetic flag parity. The control plane should normalize intent, not pretend the CLIs are identical.

## Starter Fleet Roles

- `codebase-mapper`: read-heavy repo exploration and execution-path mapping.
- `reviewer`: correctness, regression, and security review.
- `docs-researcher`: citation-backed upstream docs and changelog synthesis.
- `multi-agent-coordinator`: plan fan-out, work ownership, and merge-safe sequencing.
- `task-distributor`: split large tasks into bounded write scopes.
- `knowledge-synthesizer`: compress findings into durable notes, runbooks, and handoff artifacts.
- `fleet-bootstrap`: install or scaffold native provider role surfaces and supporting files.

## Current Gaps

- Existing runtime capability docs in this repo still contain pre-native-Gemini assumptions and should be reconciled when in-place edits are available.
- Role projection generation exists as a checked-in script in this branch, but it is not yet wired into `make skill-surface` or CI drift checks.
- Gemini upstream parallel subagent execution remains an active product area; fleet runtime should assume write isolation is still the control plane's job.
