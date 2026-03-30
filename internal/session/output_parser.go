package session

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// ParsedOutput is the structured result of parsing LLM session output.
// It aggregates files modified, test results, errors, git operations, and
// token/cost usage extracted from a stream of output lines.
type ParsedOutput struct {
	FilesModified  []string       `json:"files_modified"`
	TestResults    TestResults    `json:"test_results"`
	Errors         []ParsedError  `json:"errors"`
	GitOperations  []GitOperation `json:"git_operations"`
	CostTokens     CostTokens    `json:"cost_tokens"`
}

// TestResults holds pass/fail/skip counts and individual test names.
type TestResults struct {
	Passed  int      `json:"passed"`
	Failed  int      `json:"failed"`
	Skipped int      `json:"skipped"`
	Total   int      `json:"total"`
	Names   []string `json:"names,omitempty"`
}

// ParsedError holds a single error extracted from session output.
type ParsedError struct {
	Message  string `json:"message"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Severity string `json:"severity"` // "error", "warning", "fatal"
}

// GitOperation holds a single git operation extracted from session output.
type GitOperation struct {
	Command string   `json:"command"`            // "commit", "push", "branch", "merge", "checkout", "add", "reset", "stash", "rebase", "cherry-pick"
	Args    []string `json:"args,omitempty"`      // significant arguments (branch name, file paths, commit hash)
	Summary string   `json:"summary,omitempty"`   // human-readable summary if available
}

// CostTokens holds token usage and cost extracted from session output.
type CostTokens struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	TotalTokens  int     `json:"total_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CacheRead    int     `json:"cache_read_tokens,omitempty"`
	CacheWrite   int     `json:"cache_write_tokens,omitempty"`
}

// OutputParser parses structured output from LLM sessions. It supports both
// Claude Code JSON streaming output and plain text fallback. Each line is fed
// via ParseLine; the final result is retrieved with Result.
type OutputParser struct {
	result   ParsedOutput
	filesSeen map[string]bool
	gitSeen   map[string]bool
	errSeen   map[string]bool
}

// NewOutputParser creates a new parser ready to accept lines.
func NewOutputParser() *OutputParser {
	return &OutputParser{
		filesSeen: make(map[string]bool),
		gitSeen:   make(map[string]bool),
		errSeen:   make(map[string]bool),
	}
}

// ParseLine processes a single line of session output. It first attempts
// to parse the line as Claude Code JSON streaming output, then falls back
// to plain text regex extraction.
func (p *OutputParser) ParseLine(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}

	if looksLikeJSON(line) {
		p.parseJSONLine(line)
		return
	}

	p.parseTextLine(line)
}

// ParseLines processes multiple lines of session output (newline-separated).
func (p *OutputParser) ParseLines(text string) {
	for _, line := range strings.Split(text, "\n") {
		p.ParseLine(line)
	}
}

// Result returns the accumulated parsed output. It deduplicates and sorts
// file paths before returning.
func (p *OutputParser) Result() ParsedOutput {
	result := p.result
	// Sort files for deterministic output.
	sort.Strings(result.FilesModified)
	// Recompute total from pass/fail/skip.
	result.TestResults.Total = result.TestResults.Passed +
		result.TestResults.Failed + result.TestResults.Skipped
	return result
}

// parseJSONLine handles a single JSON line from Claude Code stream-json output.
func (p *OutputParser) parseJSONLine(line string) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		// Not valid JSON despite looking like it; fall back to text.
		p.parseTextLine(line)
		return
	}

	p.extractJSONToolUse(raw)
	p.extractJSONCost(raw)
	p.extractJSONContent(raw)
}

