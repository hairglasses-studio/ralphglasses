package session

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
)

// buildCrushCmd constructs the Crush CLI command from LaunchOptions.
//
// Crush (by Charmbracelet) is an AI coding agent TUI built with Bubble Tea v2.
// Originally forked from OpenCode, it uses a Fantasy abstraction layer for
// multi-model support (OpenAI, Anthropic, Google). Features SQLite session
// persistence, LSP integration, and MCP client support.
//
// Headless mode flags:
//
//	crush --headless --json-output [--model MODEL] [--resume SESSION_ID]
//	      [--system-prompt PROMPT] [PROMPT]
//
// The --headless flag disables the TUI and emits NDJSON events on stdout.
// The --json-output flag ensures structured output compatible with stream parsing.
func buildCrushCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"--headless", "--json-output"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--system-prompt", opts.SystemPrompt)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	} else if opts.Continue {
		args = append(args, "--continue")
	}

	// Prompt is passed as a positional argument at the end.
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "crush", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = stripNestingEnv(os.Environ())
	return cmd
}

// normalizeCrushEvent parses Crush NDJSON output into a StreamEvent.
//
// Crush in headless mode emits JSON lines with fields similar to Claude's
// stream-json format (since both derive from the same model providers).
// The Fantasy abstraction layer normalizes provider responses into a
// consistent shape with type, content, usage, and session metadata.
func normalizeCrushEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderCrush, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	event.Type = firstNonEmptyString(raw, "type", "event", "event_type")
	event.SessionID = firstNonEmptyString(raw, "session_id", "session.id", "id")
	event.Model = firstNonEmptyString(raw, "model", "metadata.model")
	event.Content = firstText(raw, "content", "message", "text", "delta", "output")
	event.Result = firstText(raw, "result", "summary", "final")
	event.Error = firstText(raw, "error", "error.message")

	// Cost extraction: Crush may emit cost via Fantasy provider responses.
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderCrush, raw)
		if event.CostUSD > 0 {
			event.CostSource = "estimated"
		}
	}

	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns")
	event.CacheReadTokens = firstNonZeroInt(raw, "usage.cache_read_input_tokens")
	event.CacheWriteTokens = firstNonZeroInt(raw, "usage.cache_creation_input_tokens")
	event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds")
	event.IsError = firstTrueBool(raw, "is_error", "error")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)

	applyEventDefaults(&event)
	return event, nil
}
