package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// buildClineCmd constructs the Cline CLI command from LaunchOptions.
//
// Cline CLI is a multi-provider AI coding agent that supports free and paid
// models via its own auth system (WorkOS OAuth). It reads .clinerules for
// repo-specific instructions and supports MCP servers.
//
// Headless mode flags:
//
//	cline task --yolo --json --cwd <repo_path> [--model MODEL]
//	      [--timeout SECONDS] [--reasoning-effort EFFORT]
//	      [--auto-condense] [--taskId ID] [--continue]
//	      [--hooks-dir PATH] "<PROMPT>"
//
// The --json flag emits NDJSON events on stdout for stream parsing.
// The --yolo flag auto-approves all actions for headless operation.
func buildClineCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"task", "--yolo", "--json"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.RepoPath != "" {
		args = append(args, "--cwd", opts.RepoPath)
	}

	// Map PermissionMode to Cline's mode flags.
	// Default is --yolo (already set above) for headless autonomous operation.
	// Override to --plan for read-only analysis tasks.
	// Use --auto-approve-all for rich-output auto-approve (keeps UI rendering).
	switch strings.ToLower(strings.TrimSpace(opts.PermissionMode)) {
	case "plan", "read-only", "readonly":
		// Replace --yolo with --plan
		for i, a := range args {
			if a == "--yolo" {
				args[i] = "--plan"
				break
			}
		}
	case "auto-approve", "autoapprove":
		// Replace --yolo with --auto-approve-all (keeps rich output).
		for i, a := range args {
			if a == "--yolo" {
				args[i] = "--auto-approve-all"
				break
			}
		}
	}

	// Map Effort to Cline's reasoning effort.
	if opts.Effort != "" {
		effort := mapClineReasoningEffort(opts.Effort)
		if effort != "" {
			args = append(args, "--reasoning-effort", effort)
		}
	}

	// Extended thinking support: --thinking [tokens] for complex reasoning.
	if opts.ThinkingBudget > 0 {
		args = append(args, "--thinking", fmt.Sprintf("%d", opts.ThinkingBudget))
	}

	// MaxTurns maps to --max-consecutive-mistakes as the closest analog.
	// Cline doesn't have a max-turns flag, but this limits runaway loops.
	if opts.MaxTurns > 0 {
		args = append(args, "--max-consecutive-mistakes", fmt.Sprintf("%d", opts.MaxTurns))
	}

	// Timeout support
	if opts.MaxBudgetUSD > 0 {
		// Estimate timeout from budget: assume ~$0.01/turn, 30s/turn average.
		// For free-tier models, use a generous timeout instead.
		timeoutSec := int(opts.MaxBudgetUSD * 3000) // ~$1 = 50 min
		if timeoutSec < 300 {
			timeoutSec = 300 // minimum 5 minutes
		}
		args = append(args, "--timeout", fmt.Sprintf("%d", timeoutSec))
	}

	// Image inputs for multimodal tasks (e.g., UI screenshot analysis).
	if len(opts.Images) > 0 {
		args = append(args, "--images")
		args = append(args, opts.Images...)
	}

	// Resume support
	if opts.Resume != "" {
		args = append(args, "--taskId", opts.Resume)
	} else if opts.Continue {
		args = append(args, "--continue")
	}

	// Per-session config isolation: use --config to prevent concurrent session conflicts.
	// Each session gets its own config directory under .ralph/cline-sessions/.
	if opts.SessionID != "" {
		sessionConfigDir := filepath.Join(opts.RepoPath, ".ralph", "cline-sessions", opts.SessionID)
		if err := os.MkdirAll(sessionConfigDir, 0755); err == nil {
			args = append(args, "--config", sessionConfigDir)
		}
	}

	// Auto-condense for long-running sessions
	args = append(args, "--auto-condense")

	// Hooks directory for runtime hook injection
	hooksDir := filepath.Join(opts.RepoPath, ".ralph", "hooks")
	if info, err := os.Stat(hooksDir); err == nil && info.IsDir() {
		args = append(args, "--hooks-dir", hooksDir)
	}

	// Cline has no dedicated system prompt flag. Emulate it by prefixing the
	// task prompt so the instructions are actually delivered to the model.
	if prompt := buildClinePrompt(opts.SystemPrompt, opts.Prompt); prompt != "" {
		args = append(args, prompt)
	}

	cmd := exec.CommandContext(ctx, "cline", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Build environment with session isolation and optional command sandboxing.
	env := quietAgentSessionEnv(stripClineNestingEnv(os.Environ()))
	env = applyClineCommandPermissions(env, opts)
	cmd.Env = env

	// Stdin pipe support: inject context (diffs, logs, file contents) via stdin.
	if opts.StdinContent != "" {
		cmd.Stdin = strings.NewReader(opts.StdinContent)
	}

	return cmd
}