// extractJSONToolUse extracts file modifications and git operations from
// Claude Code tool_use events (e.g., Write, Edit, Bash with git commands).
func (p *OutputParser) extractJSONToolUse(raw map[string]any) {
	eventType := stringVal(raw, "type")

	// Claude Code emits tool_use and tool_result events.
	if eventType != "tool_use" && eventType != "tool_result" {
		// Also check for nested tool info in content_block events.
		if eventType == "content_block_start" || eventType == "content_block_delta" {
			if cb, ok := raw["content_block"].(map[string]any); ok {
				p.extractJSONToolUse(cb)
			}
		}
		return
	}

	toolName := stringVal(raw, "name")
	if toolName == "" {
		toolName = stringVal(raw, "tool_name")
	}

	inputRaw, _ := raw["input"].(map[string]any)

	switch toolName {
	case "Write", "write_file", "write":
		if fp := stringVal(inputRaw, "file_path"); fp != "" {
			p.addFile(fp)
		}
		if fp := stringVal(inputRaw, "path"); fp != "" {
			p.addFile(fp)
		}
	case "Edit", "edit_file", "edit":
		if fp := stringVal(inputRaw, "file_path"); fp != "" {
			p.addFile(fp)
		}
		if fp := stringVal(inputRaw, "path"); fp != "" {
			p.addFile(fp)
		}
	case "MultiEdit", "multi_edit":
		if fp := stringVal(inputRaw, "file_path"); fp != "" {
			p.addFile(fp)
		}
		if edits, ok := inputRaw["edits"].([]any); ok {
			for _, e := range edits {
				if em, ok := e.(map[string]any); ok {
					if fp := stringVal(em, "file_path"); fp != "" {
						p.addFile(fp)
					}
				}
			}
		}
	case "Bash", "bash", "shell", "terminal":
		cmd := stringVal(inputRaw, "command")
		if cmd == "" {
			cmd = stringVal(inputRaw, "cmd")
		}
		if cmd != "" {
			p.parseGitFromCommand(cmd)
			p.parseTestsFromCommand(cmd)
		}
	}

	// Check tool_result for test output and errors.
	if eventType == "tool_result" {
		output := stringVal(raw, "output")
		if output == "" {
			output = stringVal(raw, "content")
		}
		if output != "" {
			p.parseTestOutput(output)
			p.parseErrorsFromText(output)
		}
	}
}

// extractJSONCost extracts token usage and cost from JSON events.
func (p *OutputParser) extractJSONCost(raw map[string]any) {
	// Direct cost_usd field.
	if cost, ok := floatVal(raw, "cost_usd"); ok && cost > 0 {
		p.result.CostTokens.CostUSD = cost
	}

	// Nested usage object.
	usage, _ := raw["usage"].(map[string]any)
	if usage == nil {
		return
	}

	if cost, ok := floatVal(usage, "cost_usd"); ok && cost > 0 {
		p.result.CostTokens.CostUSD = cost
	}
	if cost, ok := floatVal(usage, "total_cost_usd"); ok && cost > 0 {
		p.result.CostTokens.CostUSD = cost
	}
	if v, ok := intVal(usage, "input_tokens"); ok && v > 0 {
		p.result.CostTokens.InputTokens = v
	}
	if v, ok := intVal(usage, "output_tokens"); ok && v > 0 {
		p.result.CostTokens.OutputTokens = v
	}
	if v, ok := intVal(usage, "total_tokens"); ok && v > 0 {
		p.result.CostTokens.TotalTokens = v
	} else {
		p.result.CostTokens.TotalTokens = p.result.CostTokens.InputTokens + p.result.CostTokens.OutputTokens
	}
	if v, ok := intVal(usage, "cache_read_input_tokens"); ok && v > 0 {
		p.result.CostTokens.CacheRead = v
	}
	if v, ok := intVal(usage, "cache_creation_input_tokens"); ok && v > 0 {
		p.result.CostTokens.CacheWrite = v
	}
}

// extractJSONContent parses text content from assistant/result events for
// file paths, test results, errors, and git operations.
func (p *OutputParser) extractJSONContent(raw map[string]any) {
	text := ""
	for _, key := range []string{"content", "text", "result", "message"} {
		if s := stringVal(raw, key); s != "" {
			text = s
			break
		}
	}
	// Also try nested content blocks (array of {type:"text", text:"..."}).
	if text == "" {
		if arr, ok := raw["content"].([]any); ok {
			var parts []string
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					if s := stringVal(m, "text"); s != "" {
						parts = append(parts, s)
					}
				}
			}
			text = strings.Join(parts, "\n")
		}
	}
	if text == "" {
		return
	}

	p.parseTestOutput(text)
	p.parseErrorsFromText(text)
	p.parseFilesFromText(text)
	p.parseGitFromText(text)
}

// --- Plain text parsing ---

