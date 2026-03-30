// Package worktree wraps git worktree operations (add, list, remove, prune)
// with context-aware cancellation and structured error reporting.
//
// This package is intentionally independent — it has no imports from other
// internal packages and can be used standalone.
package worktree

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// WorktreeInfo holds parsed metadata for a single git worktree entry
// as returned by `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path       string // filesystem path to the worktree
	Branch     string // full ref (e.g. "refs/heads/main"), empty if detached
	HEAD       string // commit SHA at HEAD
	IsBare     bool   // true for the bare worktree entry
	IsDetached bool   // true when HEAD is detached (no branch)
}

// Create adds a new git worktree at worktreePath, checked out on a new branch.
// It runs: git worktree add -b <branch> <worktreePath>
//
// The repoPath must point to an existing git repository (or worktree within one).
func Create(ctx context.Context, repoPath, worktreePath, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "add", "-b", branch, worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: create %q branch %q: %w: %s", worktreePath, branch, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CreateFromRef adds a new git worktree at worktreePath based on an existing ref
// (branch, tag, or commit), without creating a new branch.
// It runs: git worktree add <worktreePath> <ref>
func CreateFromRef(ctx context.Context, repoPath, worktreePath, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "add", worktreePath, ref)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: create from ref %q at %q: %w: %s", ref, worktreePath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns all worktrees registered in the repository at repoPath.
// It parses the porcelain output of `git worktree list --porcelain`.
func List(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("worktree: list: %w: %s", err, stderr)
	}
	return parsePorcelain(out)
}

// Remove removes the worktree at worktreePath from the repository.
// If force is true, it passes --force to allow removal of dirty worktrees.
func Remove(ctx context.Context, repoPath, worktreePath string, force bool) error {
	args := []string{"-C", repoPath, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)

	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: remove %q: %w: %s", worktreePath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Prune removes stale worktree administrative entries (e.g. when the worktree
// directory has been deleted manually).
func Prune(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: prune: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parsePorcelain parses the output of `git worktree list --porcelain`.
//
// Porcelain format emits blocks separated by blank lines. Each block contains:
//
//	worktree <path>
//	HEAD <sha>
//	branch <ref>        (or "detached" on a line by itself)
//	bare                (optional, for bare repos)
func parsePorcelain(data []byte) ([]WorktreeInfo, error) {
	var result []WorktreeInfo
	var current *WorktreeInfo

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		// Blank line terminates a worktree entry.
		if line == "" {
			if current != nil {
				result = append(result, *current)
				current = nil
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "worktree "):
			current = &WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		case strings.HasPrefix(line, "HEAD "):
			if current != nil {
				current.HEAD = strings.TrimPrefix(line, "HEAD ")
			}
		case strings.HasPrefix(line, "branch "):
			if current != nil {
				current.Branch = strings.TrimPrefix(line, "branch ")
			}
		case line == "detached":
			if current != nil {
				current.IsDetached = true
			}
		case line == "bare":
			if current != nil {
				current.IsBare = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("worktree: parse porcelain: %w", err)
	}

	// Handle final entry if there's no trailing blank line.
	if current != nil {
		result = append(result, *current)
	}

	return result, nil
}
