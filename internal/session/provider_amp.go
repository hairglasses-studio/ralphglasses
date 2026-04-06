package session

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
)

// buildAmpCmd constructs the Amp CLI command from LaunchOptions.
//
// Amp (by Sourcegraph, spun out as Amp Inc.) is an AI coding agent with deep
// code intelligence from Sourcegraph's code search platform. It supports the
// widest IDE range (VS Code, Cursor, Windsurf, JetBrains, Neovim) and a
// terminal CLI mode. Uses frontier models with no artificial token limits.
//
// Headless mode flags:
//
//	amp run [--model MODEL] [--output-format json] [--non-interactive] [PROMPT]
//
// The "run" subcommand starts a non-interactive coding session. The --output-format
// json flag enables NDJSON streaming output. --non-interactive ensures no TTY
// prompts interrupt headless execution.
func buildAmpCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"run", "--output-format", "json", "--non-interactive"}

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	// Prompt is passed as a positional argument at the end.
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}

	cmd := exec.CommandContext(ctx, "amp", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = stripNestingEnv(os.Environ())
	return cmd
}

// normalizeAmpEvent parses Amp NDJSON output into a StreamEvent.
//
// Amp emits JSON events with fields for type, content, usage, and session
// metadata. The underlying model provider determines the specific field
// names for cost and token data. We try common paths from multiple
// providers since Amp abstracts the model choice.
func normalizeAmpEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderAmp, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	event.Type = firstNonEmptyString(raw, "type", "event", "event_type")
	event.SessionID = firstNonEmptyString(raw, "session_id", "session.id", "id", "thread_id")
	event.Model = firstNonEmptyString(raw, "model", "metadata.model")
	event.Content = firstText(raw, "content", "message", "text", "delta", "output", "response")
	event.Result = firstText(raw, "result", "summary", "final")
	event.Error = firstText(raw, "error", "error.message")

	// Cost extraction: Amp may emit cost from the underlying model provider.
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderAmp, raw)
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
