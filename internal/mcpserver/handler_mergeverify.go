package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// StepResult holds the outcome of a single verification step.
type StepResult struct {
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	ElapsedSeconds  float64  `json:"elapsed_seconds"`
	Output          string   `json:"output"`
	Stderr          string   `json:"stderr,omitempty"`
	FailureCategory string   `json:"failure_category,omitempty"`
	SuggestedFix    string   `json:"suggested_fix,omitempty"`
	Coverage        *float64 `json:"coverage,omitempty"`
}

// MergeVerifyResult holds the full verification outcome.
type MergeVerifyResult struct {
	Repo                string       `json:"repo"`
	Overall             string       `json:"overall"`
	Steps               []StepResult `json:"steps"`
	FailedAt            string       `json:"failed_at,omitempty"`
	TotalElapsedSeconds float64      `json:"total_elapsed_seconds"`
}

const maxStepOutput = 5000

// classifyFailure inspects combined stdout+stderr output and returns a failure
// category and canned suggested fix.
func classifyFailure(combined string) (category, suggestion string) {
	switch {
	case strings.Contains(combined, "cannot") ||
		strings.Contains(combined, "undefined:") ||
		strings.Contains(combined, "imported and not used"):
		return "compile", "Check imports and type signatures in the changed files"
	case strings.Contains(combined, "--- FAIL:") ||
		strings.Contains(combined, "FAIL"):
		return "test_fail", "Review test assertions — the changed code may have broken expectations"
	case strings.Contains(combined, "context deadline exceeded") ||
		strings.Contains(combined, "test timed out"):
		return "timeout", "Tests may be hanging — check for missing context cancellation or infinite loops"
	case strings.Contains(combined, "go vet"):
		return "vet", "Run 'go vet ./...' locally to see the specific issue"
	default:
		return "unknown", ""
	}
}

// runVerifyStep runs a single command in dir and returns the result.
func runVerifyStep(ctx context.Context, dir, name string, args []string) StepResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	elapsed := time.Since(start).Seconds()

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()

	// Combined output for display and classification (matches old behavior).
	output := stdoutStr + stderrStr
	if len(output) > maxStepOutput {
		output = output[:maxStepOutput] + "\n... (truncated)"
	}

	status := "pass"
	var failureCategory, suggestedFix, stderrOut string
	if err != nil {
		status = "fail"
		// Include the error message if output is empty.
		if output == "" {
			output = err.Error()
		}
		failureCategory, suggestedFix = classifyFailure(output)
		stderrOut = strings.TrimSpace(stderrStr)
		if len(stderrOut) > maxStepOutput {
			stderrOut = stderrOut[:maxStepOutput] + "\n... (truncated)"
		}
	}

	return StepResult{
		Name:            name,
		Status:          status,
		ElapsedSeconds:  elapsed,
		Output:          strings.TrimSpace(output),
		Stderr:          stderrOut,
		FailureCategory: failureCategory,
		SuggestedFix:    suggestedFix,
	}
}

// parseCoverageTotal reads a coverage profile and extracts the total percentage.
func parseCoverageTotal(profilePath string) (float64, error) {
	data, err := os.ReadFile(profilePath)
	if err != nil {
		return 0, err
	}

	// Use go tool cover -func to parse, but as a fallback parse the profile directly.
	// The profile format has lines like: "ok  pkg  1.234s  coverage: 78.5% of statements"
	// But that's stdout from go test. The profile itself needs `go tool cover -func`.
	// Since the test output may contain coverage info, try to extract from test output first.
	// This function is called separately, so we run go tool cover.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "tool", "cover", "-func", profilePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: try to find percentage in the raw profile data.
		_ = data
		return 0, fmt.Errorf("go tool cover: %v: %s", err, string(out))
	}

	// Last line looks like: "total:	(statements)	78.5%"
	re := regexp.MustCompile(`total:\s+\(statements\)\s+([\d.]+)%`)
	matches := re.FindStringSubmatch(string(out))
	if len(matches) < 2 {
		return 0, fmt.Errorf("could not parse coverage total from output")
	}
	return strconv.ParseFloat(matches[1], 64)
}

func (s *Server) handleMergeVerify(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo is required"), nil
	}

	// Resolve and validate repo path.
	repoPath, err := filepath.Abs(repo)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo path: %v", err)), nil
	}
	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		return codedError(ErrInvalidParams, fmt.Sprintf("repo path does not exist or is not a directory: %s", repoPath)), nil
	}

	// Read optional params.
	fast := getBoolArg(req, "fast")
	coverage := getBoolArg(req, "coverage")
	// Race defaults to true.
	race := true
	if m := argsMap(req); m != nil {
		if v, ok := m["race"].(bool); ok {
			race = v
		}
	}
	packages := getStringArg(req, "packages")
	if packages == "" {
		packages = "./..."
	}

	stepTimeout := 5 * time.Minute
	result := MergeVerifyResult{
		Repo:    repoPath,
		Overall: "pass",
	}
	totalStart := time.Now()

	// Step 1: go build ./...
	buildCtx, buildCancel := context.WithTimeout(ctx, stepTimeout)
	buildResult := runVerifyStep(buildCtx, repoPath, "build", []string{"go", "build", "./..."})
	buildCancel()
	result.Steps = append(result.Steps, buildResult)
	if buildResult.Status == "fail" {
		result.Overall = "fail"
		result.FailedAt = "build"
		result.TotalElapsedSeconds = time.Since(totalStart).Seconds()
		return jsonResult(result), nil
	}

	// Step 2: go vet ./...
	vetCtx, vetCancel := context.WithTimeout(ctx, stepTimeout)
	vetResult := runVerifyStep(vetCtx, repoPath, "vet", []string{"go", "vet", "./..."})
	vetCancel()
	result.Steps = append(result.Steps, vetResult)
	if vetResult.Status == "fail" {
		result.Overall = "fail"
		result.FailedAt = "vet"
		result.TotalElapsedSeconds = time.Since(totalStart).Seconds()
		return jsonResult(result), nil
	}

	// Step 3: go test
	testArgs := []string{"go", "test"}
	if race {
		testArgs = append(testArgs, "-race")
	}
	if fast {
		testArgs = append(testArgs, "-short")
	}

	var coverProfile string
	if coverage {
		tmpFile, err := os.CreateTemp("", "mergeverify-cover-*.out")
		if err == nil {
			coverProfile = tmpFile.Name()
			tmpFile.Close()
			defer os.Remove(coverProfile)
			testArgs = append(testArgs, "-coverprofile="+coverProfile)
		}
	}

	testArgs = append(testArgs, packages)

	testCtx, testCancel := context.WithTimeout(ctx, stepTimeout)
	testResult := runVerifyStep(testCtx, repoPath, "test", testArgs)
	testCancel()

	// Parse coverage if requested and test passed.
	if coverage && coverProfile != "" && testResult.Status == "pass" {
		if cov, err := parseCoverageTotal(coverProfile); err == nil {
			testResult.Coverage = &cov
		}
	}

	result.Steps = append(result.Steps, testResult)
	if testResult.Status == "fail" {
		result.Overall = "fail"
		result.FailedAt = "test"
	}

	result.TotalElapsedSeconds = time.Since(totalStart).Seconds()

	// Return structured JSON.
	data, err := json.Marshal(result)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("json marshal: %v", err)), nil
	}
	return textResult(string(data)), nil
}
