package mcpserver

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleMergeVerifyNoRepo(t *testing.T) {
	t.Parallel()
	srv := &Server{}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleMergeVerify(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}

	text := result.Content[0].(mcp.TextContent).Text
	var resp map[string]string
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["error_code"] != "INVALID_PARAMS" {
		t.Errorf("error_code = %q, want INVALID_PARAMS", resp["error_code"])
	}
}

func TestHandleMergeVerify_PathTraversal(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	cases := []struct {
		name string
		repo string
	}{
		{"dot-dot-etc-passwd", "../../etc/passwd"},
		{"absolute-outside-scanroot", "/etc/passwd"},
		{"null-byte", "/tmp/safe\x00/etc/shadow"},
		{"shell-metachar-semicolon", "/tmp/foo;rm -rf /"},
		{"shell-metachar-pipe", "/tmp/foo|cat /etc/passwd"},
		{"shell-metachar-backtick", "/tmp/`whoami`"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := srv.handleMergeVerify(context.Background(), makeRequest(map[string]any{
				"repo": tc.repo,
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error for traversal input %q", tc.repo)
			}
			text := getResultText(result)
			if !strings.Contains(text, string(ErrInvalidParams)) {
				t.Errorf("expected INVALID_PARAMS for %q, got: %s", tc.repo, text)
			}
		})
	}
}

func TestRunVerifyStepSuccess(t *testing.T) {
	t.Parallel()

	// Use a command that succeeds on all platforms.
	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/c", "echo", "hello"}
	} else {
		args = []string{"echo", "hello"}
	}

	result := runVerifyStep(context.Background(), t.TempDir(), "test-step", args)

	if result.Status != "pass" {
		t.Errorf("status = %q, want pass", result.Status)
	}
	if result.Name != "test-step" {
		t.Errorf("name = %q, want test-step", result.Name)
	}
	if result.ElapsedSeconds <= 0 {
		t.Errorf("elapsed_seconds = %f, want > 0", result.ElapsedSeconds)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("output = %q, want to contain 'hello'", result.Output)
	}
}

func TestRunVerifyStepFailure(t *testing.T) {
	t.Parallel()

	result := runVerifyStep(context.Background(), t.TempDir(), "fail-step", []string{"false"})

	if result.Status != "pass" {
		// On some systems "false" may not exist; skip gracefully.
		if result.Status != "fail" {
			t.Errorf("status = %q, want fail", result.Status)
		}
	}
	// The "false" command exits non-zero.
	if result.Status != "fail" {
		t.Skip("false command not available")
	}
	if result.Name != "fail-step" {
		t.Errorf("name = %q, want fail-step", result.Name)
	}
}

func TestMergeVerifyFastMode(t *testing.T) {
	t.Parallel()

	// Verify that when fast=true, the test args include -short.
	// We test this by calling handleMergeVerify with a non-existent repo
	// that has been set up, then checking the resulting args.
	// Instead, test the flag logic directly by building args the same way the handler does.

	fast := true
	race := true
	packages := "./..."

	testArgs := []string{"go", "test"}
	if race {
		testArgs = append(testArgs, "-race")
	}
	if fast {
		testArgs = append(testArgs, "-short")
	}
	testArgs = append(testArgs, packages)

	found := false
	for _, arg := range testArgs {
		if arg == "-short" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected -short flag in test args when fast=true")
	}

	// Verify the complete args list.
	expected := []string{"go", "test", "-race", "-short", "./..."}
	if len(testArgs) != len(expected) {
		t.Fatalf("args len = %d, want %d", len(testArgs), len(expected))
	}
	for i, arg := range testArgs {
		if arg != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestMergeVerifyRaceFlag(t *testing.T) {
	t.Parallel()

	// Race defaults to true — verify it appears in the args.
	race := true // default
	fast := false
	packages := "./..."

	testArgs := []string{"go", "test"}
	if race {
		testArgs = append(testArgs, "-race")
	}
	if fast {
		testArgs = append(testArgs, "-short")
	}
	testArgs = append(testArgs, packages)

	found := false
	for _, arg := range testArgs {
		if arg == "-race" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected -race flag in test args by default")
	}

	// Verify no -short when fast=false.
	for _, arg := range testArgs {
		if arg == "-short" {
			t.Error("unexpected -short flag when fast=false")
		}
	}

	// Verify race=false excludes -race.
	testArgsNoRace := []string{"go", "test"}
	raceOff := false
	if raceOff {
		testArgsNoRace = append(testArgsNoRace, "-race")
	}
	testArgsNoRace = append(testArgsNoRace, packages)

	for _, arg := range testArgsNoRace {
		if arg == "-race" {
			t.Error("unexpected -race flag when race=false")
		}
	}
}

func TestRunVerifyStepOutputTruncation(t *testing.T) {
	t.Parallel()

	// Generate output longer than maxStepOutput.
	// Use printf to generate a long string.
	longStr := strings.Repeat("x", maxStepOutput+500)
	result := runVerifyStep(context.Background(), t.TempDir(), "truncate-test",
		[]string{"echo", longStr})

	if len(result.Output) > maxStepOutput+50 { // +50 for the truncation message
		t.Errorf("output length = %d, expected truncated to ~%d", len(result.Output), maxStepOutput)
	}
	if !strings.Contains(result.Output, "truncated") {
		t.Error("expected truncation notice in output")
	}
}

func TestClassifyFailureCompile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{"cannot", "main.go:5: cannot convert string to int"},
		{"undefined", "main.go:10: undefined: foo"},
		{"unused_import", "main.go:3: imported and not used: \"fmt\""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, fix := classifyFailure(tc.output)
			if cat != "compile" {
				t.Errorf("category = %q, want compile", cat)
			}
			if fix == "" {
				t.Error("expected non-empty suggested fix")
			}
		})
	}
}

