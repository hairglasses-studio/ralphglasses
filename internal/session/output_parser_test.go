package session

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// testdataDir returns the absolute path to the testdata/output_parser directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "output_parser")
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return string(data)
}

// --- Golden fixture tests ---

func TestGoldenClaudeStreamJSON(t *testing.T) {
	text := readFixture(t, "claude_stream.jsonl")
	result := ParseSessionOutputText(text)

	// Files: handler.go (from Edit), handler_test.go (from Write),
	// plus handler.go and handler_test.go from the git add command.
	assertContainsFile(t, result.FilesModified, "internal/auth/handler.go")
	assertContainsFile(t, result.FilesModified, "internal/auth/handler_test.go")

	// Tests: 2 pass, 1 fail from the go test output in the Bash tool_result.
	if result.TestResults.Passed != 2 {
		t.Errorf("TestResults.Passed = %d, want 2", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 1 {
		t.Errorf("TestResults.Failed = %d, want 1", result.TestResults.Failed)
	}
	if result.TestResults.Total != 3 {
		t.Errorf("TestResults.Total = %d, want 3", result.TestResults.Total)
	}

	// Git ops: add + commit from bash, commit confirmation from output.
	hasCommit := false
	for _, op := range result.GitOperations {
		if op.Command == "commit" {
			hasCommit = true
		}
	}
	if !hasCommit {
		t.Error("expected git commit operation")
	}

	// Cost: explicit cost_usd in result event.
	if result.CostTokens.CostUSD != 0.0847 {
		t.Errorf("CostTokens.CostUSD = %f, want 0.0847", result.CostTokens.CostUSD)
	}
	if result.CostTokens.InputTokens != 12500 {
		t.Errorf("CostTokens.InputTokens = %d, want 12500", result.CostTokens.InputTokens)
	}
	if result.CostTokens.OutputTokens != 3200 {
		t.Errorf("CostTokens.OutputTokens = %d, want 3200", result.CostTokens.OutputTokens)
	}
	if result.CostTokens.CacheRead != 8000 {
		t.Errorf("CostTokens.CacheRead = %d, want 8000", result.CostTokens.CacheRead)
	}
	if result.CostTokens.CacheWrite != 1500 {
		t.Errorf("CostTokens.CacheWrite = %d, want 1500", result.CostTokens.CacheWrite)
	}
}

func TestGoldenPlainTextGoTest(t *testing.T) {
	text := readFixture(t, "plain_text_go_test.txt")
	result := ParseSessionOutputText(text)

	if result.TestResults.Passed != 3 {
		t.Errorf("Passed = %d, want 3", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.TestResults.Failed)
	}
	if result.TestResults.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.TestResults.Skipped)
	}
	if result.TestResults.Total != 5 {
		t.Errorf("Total = %d, want 5", result.TestResults.Total)
	}

	// Verify test names were captured.
	assertContainsStr(t, result.TestResults.Names, "TestAdd")
	assertContainsStr(t, result.TestResults.Names, "TestDivide")
	assertContainsStr(t, result.TestResults.Names, "TestDivideByZero")
}

func TestGoldenPlainTextErrors(t *testing.T) {
	text := readFixture(t, "plain_text_errors.txt")
	result := ParseSessionOutputText(text)

	// Should find 3 Go compilation errors + 2 generic errors (error + fatal).
	if len(result.Errors) < 3 {
		t.Errorf("len(Errors) = %d, want >= 3", len(result.Errors))
	}

	// Check specific compilation error.
	foundUndefined := false
	for _, e := range result.Errors {
		if strings.Contains(e.Message, "undefined: ValidateRequest") {
			foundUndefined = true
			if e.File != "internal/server/handler.go" {
				t.Errorf("error file = %q, want internal/server/handler.go", e.File)
			}
			if e.Line != 42 {
				t.Errorf("error line = %d, want 42", e.Line)
			}
			if e.Severity != "error" {
				t.Errorf("error severity = %q, want error", e.Severity)
			}
		}
	}
	if !foundUndefined {
		t.Error("expected to find 'undefined: ValidateRequest' error")
	}

	// Check fatal error.
	foundFatal := false
	for _, e := range result.Errors {
		if e.Severity == "fatal" && strings.Contains(e.Message, "not a git repository") {
			foundFatal = true
		}
	}
	if !foundFatal {
		t.Error("expected to find fatal 'not a git repository' error")
	}

	// Files from compilation errors should be tracked.
	assertContainsFile(t, result.FilesModified, "internal/server/handler.go")
}

