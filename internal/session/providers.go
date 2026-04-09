package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/google/uuid"
)

const agentSessionQuietEnvVar = "HG_AGENT_SESSION_QUIET"

// estimateCostFromTokens computes a cost estimate from token counts in raw JSON
// when the provider does not emit an explicit cost_usd field. Uses the rates
// from ProviderCostRates (defined in costnorm.go). Returns 0 if no token data
// is found or the provider is unknown.
func estimateCostFromTokens(provider Provider, raw map[string]any) float64 {
	rates, ok := getProviderCostRate(provider)
	if !ok {
		return 0
	}

	// Try provider-specific token count field paths
	inputPaths := []string{"usage.input_tokens", "usage_metadata.prompt_token_count", "usage.prompt_tokens"}
	outputPaths := []string{"usage.output_tokens", "usage_metadata.candidates_token_count", "usage.completion_tokens"}

	var inputTokens, outputTokens float64
	for _, p := range inputPaths {
		if n, ok := asFloat(valueAtPath(raw, p)); ok && n > 0 {
			inputTokens = n
			break
		}
	}
	for _, p := range outputPaths {
		if n, ok := asFloat(valueAtPath(raw, p)); ok && n > 0 {
			outputTokens = n
			break
		}
	}

	if inputTokens == 0 && outputTokens == 0 {
		return 0
	}
	return (inputTokens/1_000_000)*rates.InputPer1M + (outputTokens/1_000_000)*rates.OutputPer1M
}

// ValidateProvider checks that a provider's CLI binary is available on PATH.
func ValidateProvider(p Provider) error {
	// A2A is HTTP-based, not a CLI binary — always valid.
	if p == ProviderA2A {
		return nil
	}
	bin := providerBinary(p)
	if bin == "" {
		return fmt.Errorf("unknown provider: %q (valid: claude, gemini, codex, cline, crush, goose, amp, a2a)", p)
	}
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Errorf("%s binary not found on PATH: %w", bin, err)
	}
	return nil
}

// providerEnvVar returns the environment variable name required for a provider.
func providerEnvVar(p Provider) string {
	switch p {
	case ProviderGemini:
		return "GOOGLE_API_KEY"
	case ProviderCodex:
		return "OPENAI_API_KEY"
	case ProviderCrush:
		return "ANTHROPIC_API_KEY"
	case ProviderGoose:
		return "GOOSE_API_KEY"
	case ProviderAmp:
		return "AMP_ACCESS_TOKEN"
	case ProviderCline:
		return "" // Cline uses WorkOS OAuth, not an env var API key
	case ProviderA2A:
		return "A2A_AGENT_URL" // Not a secret, but used for configuration
	default:
		return "ANTHROPIC_API_KEY"
	}
}