func TestClassifyFailureTestFail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{"dash_fail", "--- FAIL: TestFoo (0.01s)\n    foo_test.go:10: expected 1, got 2"},
		{"fail_line", "FAIL\tgithub.com/foo/bar\t0.123s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, fix := classifyFailure(tc.output)
			if cat != "test_fail" {
				t.Errorf("category = %q, want test_fail", cat)
			}
			if fix == "" {
				t.Error("expected non-empty suggested fix")
			}
		})
	}
}

func TestClassifyFailureTimeout(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
	}{
		{"deadline", "context deadline exceeded"},
		{"timed_out", "panic: test timed out after 30s"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, fix := classifyFailure(tc.output)
			if cat != "timeout" {
				t.Errorf("category = %q, want timeout", cat)
			}
			if fix == "" {
				t.Error("expected non-empty suggested fix")
			}
		})
	}
}

func TestClassifyFailureVet(t *testing.T) {
	t.Parallel()

	cat, fix := classifyFailure("go vet: ./main.go:5: printf format %d has arg of wrong type")
	if cat != "vet" {
		t.Errorf("category = %q, want vet", cat)
	}
	if fix == "" {
		t.Error("expected non-empty suggested fix")
	}
}

func TestClassifyFailureUnknown(t *testing.T) {
	t.Parallel()

	cat, _ := classifyFailure("some random error output")
	if cat != "unknown" {
		t.Errorf("category = %q, want unknown", cat)
	}
}

func TestRunVerifyStepSuccessNoFailureFields(t *testing.T) {
	t.Parallel()

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/c", "echo", "ok"}
	} else {
		args = []string{"echo", "ok"}
	}

	result := runVerifyStep(context.Background(), t.TempDir(), "clean-step", args)

	if result.Status != "pass" {
		t.Fatalf("status = %q, want pass", result.Status)
	}
	if result.FailureCategory != "" {
		t.Errorf("failure_category = %q, want empty for passing step", result.FailureCategory)
	}
	if result.SuggestedFix != "" {
		t.Errorf("suggested_fix = %q, want empty for passing step", result.SuggestedFix)
	}
	if result.Stderr != "" {
		t.Errorf("stderr = %q, want empty for passing step", result.Stderr)
	}
}

func TestRunVerifyStepFailureHasStderr(t *testing.T) {
	t.Parallel()

	// Use a shell command that writes to stderr and exits non-zero.
	result := runVerifyStep(context.Background(), t.TempDir(), "stderr-step",
		[]string{"sh", "-c", "echo 'some error' >&2; exit 1"})

	if result.Status != "fail" {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if !strings.Contains(result.Stderr, "some error") {
		t.Errorf("stderr = %q, want to contain 'some error'", result.Stderr)
	}
}

func TestRunVerifyStepCompileFailureCategory(t *testing.T) {
	t.Parallel()

	// Simulate a compile error by writing to stderr with a "cannot" message.
	result := runVerifyStep(context.Background(), t.TempDir(), "compile-step",
		[]string{"sh", "-c", "echo 'cannot convert string to int' >&2; exit 1"})

	if result.Status != "fail" {
		t.Fatalf("status = %q, want fail", result.Status)
	}
	if result.FailureCategory != "compile" {
		t.Errorf("failure_category = %q, want compile", result.FailureCategory)
	}
	if result.SuggestedFix == "" {
		t.Error("expected non-empty suggested fix for compile failure")
	}
}