func TestGoldenPlainTextGitOps(t *testing.T) {
	text := readFixture(t, "plain_text_git_ops.txt")
	result := ParseSessionOutputText(text)

	// Should detect: checkout, add, commit, push.
	cmdSet := make(map[string]bool)
	for _, op := range result.GitOperations {
		cmdSet[op.Command] = true
	}
	for _, want := range []string{"checkout", "add", "commit", "push"} {
		if !cmdSet[want] {
			t.Errorf("missing git operation %q", want)
		}
	}

	// Files from text patterns.
	assertContainsFile(t, result.FilesModified, "internal/auth/handler.go")
	assertContainsFile(t, result.FilesModified, "internal/auth/middleware.go")
	assertContainsFile(t, result.FilesModified, "internal/auth/handler_test.go")

	// Cost from plain text.
	if result.CostTokens.CostUSD != 0.0523 {
		t.Errorf("CostUSD = %f, want 0.0523", result.CostTokens.CostUSD)
	}
	if result.CostTokens.InputTokens != 8500 {
		t.Errorf("InputTokens = %d, want 8500", result.CostTokens.InputTokens)
	}
	if result.CostTokens.OutputTokens != 2100 {
		t.Errorf("OutputTokens = %d, want 2100", result.CostTokens.OutputTokens)
	}
}

func TestGoldenClaudeNestedUsage(t *testing.T) {
	text := readFixture(t, "claude_nested_usage.jsonl")
	result := ParseSessionOutputText(text)

	if result.CostTokens.CostUSD != 0.0312 {
		t.Errorf("CostUSD = %f, want 0.0312", result.CostTokens.CostUSD)
	}
	if result.CostTokens.InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", result.CostTokens.InputTokens)
	}
	if result.CostTokens.OutputTokens != 2000 {
		t.Errorf("OutputTokens = %d, want 2000", result.CostTokens.OutputTokens)
	}
	if result.CostTokens.TotalTokens != 7000 {
		t.Errorf("TotalTokens = %d, want 7000", result.CostTokens.TotalTokens)
	}
	if result.CostTokens.CacheRead != 3000 {
		t.Errorf("CacheRead = %d, want 3000", result.CostTokens.CacheRead)
	}
	if result.CostTokens.CacheWrite != 500 {
		t.Errorf("CacheWrite = %d, want 500", result.CostTokens.CacheWrite)
	}
}

func TestGoldenMixedOutput(t *testing.T) {
	text := readFixture(t, "mixed_output.txt")
	result := ParseSessionOutputText(text)

	// Files from diff headers, "Created", "Written", "Modified" patterns, and git add.
	assertContainsFile(t, result.FilesModified, "internal/handler.go")
	assertContainsFile(t, result.FilesModified, "cmd/server/main.go")
	assertContainsFile(t, result.FilesModified, "internal/config/config.go")

	// Tests: 3 pass + 1 fail.
	if result.TestResults.Passed != 3 {
		t.Errorf("Passed = %d, want 3", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.TestResults.Failed)
	}

	// Git: add + commit.
	hasAdd := false
	hasCommit := false
	for _, op := range result.GitOperations {
		if op.Command == "add" {
			hasAdd = true
		}
		if op.Command == "commit" {
			hasCommit = true
		}
	}
	if !hasAdd {
		t.Error("expected git add")
	}
	if !hasCommit {
		t.Error("expected git commit")
	}

	// Cost from text.
	if result.CostTokens.CostUSD != 0.1245 {
		t.Errorf("CostUSD = %f, want 0.1245", result.CostTokens.CostUSD)
	}
}

// --- Unit tests for individual parsing functions ---

func TestParseLineEmpty(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("")
	p.ParseLine("   ")
	p.ParseLine("\t")
	result := p.Result()

	if len(result.FilesModified) != 0 {
		t.Errorf("expected no files, got %v", result.FilesModified)
	}
	if result.TestResults.Total != 0 {
		t.Errorf("expected no tests, got %d", result.TestResults.Total)
	}
}

func TestParseLineJSONToolUseWrite(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"tool_use","name":"Write","input":{"file_path":"pkg/api/server.go"}}`)
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "pkg/api/server.go")
}

func TestParseLineJSONToolUseEdit(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"tool_use","name":"Edit","input":{"file_path":"internal/db/query.go","old_string":"foo","new_string":"bar"}}`)
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "internal/db/query.go")
}