// ValidateProviderEnv checks that the required API key environment variable is set.
func ValidateProviderEnv(p Provider) error {
	// A2A is HTTP-based; agent URL is passed at session launch, not via env var.
	if p == ProviderA2A {
		return nil
	}
	// Cline manages its own auth via WorkOS OAuth (~/.cline/data/settings/providers.json).
	// No environment variable is required.
	if p == ProviderCline {
		return nil
	}
	envVar := providerEnvVar(p)
	if os.Getenv(envVar) == "" {
		// Gemini also accepts GEMINI_API_KEY
		if p == ProviderGemini && os.Getenv("GEMINI_API_KEY") != "" {
			return nil
		}
		// Goose accepts ANTHROPIC_API_KEY or OPENAI_API_KEY as fallbacks
		if p == ProviderGoose && (os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "") {
			return nil
		}
		// Crush accepts OPENAI_API_KEY or GOOGLE_API_KEY as fallbacks
		if p == ProviderCrush && (os.Getenv("OPENAI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "") {
			return nil
		}
		return fmt.Errorf("%s not set (required for provider %q)", envVar, p)
	}
	return nil
}

// UnsupportedOptionsWarnings returns warnings for LaunchOptions fields that are
// not native for the target provider.
func UnsupportedOptionsWarnings(p Provider, opts LaunchOptions) []string {
	if p == "" {
		p = DefaultPrimaryProvider()
	}

	if _, ok := ProviderCapabilityMatrixFor(p); ok {
		var warnings []string
		for _, field := range activeLaunchOptionFields(opts) {
			capability := ProviderCapabilityFor(p, field)
			switch capability.Support {
			case CapabilityNative:
				continue
			case CapabilityInstallDependent:
				if capability.RuntimeAvailable != nil && *capability.RuntimeAvailable {
					continue
				}
			}
			warnings = append(warnings, providerOptionWarning(p, field, capability))
		}
		return warnings
	}

	var warnings []string
	switch p {
	case ProviderCrush, ProviderGoose, ProviderAmp:
		name := string(p)
		if opts.SystemPrompt != "" && p != ProviderCrush {
			warnings = append(warnings, "system_prompt is ignored by "+name+" provider")
		}
		if opts.MaxBudgetUSD > 0 {
			warnings = append(warnings, "max_budget_usd is ignored by "+name+" provider")
		}
		if opts.Agent != "" {
			warnings = append(warnings, "agent is ignored by "+name+" provider")
		}
		if opts.MaxTurns > 0 {
			warnings = append(warnings, "max_turns is ignored by "+name+" provider")
		}
		if len(opts.AllowedTools) > 0 {
			warnings = append(warnings, "allowed_tools is ignored by "+name+" provider")
		}
		if opts.Worktree != "" {
			warnings = append(warnings, "worktree is ignored by "+name+" provider")
		}
	case ProviderA2A:
		if opts.SystemPrompt != "" {
			warnings = append(warnings, "system_prompt is ignored by a2a provider (use agent's own system prompt)")
		}
		if opts.MaxBudgetUSD > 0 {
			warnings = append(warnings, "max_budget_usd is ignored by a2a provider (budget managed by remote agent)")
		}
		if opts.Agent != "" {
			warnings = append(warnings, "agent is ignored by a2a provider (use agent_url instead)")
		}
		if opts.MaxTurns > 0 {
			warnings = append(warnings, "max_turns is ignored by a2a provider")
		}
		if len(opts.AllowedTools) > 0 {
			warnings = append(warnings, "allowed_tools is ignored by a2a provider (tools managed by remote agent)")
		}
		if opts.Worktree != "" {
			warnings = append(warnings, "worktree is ignored by a2a provider")
		}
		if opts.Resume != "" {
			warnings = append(warnings, "resume is ignored by a2a provider (use task_id for continuation)")
		}
	}
	return warnings
}

// ProviderDefaults returns the default model for a given provider.
func ProviderDefaults(p Provider) (model string) {
	switch p {
	case ProviderGemini:
		return "gemini-3.1-pro"
	case ProviderCodex:
		return "gpt-5.4"
	case ProviderCrush:
		return "sonnet"
	case ProviderGoose:
		return "claude-sonnet-4-6"
	case ProviderAmp:
		return "amp-default"
	case ProviderCline:
		return "" // Cline uses its own configured model; empty means use Cline's default
	case ProviderA2A:
		return "a2a-remote"
	default:
		return "sonnet"
	}
}

func providerBinary(p Provider) string {
	switch p {
	case "":
		return providerBinary(DefaultPrimaryProvider())
	case ProviderClaude:
		return "claude"
	case ProviderGemini:
		return "gemini"
	case ProviderCodex:
		return "codex"
	case ProviderCrush:
		return "crush"
	case ProviderGoose:
		return "goose"
	case ProviderAmp:
		return "amp"
	case ProviderCline:
		return "cline"
	default:
		return ""
	}
}

// buildCmdForProvider dispatches to the correct per-provider command builder.
// Returns an error for A2A since it uses HTTP, not CLI subprocesses.
func buildCmdForProvider(ctx context.Context, opts LaunchOptions) (*exec.Cmd, error) {
	p := opts.Provider
	if p == "" {
		p = DefaultPrimaryProvider()
	}
	opts.Provider = p
	if opts.Model == "" {
		opts.Model = ProviderDefaults(p)
	}
	if p == ProviderA2A {
		return nil, fmt.Errorf("a2a provider does not use CLI commands; use launchA2A instead")
	}
	if err := ValidateProvider(p); err != nil {
		return nil, err
	}
	if err := ValidateProviderEnv(p); err != nil {
		return nil, err
	}
	if err := validateLaunchOptions(opts); err != nil {
		return nil, err
	}

	switch p {
	case ProviderGemini:
		return buildGeminiCmd(ctx, opts), nil
	case ProviderCodex:
		return buildCodexCmd(ctx, opts), nil
	case ProviderCrush:
		return buildCrushCmd(ctx, opts), nil
	case ProviderGoose:
		return buildGooseCmd(ctx, opts), nil
	case ProviderAmp:
		return buildAmpCmd(ctx, opts), nil
	case ProviderCline:
		return buildClineCmd(ctx, opts), nil
	default:
		return buildClaudeCmd(ctx, opts), nil
	}
}

// buildClaudeCmd constructs the claude CLI command from LaunchOptions.
// Extracted from the original buildCmd.
func buildClaudeCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"-p", "--verbose", "--output-format", "stream-json"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", opts.MaxBudgetUSD))
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}
	if opts.Agent != "" {
		args = append(args, "--agent", opts.Agent)
	}
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	} else if opts.Continue {
		args = append(args, "--continue")
	}
	if opts.Worktree != "" {
		if opts.Worktree == "true" {
			args = append(args, "-w")
		} else {
			args = append(args, "-w", opts.Worktree)
		}
	}
	// SessionName is tracked internally; Claude CLI has no --name flag.

	if opts.Bare {
		args = append(args, "--bare")
	}
	if opts.Effort != "" {
		args = append(args, "--effort", opts.Effort)
	}
	for _, beta := range opts.Betas {
		args = append(args, "--betas", beta)
	}
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}
	if opts.NoSessionPersistence {
		args = append(args, "--no-session-persistence")
	}
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	if len(opts.OutputSchema) > 0 {
		args = append(args, "--json-schema", string(opts.OutputSchema))
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Strip nesting env vars so child sessions don't detect nesting and refuse to start.
	cmd.Env = quietAgentSessionEnv(os.Environ())

	return cmd
}