// Compiled regexes for text extraction. These handle common LLM output
// patterns including go test output, compilation errors, and git commands.
var (
	// Go test patterns.
	goTestPassRe  = regexp.MustCompile(`(?m)^---\s+PASS:\s+(\S+)`)
	goTestFailRe  = regexp.MustCompile(`(?m)^---\s+FAIL:\s+(\S+)`)
	goTestSkipRe  = regexp.MustCompile(`(?m)^---\s+SKIP:\s+(\S+)`)
	goTestOKRe    = regexp.MustCompile(`(?m)^ok\s+\S+\s+[\d.]+s`)
	goTestFAILRe  = regexp.MustCompile(`(?m)^FAIL\s+\S+`)
	goTestRunRe   = regexp.MustCompile(`(?m)^===\s+RUN\s+(\S+)`)
	goTestSummRe  = regexp.MustCompile(`(?m)^(PASS|FAIL|ok)\s`)

	// Generic test patterns (pytest, jest, etc.).
	// pytestSummRe requires "passed" preceded by start-of-string or === to avoid double-counting jest output.
	pytestSummRe  = regexp.MustCompile(`(?:^|===+\s+)(\d+)\s+passed(?:.*?(\d+)\s+failed)?(?:.*?(\d+)\s+skipped)?`)
	jestSummRe    = regexp.MustCompile(`Tests:\s+(?:(\d+)\s+failed,?\s*)?(?:(\d+)\s+skipped,?\s*)?(\d+)\s+passed`)

	// File modification patterns.
	fileWriteRe   = regexp.MustCompile(`(?i)(?:created?|writ(?:e|ten|ing)|modified|updated|edited|saved)\s+(?:file\s+)?` + "`?" + `([^\s` + "`" + `]+\.\w{1,10})` + "`?")
	diffFileRe    = regexp.MustCompile(`(?m)^(?:---|\+\+\+)\s+[ab]/(.+)$`)

	// Git operation patterns. Uses a non-greedy args capture that stops at
	// command chaining operators (&&, ;, ||) or end of string. This lets the
	// regex find multiple git commands in a single chained shell line.
	gitCmdRe      = regexp.MustCompile(`git\s+(add|commit|push|pull|merge|checkout|branch|rebase|cherry-pick|stash|reset|tag|diff|log|status|switch|restore|revert|fetch|clone)\b([^&;|]*)`)
	gitCommitMsgRe = regexp.MustCompile(`(?m)\[[\w/.-]+\s+[0-9a-f]+\]\s+(.+)`)

	// Error patterns.
	goCompileErrRe = regexp.MustCompile(`(?m)^(.+\.go):(\d+):\d+:\s+(.+)$`)
	genericErrRe   = regexp.MustCompile(`(?mi)^(?:error|fatal|panic)(?:\[[\w]+\])?:\s*(.+)$`)
	goVetErrRe     = regexp.MustCompile(`(?m)^#\s+\S+\n(.+\.go):(\d+):\d+:\s+(.+)$`)

	// Token/cost patterns in plain text.
	tokenUsageRe   = regexp.MustCompile(`(?i)(?:input|prompt)\s+tokens?:?\s*(\d[\d,]*)`)
	outputTokenRe  = regexp.MustCompile(`(?i)(?:output|completion|response)\s+tokens?:?\s*(\d[\d,]*)`)
	totalTokenRe   = regexp.MustCompile(`(?i)total\s+tokens?:?\s*(\d[\d,]*)`)
	costTextRe     = regexp.MustCompile(`(?i)(?:total\s+)?(?:session\s+)?cost(?:_usd)?:?\s*\$?([\d]+\.[\d]+)`)
)

// parseTextLine processes a single plain text line.
func (p *OutputParser) parseTextLine(line string) {
	p.parseTestOutput(line)
	p.parseErrorsFromText(line)
	p.parseFilesFromText(line)
	p.parseGitFromText(line)
	p.parseCostFromText(line)
}

// parseTestOutput extracts test results from go test, pytest, or jest output.
func (p *OutputParser) parseTestOutput(text string) {
	// Go test individual results.
	for _, m := range goTestPassRe.FindAllStringSubmatch(text, -1) {
		p.result.TestResults.Passed++
		p.addTestName(m[1])
	}
	for _, m := range goTestFailRe.FindAllStringSubmatch(text, -1) {
		p.result.TestResults.Failed++
		p.addTestName(m[1])
	}
	for _, m := range goTestSkipRe.FindAllStringSubmatch(text, -1) {
		p.result.TestResults.Skipped++
		p.addTestName(m[1])
	}

	// Pytest summary: "5 passed, 2 failed, 1 skipped".
	if m := pytestSummRe.FindStringSubmatch(text); m != nil {
		if v := parseCommaInt(m[1]); v > 0 {
			p.result.TestResults.Passed += v
		}
		if len(m) > 2 {
			if v := parseCommaInt(m[2]); v > 0 {
				p.result.TestResults.Failed += v
			}
		}
		if len(m) > 3 {
			if v := parseCommaInt(m[3]); v > 0 {
				p.result.TestResults.Skipped += v
			}
		}
	}

	// Jest summary: "Tests: 1 failed, 2 skipped, 5 passed".
	if m := jestSummRe.FindStringSubmatch(text); m != nil {
		if v := parseCommaInt(m[3]); v > 0 {
			p.result.TestResults.Passed += v
		}
		if v := parseCommaInt(m[1]); v > 0 {
			p.result.TestResults.Failed += v
		}
		if v := parseCommaInt(m[2]); v > 0 {
			p.result.TestResults.Skipped += v
		}
	}
}

