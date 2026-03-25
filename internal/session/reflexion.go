package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Reflection records a single reflexion loop observation: what failed, why,
// and the generated correction to inject into subsequent attempts.
type Reflection struct {
	Timestamp     time.Time `json:"ts"`
	LoopID        string    `json:"loop_id"`
	IterationNum  int       `json:"iteration"`
	TaskTitle     string    `json:"task_title"`
	FailureMode   string    `json:"failure_mode"` // "verify_failed", "worker_error", "planner_error"
	RootCause     string    `json:"root_cause"`
	Correction    string    `json:"correction"`
	FilesInvolved []string  `json:"files_involved"`
	Applied       bool      `json:"applied"`
}

// ReflexionStore persists and queries reflections in JSONL format.
type ReflexionStore struct {
	mu          sync.Mutex
	reflections []Reflection
	stateDir    string
}

// filePathRe matches source file paths. It requires EITHER:
//   - a `/` separator in the path (e.g. internal/session/loop.go), OR
//   - a recognized extension (.go, .mod, .sum, .toml, .yaml, .yml, .json, .md, .txt)
//
// Bare words like "main", "test", "error" do NOT match because neither
// condition is met.
var filePathRe = regexp.MustCompile(
	// Paths containing a slash (e.g. internal/session/loop.go)
	`\b[a-zA-Z0-9_][a-zA-Z0-9_\-./]*/[a-zA-Z0-9_\-./]*\.[a-zA-Z]{1,4}\b` +
		`|` +
		// Bare filenames with recognized extensions only (min 2-char basename)
		`\b[a-zA-Z][a-zA-Z0-9_\-]+\.(go|mod|sum|toml|yaml|yml|json|md|txt|ts|py|js|tsx|jsx|rs|cpp|java|rb|sh)\b`)

// NewReflexionStore loads existing reflections from the state directory.
func NewReflexionStore(stateDir string) *ReflexionStore {
	rs := &ReflexionStore{stateDir: stateDir}
	rs.load()
	return rs
}

// ExtractReflection classifies a failed iteration and generates a correction.
// Returns nil if the iteration did not fail.
func (rs *ReflexionStore) ExtractReflection(loopID string, iter LoopIteration) *Reflection {
	if iter.Status != "failed" {
		return nil
	}

	r := Reflection{
		Timestamp:    time.Now(),
		LoopID:       loopID,
		IterationNum: iter.Number,
		TaskTitle:    iter.Task.Title,
	}

	// Classify failure mode.
	r.FailureMode = classifyFailureMode(iter)

	// Extract root cause from error output.
	r.RootCause = extractRootCause(iter)

	// Generate correction based on failure mode.
	r.Correction = generateCorrection(r.FailureMode, r.RootCause, iter)

	// Extract file paths from error output.
	r.FilesInvolved = extractFilePaths(iter)

	return &r
}

// Store appends a reflection to the in-memory list and persists it to disk.
func (rs *ReflexionStore) Store(r Reflection) {
	rs.mu.Lock()
	rs.reflections = append(rs.reflections, r)
	rs.mu.Unlock()

	rs.appendToFile(r)
}

// RecentForTask returns the most recent reflections whose TaskTitle has
// keyword overlap with the given title, up to limit results (newest first).
// When taskTitle is empty, returns the most recent reflections regardless
// of keyword matching (useful at the planner stage before the task is known).
func (rs *ReflexionStore) RecentForTask(taskTitle string, limit int) []Reflection {
	if limit <= 0 {
		limit = 5
	}

	queryWords := toWordSet(taskTitle)
	matchAll := len(queryWords) == 0

	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Walk backwards for recency ordering.
	var results []Reflection
	for i := len(rs.reflections) - 1; i >= 0 && len(results) < limit; i-- {
		r := rs.reflections[i]
		if matchAll || hasOverlap(queryWords, toWordSet(r.TaskTitle)) {
			results = append(results, r)
		}
	}
	return results
}

// FormatForPrompt renders reflections as a markdown section suitable for
// injection into an LLM prompt.
func (rs *ReflexionStore) FormatForPrompt(reflections []Reflection) string {
	if len(reflections) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Lessons from Previous Attempts\n\n")
	for i, r := range reflections {
		sb.WriteString(fmt.Sprintf("- **Attempt %d failed** (%s): %s\n", i+1, r.FailureMode, r.RootCause))
		sb.WriteString(fmt.Sprintf("  **Correction**: %s\n", r.Correction))
	}
	sb.WriteString("\nApply these lessons to avoid repeating the same mistakes.\n")
	return sb.String()
}

