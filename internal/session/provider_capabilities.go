package session

// CapabilitySupport describes how ralphglasses can satisfy a provider feature.
type CapabilitySupport string

const (
	CapabilityNative           CapabilitySupport = "native"
	CapabilityEmulated         CapabilitySupport = "emulated"
	CapabilityInstallDependent CapabilitySupport = "install_dependent"
	CapabilityUnsupported      CapabilitySupport = "unsupported"
)

const (
	CapabilityBudgetUSD           = "budget_usd"
	CapabilityMaxTurns            = "max_turns"
	CapabilityAgent               = "agent"
	CapabilityAllowedTools        = "allowed_tools"
	CapabilitySystemPrompt        = "system_prompt"
	CapabilityResume              = "resume"
	CapabilityWorktree            = "worktree"
	CapabilityPermissionMode      = "permission_mode"
	CapabilityOutputSchema        = "output_schema"
	CapabilitySandboxImage        = "sandbox_image"
	CapabilityProjectInstructions = "project_instructions"
	CapabilityMCPClient           = "mcp_client"
	CapabilityMCPServer           = "mcp_server"
	CapabilitySkills              = "skills"
	CapabilityPlugins             = "plugins"
	CapabilitySubagents           = "subagents"
	CapabilityHooks               = "hooks"
)

// ProviderCapability describes one feature for a provider.
type ProviderCapability struct {
	Support          CapabilitySupport `json:"support"`
	Detail           string            `json:"detail,omitempty"`
	Values           []string          `json:"values,omitempty"`
	RuntimeAvailable *bool             `json:"runtime_available,omitempty"`
}

// ProviderCapabilityMatrix is the provider-facing capability summary exposed
// through runtime APIs and used internally for validation/warnings.
type ProviderCapabilityMatrix struct {
	Provider            Provider                      `json:"provider"`
	Binary              string                        `json:"binary"`
	DefaultModel        string                        `json:"default_model"`
	ExecutionModel      string                        `json:"execution_model,omitempty"`
	Experimental        bool                          `json:"experimental,omitempty"`
	ProjectInstructions string                        `json:"project_instructions"`
	RepoConfigPath      string                        `json:"repo_config_path,omitempty"`
	AgentConfigPath     string                        `json:"agent_config_path,omitempty"`
	Capabilities        map[string]ProviderCapability `json:"capabilities"`
}

// PrimaryProviders returns the primary interactive providers in comparison order.
func PrimaryProviders() []Provider {
	return []Provider{ProviderClaude, ProviderOllama, ProviderCodex, ProviderGemini, ProviderAntigravity, ProviderCline}
}

// ProviderCapabilityMatrices returns the capability matrix for all primary providers.
func ProviderCapabilityMatrices() []ProviderCapabilityMatrix {
	providers := PrimaryProviders()
	out := make([]ProviderCapabilityMatrix, 0, len(providers))
	for _, provider := range providers {
		matrix, ok := ProviderCapabilityMatrixFor(provider)
		if ok {
			out = append(out, matrix)
		}
	}
	return out
}

