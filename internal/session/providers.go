package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

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
			warnings = append(warnings, "resume may not be fully supported by codex provider")
		}
	}
	return warnings
}

// ProviderDefaults returns the default model for a given provider.
func ProviderDefaults(p Provider) (model string) {
	switch p {
	case ProviderGemini:
		return "gemini-2.5-pro"
	case ProviderCodex:
		return "o4-mini"
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
	if err := ValidateProvider(p); err != nil {
		return nil, err
	}
	if err := ValidateProviderEnv(p); err != nil {
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

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Strip CLAUDECODE env var so child sessions don't detect nesting and refuse to start.
	cmd.Env = filterEnv(os.Environ(), "CLAUDECODE")

	return cmd
}

// buildGeminiCmd constructs the gemini CLI command.
// Gemini CLI (@google/gemini-cli): -p for headless/pipe mode, --yolo auto-approves tool use.
func buildGeminiCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"-p", "--output-format", "stream-json"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}
	args = append(args, "--yolo")

	cmd := exec.CommandContext(ctx, "gemini", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// buildCodexCmd constructs the codex CLI command.
// Codex CLI: codex exec PROMPT --json --full-auto for headless mode.
// Resume: codex exec resume SESSION_ID. Prompt is a positional arg (not stdin).
func buildCodexCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"exec"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, "--json", "--full-auto")

	if opts.Resume != "" {
		// codex exec resume SESSION_ID
		args = append(args, "resume", opts.Resume)
	} else if opts.Prompt != "" {
		// Prompt is a positional argument after flags
		args = append(args, opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "codex", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}

// normalizeEvent parses a line of streaming output into a StreamEvent,
// dispatching to provider-specific normalizers.
func normalizeEvent(provider Provider, line []byte) (StreamEvent, error) {
	if len(line) == 0 {
		return StreamEvent{}, fmt.Errorf("empty line")
	}
	switch provider {
	case ProviderGemini:
		return normalizeGeminiEvent(line)
	case ProviderCodex:
		return normalizeCodexEvent(line)
	default:
		return normalizeClaudeEvent(line)
	}
}

// normalizeClaudeEvent parses Claude stream-json output.
func normalizeClaudeEvent(line []byte) (StreamEvent, error) {
	var event StreamEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return StreamEvent{}, err
	}
	event.Raw = json.RawMessage(append([]byte(nil), line...))
	return event, nil
}

// normalizeGeminiEvent parses Gemini NDJSON output into StreamEvent.
// Gemini stream-json emits objects with "type", "content", "model", etc.
// We map them to our unified StreamEvent schema.
func normalizeGeminiEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return StreamEvent{}, err
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	if t, ok := raw["type"].(string); ok {
		event.Type = t
	}
	if sid, ok := raw["session_id"].(string); ok {
		event.SessionID = sid
	}
	if m, ok := raw["model"].(string); ok {
		event.Model = m
	}
	if c, ok := raw["content"].(string); ok {
		event.Content = c
	}
	if r, ok := raw["result"].(string); ok {
		event.Result = r
	}
	if cost, ok := raw["cost_usd"].(float64); ok {
		event.CostUSD = cost
	}
	if turns, ok := raw["num_turns"].(float64); ok {
		event.NumTurns = int(turns)
	}
	if dur, ok := raw["duration_seconds"].(float64); ok {
		event.Duration = dur
	}
	if isErr, ok := raw["is_error"].(bool); ok {
		event.IsError = isErr
	}
	if errStr, ok := raw["error"].(string); ok {
		event.Error = errStr
	}

	return event, nil
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

// normalizeCodexEvent parses Codex quiet-mode output into StreamEvent.
// Codex in quiet mode outputs JSON lines with action results.
func normalizeCodexEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return StreamEvent{}, err
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	if t, ok := raw["type"].(string); ok {
		event.Type = t
	}
	if sid, ok := raw["session_id"].(string); ok {
		event.SessionID = sid
	}
	if m, ok := raw["model"].(string); ok {
		event.Model = m
	}
	if c, ok := raw["content"].(string); ok {
		event.Content = c
	}
	if r, ok := raw["result"].(string); ok {
		event.Result = r
	}
	if cost, ok := raw["cost_usd"].(float64); ok {
		event.CostUSD = cost
	}
	if turns, ok := raw["num_turns"].(float64); ok {
		event.NumTurns = int(turns)
	}
	if isErr, ok := raw["is_error"].(bool); ok {
		event.IsError = isErr
	}
	if errStr, ok := raw["error"].(string); ok {
		event.Error = errStr
	}

	return event, nil
}