// parseErrorsFromText extracts error messages from text output.
func (p *OutputParser) parseErrorsFromText(text string) {
	// Go compilation errors: "file.go:42:10: undefined: foo".
	for _, m := range goCompileErrRe.FindAllStringSubmatch(text, -1) {
		lineNum, _ := strconv.Atoi(m[2])
		p.addError(ParsedError{
			Message:  strings.TrimSpace(m[3]),
			File:     m[1],
			Line:     lineNum,
			Severity: "error",
		})
		p.addFile(m[1])
	}

	// Go vet errors following "# package" lines.
	for _, m := range goVetErrRe.FindAllStringSubmatch(text, -1) {
		lineNum, _ := strconv.Atoi(m[2])
		p.addError(ParsedError{
			Message:  strings.TrimSpace(m[3]),
			File:     m[1],
			Line:     lineNum,
			Severity: "error",
		})
	}

	// Generic error/fatal/panic lines.
	for _, m := range genericErrRe.FindAllStringSubmatch(text, -1) {
		msg := strings.TrimSpace(m[1])
		p.addError(ParsedError{
			Message:  msg,
			Severity: classifyErrorSeverity(m[0]),
		})
	}
}

// parseFilesFromText extracts file paths from text patterns like
// "Created file.go", "Updated internal/foo/bar.go", or diff headers.
func (p *OutputParser) parseFilesFromText(text string) {
	for _, m := range fileWriteRe.FindAllStringSubmatch(text, -1) {
		fp := m[1]
		if isPlausibleFilePath(fp) {
			p.addFile(fp)
		}
	}
	for _, m := range diffFileRe.FindAllStringSubmatch(text, -1) {
		if m[1] != "/dev/null" {
			p.addFile(m[1])
		}
	}
}

// parseGitFromText extracts git operations from text describing git commands.
func (p *OutputParser) parseGitFromText(text string) {
	for _, m := range gitCmdRe.FindAllStringSubmatch(text, -1) {
		cmd := strings.ToLower(strings.TrimSpace(m[1]))
		args := parseGitArgs(m[2])
		p.addGitOp(GitOperation{Command: cmd, Args: args})
	}

	// Git commit message confirmation: "[main abc1234] Fix the bug".
	for _, m := range gitCommitMsgRe.FindAllStringSubmatch(text, -1) {
		p.addGitOp(GitOperation{
			Command: "commit",
			Summary: strings.TrimSpace(m[1]),
		})
	}
}

// parseGitFromCommand extracts git operations from a bash command string.
func (p *OutputParser) parseGitFromCommand(cmd string) {
	for _, m := range gitCmdRe.FindAllStringSubmatch(cmd, -1) {
		gitCmd := strings.ToLower(strings.TrimSpace(m[1]))
		args := parseGitArgs(m[2])
		p.addGitOp(GitOperation{Command: gitCmd, Args: args})

		// Git add/commit with file paths implies file modification.
		if gitCmd == "add" {
			for _, arg := range args {
				if !strings.HasPrefix(arg, "-") && isPlausibleFilePath(arg) {
					p.addFile(arg)
				}
			}
		}
	}
}

// parseTestsFromCommand detects test commands (go test, pytest, jest, npm test)
// in bash commands. Actual results come from the tool_result output.
func (p *OutputParser) parseTestsFromCommand(cmd string) {
	// Handled via tool_result output parsing, not the command itself.
}

// parseCostFromText extracts token/cost info from plain text output.
func (p *OutputParser) parseCostFromText(text string) {
	if m := tokenUsageRe.FindStringSubmatch(text); m != nil {
		if v := parseCommaInt(m[1]); v > 0 {
			p.result.CostTokens.InputTokens = v
		}
	}
	if m := outputTokenRe.FindStringSubmatch(text); m != nil {
		if v := parseCommaInt(m[1]); v > 0 {
			p.result.CostTokens.OutputTokens = v
		}
	}
	if m := totalTokenRe.FindStringSubmatch(text); m != nil {
		if v := parseCommaInt(m[1]); v > 0 {
			p.result.CostTokens.TotalTokens = v
		}
	}
	if m := costTextRe.FindStringSubmatch(text); m != nil {
		if cost, err := strconv.ParseFloat(m[1], 64); err == nil && cost > 0 {
			p.result.CostTokens.CostUSD = cost
		}
	}
}

