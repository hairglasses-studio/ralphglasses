package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// estimateCostFromTokens computes a cost estimate from token counts in raw JSON
// when the provider does not emit an explicit cost_usd field. Uses the rates
// from ProviderCostRates (defined in costnorm.go). Returns 0 if no token data
// is found or the provider is unknown.
func estimateCostFromTokens(provider Provider, raw map[string]any) float64 {
	rates, ok := ProviderCostRates[provider]
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
	bin := providerBinary(p)
	if bin == "" {
		return fmt.Errorf("unknown provider: %q (valid: claude, gemini, codex)", p)
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
	default:
		return "ANTHROPIC_API_KEY"
	}
}

// ValidateProviderEnv checks that the required API key environment variable is set.
func ValidateProviderEnv(p Provider) error {
	envVar := providerEnvVar(p)
	if os.Getenv(envVar) == "" {
		// Gemini also accepts GEMINI_API_KEY
		if p == ProviderGemini && os.Getenv("GEMINI_API_KEY") != "" {
			return nil
		}
		return fmt.Errorf("%s not set (required for provider %q)", envVar, p)
	}
	return nil
}

// UnsupportedOptionsWarnings returns warnings for LaunchOptions fields that are
// set but ignored by the given provider. Returns nil for Claude (supports all).
func UnsupportedOptionsWarnings(p Provider, opts LaunchOptions) []string {
	if p == ProviderClaude || p == "" {
		return nil
	}

	var warnings []string
	switch p {
	case ProviderGemini:
		if opts.SystemPrompt != "" {
			warnings = append(warnings, "system_prompt is ignored by gemini provider")
		}
		if opts.MaxBudgetUSD > 0 {
			warnings = append(warnings, "max_budget_usd is ignored by gemini provider")
		}
		if opts.Agent != "" {
			warnings = append(warnings, "agent is ignored by gemini provider (use .gemini/agents/ instead)")
		}
		if opts.MaxTurns > 0 {
			warnings = append(warnings, "max_turns is ignored by gemini provider")
		}
		if len(opts.AllowedTools) > 0 {
			warnings = append(warnings, "allowed_tools is ignored by gemini provider")
		}
		if opts.Worktree != "" {
			warnings = append(warnings, "worktree is ignored by gemini provider")
		}
	case ProviderCodex:
		if opts.SystemPrompt != "" {
			warnings = append(warnings, "system_prompt is ignored by codex provider")
		}
		if opts.MaxBudgetUSD > 0 {
			warnings = append(warnings, "max_budget_usd is ignored by codex provider")
		}
		if opts.Agent != "" {
			warnings = append(warnings, "agent is ignored by codex provider")
		}
		if opts.MaxTurns > 0 {
			warnings = append(warnings, "max_turns is ignored by codex provider")
		}
		if len(opts.AllowedTools) > 0 {
			warnings = append(warnings, "allowed_tools is ignored by codex provider")
		}
		if opts.Worktree != "" {
			warnings = append(warnings, "worktree is ignored by codex provider")
		}
		if opts.Resume != "" {
			warnings = append(warnings, "resume is unsupported by codex provider")
		}
	}
	return warnings
}

// ProviderDefaults returns the default model for a given provider.
func ProviderDefaults(p Provider) (model string) {
	switch p {
	case ProviderGemini:
		return "gemini-3-pro"
	case ProviderCodex:
		return "gpt-5.4-xhigh"
	default:
		return "sonnet"
	}
}

func providerBinary(p Provider) string {
	switch p {
	case ProviderClaude, "":
		return "claude"
	case ProviderGemini:
		return "gemini"
	case ProviderCodex:
		return "codex"
	default:
		return ""
	}
}

// buildCmdForProvider dispatches to the correct per-provider command builder.
func buildCmdForProvider(ctx context.Context, opts LaunchOptions) (*exec.Cmd, error) {
	p := opts.Provider
	if p == "" {
		p = ProviderClaude
	}
	opts.Provider = p
	if opts.Model == "" {
		opts.Model = ProviderDefaults(p)
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
	if len(opts.OutputSchema) > 0 {
		args = append(args, "--json-schema", string(opts.OutputSchema))
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Strip CLAUDECODE env var so child sessions don't detect nesting and refuse to start.
	cmd.Env = filterEnv(os.Environ(), "CLAUDECODE")

	return cmd
}

// buildGeminiCmd constructs the gemini CLI command.
// Gemini CLI (@google/gemini-cli): -p/--prompt PROMPT for headless mode,
// --yolo auto-approves tool use.
func buildGeminiCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"--output-format", "stream-json"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	args = append(args, "--approval-mode", "yolo")

	// -p/--prompt requires a string value; Gemini appends stdin to it.
	if opts.Prompt != "" {
		args = append(args, "-p", opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// buildCodexCmd constructs the codex CLI command.
// Codex CLI: codex exec PROMPT --json --full-auto for headless mode.
func buildCodexCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"exec"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, "--json", "--full-auto")
	if len(opts.OutputSchema) > 0 {
		args = append(args, "--output-schema", string(opts.OutputSchema))
	}

	if opts.Prompt != "" {
		// Prompt is a positional argument after flags
		args = append(args, opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

func validateLaunchOptions(opts LaunchOptions) error {
	if opts.Provider == ProviderCodex && opts.Resume != "" {
		return fmt.Errorf("codex provider does not support resume")
	}
	return nil
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