// --- internal helpers ---

func classifyFailureMode(iter LoopIteration) string {
	// Check for verification failures first.
	for _, v := range iter.Verification {
		if v.Status == "failed" || v.ExitCode != 0 {
			return "verify_failed"
		}
	}
	if strings.Contains(strings.ToLower(iter.Error), "worker") {
		return "worker_error"
	}
	return "planner_error"
}

// testFailRe matches Go test failure lines like "--- FAIL: TestFoo (0.05s)".
var testFailRe = regexp.MustCompile(`---\s+FAIL:\s+(\S+)\s+\(([^)]+)\)`)

// broadFailRe matches additional failure patterns: bare FAIL, Error:, panic:, expected/got.
var broadFailRe = regexp.MustCompile(`(?i)(?:^|\s)(FAIL\s+\S+|Error:\s+.+|panic:\s+.+|expected\s+.+\s+got\s+.+)`)

// compileErrorRe matches Go compile errors like "file.go:42:10: undefined: Foo".
var compileErrorRe = regexp.MustCompile(`([a-zA-Z0-9_/.\-]+\.go):(\d+):(\d+):\s*(.+)`)

func extractRootCause(iter LoopIteration) string {
	// Gather all output lines to scan.
	var lines []string
	if iter.Error != "" {
		lines = append(lines, strings.Split(iter.Error, "\n")...)
	}
	if iter.WorkerOutput != "" {
		lines = append(lines, strings.Split(iter.WorkerOutput, "\n")...)
	}
	for _, wo := range iter.WorkerOutputs {
		lines = append(lines, strings.Split(wo, "\n")...)
	}
	for _, v := range iter.Verification {
		if v.Output != "" {
			lines = append(lines, strings.Split(v.Output, "\n")...)
		}
	}

	// Pass 1: look for structured failure patterns (test failures, compile errors).
	var testFailures []string
	var compileErrors []string
	for _, line := range lines {
		if m := testFailRe.FindStringSubmatch(line); m != nil {
			testFailures = append(testFailures, fmt.Sprintf("test %s failed (%s)", m[1], m[2]))
		}
		if m := compileErrorRe.FindStringSubmatch(line); m != nil {
			msg := strings.TrimSpace(m[4])
			if len(msg) > 120 {
				msg = msg[:120]
			}
			compileErrors = append(compileErrors, fmt.Sprintf("%s:%s: %s", m[1], m[2], msg))
		}
	}

	if len(testFailures) > 0 {
		cause := strings.Join(testFailures, "; ")
		if len(cause) > 200 {
			cause = cause[:200]
		}
		return cause
	}
	if len(compileErrors) > 0 {
		cause := strings.Join(compileErrors, "; ")
		if len(cause) > 200 {
			cause = cause[:200]
		}
		return cause
	}

	// Pass 2: fallback to generic error-pattern matching.
	errorPatterns := []string{"error:", "panic:", "FAIL", "failed to", "cannot", "undefined", "compilation error", "syntax error"}
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, pat := range errorPatterns {
			if strings.Contains(lower, strings.ToLower(pat)) {
				trimmed := strings.TrimSpace(line)
				if len(trimmed) > 200 {
					trimmed = trimmed[:200]
				}
				return trimmed
			}
		}
	}

	if iter.Error != "" {
		cause := strings.TrimSpace(iter.Error)
		if len(cause) > 200 {
			cause = cause[:200]
		}
		return cause
	}
	return "unknown failure"
}

