package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
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

	// Secondary raw parse: extract nested fields that don't map to
	// StreamEvent's flat JSON tags. Claude may emit cost in usage.cost_usd,
	// sub-agent events in description/message, etc.
	var raw map[string]any
	if json.Unmarshal(line, &raw) == nil {
		// Cost may be nested under usage (e.g. {"usage":{"cost_usd":0.12}})
		if event.CostUSD == 0 {
			event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")
		}
		if event.CostUSD == 0 {
			event.CostUSD = estimateCostFromTokens(ProviderClaude, raw)
		}
		if event.NumTurns == 0 {
			event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns")
		}
		if event.Duration == 0 {
			event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds")
		}

		// Handle sub-agent events with non-standard field names
		if event.Type == "agent" || event.Type == "subagent" {
			text := firstNonEmpty(
				getString(raw, "description"),
				getString(raw, "message"),
				getString(raw, "content"),
			)
			if text != "" {
				event.Content = text
				event.Text = text
			}
			// Normalize subagent → agent for consistent downstream handling.
			event.Type = "agent"
		}
	}

	return event, nil
}

// getString safely extracts a string value from a map.
func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// normalizeGeminiEvent parses Gemini NDJSON output into StreamEvent.
// Gemini stream-json emits objects with "type", "content", "model", etc.
// We map them to our unified StreamEvent schema.
func normalizeGeminiEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderGemini, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	event.Type = firstNonEmptyString(raw, "type", "event", "event_type")
	event.SessionID = firstNonEmptyString(raw, "session_id", "session.id", "metadata.session_id", "id")
	event.Model = firstNonEmptyString(raw, "model", "metadata.model")
	event.Content = firstText(raw, "content", "message", "text", "delta", "candidate", "response", "output")
	event.Result = firstText(raw, "result", "summary", "final", "response")
	event.Error = firstText(raw, "error", "error.message", "details.error", "details.message")
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderGemini, raw)
	}
	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns")
	event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds")
	event.IsError = firstTrueBool(raw, "is_error", "error")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)
	applyEventDefaults(&event)
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

// sanitizeStderr cleans provider-specific noise from stderr output.
// For Gemini, strips JS stack traces and extracts the actionable error message.
// For other providers, returns the input unchanged.
func sanitizeStderr(provider Provider, raw string) string {
	if raw == "" {
		return raw
	}
	switch provider {
	case ProviderGemini:
		return sanitizeGeminiStderr(raw)
	default:
		return raw
	}
}

// sanitizeGeminiStderr extracts actionable error lines from Gemini CLI's
// Node.js stack traces. Keeps lines matching known error patterns and
// drops "    at " stack frames.
func sanitizeGeminiStderr(raw string) string {
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
		kept = append(kept, trimmed)
	}
	if len(kept) == 0 {
		return raw
	}
	return strings.Join(kept, "\n")
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// cleanProviderOutput extracts human-readable output from stderr for
// providers whose stdout JSON stream may not capture all output.
// For Codex, strips ANSI codes and returns the last non-empty line
// (typically the summary). For other providers, returns empty string.
func cleanProviderOutput(provider Provider, raw string) string {
	if provider != ProviderCodex || raw == "" {
		return ""
	}
	cleaned := ansiRe.ReplaceAllString(raw, "")
	lines := strings.Split(cleaned, "\n")
	// Walk backwards to find the last non-empty line
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

// normalizeCodexEvent parses Codex quiet-mode output into StreamEvent.
// Codex in quiet mode outputs JSON lines with action results.
func normalizeCodexEvent(line []byte) (StreamEvent, error) {
	var raw map[string]any
	if err := json.Unmarshal(line, &raw); err != nil {
		return fallbackTextEvent(ProviderCodex, line)
	}

	event := StreamEvent{
		Raw: json.RawMessage(append([]byte(nil), line...)),
	}

	event.Type = firstNonEmptyString(raw, "type", "event", "item.type")
	event.SessionID = firstNonEmptyString(raw, "session_id", "session.id", "id")
	event.Model = firstNonEmptyString(raw, "model", "metadata.model")
	event.Content = firstText(raw, "content", "message", "output_text", "text", "summary", "delta", "output")
	event.Result = firstText(raw, "result", "summary", "final", "content", "message")
	event.Error = firstText(raw, "error", "error.message", "message.error")
	event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderCodex, raw)
	}
	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns")
	event.IsError = firstTrueBool(raw, "is_error", "error")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)
	applyEventDefaults(&event)
	return event, nil
}

