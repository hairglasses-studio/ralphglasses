package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SelfTestConfig configures a recursive self-test run where ralphglasses
// builds a snapshot binary and tests itself in isolation.
type SelfTestConfig struct {
	RepoPath       string   `json:"repo_path"`
	BinaryPath     string   `json:"binary_path"`
	MaxIterations  int      `json:"max_iterations"`
	BudgetUSD      float64  `json:"budget_usd"`
	AllowedPaths   []string `json:"allowed_paths,omitempty"`
	ForbiddenPaths []string `json:"forbidden_paths,omitempty"`
	UseSnapshot    bool     `json:"use_snapshot"`
	DryRun         bool     `json:"dry_run,omitempty"`
	BudgetOverride float64  `json:"budget_override,omitempty"`
}

// DefaultSelfTestConfig returns a SelfTestConfig with sane defaults.
func DefaultSelfTestConfig(repoPath string) SelfTestConfig {
	return SelfTestConfig{
		RepoPath:      repoPath,
		MaxIterations: 3,
		BudgetUSD:     5.0,
		UseSnapshot:   true,
	}
}

// applyDefaults fills zero-value fields with defaults.
func (c *SelfTestConfig) applyDefaults() {
	if c.MaxIterations <= 0 {
		c.MaxIterations = 3
	}
	if c.BudgetOverride > 0 {
		c.BudgetUSD = c.BudgetOverride
	}
	if c.BudgetUSD <= 0 {
		c.BudgetUSD = 5.0
	}
}

// SelfTestRunner holds the prepared state for a self-test execution.
type SelfTestRunner struct {
	Config     SelfTestConfig
	BinaryHash string
	BinaryPath string
	PreparedAt time.Time
	GitTag     string

	observations []map[string]any
}

// SelfTestResult captures the outcome of a complete self-test run.
type SelfTestResult struct {
	Iterations   int              `json:"iterations"`
	Observations []map[string]any `json:"observations"`
	TotalCostUSD float64          `json:"total_cost_usd"`
	Duration     time.Duration    `json:"duration"`
	BinaryHash   string           `json:"binary_hash"`
}

// Prepare builds (or locates) the snapshot binary and returns a runner.
// If UseSnapshot is true, it compiles a fresh binary into .ralph/bin/.
// It also tags the current commit for traceability.
func Prepare(ctx context.Context, config SelfTestConfig) (*SelfTestRunner, error) {
	config.applyDefaults()

	if config.RepoPath == "" {
		return nil, fmt.Errorf("selftest: RepoPath is required")
	}

	runner := &SelfTestRunner{
		Config:     config,
		PreparedAt: time.Now(),
	}

	if config.UseSnapshot {
		binDir := filepath.Join(config.RepoPath, ".ralph", "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			return nil, fmt.Errorf("selftest: create bin dir: %w", err)
		}

		snapshotPath := filepath.Join(binDir, "ralphglasses-snapshot")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", snapshotPath, "./...")
		cmd.Dir = config.RepoPath
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("selftest: build snapshot: %w\n%s", err, output)
		}

		runner.BinaryPath = snapshotPath
	} else {
		// Use the provided binary path or fall back to repo path default.
		if config.BinaryPath != "" {
			runner.BinaryPath = config.BinaryPath
		} else {
			runner.BinaryPath = filepath.Join(config.RepoPath, "ralphglasses")
		}
	}

	// Compute SHA256 of the binary (skip if file doesn't exist yet in non-snapshot mode).
	hash, err := hashFile(runner.BinaryPath)
	if err != nil && config.UseSnapshot {
		return nil, fmt.Errorf("selftest: hash binary: %w", err)
	}
	runner.BinaryHash = hash

	// Tag current commit for traceability.
	tag := fmt.Sprintf("selftest-%d", runner.PreparedAt.Unix())
	tagCmd := exec.CommandContext(ctx, "git", "tag", "-f", tag)
	tagCmd.Dir = config.RepoPath
	if output, err := tagCmd.CombinedOutput(); err != nil {
		// Non-fatal: tagging may fail if not in a git repo or no commits.
		_ = output
	} else {
		runner.GitTag = tag
	}

	return runner, nil
}

// Run executes the self-test loop, invoking the snapshot binary for each
// iteration and collecting observations. It respects the budget limit.
// If DryRun is true, validates config and returns immediately with zero iterations.
func (r *SelfTestRunner) Run(ctx context.Context) (*SelfTestResult, error) {
	start := time.Now()
	result := &SelfTestResult{
		BinaryHash: r.BinaryHash,
	}

	if r.Config.DryRun {
		result.Iterations = 0
		result.Duration = time.Since(start)
		result.Observations = []map[string]any{{
			"status":  "dry_run",
			"message": "config validated, no iterations executed",
		}}
		return result, nil
	}

	var totalCost float64

	for i := 0; i < r.Config.MaxIterations; i++ {
		select {
		case <-ctx.Done():
			result.Iterations = i
			result.Observations = r.observations
			result.TotalCostUSD = totalCost
			result.Duration = time.Since(start)
			return result, ctx.Err()
		default:
		}

		// Budget check before each iteration.
		if totalCost >= r.Config.BudgetUSD {
			break
		}

		obs, cost, err := r.runIteration(ctx, i)
		if err != nil {
			obs["error"] = err.Error()
		}
		obs["iteration"] = i
		obs["timestamp"] = time.Now().UTC().Format(time.RFC3339)

		r.observations = append(r.observations, obs)
		totalCost += cost
		result.Iterations = i + 1
	}

	result.Observations = r.observations
	result.TotalCostUSD = totalCost
	result.Duration = time.Since(start)
	return result, nil
}

