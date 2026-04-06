package session

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// buildGooseCmd constructs the Goose CLI command from LaunchOptions.
//
// Goose (by Block) is an open-source AI coding agent written in Rust. It uses
// MCP as its primary extension mechanism (every extension is an MCP server).
// Multi-model configuration supports cost/performance optimization across
// providers. Features autonomous project scaffolding, workflow orchestration,
// and API interaction.
//
// Headless mode flags:
//
//	goose session run [--model MODEL] [--provider PROVIDER] [--profile PROFILE]
//	                  [--output-format json] [--prompt PROMPT]
//
// The "session run" subcommand starts a non-interactive session. The --output-format
// json flag enables NDJSON streaming output.
func buildGooseCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"session", "run", "--output-format", "json"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	// Prompt is passed via --prompt flag.
	if opts.Prompt != "" {
		args = append(args, "--prompt", opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "goose", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = stripNestingEnv(os.Environ())
	return cmd
}

// normalizeGooseEvent parses Goose NDJSON output into a StreamEvent.
//
// Goose emits JSON events with its own schema. The Rust core provides
// structured output including event types, content, usage metrics, and
// session metadata. We map Goose-specific field names to the unified
// StreamEvent schema.
func normalizeGooseEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderGoose, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	event.Type = firstNonEmptyString(raw, "type", "event", "event_type", "kind")
	event.SessionID = firstNonEmptyString(raw, "session_id", "session.id", "id")
	event.Model = firstNonEmptyString(raw, "model", "metadata.model", "config.model")
	event.Content = firstText(raw, "content", "message", "text", "delta", "output", "response")
	event.Result = firstText(raw, "result", "summary", "final", "response")
	event.Error = firstText(raw, "error", "error.message", "error.detail")

	// Cost extraction: Goose may emit cost from the underlying provider.
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd", "metrics.cost_usd")
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderGoose, raw)
		if event.CostUSD > 0 {
			event.CostSource = "estimated"
		}
	}

	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns", "metrics.turns")
	event.CacheReadTokens = firstNonZeroInt(raw, "usage.cache_read_input_tokens")
	event.CacheWriteTokens = firstNonZeroInt(raw, "usage.cache_creation_input_tokens")
	event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds", "metrics.duration")
	event.IsError = firstTrueBool(raw, "is_error", "error")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)

	applyEventDefaults(&event)
	return event, nil
}

// sanitizeGooseStderr strips Goose-specific noise from stderr output.
// Goose is written in Rust and may emit Rust panic/backtrace frames
// or tracing subscriber output that should be filtered.
func sanitizeGooseStderr(raw string) string {
	var kept []string
	for line := range strings.SplitSeq(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Skip Rust backtrace frames
		if strings.HasPrefix(trimmed, "stack backtrace:") ||
			strings.Contains(trimmed, "thread '") && strings.Contains(trimmed, "panicked at") {
			continue
		}
		// Skip numeric backtrace entries like "  0: ..." or "  1: ..."
		if len(trimmed) > 3 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == ':' {
			continue
		}
		kept = append(kept, trimmed)
	}
	if len(kept) == 0 {
		return raw
	}
	return strings.Join(kept, "\n")
}
