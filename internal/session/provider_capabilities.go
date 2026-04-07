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
	ProjectInstructions string                        `json:"project_instructions"`
	RepoConfigPath      string                        `json:"repo_config_path,omitempty"`
	AgentConfigPath     string                        `json:"agent_config_path,omitempty"`
	Capabilities        map[string]ProviderCapability `json:"capabilities"`
}

// PrimaryProviders returns the primary interactive providers in comparison order.
func PrimaryProviders() []Provider {
	return []Provider{ProviderClaude, ProviderCodex, ProviderGemini}
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
	switch provider {
	case "", ProviderCodex:
		return ProviderCapabilityMatrix{
			Provider:            ProviderCodex,
			Binary:              "codex",
			DefaultModel:        ProviderDefaults(ProviderCodex),
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
			},
		}, true
	case ProviderClaude:
		return ProviderCapabilityMatrix{
			Provider:            ProviderClaude,
			Binary:              "claude",
			DefaultModel:        ProviderDefaults(ProviderClaude),
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
	case ProviderGemini:
		return ProviderCapabilityMatrix{
			Provider:            ProviderGemini,
			Binary:              "gemini",
			DefaultModel:        ProviderDefaults(ProviderGemini),
			ProjectInstructions: "GEMINI.md",
			RepoConfigPath:      ".gemini/settings.json",
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
					Detail:  "The installed Gemini CLI has no --agent flag.",
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
					Detail:  "Gemini reads GEMINI.md and project .gemini/settings.json.",
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
					Detail:  "Gemini exposes `gemini skills`.",
				},
				CapabilityPlugins: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes extensions as its plugin surface.",
				},
				CapabilityHooks: {
					Support: CapabilityNative,
					Detail:  "Gemini exposes `gemini hooks`.",
				},
			},
		}, true
	default:
		return ProviderCapabilityMatrix{}, false
	}
}

// ProviderCapabilityFor returns the capability entry for a provider/key pair.
func ProviderCapabilityFor(provider Provider, key string) ProviderCapability {
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
