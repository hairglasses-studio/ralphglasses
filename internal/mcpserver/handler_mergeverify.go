package mcpserver

import (
	"bytes"
	"context"
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
	Summary             string       `json:"summary"`
	FailedAt            string       `json:"failed_at,omitempty"`
	TotalElapsedSeconds float64      `json:"total_elapsed_seconds"`
}

// buildSummary generates a human-readable summary from step results.
func buildSummary(steps []StepResult) string {
	var passed, failed, skipped []string
	for _, s := range steps {
		switch s.Status {
		case "pass":
			passed = append(passed, s.Name)
		case "fail":
			failed = append(failed, s.Name)
		case "skip":
			skipped = append(skipped, s.Name)
		}
	}

	var parts []string
	if len(passed) > 0 {
		parts = append(parts, strings.Join(passed, " and ")+" passed")
	}
	if len(failed) > 0 {
		parts = append(parts, strings.Join(failed, " and ")+" failed")
	}
	if len(skipped) > 0 {
		parts = append(parts, strings.Join(skipped, " and ")+" skipped")
	}
	if len(parts) == 0 {
		return "no steps executed"
	}
	return strings.Join(parts, ", ")
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
func parseCoverageTotal(ctx context.Context, profilePath string) (float64, error) {
	// Use go tool cover -func to parse the profile.
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "go", "tool", "cover", "-func", profilePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("go tool cover: %w: %s", err, string(out))
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

	// Validate path before resolving — reject traversal and escapes.
	if err := ValidatePath(repo, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo path: %v", err)), nil
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

	// skipRemaining tracks whether to skip subsequent steps (fast mode).
	skipRemaining := false

	// Step 1: go build ./...
	buildCtx, buildCancel := context.WithTimeout(ctx, stepTimeout)
	buildResult := runVerifyStep(buildCtx, repoPath, "build", []string{"go", "build", "./..."})
	buildCancel()
	result.Steps = append(result.Steps, buildResult)
	if buildResult.Status == "fail" {
		result.Overall = "fail"
		result.FailedAt = "build"
		if fast {
			skipRemaining = true
		}
	}

	// Step 2: go vet ./...
	if skipRemaining {
		result.Steps = append(result.Steps, StepResult{Name: "vet", Status: "skip"})
	} else {
		vetCtx, vetCancel := context.WithTimeout(ctx, stepTimeout)
		vetResult := runVerifyStep(vetCtx, repoPath, "vet", []string{"go", "vet", "./..."})
		vetCancel()
		result.Steps = append(result.Steps, vetResult)
		if vetResult.Status == "fail" {
			result.Overall = "fail"
			if result.FailedAt == "" {
				result.FailedAt = "vet"
			}
			if fast {
				skipRemaining = true
			}
		}
	}

	// Step 3: go test
	if skipRemaining {
		result.Steps = append(result.Steps, StepResult{Name: "test", Status: "skip"})
	} else {
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
			if cov, err := parseCoverageTotal(ctx, coverProfile); err == nil {
				testResult.Coverage = &cov
			}
		}

		result.Steps = append(result.Steps, testResult)
		if testResult.Status == "fail" {
			result.Overall = "fail"
			if result.FailedAt == "" {
				result.FailedAt = "test"
			}
		}
	}

	result.TotalElapsedSeconds = time.Since(totalStart).Seconds()
	result.Summary = buildSummary(result.Steps)

	return jsonResult(result), nil
}