func TestParseLineJSONToolUseBashGit(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"tool_use","name":"Bash","input":{"command":"git add main.go && git commit -m \"initial commit\""}}`)
	result := p.Result()

	hasAdd := false
	hasCommit := false
	for _, op := range result.GitOperations {
		if op.Command == "add" {
			hasAdd = true
		}
		if op.Command == "commit" {
			hasCommit = true
		}
	}
	if !hasAdd {
		t.Error("expected git add operation")
	}
	if !hasCommit {
		t.Error("expected git commit operation")
	}
	// git add main.go should track main.go as modified.
	assertContainsFile(t, result.FilesModified, "main.go")
}

func TestParseLineJSONToolResultTestOutput(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"tool_result","output":"=== RUN   TestFoo\n--- PASS: TestFoo (0.01s)\n=== RUN   TestBar\n--- FAIL: TestBar (0.02s)"}`)
	result := p.Result()

	if result.TestResults.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.TestResults.Failed)
	}
}

func TestParseLineJSONResultCost(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"result","cost_usd":0.25,"usage":{"input_tokens":10000,"output_tokens":5000}}`)
	result := p.Result()

	if result.CostTokens.CostUSD != 0.25 {
		t.Errorf("CostUSD = %f, want 0.25", result.CostTokens.CostUSD)
	}
	if result.CostTokens.InputTokens != 10000 {
		t.Errorf("InputTokens = %d, want 10000", result.CostTokens.InputTokens)
	}
	if result.CostTokens.OutputTokens != 5000 {
		t.Errorf("OutputTokens = %d, want 5000", result.CostTokens.OutputTokens)
	}
	if result.CostTokens.TotalTokens != 15000 {
		t.Errorf("TotalTokens = %d, want 15000", result.CostTokens.TotalTokens)
	}
}

func TestParseLineTextGoTestPass(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("--- PASS: TestSomething (0.01s)")
	result := p.Result()

	if result.TestResults.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.TestResults.Passed)
	}
	assertContainsStr(t, result.TestResults.Names, "TestSomething")
}

func TestParseLineTextGoTestFail(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("--- FAIL: TestBroken (0.05s)")
	result := p.Result()

	if result.TestResults.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.TestResults.Failed)
	}
}

func TestParseLineTextGoTestSkip(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("--- SKIP: TestFlaky (0.00s)")
	result := p.Result()

	if result.TestResults.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.TestResults.Skipped)
	}
}

func TestParseLineTextPytest(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("====== 12 passed, 3 failed, 2 skipped in 4.56s ======")
	result := p.Result()

	if result.TestResults.Passed != 12 {
		t.Errorf("Passed = %d, want 12", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 3 {
		t.Errorf("Failed = %d, want 3", result.TestResults.Failed)
	}
	if result.TestResults.Skipped != 2 {
		t.Errorf("Skipped = %d, want 2", result.TestResults.Skipped)
	}
}

func TestParseLineTextJest(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("Tests: 2 failed, 1 skipped, 8 passed, 11 total")
	result := p.Result()

	if result.TestResults.Passed != 8 {
		t.Errorf("Passed = %d, want 8", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.TestResults.Failed)
	}
	if result.TestResults.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", result.TestResults.Skipped)
	}
}

func TestParseLineTextCompileError(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("main.go:15:5: undefined: Foo")
	result := p.Result()

	if len(result.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1", len(result.Errors))
	}
	e := result.Errors[0]
	if e.File != "main.go" {
		t.Errorf("File = %q, want main.go", e.File)
	}
	if e.Line != 15 {
		t.Errorf("Line = %d, want 15", e.Line)
	}
	if e.Message != "undefined: Foo" {
		t.Errorf("Message = %q, want 'undefined: Foo'", e.Message)
	}
	if e.Severity != "error" {
		t.Errorf("Severity = %q, want error", e.Severity)
	}
}

func TestParseLineTextGenericError(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("error: connection refused")
	result := p.Result()

	if len(result.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].Message != "connection refused" {
		t.Errorf("Message = %q, want 'connection refused'", result.Errors[0].Message)
	}
}

func TestParseLineTextFatalError(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("fatal: remote origin already exists")
	result := p.Result()

	if len(result.Errors) != 1 {
		t.Fatalf("len(Errors) = %d, want 1", len(result.Errors))
	}
	if result.Errors[0].Severity != "fatal" {
		t.Errorf("Severity = %q, want fatal", result.Errors[0].Severity)
	}
}

func TestParseLineTextFileCreated(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("Created internal/handler.go")
	p.ParseLine("Written pkg/util/helper.go")
	p.ParseLine("Modified cmd/main.go")
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "internal/handler.go")
	assertContainsFile(t, result.FilesModified, "pkg/util/helper.go")
	assertContainsFile(t, result.FilesModified, "cmd/main.go")
}

func TestParseLineTextDiffHeaders(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("--- a/internal/server.go")
	p.ParseLine("+++ b/internal/server.go")
	p.ParseLine("--- a/internal/client.go")
	p.ParseLine("+++ b/internal/client.go")
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "internal/server.go")
	assertContainsFile(t, result.FilesModified, "internal/client.go")
}

func TestParseLineTextGitCommands(t *testing.T) {
	tests := []struct {
		line    string
		wantCmd string
	}{
		{"$ git commit -m \"fix bug\"", "commit"},
		{"$ git push origin main", "push"},
		{"$ git merge feature-branch", "merge"},
		{"$ git checkout -b new-branch", "checkout"},
		{"$ git rebase main", "rebase"},
		{"> git stash", "stash"},
		{"git cherry-pick abc123", "cherry-pick"},
	}
	for _, tt := range tests {
		t.Run(tt.wantCmd, func(t *testing.T) {
			p := NewOutputParser()
			p.ParseLine(tt.line)
			result := p.Result()

			found := false
			for _, op := range result.GitOperations {
				if op.Command == tt.wantCmd {
					found = true
				}
			}
			if !found {
				t.Errorf("expected git %s operation from %q", tt.wantCmd, tt.line)
			}
		})
	}
}

func TestParseLineTextGitCommitConfirmation(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("[main abc1234] feat: add new feature")
	result := p.Result()

	found := false
	for _, op := range result.GitOperations {
		if op.Command == "commit" && op.Summary == "feat: add new feature" {
			found = true
		}
	}
	if !found {
		t.Error("expected git commit with summary from confirmation line")
	}
}

func TestParseLineTextCostTokens(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("Input tokens: 5,000")
	p.ParseLine("Output tokens: 1,200")
	p.ParseLine("Total cost: $0.0345")
	result := p.Result()

	if result.CostTokens.InputTokens != 5000 {
		t.Errorf("InputTokens = %d, want 5000", result.CostTokens.InputTokens)
	}
	if result.CostTokens.OutputTokens != 1200 {
		t.Errorf("OutputTokens = %d, want 1200", result.CostTokens.OutputTokens)
	}
	if result.CostTokens.CostUSD != 0.0345 {
		t.Errorf("CostUSD = %f, want 0.0345", result.CostTokens.CostUSD)
	}
}

// --- Deduplication tests ---

func TestDeduplicateFiles(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("Created internal/handler.go")
	p.ParseLine("Modified internal/handler.go")
	p.ParseLine("Updated internal/handler.go")
	result := p.Result()

	count := 0
	for _, f := range result.FilesModified {
		if f == "internal/handler.go" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("file appeared %d times, want 1", count)
	}
}

func TestDeduplicateErrors(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("main.go:10:5: undefined: Bar")
	p.ParseLine("main.go:10:5: undefined: Bar")
	result := p.Result()

	if len(result.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1 (duplicate should be deduped)", len(result.Errors))
	}
}

func TestDeduplicateGitOps(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("$ git push origin main")
	p.ParseLine("$ git push origin main")
	result := p.Result()

	pushCount := 0
	for _, op := range result.GitOperations {
		if op.Command == "push" {
			pushCount++
		}
	}
	if pushCount != 1 {
		t.Errorf("push appeared %d times, want 1", pushCount)
	}
}

// --- Convenience function tests ---

func TestParseSessionOutput(t *testing.T) {
	lines := []string{
		"--- PASS: TestA (0.01s)",
		"--- FAIL: TestB (0.02s)",
		"Created pkg/new.go",
	}
	result := ParseSessionOutput(lines)

	if result.TestResults.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.TestResults.Passed)
	}
	if result.TestResults.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.TestResults.Failed)
	}
	assertContainsFile(t, result.FilesModified, "pkg/new.go")
}

func TestParseSessionOutputText(t *testing.T) {
	text := "--- PASS: TestX (0.01s)\n--- PASS: TestY (0.02s)\nCreated internal/foo.go"
	result := ParseSessionOutputText(text)

	if result.TestResults.Passed != 2 {
		t.Errorf("Passed = %d, want 2", result.TestResults.Passed)
	}
	assertContainsFile(t, result.FilesModified, "internal/foo.go")
}

// --- Edge cases ---

func TestParseInvalidJSON(t *testing.T) {
	p := NewOutputParser()
	// Looks like JSON but is not valid -- should fall back to text parsing.
	p.ParseLine(`{not valid json}`)
	result := p.Result()

	// Should not panic, and should produce no meaningful output.
	if result.TestResults.Total != 0 {
		t.Errorf("expected no test results from invalid JSON")
	}
}

func TestParseJSONWithContentBlocks(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"assistant","content":[{"type":"text","text":"Created internal/service.go and ran tests.\n--- PASS: TestService (0.01s)"}]}`)
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "internal/service.go")
	if result.TestResults.Passed != 1 {
		t.Errorf("Passed = %d, want 1", result.TestResults.Passed)
	}
}