// runIteration executes a single self-test iteration by invoking the binary.
// Returns the observation map, cost for this iteration, and any error.
func (r *SelfTestRunner) runIteration(ctx context.Context, iteration int) (map[string]any, float64, error) {
	obs := make(map[string]any)
	iterStart := time.Now()

	cmd := exec.CommandContext(ctx, r.BinaryPath, "selftest", "--iteration", fmt.Sprintf("%d", iteration))
	cmd.Dir = r.Config.RepoPath
	cmd.Env = append(os.Environ(),
		"RALPH_SELF_TEST=1",
		fmt.Sprintf("RALPH_SELFTEST_ITERATION=%d", iteration),
		fmt.Sprintf("RALPH_SELFTEST_BUDGET=%.2f", r.Config.BudgetUSD),
	)

	// Apply path restrictions via environment.
	if len(r.Config.AllowedPaths) > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RALPH_SELFTEST_ALLOWED=%s",
			joinPathList(r.Config.AllowedPaths)))
	}
	if len(r.Config.ForbiddenPaths) > 0 {
		cmd.Env = append(cmd.Env, fmt.Sprintf("RALPH_SELFTEST_FORBIDDEN=%s",
			joinPathList(r.Config.ForbiddenPaths)))
	}

	output, err := cmd.CombinedOutput()
	elapsed := time.Since(iterStart)

	obs["duration_ms"] = elapsed.Milliseconds()
	if cmd.ProcessState != nil {
		obs["exit_code"] = cmd.ProcessState.ExitCode()
	}
	obs["binary_hash"] = r.BinaryHash

	// Try to parse structured output from the binary.
	var structured map[string]any
	if json.Unmarshal(output, &structured) == nil {
		obs["output"] = structured
		if cost, ok := structured["cost_usd"].(float64); ok {
			return obs, cost, err
		}
	} else {
		// Truncate raw output to avoid bloating observations.
		raw := string(output)
		if len(raw) > 4096 {
			raw = raw[:4096] + "...(truncated)"
		}
		obs["raw_output"] = raw
	}

	return obs, 0, err
}

// hashFile computes the SHA256 hex digest of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// joinPathList joins a slice of paths with ":" separator for env vars.
func joinPathList(paths []string) string {
	return strings.Join(paths, ":")
}

// RegressionSeverity describes the severity of a detected regression.
type RegressionSeverity string

const (
	SeverityCritical RegressionSeverity = "critical"
	SeverityWarning  RegressionSeverity = "warning"
	SeverityInfo     RegressionSeverity = "info"
)

// Regression describes a single regression detected between two self-test runs.
type Regression struct {
	Field    string             `json:"field"`
	OldValue any                `json:"old_value"`
	NewValue any                `json:"new_value"`
	Severity RegressionSeverity `json:"severity"`
	Message  string             `json:"message"`
}

// CompareResults compares two SelfTestResult values and returns any regressions
// detected. It checks for: iteration count drops, cost increases, duration
// regressions, and new errors in observations.
func CompareResults(current, previous SelfTestResult) []Regression {
	var regressions []Regression

	// Iteration count drop (fewer iterations completed).
	if current.Iterations < previous.Iterations && previous.Iterations > 0 {
		regressions = append(regressions, Regression{
			Field:    "iterations",
			OldValue: previous.Iterations,
			NewValue: current.Iterations,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("iteration count dropped from %d to %d", previous.Iterations, current.Iterations),
		})
	}

	// Cost increase > 50%.
	if previous.TotalCostUSD > 0 && current.TotalCostUSD > previous.TotalCostUSD*1.5 {
		regressions = append(regressions, Regression{
			Field:    "total_cost_usd",
			OldValue: previous.TotalCostUSD,
			NewValue: current.TotalCostUSD,
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("cost increased from $%.2f to $%.2f (>50%%)", previous.TotalCostUSD, current.TotalCostUSD),
		})
	}

	// Duration regression > 100% (took more than 2x as long).
	if previous.Duration > 0 && current.Duration > previous.Duration*2 {
		regressions = append(regressions, Regression{
			Field:    "duration",
			OldValue: previous.Duration.String(),
			NewValue: current.Duration.String(),
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("duration increased from %s to %s (>2x)", previous.Duration, current.Duration),
		})
	}

	// New errors in observations that weren't present before.
	prevErrors := countObservationErrors(previous.Observations)
	currErrors := countObservationErrors(current.Observations)
	if currErrors > prevErrors {
		sev := SeverityWarning
		if currErrors-prevErrors >= 3 {
			sev = SeverityCritical
		}
		regressions = append(regressions, Regression{
			Field:    "observation_errors",
			OldValue: prevErrors,
			NewValue: currErrors,
			Severity: sev,
			Message:  fmt.Sprintf("observation errors increased from %d to %d", prevErrors, currErrors),
		})
	}

	// Binary hash changed (informational).
	if previous.BinaryHash != "" && current.BinaryHash != "" && current.BinaryHash != previous.BinaryHash {
		regressions = append(regressions, Regression{
			Field:    "binary_hash",
			OldValue: previous.BinaryHash,
			NewValue: current.BinaryHash,
			Severity: SeverityInfo,
			Message:  "binary hash changed between runs",
		})
	}

	return regressions
}

// countObservationErrors counts observations containing an "error" key.
func countObservationErrors(observations []map[string]any) int {
	count := 0
	for _, obs := range observations {
		if _, hasErr := obs["error"]; hasErr {
			count++
		}
	}
	return count
}
