package mcpserver

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// PackageCoverage holds coverage data for a single package.
type PackageCoverage struct {
	Name     string  `json:"name"`
	Coverage float64 `json:"coverage"`
	Pass     bool    `json:"pass"`
}

// parseCoverOutput parses the output of `go tool cover -func` and returns
// per-package coverage totals and the overall coverage percentage.
func parseCoverOutput(output string) ([]PackageCoverage, float64) {
	// Track per-package function counts and total coverage for averaging.
	type pkgAccum struct {
		totalPct float64
		funcs    int
	}
	pkgs := make(map[string]*pkgAccum)
	var pkgOrder []string
	var overall float64

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// The "total:" line has the overall coverage.
		if strings.HasPrefix(line, "total:") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				pctStr := strings.TrimSuffix(fields[len(fields)-1], "%")
				if v, err := strconv.ParseFloat(pctStr, 64); err == nil {
					overall = v
				}
			}
			continue
		}

		// Regular lines: path/to/pkg/file.go:line:  FuncName  XX.X%
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pctStr := strings.TrimSuffix(fields[len(fields)-1], "%")
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			continue
		}

		// Extract package path from the file path (everything before the last /).
		filePath := fields[0]
		// Remove :line: suffix
		if idx := strings.Index(filePath, ":"); idx > 0 {
			filePath = filePath[:idx]
		}
		// Package is the directory portion.
		pkgPath := filepath.Dir(filePath)

		acc, ok := pkgs[pkgPath]
		if !ok {
			acc = &pkgAccum{}
			pkgs[pkgPath] = acc
			pkgOrder = append(pkgOrder, pkgPath)
		}
		acc.totalPct += pct
		acc.funcs++
	}

	// Build result with averaged coverage per package.
	result := make([]PackageCoverage, 0, len(pkgOrder))
	for _, name := range pkgOrder {
		acc := pkgs[name]
		avg := 0.0
		if acc.funcs > 0 {
			avg = acc.totalPct / float64(acc.funcs)
		}
		// Round to 1 decimal place.
		avg = float64(int(avg*10+0.5)) / 10
		result = append(result, PackageCoverage{
			Name:     name,
			Coverage: avg,
		})
	}

	return result, overall
}

func (s *Server) handleCoverageReport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo is required"), nil
	}

	packages := getStringArg(req, "packages")
	if packages == "" {
		packages = "./..."
	}
	threshold := getNumberArg(req, "threshold", 70)

	// Resolve repo path: use directly if absolute, otherwise look up by name.
	var repoPath string
	if filepath.IsAbs(repo) {
		repoPath = repo
	} else {
		if err := ValidateRepoName(repo); err != nil {
			return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
		}
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		r := s.findRepo(repo)
		if r == nil {
			return notFound(fmt.Sprintf("repo not found: %s", repo)), nil
		}
		repoPath = r.Path
	}

	// Create temp file for coverage profile.
	tmpFile, err := os.CreateTemp("", "coverage-*.out")
	if err != nil {
		return internalErr(fmt.Sprintf("create temp file: %v", err)), nil
	}
	coverProfile := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(coverProfile)

	// Run go test with coverage, 2 minute timeout.
	testCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	start := time.Now()

	pkgArgs := strings.Split(packages, ",")
	args := append([]string{"test", "-coverprofile=" + coverProfile, "-count=1"}, pkgArgs...)
	testCmd := exec.CommandContext(testCtx, "go", args...)
	testCmd.Dir = repoPath

	testOutput, testErr := testCmd.CombinedOutput()
	elapsed := time.Since(start).Seconds()
	// Round to 1 decimal.
	elapsed = float64(int(elapsed*10+0.5)) / 10

	testResult := "pass"
	if testErr != nil {
		testResult = "fail"
	}

	// If the coverage profile was not created (build error), return structured error info.
	if _, statErr := os.Stat(coverProfile); os.IsNotExist(statErr) {
		return jsonResult(map[string]any{
			"overall_coverage": 0.0,
			"threshold":        threshold,
			"pass":             false,
			"packages":         []PackageCoverage{},
			"test_result":      testResult,
			"elapsed_seconds":  elapsed,
			"error":            string(testOutput),
		}), nil
	}

	// Run go tool cover -func to get per-function coverage.
	coverCmd := exec.CommandContext(testCtx, "go", "tool", "cover", "-func="+coverProfile)
	coverCmd.Dir = repoPath
	coverOutput, coverErr := coverCmd.CombinedOutput()
	if coverErr != nil {
		return jsonResult(map[string]any{
			"overall_coverage": 0.0,
			"threshold":        threshold,
			"pass":             false,
			"packages":         []PackageCoverage{},
			"test_result":      testResult,
			"elapsed_seconds":  elapsed,
			"error":            fmt.Sprintf("cover -func failed: %s", string(coverOutput)),
		}), nil
	}

	// Parse coverage output.
	pkgCoverages, overall := parseCoverOutput(string(coverOutput))

	// Apply threshold to each package and overall.
	overallPass := overall >= threshold
	for i := range pkgCoverages {
		pkgCoverages[i].Pass = pkgCoverages[i].Coverage >= threshold
	}

	result := map[string]any{
		"overall_coverage": overall,
		"threshold":        threshold,
		"pass":             overallPass && testResult == "pass",
		"packages":         pkgCoverages,
		"test_result":      testResult,
		"elapsed_seconds":  elapsed,
	}

	return jsonResult(result), nil
}
