package session

import (
	"context"
	"errors"
	"fmt"
)

func (m *Manager) handleSelfImprovementAcceptance(ctx context.Context, run *LoopRun, index int, worktrees []string) (*AcceptanceResult, error) {
	// Collect all diff paths across worktrees.
	var allPaths []string
	seen := make(map[string]bool)
	for _, wt := range worktrees {
		if wt == "" {
			continue
		}
		paths, err := gitDiffPathsForWorktree(wt)
		if err != nil {
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
		return &AcceptanceResult{}, nil
	}

	safe, review := ClassifySelfImprovePaths(allPaths)
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
			if err := AutoCommitAndMerge(wt, mainBranch, msg); err != nil {
				if errors.Is(err, ErrRebaseConflict) {
					// Rebase had conflicts — fall through to PR creation.
					review = append(review, safe...)
					safe = nil
					rebaseConflict = true
					break
				}
				result.Error = err.Error()
				return result, err
			}
		}
		if !rebaseConflict {
			result.AutoMerged = true
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
				return result, err
			}
			result.PRCreated = true
			result.PRURL = url
			break
		}
	}

	return result, nil
}
