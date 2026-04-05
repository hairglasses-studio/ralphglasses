package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// CIFailureType classifies the type of CI failure for targeted fix generation.
type CIFailureType string

const (
	CIFailureBuild   CIFailureType = "build"
	CIFailureTest    CIFailureType = "test"
	CIFailureLint    CIFailureType = "lint"
	CIFailureVet     CIFailureType = "vet"
	CIFailureUnknown CIFailureType = "unknown"
)

// CIFixResult captures the outcome of an auto-fix attempt.
type CIFixResult struct {
	Attempt       int           `json:"attempt"`
	FailureType   CIFailureType `json:"failure_type"`
	FixSessionID  string        `json:"fix_session_id,omitempty"`
	FixSucceeded  bool          `json:"fix_succeeded"`
	FixError      string        `json:"fix_error,omitempty"`
	FixDurationMs int64         `json:"fix_duration_ms"`
}

// classifyCIFailure determines what type of CI failure occurred from
// the verify command and its output.
func classifyCIFailure(command, output string) CIFailureType {
	lower := strings.ToLower(command + " " + output)

	switch {
	case strings.Contains(lower, "go vet") || strings.Contains(lower, "govet"):
		return CIFailureVet
	case strings.Contains(lower, "golangci-lint") || strings.Contains(lower, "go lint") ||
		strings.Contains(lower, "lint"):
		return CIFailureLint
	case strings.Contains(lower, "go test") || strings.Contains(lower, "test"):
		// Check output patterns to distinguish build vs test failures
		if strings.Contains(lower, "cannot find package") ||
			strings.Contains(lower, "undefined:") ||
			strings.Contains(lower, "imported and not used") ||
			strings.Contains(lower, "syntax error") ||
			strings.Contains(lower, "does not compile") {
			return CIFailureBuild
		}
		return CIFailureTest
	case strings.Contains(lower, "go build") || strings.Contains(lower, "build"):
		return CIFailureBuild
	default:
		// Detect build errors from output patterns
		if strings.Contains(lower, "undefined:") ||
			strings.Contains(lower, "cannot find package") ||
			strings.Contains(lower, "syntax error") {
			return CIFailureBuild
		}
		return CIFailureUnknown
	}
}

// buildFixPrompt generates a targeted fix prompt from the CI failure context.
func buildFixPrompt(failureType CIFailureType, command, output string) string {
	var sb strings.Builder

	sb.WriteString("A CI verification step has failed. Fix the issue and ensure the verification passes.\n\n")

	switch failureType {
	case CIFailureBuild:
		sb.WriteString("## Build Failure\n\n")
		sb.WriteString("The code does not compile. Fix all compilation errors.\n\n")
	case CIFailureTest:
		sb.WriteString("## Test Failure\n\n")
		sb.WriteString("One or more tests are failing. Fix the test failures without disabling tests.\n\n")
	case CIFailureLint:
		sb.WriteString("## Lint Failure\n\n")
		sb.WriteString("Linting found issues. Fix all lint warnings and errors.\n\n")
	case CIFailureVet:
		sb.WriteString("## Vet Failure\n\n")
		sb.WriteString("Go vet found issues. Fix all vet warnings.\n\n")
	default:
		sb.WriteString("## Verification Failure\n\n")
		sb.WriteString("A verification command failed. Analyze the output and fix the underlying issue.\n\n")
	}

	sb.WriteString("### Failed Command\n```\n")
	sb.WriteString(command)
	sb.WriteString("\n```\n\n")

	sb.WriteString("### Error Output\n```\n")
	// Truncate output to keep prompt reasonable
	out := output
	if len(out) > 3000 {
		out = out[:3000] + "\n... (truncated)"
	}
	sb.WriteString(out)
	sb.WriteString("\n```\n\n")

	sb.WriteString("### Instructions\n")
	sb.WriteString("1. Read the error output carefully to identify the root cause\n")
	sb.WriteString("2. Fix the issue in the source code\n")
	sb.WriteString("3. Verify your fix resolves the error\n")
	sb.WriteString("4. Do not introduce new issues or modify unrelated code\n")

	return sb.String()
}

// attemptAutoFix tries to fix a CI failure by launching a worker session
// with a targeted fix prompt. Returns the fix result and whether verification
// now passes.
func (m *Manager) attemptAutoFix(
	ctx context.Context,
	run *LoopRun,
	worktreePath string,
	failedVerification LoopVerification,
	attempt int,
) (*CIFixResult, error) {
	start := time.Now()
	profile := run.Profile

	failureType := classifyCIFailure(failedVerification.Command, failedVerification.Output)
	fixPrompt := buildFixPrompt(failureType, failedVerification.Command, failedVerification.Output)

	result := &CIFixResult{
		Attempt:     attempt,
		FailureType: failureType,
	}

	slog.Info("auto-ci-fix: attempting fix",
		"loop", run.ID,
		"attempt", attempt,
		"failure_type", failureType,
		"command", failedVerification.Command,
	)

	// Launch a worker session in the same worktree to fix the issue
	fixOpts := LaunchOptions{
		Provider: profile.WorkerProvider,
		RepoPath: worktreePath,
		Prompt:   fixPrompt,
		Model:    profile.WorkerModel,
	}
	if profile.WorkerBudgetUSD > 0 {
		fixOpts.MaxBudgetUSD = profile.WorkerBudgetUSD / 2 // half budget for fix attempts
	}
	if profile.MaxWorkerTurns > 0 {
		fixOpts.MaxTurns = max(
			// half turns for fix attempts
			profile.MaxWorkerTurns/2, 5)
	}

	fixSession, err := m.Launch(ctx, fixOpts)
	if err != nil {
		result.FixError = fmt.Sprintf("launch fix session: %v", err)
		result.FixDurationMs = time.Since(start).Milliseconds()
		return result, err
	}
	result.FixSessionID = fixSession.ID

	// Wait for the fix session to complete by blocking on its done channel.
	select {
	case <-fixSession.doneCh:
	case <-ctx.Done():
		result.FixError = "context canceled while waiting for fix session"
		result.FixDurationMs = time.Since(start).Milliseconds()
		return result, ctx.Err()
	}

	// Re-run verification
	reVerification, reErr := runLoopVerification(ctx, worktreePath, profile.VerifyCommands)
	result.FixDurationMs = time.Since(start).Milliseconds()

	if reErr != nil {
		result.FixError = fmt.Sprintf("re-verification failed: %v", reErr)
		slog.Warn("auto-ci-fix: fix attempt failed",
			"loop", run.ID,
			"attempt", attempt,
			"error", reErr,
		)
		return result, reErr
	}

	// Check all verification results passed
	allPassed := true
	for _, v := range reVerification {
		if v.Status != "completed" {
			allPassed = false
			break
		}
	}

	result.FixSucceeded = allPassed
	if allPassed {
		slog.Info("auto-ci-fix: fix succeeded",
			"loop", run.ID,
			"attempt", attempt,
			"failure_type", failureType,
		)
	}

	return result, nil
}

// maxAutoFixRetries returns the effective max retry count for auto-CI-fix.
func maxAutoFixRetries(profile LoopProfile) int {
	if !profile.AutoFixOnVerifyFail {
		return 0
	}
	if profile.MaxAutoFixRetries > 0 {
		return profile.MaxAutoFixRetries
	}
	return 2 // default when auto-fix is enabled
}
