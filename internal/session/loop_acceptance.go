package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// AcceptanceTraceResult bundles the acceptance result with the detailed trace.
type AcceptanceTraceResult struct {
	Result *AcceptanceResult
	Trace  AcceptanceTrace
}

func (m *Manager) handleSelfImprovementAcceptance(ctx context.Context, run *LoopRun, index int, worktrees []string) (*AcceptanceResult, error) {
	atr, err := m.handleSelfImprovementAcceptanceTraced(ctx, run, index, worktrees)
	return atr.Result, err
}

func (m *Manager) handleSelfImprovementAcceptanceTraced(ctx context.Context, run *LoopRun, index int, worktrees []string) (AcceptanceTraceResult, error) {
	var trace AcceptanceTrace

	// Collect all diff paths across worktrees.
	var allPaths []string
	seen := make(map[string]bool)
	for _, wt := range worktrees {
		if wt == "" {
			continue
		}
		paths, err := gitDiffPathsForWorktree(wt)
		if err != nil {
			slog.Warn("acceptance: failed to get diff paths", "worktree", wt, "error", err)
			continue
		}
		for _, p := range paths {
			if !seen[p] {
				seen[p] = true
				allPaths = append(allPaths, p)
			}
		}
	}

	if len(allPaths) == 0 {
		trace.Reason = "worker_no_changes"
		slog.Info("acceptance: no diff paths from any worktree", "worktree_count", len(worktrees))
		return AcceptanceTraceResult{
			Result: &AcceptanceResult{},
			Trace:  trace,
		}, nil
	}

	safe, review := ClassifySelfImprovePaths(allPaths)
	trace.SafePaths = safe
	trace.ReviewPaths = review

	result := &AcceptanceResult{
		SafePaths:   safe,
		ReviewPaths: review,
	}

	mainBranch := "main"
	autoMerge := run.Profile.AutoMergeAll || len(review) == 0
	rebaseConflict := false
	if autoMerge {
		// Auto-merge: either all paths are safe, or AutoMergeAll is set (verification passed).
		for _, wt := range worktrees {
			if wt == "" {
				continue
			}
			msg := fmt.Sprintf("self-improve: auto-merge (%s)", buildDiffSummary(safe))
			commitTrace, err := AutoCommitAndMergeTraced(wt, mainBranch, msg)
			trace.StagedFileCount += commitTrace.StagedFileCount
			if commitTrace.Reason == "no_staged_files" {
				slog.Warn("acceptance: AutoCommitAndMerge found no staged files despite diff paths",
					"worktree", wt, "diff_paths", len(allPaths))
			}
			if err != nil {
				if errors.Is(err, ErrRebaseConflict) {
					// Rebase had conflicts — fall through to PR creation.
					review = append(review, safe...)
					safe = nil
					rebaseConflict = true
					break
				}
				result.Error = err.Error()
				trace.Reason = commitTrace.Reason
				return AcceptanceTraceResult{Result: result, Trace: trace}, err
			}
		}
		if !rebaseConflict {
			result.AutoMerged = true
			trace.Reason = "auto_merged"
		}
	}
	if !autoMerge || rebaseConflict {
		// Needs review — create PR from the first non-empty worktree.
		for _, wt := range worktrees {
			if wt == "" {
				continue
			}
			title := fmt.Sprintf("self-improve: %s", buildDiffSummary(allPaths))
			url, err := CreateReviewPR(wt, mainBranch, title, review)
			if err != nil {
				result.Error = err.Error()
				trace.Reason = "pr_failed"
				return AcceptanceTraceResult{Result: result, Trace: trace}, err
			}
			result.PRCreated = true
			result.PRURL = url
			trace.Reason = "pr_created"
			break
		}
	}

	return AcceptanceTraceResult{Result: result, Trace: trace}, nil
}