// buildGeminiCmd constructs the gemini CLI command.
// Gemini CLI (@google/gemini-cli): gemini [COMMAND] -p/--prompt PROMPT
func buildGeminiCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"--output-format", "stream-json"}

	// JIT Agent / System Prompt handling
	agentName := opts.Agent
	if opts.SystemPrompt != "" {
		// Generate a temporary command/agent name for the system prompt
		jitName := "jit-" + uuid.New().String()[:8]
		jitPath := filepath.Join(opts.RepoPath, ".gemini", "commands", jitName+".toml")
		_ = os.MkdirAll(filepath.Dir(jitPath), 0755)
		content := fmt.Sprintf("description = \"JIT dynamic agent\"\nprompt = %q\n", opts.SystemPrompt)
		if err := os.WriteFile(jitPath, []byte(content), 0644); err == nil {
			agentName = jitName
			// Note: cleanup of JIT file should ideally be handled by the session manager/runner
			// but for now we rely on periodic pruning or subsequent runs.
		}
	}

	if agentName != "" {
		args = append(args, agentName)
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	args = append(args, "--approval-mode", normalizeGeminiApprovalMode(opts.PermissionMode))
	if opts.Worktree != "" {
		if opts.Worktree == "true" {
			args = append(args, "--worktree")
		} else {
			args = append(args, "--worktree", opts.Worktree)
		}
	}
	if opts.Sandbox {
		args = append(args, "--sandbox")
	}
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(opts.AllowedTools, ","))
	}
	// Disable MCP servers in headless mode to prevent recursive spawning.
	// Pass a non-existent name so no servers match.
	args = append(args, "--allowed-mcp-server-names", "__none__")

	// -p/--prompt requires a string value; Gemini appends stdin to it.
	if opts.Prompt != "" {
		args = append(args, "-p", opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = quietAgentSessionEnv(os.Environ())
	return cmd
}

// buildCodexCmd constructs the codex CLI command.
// Codex CLI: codex exec [AGENT] PROMPT --json --full-auto
func buildCodexCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"exec"}

	// JIT Agent / System Prompt handling
	agentName := opts.Agent
	if opts.SystemPrompt != "" {
		jitName := "jit-" + uuid.New().String()[:8]
		jitPath := filepath.Join(opts.RepoPath, ".codex", "agents", jitName+".toml")
		_ = os.MkdirAll(filepath.Dir(jitPath), 0755)
		content := fmt.Sprintf("name = %q\ndescription = \"JIT dynamic agent\"\ndeveloper_instructions = %q\n", jitName, opts.SystemPrompt)
		if err := os.WriteFile(jitPath, []byte(content), 0644); err == nil {
			agentName = jitName
		}
	}

	if agentName != "" {
		args = append(args, agentName)
	}

	if opts.Resume != "" {
		args = append(args, "resume")
	}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// Map Effort to Codex reasoning effort flags
	effort := opts.Effort
	if effort == "" {
		// Default to high reasoning effort for complex tasks
		effort = "high"
		if strings.Contains(strings.ToLower(agentName), "planner") || strings.Contains(strings.ToLower(opts.Prompt), "plan") {
			effort = "max"
		}
	}
	// Note: codex CLI does not support --reasoning-effort yet.
	// switch strings.ToLower(effort) { ... }

	args = append(args, "--json", "--full-auto")
	if sandboxMode := codexSandboxMode(opts); sandboxMode != "" {
		args = append(args, "--sandbox", sandboxMode)
	}
	if len(opts.OutputSchema) > 0 {
		args = append(args, "--output-schema", string(opts.OutputSchema))
	}

	if opts.Resume != "" {
		args = append(args, opts.Resume)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = quietAgentSessionEnv(os.Environ())
	return cmd
}

