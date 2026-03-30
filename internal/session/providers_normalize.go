package session

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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
	// sub-agent events in description/message, content as array of blocks, etc.
	var raw map[string]any
	if json.Unmarshal(line, &raw) == nil {
		// Claude CLI emits assistant messages with content as an array of
		// content blocks (e.g. [{"type":"text","text":"..."}]). The flat
		// json.Unmarshal above silently drops these since Content is a string.
		// Use firstText which recursively extracts text from arrays/objects.
		if event.Content == "" {
			event.Content = firstText(raw, "content", "message", "text")
		}
		if event.Result == "" {
			event.Result = firstText(raw, "result", "summary")
		}
		if event.Text == "" {
			event.Text = firstNonEmpty(event.Content, event.Result)
		}

		// Cost may be nested under usage (e.g. {"usage":{"cost_usd":0.12}})
		if event.CostUSD == 0 {
			event.CostUSD = firstNonZeroFloat(raw, "cost_usd", "usage.cost_usd", "usage.total_cost_usd")
		}
		if event.CostUSD > 0 && event.CostSource == "" {
			event.CostSource = "structured"
		}
		if event.CostUSD == 0 {
			event.CostUSD = estimateCostFromTokens(ProviderClaude, raw)
			if event.CostUSD > 0 {
				event.CostSource = "estimated"
			}
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
				event.Content,
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
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderGemini, raw)
		if event.CostUSD > 0 {
			event.CostSource = "estimated"
		}
	}
	event.NumTurns = firstNonZeroInt(raw, "num_turns", "turns", "usage.turns")
	event.Duration = firstNonZeroFloat(raw, "duration_seconds", "duration", "metadata.duration_seconds")
	event.IsError = firstTrueBool(raw, "is_error", "error")
	event.Text = firstNonEmpty(event.Content, event.Result, event.Error)
	applyEventDefaults(&event)
	return event, nil
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

// stderrCostRe matches common LLM CLI cost output patterns like:
//   - "Cost: $0.0023"
//   - "Total cost: 0.0023"
//   - "cost_usd: $1.23"
//   - "Session cost: $0.05"
var stderrCostRe = regexp.MustCompile(`(?i)(?:total\s+)?(?:session\s+)?cost(?:_usd)?:\s*\$?([\d]+\.[\d]+)`)

// ParseCostFromStderr attempts to extract a cost value from stderr output using
// regex patterns that match common LLM CLI formats. Returns 0 if no cost found.
// This serves as a fallback when structured cost/usage fields are absent.
func ParseCostFromStderr(stderr string) float64 {
	cost, _ := ParseProviderCostFromStderr(ProviderClaude, stderr)
	return cost
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// Provider-specific stderr cost patterns.
var (
	// geminiTokensRe matches "N tokens used" or "prompt_token_count: N, candidates_token_count: M"
	geminiTokensUsedRe = regexp.MustCompile(`(?i)(\d+)\s+tokens?\s+used`)
	geminiTokenCountRe = regexp.MustCompile(`(?i)prompt_token_count:\s*(\d+).*?candidates_token_count:\s*(\d+)`)

	// codexTokensRe matches "Tokens: N input / M output" or "N input tokens, M output tokens"
	codexTokensRe  = regexp.MustCompile(`(?i)(\d+)\s+input\s+tokens?.*?(\d+)\s+output\s+tokens?`)
	codexTokensRe2 = regexp.MustCompile(`(?i)tokens:\s*(\d+)\s+input\s*/\s*(\d+)\s+output`)
)

// ParseProviderCostFromStderr extracts a cost value from stderr using
// provider-specific patterns. Returns the cost and true if found, or 0 and
// false if no cost could be determined.
//
// For Claude: looks for "Cost: $X.XX", "Total cost: X.XX" patterns.
// For Gemini: looks for token count patterns and estimates cost from rates.
// For Codex: looks for cost patterns first, then token count patterns.
func ParseProviderCostFromStderr(provider Provider, stderr string) (float64, bool) {
	if stderr == "" {
		return 0, false
	}
	cleaned := ansiRe.ReplaceAllString(stderr, "")

	// All providers: try the universal cost regex first
	if matches := stderrCostRe.FindAllStringSubmatch(cleaned, -1); len(matches) > 0 {
		last := matches[len(matches)-1]
		if cost, err := strconv.ParseFloat(last[1], 64); err == nil && cost > 0 {
			return cost, true
		}
	}

	// Provider-specific token-based cost estimation
	switch provider {
	case ProviderGemini:
		return parseGeminiStderrCost(cleaned)
	case ProviderCodex:
		return parseCodexStderrCost(cleaned)
	}

	return 0, false
}

// parseGeminiStderrCost extracts cost from Gemini-specific stderr patterns.
func parseGeminiStderrCost(cleaned string) (float64, bool) {
	rates, ok := getProviderCostRate(ProviderGemini)
	if !ok {
		return 0, false
	}

	// Try "prompt_token_count: N, candidates_token_count: M"
	if m := geminiTokenCountRe.FindStringSubmatch(cleaned); len(m) == 3 {
		input, err1 := strconv.ParseFloat(m[1], 64)
		output, err2 := strconv.ParseFloat(m[2], 64)
		if err1 == nil && err2 == nil && (input > 0 || output > 0) {
			cost := (input/1_000_000)*rates.InputPer1M + (output/1_000_000)*rates.OutputPer1M
			return cost, true
		}
	}

	// Try "N tokens used" (assume 50/50 input/output split)
	if m := geminiTokensUsedRe.FindStringSubmatch(cleaned); len(m) == 2 {
		total, err := strconv.ParseFloat(m[1], 64)
		if err == nil && total > 0 {
			blended := (rates.InputPer1M + rates.OutputPer1M) / 2
			cost := (total / 1_000_000) * blended
			return cost, true
		}
	}

	return 0, false
}

// parseCodexStderrCost extracts cost from Codex-specific stderr patterns.
func parseCodexStderrCost(cleaned string) (float64, bool) {
	rates, ok := getProviderCostRate(ProviderCodex)
	if !ok {
		return 0, false
	}

	// Try "N input tokens, M output tokens" or "Tokens: N input / M output"
	for _, re := range []*regexp.Regexp{codexTokensRe, codexTokensRe2} {
		if m := re.FindStringSubmatch(cleaned); len(m) == 3 {
			input, err1 := strconv.ParseFloat(m[1], 64)
			output, err2 := strconv.ParseFloat(m[2], 64)
			if err1 == nil && err2 == nil && (input > 0 || output > 0) {
				cost := (input/1_000_000)*rates.InputPer1M + (output/1_000_000)*rates.OutputPer1M
				return cost, true
			}
		}
	}

	return 0, false
}

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
	if event.CostUSD > 0 {
		event.CostSource = "structured"
	}
	if event.CostUSD == 0 {
		event.CostUSD = estimateCostFromTokens(ProviderCodex, raw)
		if event.CostUSD > 0 {
			event.CostSource = "estimated"
		}
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
