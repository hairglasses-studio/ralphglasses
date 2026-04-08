# Provider Parity Objectives

Status as of 2026-04-07: ralphglasses treats Claude Code, Gemini CLI, and OpenAI Codex CLI as first-class providers, but parity is capability-aware rather than flag-identical.

## Source Of Truth

- Runtime capability registry: `internal/session/provider_capabilities.go`
- Launch validation and warnings: `internal/session/providers.go`
- MCP inspection tools:
  - `ralphglasses_provider_capabilities`
  - `ralphglasses_provider_compare`
  - `ralphglasses_provider_recommend`

## Objectives

- Keep one repo-local config surface per provider:
  - Claude: `.claude/settings.json`
  - Gemini: `.gemini/settings.json`
  - Codex: `.codex/config.toml`
- Keep provider review/security workflows symmetrical enough that operators can swap providers without learning a new GitHub automation model.
- Encode non-parity explicitly:
  - native
  - emulated
  - install-dependent
  - unsupported
- Prefer repo-native provider surfaces over synthetic flags when a CLI lacks direct support.
- Make recommendation output capability-aware so the cheapest provider is not suggested when it cannot satisfy the requested controls.

## Capability Snapshot

| Capability | Claude | Gemini | Codex |
|------------|--------|--------|-------|
| Budget | Native | Emulated by ralphglasses | Emulated by ralphglasses |
| Max turns | Native | Unsupported | Unsupported |
| Agent flag | Native | Unsupported | Unsupported |
| Allowed tools | Native | Native but deprecated upstream | Unsupported |
| Worktree | Native | Native | Unsupported |
| Permission mode | Native | Native via `--approval-mode` | Emulated via `--sandbox` |
| Output schema | Native | Unsupported | Native |
| MCP client | Native | Native | Native |
| MCP server | Unsupported | Unsupported | Native |
| Hooks | Native | Native | Unsupported |
| Plugins/extensions | Native | Native via extensions | Native |
| Subagents | Native | Unsupported | Native |

## Repo Surfaces

- `AGENTS.md` remains the canonical project instruction file.
- `CLAUDE.md` and `GEMINI.md` are compatibility documents with provider-specific caveats and links back to `AGENTS.md`.
- `.mcp.json` is the shared MCP command source of truth; provider configs must preserve the same command and `cwd`.
- `.claude/settings.json`, `.gemini/settings.json`, and `.codex/config.toml` are all baseline-guarded.

## Workflow Parity

- Automatic PR review and security review should skip docs-only changes.
- Mention-triggered assistance should only run for trusted commenters on PR context.
- Review/security jobs should refuse fork PR execution before checkout when secrets are in scope.
- Reusable org workflows should be pinned, not referenced via `@main`.

## Remaining Intentional Differences

- Claude remains the only provider with native budget, system prompt, agent, and max-turn controls.
- Gemini remains the best low-cost native worktree option when Codex worktree isolation is required.
- Codex remains the only provider with native MCP server mode and the default control-plane provider.

## Operator Rule

When provider behavior changes, update all three layers in the same change:

1. `internal/session/provider_capabilities.go`
2. MCP tool descriptions and workflow/config surfaces
3. Operator docs in `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, and `docs/PROVIDER-SETUP.md`