func generateCorrection(failureMode, rootCause string, iter LoopIteration) string {
	switch failureMode {
	case "verify_failed":
		// Try to extract specific failing test names from verification output.
		var failingTests []string
		var broadMatches []string
		for _, v := range iter.Verification {
			if (v.Status == "failed" || v.ExitCode != 0) && v.Output != "" {
				for _, m := range testFailRe.FindAllStringSubmatch(v.Output, -1) {
					failingTests = append(failingTests, fmt.Sprintf("%s (%s)", m[1], m[2]))
				}
				// Broader patterns: FAIL pkg, Error:, panic:, expected/got
				for _, m := range broadFailRe.FindAllStringSubmatch(v.Output, 5) {
					line := strings.TrimSpace(m[1])
					if len(line) > 120 {
						line = line[:120]
					}
					broadMatches = append(broadMatches, line)
				}
			}
		}
		if len(failingTests) > 0 {
			names := strings.Join(failingTests, ", ")
			if len(names) > 150 {
				names = names[:150]
			}
			return fmt.Sprintf("Fix failing test(s): %s. Ensure all verification commands pass before completing.", names)
		}
		if len(broadMatches) > 0 {
			details := strings.Join(broadMatches, "; ")
			if len(details) > 200 {
				details = details[:200]
			}
			return fmt.Sprintf("Fix verification failure: %s. Ensure all verification commands pass before completing.", details)
		}

		// Fallback: include a snippet of verify output.
		verifySnippet := ""
		for _, v := range iter.Verification {
			if (v.Status == "failed" || v.ExitCode != 0) && v.Output != "" {
				verifySnippet = v.Output
				break
			}
		}
		if len(verifySnippet) > 200 {
			verifySnippet = verifySnippet[:200]
		}
		return fmt.Sprintf("Ensure all verification commands pass before completing. Previous verify output: %s", verifySnippet)

	case "worker_error":
		// Include the actual error message from iter.Error if available.
		errMsg := strings.TrimSpace(iter.Error)
		if errMsg == "" {
			errMsg = rootCause
		}
		if len(errMsg) > 200 {
			errMsg = errMsg[:200]
		}
		return fmt.Sprintf("The worker encountered: %s. Ensure error handling for this case.", errMsg)

	default:
		// Include the parsing error details.
		errDetail := strings.TrimSpace(iter.Error)
		if errDetail == "" {
			errDetail = rootCause
		}
		if len(errDetail) > 200 {
			errDetail = errDetail[:200]
		}
		return fmt.Sprintf("The planner could not parse tasks: %s. Ensure output follows the expected format.", errDetail)
	}
}

func extractFilePaths(iter LoopIteration) []string {
	var combined strings.Builder
	combined.WriteString(iter.Error)
	combined.WriteString("\n")
	combined.WriteString(iter.WorkerOutput)
	for _, wo := range iter.WorkerOutputs {
		combined.WriteString("\n")
		combined.WriteString(wo)
	}
	for _, v := range iter.Verification {
		combined.WriteString("\n")
		combined.WriteString(v.Output)
	}

	matches := filePathRe.FindAllString(combined.String(), -1)
	seen := make(map[string]bool)
	var unique []string
	for _, m := range matches {
		// Skip paths shorter than 4 chars (e.g. "a.go") — likely fragments.
		if len(m) < 4 {
			continue
		}
		// Skip domain-like fragments with no slash and single-char base name.
		if !strings.Contains(m, "/") {
			base := m[:strings.LastIndex(m, ".")]
			if len(base) < 2 {
				continue
			}
		}
		if !seen[m] {
			seen[m] = true
			unique = append(unique, m)
		}
	}
	return unique
}

func (rs *ReflexionStore) appendToFile(r Reflection) {
	if rs.stateDir == "" {
		return
	}
	_ = os.MkdirAll(rs.stateDir, 0755)

	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	data = append(data, '\n')

	path := filepath.Join(rs.stateDir, "reflections.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}

func (rs *ReflexionStore) load() {
	if rs.stateDir == "" {
		return
	}
	path := filepath.Join(rs.stateDir, "reflections.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var reflections []Reflection
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var r Reflection
		if json.Unmarshal(line, &r) == nil {
			reflections = append(reflections, r)
		}
	}

	rs.mu.Lock()
	rs.reflections = reflections
	rs.mu.Unlock()
}

// toWordSet splits a string into a set of lowercased words (length >= 2).
func toWordSet(s string) map[string]bool {
	words := make(map[string]bool)
	for _, w := range strings.Fields(strings.ToLower(s)) {
		// Skip very short words (articles, etc.)
		if len(w) >= 2 {
			words[w] = true
		}
	}
	return words
}

// hasOverlap returns true if the two word sets share at least one word.
func hasOverlap(a, b map[string]bool) bool {
	for w := range a {
		if b[w] {
			return true
		}
	}
	return false
}