func buildClinePrompt(systemPrompt, prompt string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	prompt = strings.TrimSpace(prompt)

	switch {
	case systemPrompt == "":
		return prompt
	case prompt == "":
		return systemPrompt
	default:
		return fmt.Sprintf("System instructions:\n%s\n\nTask:\n%s", systemPrompt, prompt)
	}
}

// mapClineReasoningEffort maps generic effort levels to Cline's --reasoning-effort values.
func mapClineReasoningEffort(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low"
	case "medium", "med":
		return "medium"
	case "high":
		return "high"
	case "max", "xhigh":
		return "xhigh"
	case "none", "":
		return "none"
	default:
		return effort
	}
}

// applyClineCommandPermissions sets CLINE_COMMAND_PERMISSIONS to restrict
// shell commands based on task complexity. L1/L2 tasks get restrictive
// permissions; L3+ tasks get broader access.
func applyClineCommandPermissions(env []string, opts LaunchOptions) []string {
	complexity := strings.ToLower(strings.TrimSpace(opts.Complexity))
	var permissions string
	switch complexity {
	case "l1", "trivial":
		// L1: read-only shell commands only
		permissions = `{"allow":["cat *","ls *","find *","grep *","head *","tail *","wc *","echo *"],"deny":["rm *","sudo *","chmod *","chown *","mv *","cp *"],"allowRedirects":false}`
	case "l2", "routine":
		// L2: allow writes to workspace, deny destructive ops
		permissions = `{"allow":["*"],"deny":["rm -rf *","sudo *","chmod 777 *","git push --force*","git push -f*"],"allowRedirects":true}`
	default:
		// L3+: no additional restrictions (Cline defaults apply)
		return env
	}
	return append(env, "CLINE_COMMAND_PERMISSIONS="+permissions)
}

// stripClineNestingEnv removes env vars that cause Cline to detect nesting.
func stripClineNestingEnv(env []string) []string {
	for _, key := range []string{
		"CLINE_TASK_ID",
		"CLINE_SESSION_ID",
		"CLINE_PARENT_TASK",
	} {
		env = filterEnv(env, key)
	}
	return env
}

// normalizeClineEvent parses Cline --json NDJSON output into a StreamEvent.
//
// Cline in --json mode emits JSON lines with structured event data including
// type, content, usage information, and task metadata. The format varies by
// the underlying model provider Cline routes to.
func normalizeClineEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderCline, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	event.Type = firstNonEmptyString(raw, "type", "event", "event_type", "messageType")
	event.SessionID = firstNonEmptyString(raw, "session_id", "taskId", "task_id", "id")
	event.Model = firstNonEmptyString(raw, "model", "metadata.model", "apiConfiguration.model")
	event.Content = firstText(raw, "content", "message", "text", "delta", "output", "say")
	event.Result = firstText(raw, "result", "summary", "final", "completion")
	event.Error = firstText(raw, "error", "error.message", "errorMessage")

	// Cost extraction: Cline may report usage from its underlying providers.
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd", "totalCost")
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderCline, raw)
		if event.CostUSD > 0 {
			event.CostSource = "estimated"
		}
	}

	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns", "apiRequestCount")
	event.CacheReadTokens = firstNonZeroInt(raw, "usage.cache_read_input_tokens", "cacheReads")
	event.CacheWriteTokens = firstNonZeroInt(raw, "usage.cache_creation_input_tokens", "cacheWrites")
	event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds", "elapsedTime")
	event.IsError = firstTrueBool(raw, "is_error", "error", "isError")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)

	applyEventDefaults(&event)
	return event, nil
}

// sanitizeClineStderr strips noise from Cline CLI stderr output.
// Drops Node.js stack traces, npm warnings, and debug lines.
func sanitizeClineStderr(raw string) string {
	var kept []string
	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip JS stack trace frames
		if strings.HasPrefix(trimmed, "at ") {
			continue
		}
		// Skip debug/trace lines
		if strings.HasPrefix(trimmed, "[debug]") || strings.HasPrefix(trimmed, "[trace]") {
			continue
		}
		// Skip npm warnings
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "npm warn") || strings.HasPrefix(lower, "deprecationwarning:") {
			continue
		}
		kept = append(kept, trimmed)
	}
	if len(kept) == 0 {
		return raw
	}
	return strings.Join(kept, "\n")
}
