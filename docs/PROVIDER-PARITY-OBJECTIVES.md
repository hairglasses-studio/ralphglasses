# Provider Parity Objectives

Status as of 2026-04-09: ralphglasses treats Claude Code, Gemini CLI, and OpenAI Codex CLI as first-class streaming providers, and Google Antigravity as an experimental external-manager provider. Parity is capability-aware rather than flag-identical. Reusable fleet roles are provider-neutral first and projected into each provider's native subagent or workflow surface.

## Source Of Truth

- Runtime capability registry: `internal/session/provider_capabilities.go`
- Launch validation and warnings: `internal/session/providers.go`
- Canonical reusable role catalog: `.agents/roles/*.json`
- Canonical workflow surface: `.agents/skills/`
- Canonical Antigravity/Gemini workflow commands: `.agents/workflows/`
- Canonical Antigravity/Gemini rules: `.agents/rules/`
- Provider role projection generator: `scripts/sync-provider-roles.py`
- MCP inspection tools:
  - `ralphglasses_provider_capabilities`
  - `ralphglasses_provider_compare`
  - `ralphglasses_provider_recommend`

## Objectives

- Keep one repo-local config surface per provider:
  - Claude: `.claude/settings.json`
  - Gemini: `.gemini/settings.json`
  - Codex: `.codex/config.toml`
  - Antigravity: `.mcp.json` plus repo-native `.agents/rules/` and `.agents/workflows/`
- Keep one canonical reusable role surface for all providers:
  - `.agents/roles/*.json`
- Keep one canonical workflow and skill surface for all providers:
  - `.agents/skills/`
- Keep provider review and security workflows symmetrical enough that operators can swap providers without learning a new GitHub automation model.
- Encode non-parity explicitly:
  - native
  - emulated
  - install-dependent
  - unsupported
- Prefer repo-native provider surfaces over synthetic flags when a CLI lacks direct support.
- Make recommendation output capability-aware so the cheapest provider is not suggested when it cannot satisfy the requested controls.

## Capability Snapshot

| Capability | Claude | Gemini | Codex | Antigravity |
|------------|--------|--------|-------|-------------|
| Budget | Native | Emulated by ralphglasses | Emulated by ralphglasses | Emulated by ralphglasses |
| Max turns | Native | Unsupported | Unsupported | Unsupported |
| Agent flag | Native | Unsupported (no `--agent` flag) | Unsupported | Unsupported |
| Allowed tools | Native | Native but deprecated upstream | Unsupported | Unsupported |
| Worktree | Native | Native | Unsupported | Unsupported |
| Permission mode | Native | Native via `--approval-mode` | Emulated via `--sandbox` | Unsupported |
| Output schema | Native | Unsupported | Native | Unsupported |
| MCP client | Native | Native | Native | Native |
| MCP server | Unsupported | Unsupported | Native | Unsupported |
| Hooks | Native | Native | Unsupported | Unsupported |
| Plugins/extensions | Native | Native via extensions | Native | Native via Gemini-style extensions |
| Subagents | Native | Native | Native | Unsupported in Ralph runtime terms |

## Repo Surfaces

- `AGENTS.md` remains the canonical project instruction file.
- `CLAUDE.md` and `GEMINI.md` are compatibility documents with provider-specific caveats and links back to `AGENTS.md`.
- `.mcp.json` is the shared MCP command source of truth; provider configs must preserve the same command and `cwd`.
- `.claude/settings.json`, `.gemini/settings.json`, and `.codex/config.toml` are all baseline-guarded.
- `.agents/roles/*.json` is the canonical reusable role catalog.
- `.agents/skills/` is the canonical workflow and skill catalog.
- `.agents/workflows/*.md` is the canonical generated Antigravity/Gemini workflow command catalog.
- `.agents/rules/ralphglasses.md` is the canonical generated Antigravity/Gemini repo rule surface.
- `.codex/agents/*.toml`, `.claude/agents/*.md`, and `.gemini/agents/*.md` are native provider projections of the shared role catalog.
- `.gemini/commands/ralph/*.toml` is a generated local command mirror of the canonical workflow catalog.
- `.gemini/extensions/ralphglasses-workspace/` is a generated thin extension bundle that wraps the repo MCP server, workflow commands, and `AGENTS.md`.

## Workflow Parity

- Automatic PR review and security review should skip docs-only changes.
- Mention-triggered assistance should only run for trusted commenters on PR context.
- Review and security jobs should refuse fork PR execution before checkout when secrets are in scope.
- Reusable org workflows should be pinned, not referenced via `@main`.
- Cross-provider role projections should be deterministic so drift is reviewable and testable.

## Remaining Intentional Differences

- Claude remains the only provider with native budget, system prompt, agent flag, and max-turn controls.
- Gemini supports native local subagents, remote A2A agents, extension-bundled subagents, and extension-bundled skills, but its concurrency and routing behavior still differs from Codex explicit delegation.
- Codex remains the only provider with native MCP server mode and the default control-plane provider.
- Codex uses explicit subagent delegation and parallel fan-out; Gemini can route via native `.gemini/agents/*.md` roles and prompt-level `@agent-name` delegation.
- Antigravity is intentionally launch-only in ralphglasses. It opens an external interactive session, records a synthetic handoff result, and is excluded from teams, loops, marathon, failover, and structured streaming telemetry.

## Operator Rule

When provider behavior changes, update all three layers in the same change:

1. `internal/session/provider_capabilities.go`
2. MCP tool descriptions, generators, and workflow or config surfaces
3. Operator docs in `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `docs/PROVIDER-SETUP.md`, and any affected role catalog or projection docs