// ProviderCapabilityMatrixFor returns the capability matrix for a provider.
func ProviderCapabilityMatrixFor(provider Provider) (ProviderCapabilityMatrix, bool) {
	provider = normalizeSessionProvider(provider)
	switch provider {
	case "", ProviderCodex:
		return ProviderCapabilityMatrix{
			Provider:            ProviderCodex,
			Binary:              "codex",
			DefaultModel:        ProviderDefaults(ProviderCodex),
			ExecutionModel:      "streaming_cli",
			ProjectInstructions: "AGENTS.md",
			RepoConfigPath:      ".codex/config.toml",
			AgentConfigPath:     ".codex/agents/*.toml",
			Capabilities: map[string]ProviderCapability{
				CapabilityBudgetUSD: {
					Support: CapabilityEmulated,
					Detail:  "ralphglasses enforces budget limits externally because Codex CLI has no budget flag.",
				},
				CapabilityMaxTurns: {
					Support: CapabilityUnsupported,
					Detail:  "Codex exec has no max-turns flag.",
				},
				CapabilityAgent: {
					Support: CapabilityUnsupported,
					Detail:  "Use .codex/agents/*.toml or repo instructions instead of a Codex exec flag.",
				},
				CapabilityAllowedTools: {
					Support: CapabilityUnsupported,
					Detail:  "Codex exec does not expose an allowed-tools flag.",
				},
				CapabilitySystemPrompt: {
					Support: CapabilityUnsupported,
					Detail:  "Use AGENTS.md for repo instructions; Codex exec has no system-prompt flag.",
				},
				CapabilityResume: {
					Support:          CapabilityInstallDependent,
					Detail:           "Codex resume depends on the installed CLI exposing `codex exec resume`.",
					RuntimeAvailable: boolPtr(codexExecResumeSupported()),
				},
				CapabilityWorktree: {
					Support: CapabilityUnsupported,
					Detail:  "Codex CLI has no --worktree flag.",
				},
				CapabilityPermissionMode: {
					Support: CapabilityEmulated,
					Detail:  "ralphglasses maps generic permission modes onto Codex sandbox policies.",
					Values:  []string{"plan->read-only", "default/auto->workspace-write", "bypassPermissions->danger-full-access"},
				},
				CapabilityOutputSchema: {
					Support: CapabilityNative,
					Detail:  "Codex exec exposes --output-schema.",
				},
				CapabilitySandboxImage: {
					Support: CapabilityUnsupported,
					Detail:  "Codex CLI supports sandbox modes, not Docker image selection.",
				},
				CapabilityProjectInstructions: {
					Support: CapabilityNative,
					Detail:  "Codex reads AGENTS.md automatically.",
				},
				CapabilityMCPClient: {
					Support: CapabilityNative,
					Detail:  "Codex reads MCP servers from .codex/config.toml.",
				},
				CapabilityMCPServer: {
					Support: CapabilityNative,
					Detail:  "Codex exposes `codex mcp-server`.",
				},
				CapabilitySkills: {
					Support: CapabilityNative,
					Detail:  "Codex supports project skills.",
				},
				CapabilityPlugins: {
					Support: CapabilityNative,
					Detail:  "Codex supports plugins and plugin manifests.",
				},
				CapabilitySubagents: {
					Support: CapabilityNative,
					Detail:  "Codex project subagents live in .codex/agents/*.toml.",
				},
				CapabilityHooks: {
					Support: CapabilityUnsupported,
					Detail:  "Codex CLI does not expose a repo hook system comparable to Claude or Gemini.",
				},
			},
		}, true
	case ProviderClaude:
		return ProviderCapabilityMatrix{
			Provider:            ProviderClaude,
			Binary:              "claude",
			DefaultModel:        ProviderDefaults(ProviderClaude),
			ExecutionModel:      "streaming_cli",
			ProjectInstructions: "CLAUDE.md",
			RepoConfigPath:      ".claude/settings.json",
			AgentConfigPath:     ".claude/agents/*.md",
			Capabilities: map[string]ProviderCapability{
				CapabilityBudgetUSD: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --max-budget-usd.",
				},
				CapabilityMaxTurns: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --max-turns.",
				},
				CapabilityAgent: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --agent and project subagents in .claude/agents/*.md.",
				},
				CapabilityAllowedTools: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --allowedTools / --allowed-tools.",
				},
				CapabilitySystemPrompt: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --append-system-prompt and --system-prompt.",
				},
				CapabilityResume: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --resume.",
				},
				CapabilityWorktree: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --worktree.",
				},
				CapabilityPermissionMode: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --permission-mode.",
				},
				CapabilityOutputSchema: {
					Support: CapabilityNative,
					Detail:  "Claude exposes --json-schema.",
				},
				CapabilitySandboxImage: {
					Support: CapabilityUnsupported,
					Detail:  "Claude CLI does not accept a Docker sandbox image override.",
				},
				CapabilityProjectInstructions: {
					Support: CapabilityNative,
					Detail:  "Claude reads CLAUDE.md by default.",
				},
				CapabilityMCPClient: {
					Support: CapabilityNative,
					Detail:  "Claude can load MCP servers via .mcp.json or --mcp-config.",
				},
				CapabilityMCPServer: {
					Support: CapabilityUnsupported,
					Detail:  "Claude acts as an MCP client, not an MCP server.",
				},
				CapabilitySkills: {
					Support: CapabilityNative,
					Detail:  "Claude supports project skills.",
				},
				CapabilityPlugins: {
					Support: CapabilityNative,
					Detail:  "Claude supports plugins and --plugin-dir.",
				},
				CapabilitySubagents: {
					Support: CapabilityNative,
					Detail:  "Claude project subagents live in .claude/agents/*.md.",
				},
				CapabilityHooks: {
					Support: CapabilityNative,
					Detail:  "Claude supports hooks through settings.json.",
				},
			},
		}, true
	case ProviderOllama:
		return ProviderCapabilityMatrix{
			Provider:            ProviderOllama,
			Binary:              "claude",
			DefaultModel:        ProviderDefaults(ProviderOllama),
			ExecutionModel:      "streaming_cli",
			Experimental:        true,
			ProjectInstructions: "CLAUDE.md",
			RepoConfigPath:      ".claude/settings.json",
			AgentConfigPath:     ".claude/agents/*.md",
			Capabilities: map[string]ProviderCapability{
				CapabilityBudgetUSD: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI flags. Local billing is not exposed by the backend, so ralphglasses reports zero provider-side token cost.",
				},
				CapabilityMaxTurns: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI --max-turns.",
				},
				CapabilityAgent: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI subagents in .claude/agents/*.md.",
				},
				CapabilityAllowedTools: {
					Support: CapabilityNative,
					Detail:  "Claude CLI still enforces tool allowlists, but backend-side tool forcing guarantees depend on Ollama's compatibility layer.",
				},
				CapabilitySystemPrompt: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI system-prompt flags.",
				},
				CapabilityResume: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI resume support.",
				},
				CapabilityWorktree: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI --worktree.",
				},
				CapabilityPermissionMode: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI --permission-mode.",
				},
				CapabilityOutputSchema: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude CLI --json-schema, subject to compatibility-layer fidelity.",
				},
				CapabilitySandboxImage: {
					Support: CapabilityUnsupported,
					Detail:  "Claude CLI does not accept a Docker sandbox image override.",
				},
				CapabilityProjectInstructions: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions still read CLAUDE.md because the runtime path is Claude CLI plus an Ollama compatibility env overlay.",
				},
				CapabilityMCPClient: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions still use Claude's MCP client support.",
				},
				CapabilityMCPServer: {
					Support: CapabilityUnsupported,
					Detail:  "The runtime is still an MCP client, not an MCP server.",
				},
				CapabilitySkills: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude project skills.",
				},
				CapabilityPlugins: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude plugin loading.",
				},
				CapabilitySubagents: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude subagents, with backend semantics governed by the local Anthropic-compatible endpoint.",
				},
				CapabilityHooks: {
					Support: CapabilityNative,
					Detail:  "Ollama sessions reuse Claude hooks.",
				},
			},
		}, true
	case ProviderGemini:
		return ProviderCapabilityMatrix{
			Provider:            ProviderGemini,
			Binary:              "gemini",
			DefaultModel:        ProviderDefaults(ProviderGemini),
			ExecutionModel:      "streaming_cli",
			ProjectInstructions: "GEMINI.md",
			RepoConfigPath:      ".gemini/settings.json",
			AgentConfigPath:     ".gemini/agents/*.md",
			Capabilities: map[string]ProviderCapability{
				CapabilityBudgetUSD: {
					Support: CapabilityEmulated,
					Detail:  "ralphglasses tracks and enforces budget externally because Gemini CLI has no budget flag.",
				},
				CapabilityMaxTurns: {
					Support: CapabilityUnsupported,
					Detail:  "Gemini CLI has no max-turns flag.",
				},
				CapabilityAgent: {
					Support: CapabilityUnsupported,
					Detail:  "Gemini has no `--agent` flag, but supports native `.gemini/agents/*.md` roles and prompt-level `@agent-name` routing.",
				},
				CapabilityAllowedTools: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes --allowed-tools, though it is deprecated in favor of policy files.",
				},
				CapabilitySystemPrompt: {
					Support: CapabilityUnsupported,
					Detail:  "Use GEMINI.md for repo instructions; Gemini CLI has no system-prompt flag.",
				},
				CapabilityResume: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes --resume.",
				},
				CapabilityWorktree: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes --worktree.",
				},
				CapabilityPermissionMode: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes --approval-mode.",
					Values:  []string{"default", "auto_edit", "yolo", "plan"},
				},
				CapabilityOutputSchema: {
					Support: CapabilityUnsupported,
					Detail:  "Gemini CLI has no output-schema flag.",
				},
				CapabilitySandboxImage: {
					Support: CapabilityUnsupported,
					Detail:  "Gemini supports sandbox mode, not Docker image selection.",
				},
				CapabilityProjectInstructions: {
					Support: CapabilityNative,
					Detail:  "Gemini reads GEMINI.md, project .gemini/settings.json, and native .gemini/agents/*.md role surfaces.",
				},
				CapabilityMCPClient: {
					Support: CapabilityNative,
					Detail:  "Gemini supports MCP via settings.json and `gemini mcp`.",
				},
				CapabilityMCPServer: {
					Support: CapabilityUnsupported,
					Detail:  "Gemini CLI acts as an MCP client, not an MCP server.",
				},
				CapabilitySkills: {
					Support: CapabilityNative,
					Detail:  "Gemini supports agent skills plus extension-bundled skills.",
				},
				CapabilityPlugins: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes extensions as its plugin surface.",
				},
				CapabilitySubagents: {
					Support: CapabilityNative,
					Detail:  "Gemini supports local `.gemini/agents/*.md` subagents, remote A2A agents, and extension-bundled subagents.",
				},
				CapabilityHooks: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes `gemini hooks`.",
				},
			},
		}, true
	case ProviderAntigravity:
		return ProviderCapabilityMatrix{
			Provider:            ProviderAntigravity,
			Binary:              "antigravity",
			DefaultModel:        "",
			ExecutionModel:      "external_manager",
			Experimental:        true,
			ProjectInstructions: "AGENTS.md + .agents/rules/ralphglasses.md",
			RepoConfigPath:      ".mcp.json",
			AgentConfigPath:     ".agents/workflows/*.md",
			Capabilities: map[string]ProviderCapability{
				CapabilityBudgetUSD: {
					Support: CapabilityEmulated,
					Detail:  "ralphglasses tracks Antigravity budget intent externally because the public Antigravity CLI does not expose a budget flag.",
				},
				CapabilityMaxTurns: {
					Support: CapabilityUnsupported,
					Detail:  "Antigravity's public CLI does not expose a max-turns flag.",
				},
				CapabilityAgent: {
					Support: CapabilityUnsupported,
					Detail:  "Use repo-native workflows, skills, and rules instead of a runtime --agent flag.",
				},
				CapabilityAllowedTools: {
					Support: CapabilityUnsupported,
					Detail:  "Tool permissions are managed inside Antigravity, not via a CLI allowlist flag.",
				},
				CapabilitySystemPrompt: {
					Support: CapabilityUnsupported,
					Detail:  "Use AGENTS.md, .agents/rules, and .agents/workflows; the public Antigravity CLI has no system-prompt flag.",
				},
				CapabilityResume: {
					Support: CapabilityUnsupported,
					Detail:  "ralphglasses only supports Antigravity as an external handoff launch, not a resumable managed session.",
				},
				CapabilityWorktree: {
					Support: CapabilityUnsupported,
					Detail:  "Antigravity uses the selected workspace path directly and exposes no worktree flag through the public CLI.",
				},
				CapabilityPermissionMode: {
					Support: CapabilityUnsupported,
					Detail:  "Permissions are managed in the Antigravity UI rather than through a launch flag.",
				},
				CapabilityOutputSchema: {
					Support: CapabilityUnsupported,
					Detail:  "Antigravity launches an interactive managed session and does not expose a structured output-schema flag.",
				},
				CapabilitySandboxImage: {
					Support: CapabilityUnsupported,
					Detail:  "The public Antigravity CLI does not accept a sandbox image override.",
				},
				CapabilityProjectInstructions: {
					Support: CapabilityNative,
					Detail:  "Antigravity reads repo instructions from AGENTS.md plus repo-native .agents/rules and .agents/workflows surfaces.",
				},
				CapabilityMCPClient: {
					Support: CapabilityNative,
					Detail:  "Antigravity can consume repo and global MCP server registrations.",
				},
				CapabilityMCPServer: {
					Support: CapabilityUnsupported,
					Detail:  "Antigravity is an MCP client, not an MCP server.",
				},
				CapabilitySkills: {
					Support: CapabilityNative,
					Detail:  "Antigravity supports repo-native skills and generated workflow surfaces.",
				},
				CapabilityPlugins: {
					Support: CapabilityNative,
					Detail:  "Gemini-style extension bundles are the supported Antigravity plugin surface.",
				},
				CapabilitySubagents: {
					Support: CapabilityUnsupported,
					Detail:  "Antigravity may orchestrate internally, but ralphglasses does not expose it as a managed subagent runtime.",
				},
				CapabilityHooks: {
					Support: CapabilityUnsupported,
					Detail:  "The public Antigravity CLI does not expose repo hooks compatible with ralphglasses runtime semantics.",
				},
			},
		}, true
	case ProviderCline:
		return ProviderCapabilityMatrix{
			Provider:            ProviderCline,
			Binary:              "cline",
			DefaultModel:        ProviderDefaults(ProviderCline),
			ExecutionModel:      "streaming_cli",
			ProjectInstructions: ".clinerules",
			RepoConfigPath:      ".cline/mcp.json",
			AgentConfigPath:     ".clinerules",
			Capabilities: map[string]ProviderCapability{
				CapabilityBudgetUSD: {
					Support: CapabilityEmulated,
					Detail:  "ralphglasses enforces budget limits externally; Cline CLI has no budget flag.",
				},
				CapabilityMaxTurns: {
					Support: CapabilityEmulated,
					Detail:  "Mapped to --max-consecutive-mistakes as closest analog.",
				},
				CapabilityAgent: {
					Support: CapabilityUnsupported,
					Detail:  "Cline has no --agent flag; use .clinerules for repo-specific instructions.",
				},
				CapabilityAllowedTools: {
					Support: CapabilityUnsupported,
					Detail:  "Cline CLI does not expose an allowed-tools flag.",
				},
				CapabilitySystemPrompt: {
					Support: CapabilityEmulated,
					Detail:  "Emulated by prefixing system instructions into the task prompt because Cline has no dedicated system-prompt flag.",
				},
				CapabilityResume: {
					Support: CapabilityNative,
					Detail:  "Cline exposes --taskId for resume and --continue for continuation.",
				},
				CapabilityWorktree: {
					Support: CapabilityUnsupported,
					Detail:  "Cline CLI has no worktree support.",
				},
				CapabilityPermissionMode: {
					Support: CapabilityNative,
					Detail:  "Cline exposes --yolo (auto-approve), --plan (read-only), and --act modes.",
					Values:  []string{"yolo", "plan", "act"},
				},
				CapabilityOutputSchema: {
					Support: CapabilityUnsupported,
					Detail:  "Cline CLI has no output-schema flag.",
				},
				CapabilitySandboxImage: {
					Support: CapabilityUnsupported,
					Detail:  "Cline CLI has no sandbox/container support.",
				},
				CapabilityProjectInstructions: {
					Support: CapabilityNative,
					Detail:  "Cline reads .clinerules and AGENTS.md from the repo root automatically.",
				},
				CapabilityMCPClient: {
					Support: CapabilityNative,
					Detail:  "Cline supports MCP servers via .cline/mcp.json and `cline mcp add`.",
				},
				CapabilityMCPServer: {
					Support: CapabilityUnsupported,
					Detail:  "Cline CLI acts as an MCP client, not an MCP server.",
				},
				CapabilitySkills: {
					Support: CapabilityNative,
					Detail:  "Cline supports skills via its skill loading mechanism.",
				},
				CapabilityPlugins: {
					Support: CapabilityUnsupported,
					Detail:  "Cline has no plugin system.",
				},
				CapabilitySubagents: {
					Support: CapabilityUnsupported,
					Detail:  "Cline CLI does not support native subagents.",
				},
				CapabilityHooks: {
					Support: CapabilityNative,
					Detail:  "Cline exposes --hooks-dir for runtime hook injection.",
				},
			},
		}, true
	default:
		return ProviderCapabilityMatrix{}, false
	}
}

// ProviderCapabilityFor returns the capability entry for a provider/key pair.
func ProviderCapabilityFor(provider Provider, key string) ProviderCapability {
	provider = normalizeSessionProvider(provider)
	matrix, ok := ProviderCapabilityMatrixFor(provider)
	if !ok {
		return ProviderCapability{Support: CapabilityUnsupported}
	}
	capability, ok := matrix.Capabilities[key]
	if !ok {
		return ProviderCapability{Support: CapabilityUnsupported}
	}
	return capability
}

// ProviderCapabilityConstraints returns operator-facing caveats for a provider.
func ProviderCapabilityConstraints(provider Provider) []string {
	provider = normalizeSessionProvider(provider)
	keys := []string{
		CapabilityBudgetUSD,
		CapabilitySystemPrompt,
		CapabilityAgent,
		CapabilityMaxTurns,
		CapabilityAllowedTools,
		CapabilityWorktree,
		CapabilityPermissionMode,
		CapabilityOutputSchema,
		CapabilitySandboxImage,
		CapabilitySubagents,
		CapabilityHooks,
		CapabilityMCPServer,
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		capability := ProviderCapabilityFor(provider, key)
		switch capability.Support {
		case CapabilityNative:
			continue
		case CapabilityInstallDependent:
			if capability.RuntimeAvailable != nil && *capability.RuntimeAvailable {
				continue
			}
			if capability.Detail != "" {
				out = append(out, key+": "+capability.Detail)
			}
		default:
			if capability.Detail != "" {
				out = append(out, key+": "+capability.Detail)
			}
		}
	}
	return out
}

func boolPtr(v bool) *bool {
	return &v
}