func applyEventDefaults(event *StreamEvent) {
	switch event.Type {
	case "message", "delta", "output":
		event.Type = "assistant"
	case "error":
		event.Type = "result"
		event.IsError = true
	}
	if event.Type == "" {
		switch {
		case event.Error != "" || event.IsError:
			event.Type = "result"
		case event.Result != "":
			event.Type = "result"
		case event.Content != "" || event.Text != "":
			event.Type = "assistant"
		case event.SessionID != "":
			event.Type = "system"
		}
	}
	if event.Text == "" {
		event.Text = firstNonEmpty(event.Content, event.Result, event.Error)
	}
	if event.Content == "" && event.Type == "assistant" {
		event.Content = event.Text
	}
	if event.Result == "" && event.Type == "result" {
		event.Result = event.Text
	}
	if event.Error == "" && event.IsError {
		event.Error = firstNonEmpty(event.Result, event.Content, event.Text)
	}
	if event.Error != "" {
		event.IsError = true
	}
}

func fallbackTextEvent(provider Provider, line []byte) (StreamEvent, error) {
	raw := string(line)
	text := strings.TrimSpace(raw)
	switch provider {
	case ProviderCodex:
		text = cleanProviderOutput(provider, raw)
	case ProviderGemini:
		text = sanitizeStderr(provider, raw)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return StreamEvent{}, fmt.Errorf("unparseable provider output")
	}

	event := StreamEvent{
		Raw:     json.RawMessage(append([]byte(nil), line...)),
		Type:    "assistant",
		Content: text,
		Text:    text,
	}
	lower := strings.ToLower(text)
	if strings.Contains(lower, "error") || strings.Contains(lower, "failed") {
		event.Type = "result"
		event.Result = text
		event.Error = text
		event.IsError = true
	}
	return event, nil
}

func firstNonEmptyString(raw map[string]any, paths ...string) string {
	for _, path := range paths {
		if s := asString(valueAtPath(raw, path)); s != "" {
			return s
		}
	}
	return ""
}

func firstText(raw map[string]any, paths ...string) string {
	for _, path := range paths {
		if s := textValue(valueAtPath(raw, path)); s != "" {
			return s
		}
	}
	return ""
}

func firstNonZeroFloat(raw map[string]any, paths ...string) float64 {
	for _, path := range paths {
		if n, ok := asFloat(valueAtPath(raw, path)); ok && n > 0 {
			return n
		}
	}
	return 0
}

func firstNonZeroInt(raw map[string]any, paths ...string) int {
	for _, path := range paths {
		if n, ok := asInt(valueAtPath(raw, path)); ok && n > 0 {
			return n
		}
	}
	return 0
}

func firstTrueBool(raw map[string]any, paths ...string) bool {
	for _, path := range paths {
		if b, ok := asBool(valueAtPath(raw, path)); ok && b {
			return true
		}
	}
	return false
}

func valueAtPath(raw map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = raw
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[part]
	}
	return cur
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	default:
		return ""
	}
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		n, err := x.Float64()
		return n, err == nil
	case string:
		n, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case json.Number:
		n, err := x.Int64()
		return int(n), err == nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil
	default:
		return 0, false
	}
}

func asBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		n, err := strconv.ParseBool(strings.TrimSpace(x))
		return n, err == nil
	default:
		if textValue(v) != "" {
			return true, true
		}
		return false, false
	}
}

func textValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := textValue(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		for _, key := range []string{"text", "content", "message", "summary", "result", "output_text", "value"} {
			if s := textValue(x[key]); s != "" {
				return s
			}
		}
		if s := textValue(x["parts"]); s != "" {
			return s
		}
		if s := textValue(x["error"]); s != "" {
			return s
		}
		return ""
	default:
		return asString(v)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