func TestParseMultiEditToolUse(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"tool_use","name":"MultiEdit","input":{"file_path":"internal/main.go","edits":[{"file_path":"internal/a.go"},{"file_path":"internal/b.go"}]}}`)
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "internal/main.go")
	assertContainsFile(t, result.FilesModified, "internal/a.go")
	assertContainsFile(t, result.FilesModified, "internal/b.go")
}

func TestParseContentBlockStartToolUse(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"content_block_start","content_block":{"type":"tool_use","name":"Write","input":{"file_path":"pkg/new_file.go"}}}`)
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "pkg/new_file.go")
}

func TestParseTotalTokensExplicit(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"result","usage":{"input_tokens":1000,"output_tokens":500,"total_tokens":1500}}`)
	result := p.Result()

	if result.CostTokens.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500 (explicit value)", result.CostTokens.TotalTokens)
	}
}

func TestParseTotalTokensComputed(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine(`{"type":"result","usage":{"input_tokens":1000,"output_tokens":500}}`)
	result := p.Result()

	if result.CostTokens.TotalTokens != 1500 {
		t.Errorf("TotalTokens = %d, want 1500 (computed from input+output)", result.CostTokens.TotalTokens)
	}
}

func TestParseFileBehindBacktick(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("Created `internal/handler.go`")
	result := p.Result()

	assertContainsFile(t, result.FilesModified, "internal/handler.go")
}

func TestParseDevNullIgnored(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("--- /dev/null")
	p.ParseLine("+++ b/new_file.go")
	result := p.Result()

	for _, f := range result.FilesModified {
		if strings.Contains(f, "dev/null") {
			t.Errorf("should not include /dev/null in files, got %v", result.FilesModified)
		}
	}
	assertContainsFile(t, result.FilesModified, "new_file.go")
}

func TestIsPlausibleFilePath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"internal/handler.go", true},
		{"main.go", true},
		{"Makefile", false},           // no extension
		{"-flag", false},              // flag
		{"http://example.com", false}, // URL
		{"", false},
		{"file.go", true},
		{"a.{b}", false}, // shell metacharacter
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isPlausibleFilePath(tt.input)
			if got != tt.want {
				t.Errorf("isPlausibleFilePath(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestClassifyErrorSeverity(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"error: something", "error"},
		{"fatal: oops", "fatal"},
		{"panic: runtime error", "fatal"},
		{"warning: deprecated", "warning"},
		{"Error: upper case", "error"},
		{"FATAL: uppercase", "fatal"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := classifyErrorSeverity(tt.input)
			if got != tt.want {
				t.Errorf("classifyErrorSeverity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseCommaInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1234", 1234},
		{"1,234", 1234},
		{"12,345,678", 12345678},
		{"", 0},
		{"  5000  ", 5000},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseCommaInt(tt.input)
			if got != tt.want {
				t.Errorf("parseCommaInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseGitArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"main.go", []string{"main.go"}},
		{"-m \"fix bug\"", []string{"fix bug"}},
		{"origin main", []string{"origin", "main"}},
		{"-b feature-branch", nil},
		{"", nil},
		{"--force origin main", []string{"origin", "main"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseGitArgs(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseGitArgs(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseGitArgs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestResultSortsFiles(t *testing.T) {
	p := NewOutputParser()
	p.ParseLine("Created z_file.go")
	p.ParseLine("Created a_file.go")
	p.ParseLine("Created m_file.go")
	result := p.Result()

	if len(result.FilesModified) != 3 {
		t.Fatalf("expected 3 files, got %d", len(result.FilesModified))
	}
	if result.FilesModified[0] != "a_file.go" {
		t.Errorf("first file = %q, want a_file.go", result.FilesModified[0])
	}
	if result.FilesModified[2] != "z_file.go" {
		t.Errorf("last file = %q, want z_file.go", result.FilesModified[2])
	}
}

// --- Test helpers ---

func assertContainsFile(t *testing.T, files []string, want string) {
	t.Helper()
	want = filepath.Clean(want)
	if slices.Contains(files, want) {
		return
	}
	t.Errorf("files %v does not contain %q", files, want)
}

func assertContainsStr(t *testing.T, strs []string, want string) {
	t.Helper()
	if slices.Contains(strs, want) {
		return
	}
	t.Errorf("strings %v does not contain %q", strs, want)
}