func validateLaunchOptions(opts LaunchOptions) error {
	if opts.Provider == "" {
		opts.Provider = DefaultPrimaryProvider()
	}
	if err := ValidateModelName(opts.Provider, opts.Model); err != nil {
		return fmt.Errorf("model %q is invalid for %s provider: %w", opts.Model, opts.Provider, err)
	}
	if opts.Provider == ProviderCodex && opts.Resume != "" && !codexExecResumeSupported() {
		return fmt.Errorf("codex provider on this install does not support exec resume")
	}
	if !opts.StrictProviderContract {
		return nil
	}

	if _, ok := ProviderCapabilityMatrixFor(opts.Provider); !ok {
		return nil
	}

	var unsupported []string
	for _, field := range activeLaunchOptionFields(opts) {
		capability := ProviderCapabilityFor(opts.Provider, field)
		switch capability.Support {
		case CapabilityUnsupported:
			unsupported = append(unsupported, field)
		case CapabilityInstallDependent:
			if capability.RuntimeAvailable != nil && !*capability.RuntimeAvailable {
				unsupported = append(unsupported, field)
			}
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("%s provider does not support %s", opts.Provider, strings.Join(unsupported, ", "))
	}
	return nil
}

func providerOptionWarning(provider Provider, field string, capability ProviderCapability) string {
	switch capability.Support {
	case CapabilityUnsupported:
		if capability.Detail != "" {
			return fmt.Sprintf("%s is ignored by %s provider (%s)", field, provider, capability.Detail)
		}
		return fmt.Sprintf("%s is ignored by %s provider", field, provider)
	case CapabilityEmulated:
		if capability.Detail != "" {
			return fmt.Sprintf("%s is emulated for %s provider (%s)", field, provider, capability.Detail)
		}
		return fmt.Sprintf("%s is emulated for %s provider", field, provider)
	case CapabilityInstallDependent:
		if capability.Detail != "" {
			return fmt.Sprintf("%s is install-dependent for %s provider (%s)", field, provider, capability.Detail)
		}
		return fmt.Sprintf("%s is install-dependent for %s provider", field, provider)
	default:
		return fmt.Sprintf("%s is not native for %s provider", field, provider)
	}
}

func activeLaunchOptionFields(opts LaunchOptions) []string {
	fields := make([]string, 0, 10)
	if opts.MaxBudgetUSD > 0 {
		fields = append(fields, CapabilityBudgetUSD)
	}
	if opts.MaxTurns > 0 {
		fields = append(fields, CapabilityMaxTurns)
	}
	if opts.Agent != "" {
		fields = append(fields, CapabilityAgent)
	}
	if len(opts.AllowedTools) > 0 {
		fields = append(fields, CapabilityAllowedTools)
	}
	if opts.SystemPrompt != "" {
		fields = append(fields, CapabilitySystemPrompt)
	}
	if opts.Resume != "" {
		fields = append(fields, CapabilityResume)
	}
	if opts.Worktree != "" {
		fields = append(fields, CapabilityWorktree)
	}
	if opts.PermissionMode != "" {
		fields = append(fields, CapabilityPermissionMode)
	}
	if len(opts.OutputSchema) > 0 {
		fields = append(fields, CapabilityOutputSchema)
	}
	if opts.SandboxImage != "" {
		fields = append(fields, CapabilitySandboxImage)
	}
	return fields
}

func normalizeGeminiApprovalMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "yolo", "danger-full-access", "bypasspermissions", "bypass-permissions":
		return "yolo"
	case "plan", "read-only", "readonly":
		return "plan"
	case "default", "on-request", "never":
		return "default"
	case "auto", "auto_edit", "workspace-write", "acceptedits", "accept-edits", "on-failure":
		return "auto_edit"
	default:
		return mode
	}
}

