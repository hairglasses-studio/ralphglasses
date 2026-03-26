package mcpserver

import (
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
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	ElapsedSeconds float64  `json:"elapsed_seconds"`
	Output         string   `json:"output"`
	Coverage       *float64 `json:"coverage,omitempty"`
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

// runVerifyStep runs a single command in dir and returns the result.
func runVerifyStep(ctx context.Context, dir, name string, args []string) StepResult {
	start := time.Now()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()

	output := string(out)
	if len(output) > maxStepOutput {
		output = output[:maxStepOutput] + "\n... (truncated)"
	}

	status := "pass"
	if err != nil {
		status = "fail"
		// Include the error message if output is empty.
		if output == "" {
			output = err.Error()
		}
	}

	return StepResult{
		Name:           name,
		Status:         status,
		ElapsedSeconds: elapsed,
		Output:         strings.TrimSpace(output),
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
		return invalidParams("repo is required"), nil
	}

	// Resolve and validate repo path.
	repoPath, err := filepath.Abs(repo)
	if err != nil {
		return invalidParams(fmt.Sprintf("invalid repo path: %v", err)), nil
	}
	info, err := os.Stat(repoPath)
	if err != nil || !info.IsDir() {
		return invalidParams(fmt.Sprintf("repo path does not exist or is not a directory: %s", repoPath)), nil
	}

	// Read optional params.
	m := argsMap(req)
	fast := false
	if m != nil {
		if v, ok := m["fast"].(bool); ok {
			fast = v
		}
	}
	coverage := false
	if m != nil {
		if v, ok := m["coverage"].(bool); ok {
			coverage = v
		}
	}
	// Race defaults to true.
	race := true
	if m != nil {
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
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return internalErr(fmt.Sprintf("json marshal: %v", err)), nil
	}
	return textResult(string(data)), nil
}
