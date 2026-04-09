# Codex Reference

Codex is now the default command-and-control provider for ralphglasses when callers omit a provider.

Current parity status: see `docs/CODEX-PARITY-STATUS.md` for the durable closeout state and future-session rules.
Cross-provider capability and workflow parity is tracked in `docs/PROVIDER-PARITY-OBJECTIVES.md`.

## Local baseline

- Verified locally on 2026-04-04: `codex-cli 0.118.0`
- Verified locally: `codex exec resume --help` is available on this install
- Repo behavior: `ralphglasses_session_resume` now allows Codex resume when the installed CLI supports it

## Runtime defaults

- Primary provider default: `codex`
- Default Codex session model: `gpt-5.4`
- Default loop planner model: `o4-mini`
- Default loop worker/verifier model: `codex-mini-latest`
- Skill export targets:
  - `.claude/skills/ralphglasses/SKILL.md`
  - `.agents/skills/ralphglasses/SKILL.md`
  - `plugins/ralphglasses/skills/ralphglasses/SKILL.md`
- Codex custom agents:
  - `.codex/agents/*.toml`
- Codex plugin bundle:
  - `plugins/ralphglasses/.codex-plugin/plugin.json`
  - `plugins/ralphglasses/.mcp.json`
  - `.agents/plugins/marketplace.json`

## Codex features to leverage

- `AGENTS.md` project instructions
- `.codex/agents/*.toml` custom subagents
- `codex exec` non-interactive/headless execution
- `codex exec resume` session continuation
- `codex mcp-server` peer-to-peer MCP exposure
- Skills
- Plugins
- Subagents
- `.codex/config.toml` profiles and MCP registration

## Codex limits to remember

- Budget enforcement is external in ralphglasses.
- Worktree support is unsupported in the Codex CLI and must be handled outside the CLI.
- Generic permission modes are mapped onto Codex sandbox modes by ralphglasses.
- System prompt, max turns, and agent selection are repo-native surfaces, not `codex exec` flags.

## Claude cache guardrails

- Resumed Claude sessions are treated as cache-unsafe until live cache reads are observed.
- ralphglasses no longer assumes Claude prompt-cache savings by default in shared cache accounting.
- Runtime anomaly: if a resumed Claude session writes cache entries but reports zero cache reads, the session records a cache anomaly and emits a `session.error` event.
- Repeated resumed-Claude cache anomalies trigger reroute of implicit long-running orchestration back to Codex.

## Research notes

- Official Anthropic docs document prompt caching behavior and invalidation rules, but as of 2026-04-04 this repo has not pinned an official Anthropic postmortem or fix notice for the recent resumed-session cache regression.
- Treat unofficial reports as operational signal, not as authoritative product guarantees.

## Community Ecosystem Sweep (2026-04-08)

- New project roles: `ecosystem_researcher`, `roadmap_synthesizer`, `automation_designer`, and `review_fixer`.
- New canonical skill: `ralph-sweep`.
- Durable local research outputs:
  - `docs/ralph-roadmap/17-external-ecosystem-link-index-2026-04-08.md`
  - `docs/ralph-roadmap/32-codex-ralph-ecosystem-sweep-2026-04-08.md`
  - `docs/ralph-roadmap/33-ecosystem-roadmap-addendum-2026-04-08.md`
- Shared docs mirrors:
  - `~/hairglasses-studio/docs/projects/ralphglasses/17-external-ecosystem-link-index-2026-04-08.md`
  - `~/hairglasses-studio/docs/projects/ralphglasses/32-codex-ralph-ecosystem-sweep-2026-04-08.md`
  - `~/hairglasses-studio/docs/projects/ralphglasses/33-ecosystem-roadmap-addendum-2026-04-08.md`
- Highest follow-on themes: automation packs, review/fixer/verifier workflows, completion-contract docs, and recurring ecosystem sweep refreshes.

## Pinned links

### OpenAI Codex

- https://developers.openai.com/codex/noninteractive
- https://developers.openai.com/codex/config-basic
- https://developers.openai.com/codex/skills
- https://developers.openai.com/codex/plugins
- https://developers.openai.com/codex/plugins/build
- https://developers.openai.com/codex/subagents
- https://developers.openai.com/codex/mcp
- https://developers.openai.com/codex/guides/agents-md

### Anthropic / Claude

- https://docs.anthropic.com/en/docs/claude-code/common-workflows
- https://docs.anthropic.com/en/docs/claude-code/hooks
- https://platform.claude.com/docs/en/build-with-claude/prompt-caching