func codexSandboxMode(opts LaunchOptions) string {
	if opts.Sandbox {
		return "workspace-write"
	}
	switch strings.ToLower(strings.TrimSpace(opts.PermissionMode)) {
	case "":
		return ""
	case "plan", "read-only", "readonly":
		return "read-only"
	case "default", "auto", "auto_edit", "workspace-write", "acceptedits", "accept-edits", "on-failure", "on-request", "never", "yolo", "dontask":
		return "workspace-write"
	case "danger-full-access", "bypasspermissions", "bypass-permissions":
		return "danger-full-access"
	default:
		return opts.PermissionMode
	}
}

var (
	codexResumeSupportOnce sync.Once
	codexResumeSupport     bool
)

var codexExecResumeSupported = func() bool {
	codexResumeSupportOnce.Do(func() {
		cmd := exec.Command("codex", "exec", "resume", "--help")
		codexResumeSupport = cmd.Run() == nil
	})
	return codexResumeSupport
}

// stripNestingEnv removes env vars that cause CLI tools to detect they're running
// inside another CLI session and refuse to start or behave differently.
func stripNestingEnv(env []string) []string {
	// Preserve CODEX_HOME so callers can intentionally select a profile, but
	// drop parent-session markers that can cause child CLIs to attach to the
	// wrong thread or inherit the parent's sandbox/runtime mode.
	for _, key := range []string{
		"CLAUDECODE",
		"CLAUDE_CODE_ENTRYPOINT",
		"CODEX_THREAD_ID",
		"CODEX_CI",
		"CODEX_SANDBOX_NETWORK_DISABLED",
	} {
		env = filterEnv(env, key)
	}
	return env
}

func quietAgentSessionEnv(env []string) []string {
	env = stripNestingEnv(env)
	quietValue := agentSessionQuietEnvVar + "=1"
	prefix := agentSessionQuietEnvVar + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = quietValue
			return env
		}
	}
	return append(env, quietValue)
}

// filterEnv returns a copy of env with any variable whose name matches key removed.
func filterEnv(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}
