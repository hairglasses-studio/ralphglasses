package session

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// MergeResult describes the outcome of merging a branch.
type MergeResult struct {
	Branch    string
	Success   bool
	Conflicts []string
	Error     string
}

// CycleMergeResult describes the combined outcome of multiple merges.
type CycleMergeResult struct {
	TargetBranch string
	Results      []MergeResult
}

// MergeParallelBranches merges a list of branches into a target branch sequentially.
// If a merge has conflicts, it uses conflictStrategy ("ours", "theirs", "manual") to resolve or abort.
func MergeParallelBranches(ctx context.Context, repoPath, targetBranch string, branches []string, conflictStrategy string) (*CycleMergeResult, error) {
	if conflictStrategy == "" {
		conflictStrategy = "manual"
	}

	// Ensure we are on the target branch
	cmd := exec.CommandContext(ctx, "git", "checkout", targetBranch)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to checkout target branch %s: %v, output: %s", targetBranch, err, string(out))
	}

	result := &CycleMergeResult{
		TargetBranch: targetBranch,
	}

	for _, branch := range branches {
		mergeRes := MergeResult{Branch: branch}

		args := []string{"merge", "--no-ff", "-m", fmt.Sprintf("Merge parallel worktree branch %s", branch)}
		if conflictStrategy == "ours" {
			args = append(args, "-s", "recursive", "-X", "ours")
		} else if conflictStrategy == "theirs" {
			args = append(args, "-s", "recursive", "-X", "theirs")
		}
		args = append(args, branch)

		cmdMerge := exec.CommandContext(ctx, "git", args...)
		cmdMerge.Dir = repoPath
		out, err := cmdMerge.CombinedOutput()
		
		if err != nil {
			mergeRes.Success = false
			mergeRes.Error = err.Error()
			
			// Detect conflicts
			if strings.Contains(string(out), "CONFLICT") {
				// Get conflicted files
				cmdStatus := exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=U")
				cmdStatus.Dir = repoPath
				if confOut, confErr := cmdStatus.Output(); confErr == nil {
					files := strings.Split(strings.TrimSpace(string(confOut)), "\n")
					for _, f := range files {
						if f != "" {
							mergeRes.Conflicts = append(mergeRes.Conflicts, f)
						}
					}
				}
				
				if conflictStrategy == "manual" {
					// Abort the merge
					cmdAbort := exec.CommandContext(ctx, "git", "merge", "--abort")
					cmdAbort.Dir = repoPath
					_ = cmdAbort.Run()
				} else {
					// If strategy is ours/theirs, but git still conflicts (e.g. binary files or edge cases)
					// we should commit or abort. Let's abort to be safe if git couldn't resolve it automatically.
					cmdAbort := exec.CommandContext(ctx, "git", "merge", "--abort")
					cmdAbort.Dir = repoPath
					_ = cmdAbort.Run()
				}
			} else {
				mergeRes.Error = fmt.Sprintf("Merge failed: %s", string(out))
			}
		} else {
			mergeRes.Success = true
		}
		
		result.Results = append(result.Results, mergeRes)
	}

	return result, nil
}