// --- helpers ---

func (p *OutputParser) addFile(fp string) {
	fp = filepath.Clean(fp)
	if p.filesSeen[fp] {
		return
	}
	p.filesSeen[fp] = true
	p.result.FilesModified = append(p.result.FilesModified, fp)
}

func (p *OutputParser) addTestName(name string) {
	p.result.TestResults.Names = append(p.result.TestResults.Names, name)
}

func (p *OutputParser) addError(e ParsedError) {
	key := e.File + ":" + strconv.Itoa(e.Line) + ":" + e.Message
	if p.errSeen[key] {
		return
	}
	p.errSeen[key] = true
	p.result.Errors = append(p.result.Errors, e)
}

func (p *OutputParser) addGitOp(op GitOperation) {
	key := op.Command + ":" + strings.Join(op.Args, ",") + ":" + op.Summary
	if p.gitSeen[key] {
		return
	}
	p.gitSeen[key] = true
	p.result.GitOperations = append(p.result.GitOperations, op)
}

// stringVal safely extracts a string from a map.
func stringVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
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

// floatVal safely extracts a float64 from a map.
func floatVal(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		n, err := x.Float64()
		return n, err == nil
	default:
		return 0, false
	}
}

// intVal safely extracts an int from a map.
func intVal(m map[string]any, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case json.Number:
		n, err := x.Int64()
		return int(n), err == nil
	default:
		return 0, false
	}
}

// parseCommaInt parses an integer that may contain commas (e.g. "1,234").
func parseCommaInt(s string) int {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

// parseGitArgs splits a git argument string into meaningful tokens,
// filtering out flags and empty strings. Handles quoted arguments that
// may span multiple Fields tokens (e.g., -m "fix the bug").
func parseGitArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Tokenize respecting quotes.
	tokens := tokenizeShellArgs(raw)

	var args []string
	skipNext := false
	for i, f := range tokens {
		if skipNext {
			skipNext = false
			continue
		}
		// Skip common flags and their values.
		if strings.HasPrefix(f, "-") {
			// Flags that take a value argument: -m, -b, --message, etc.
			if i+1 < len(tokens) && isFlagWithValue(f) {
				// Include commit message as summary.
				if f == "-m" || f == "--message" {
					args = append(args, tokens[i+1])
				}
				skipNext = true
			}
			continue
		}
		args = append(args, f)
	}
	return args
}

// tokenizeShellArgs splits a string into tokens, respecting double and single
// quotes. Quotes are stripped from the resulting tokens.
func tokenizeShellArgs(s string) []string {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if r == ' ' || r == '\t' {
			if inSingle || inDouble {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func isFlagWithValue(flag string) bool {
	switch flag {
	case "-m", "--message", "-b", "--branch", "-C", "--reuse-message",
		"--author", "--date", "-F", "--file", "--cleanup", "--gpg-sign",
		"--fixup", "--squash":
		return true
	}
	return false
}

// isPlausibleFilePath returns true if the string looks like a file path
// with an extension, not a URL or flag.
func isPlausibleFilePath(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "http") {
		return false
	}
	ext := filepath.Ext(s)
	if ext == "" || len(ext) > 11 {
		return false
	}
	// Must not contain shell metacharacters that suggest it's not a path.
	if strings.ContainsAny(s, "{}()$|<>&;") {
		return false
	}
	return true
}

// classifyErrorSeverity determines severity from the error prefix.
func classifyErrorSeverity(line string) string {
	lower := strings.ToLower(line)
	if strings.HasPrefix(lower, "fatal") {
		return "fatal"
	}
	if strings.HasPrefix(lower, "panic") {
		return "fatal"
	}
	if strings.HasPrefix(lower, "warning") {
		return "warning"
	}
	return "error"
}

// ParseSessionOutput is a convenience function that parses all output lines
// from a session and returns the structured result.
func ParseSessionOutput(lines []string) ParsedOutput {
	p := NewOutputParser()
	for _, line := range lines {
		p.ParseLine(line)
	}
	return p.Result()
}

// ParseSessionOutputText is a convenience function that parses a single
// multiline text blob and returns the structured result.
func ParseSessionOutputText(text string) ParsedOutput {
	p := NewOutputParser()
	p.ParseLines(text)
	return p.Result()
}
