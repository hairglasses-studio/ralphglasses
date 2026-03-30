package session

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CleanupLoopWorktrees removes worktree directories for a specific loop.
func CleanupLoopWorktrees(repoPath, loopID string) error {
	if strings.TrimSpace(repoPath) == "" {
		return fmt.Errorf("cleanup: repo path is empty")
	}

	// Sanitize loopID the same way createLoopWorktree does to prevent path traversal.
	sanitized := sanitizeLoopName(loopID)
	if sanitized == "" || sanitized == "loop" && loopID != "loop" {
		return fmt.Errorf("cleanup: invalid loop ID %q", loopID)
	}

	expectedBase := filepath.Join(repoPath, ".ralph", "worktrees", "loops")
	dir := filepath.Join(expectedBase, sanitized)

	// Safety: verify the resolved path is within the expected boundary.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("cleanup: resolve path: %w", err)
	}
	absBase, err := filepath.Abs(expectedBase)
	if err != nil {
		return fmt.Errorf("cleanup: resolve base: %w", err)
	}
	if !strings.HasPrefix(absDir, absBase+string(filepath.Separator)) {
		return fmt.Errorf("cleanup: path %q escapes worktree boundary %q", absDir, absBase)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}

	// Edge case 2.2.5: enumerate and handle each worktree subdirectory.
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		// Not a directory or unreadable — fall through to remove.
		entries = nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		wtPath := filepath.Join(dir, e.Name())
		if worktreeIsDirty(wtPath) {
			slog.Warn("dirty worktree on stop — committing changes before cleanup",
				"path", wtPath, "loop", loopID)
			commitDirtyWorktree(wtPath)
		}
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove loop worktrees %s: %w", sanitized, err)
	}

	// Prune stale git worktree references and orphaned branches.
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	_ = cmd.Run() // best-effort
	cleanupOrphanedLoopBranches(repoPath, sanitized)
	return nil
}

// CleanupStaleWorktrees removes loop worktree directories older than the given threshold.
// Returns the number of directories cleaned up.
func CleanupStaleWorktrees(repoPath string, olderThan time.Duration) (int, error) {
	base := filepath.Join(repoPath, ".ralph", "worktrees", "loops")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read worktree dir: %w", err)
	}

	cutoff := time.Now().Add(-olderThan)
	cleaned := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			p := filepath.Join(base, e.Name())

			// Skip worktrees with an active index.lock file to avoid
			// removing directories that are mid-operation.
			lockPath := filepath.Join(p, ".git", "index.lock")
			if _, err := os.Stat(lockPath); err == nil {
				slog.Debug("skipping worktree with index.lock", "path", p)
				continue
			}

			// Skip worktrees with a .lock file (git worktree lock).
			worktreeLock := p + ".lock"
			if _, err := os.Stat(worktreeLock); err == nil {
				slog.Debug("skipping locked worktree", "path", p)
				continue
			}

			// Skip worktrees with uncommitted changes.
			if worktreeIsDirty(p) {
				slog.Warn("skipping stale worktree with uncommitted changes", "path", p)
				continue
			}

			if err := os.RemoveAll(p); err == nil {
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		cmd := exec.Command("git", "worktree", "prune")
		cmd.Dir = repoPath
		_ = cmd.Run()
	}
	return cleaned, nil
}

// worktreeIsDirty returns true if the given path has uncommitted changes
// according to `git status --porcelain`. Returns false if the command
// fails (e.g., path is not a git directory) to avoid blocking cleanup.
func worktreeIsDirty(path string) bool {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false // assume clean if git fails (not a valid repo)
	}
	return len(strings.TrimSpace(string(out))) > 0
}

// commitDirtyWorktree does a best-effort commit of uncommitted changes in a
// worktree before cleanup. This prevents data loss when a loop is stopped
// mid-edit (edge case 2.2.5).
func commitDirtyWorktree(path string) {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")

	addCmd := exec.Command("git", "-C", path, "add", "-A")
	addCmd.Env = env
	if err := addCmd.Run(); err != nil {
		slog.Warn("failed to stage dirty worktree", "path", path, "err", err)
		return
	}

	commitCmd := exec.Command("git", "-C", path, "commit", "-m", "auto: save dirty worktree before cleanup")
	commitCmd.Env = env
	if err := commitCmd.Run(); err != nil {
		slog.Warn("failed to commit dirty worktree", "path", path, "err", err)
	}
}

// WorktreeInfo describes an active worktree directory.
type WorktreeInfo struct {
	Path    string `json:"path"`
	Loop    string `json:"loop"`
	Dirty   bool   `json:"dirty"`
	Branch  string `json:"branch,omitempty"`
	ModTime string `json:"mod_time,omitempty"`
}

// ListWorktrees returns information about all loop worktrees under a repo.
func ListWorktrees(repoPath string) ([]WorktreeInfo, error) {
	base := filepath.Join(repoPath, ".ralph", "worktrees", "loops")
	loopDirs, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read worktree dir: %w", err)
	}

	var result []WorktreeInfo
	for _, loopEntry := range loopDirs {
		if !loopEntry.IsDir() {
			continue
		}
		loopName := loopEntry.Name()
		iterDirs, err := os.ReadDir(filepath.Join(base, loopName))
		if err != nil {
			continue
		}
		for _, iterEntry := range iterDirs {
			if !iterEntry.IsDir() {
				continue
			}
			wtPath := filepath.Join(base, loopName, iterEntry.Name())
			info := WorktreeInfo{
				Path:  wtPath,
				Loop:  loopName,
				Dirty: worktreeIsDirty(wtPath),
			}
			if fi, err := iterEntry.Info(); err == nil {
				info.ModTime = fi.ModTime().Format(time.RFC3339)
			}
			// Try to get branch name.
			cmd := exec.Command("git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD")
			if out, err := cmd.Output(); err == nil {
				info.Branch = strings.TrimSpace(string(out))
			}
			result = append(result, info)
		}
	}
	return result, nil
}

// CreateWorktree creates a git worktree for a repo at a named path under .ralph/worktrees/.
// Returns the worktree path and branch name.
func CreateWorktree(repoPath, name string) (string, string, error) {
	if strings.TrimSpace(repoPath) == "" {
		return "", "", fmt.Errorf("repo path is empty")
	}
	sanitized := sanitizeLoopName(name)
	if sanitized == "" {
		return "", "", fmt.Errorf("invalid worktree name %q", name)
	}

	base := filepath.Join(repoPath, ".ralph", "worktrees", "manual")
	wtPath := filepath.Join(base, sanitized)

	if _, err := os.Stat(wtPath); err == nil {
		return "", "", fmt.Errorf("worktree already exists: %s", wtPath)
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", "", fmt.Errorf("create worktree parent: %w", err)
	}

	branch := fmt.Sprintf("ralph/wt/%s", sanitized)
	cmd := exec.Command("git", "-C", repoPath, "worktree", "add", "-B", branch, wtPath, "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return wtPath, branch, nil
}

// cleanupOrphanedLoopBranches removes branches created by a loop that no longer
// have associated worktrees. Branch names follow the pattern "loop-<id>-iter-<n>".
func cleanupOrphanedLoopBranches(repoPath, loopID string) {
	prefix := "loop-" + loopID + "-"
	cmd := exec.Command("git", "branch", "--list", prefix+"*")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		branch := strings.TrimSpace(line)
		if branch == "" || strings.HasPrefix(branch, "*") {
			continue
		}
		delCmd := exec.Command("git", "branch", "-D", branch)
		delCmd.Dir = repoPath
		if err := delCmd.Run(); err != nil {
			slog.Debug("failed to delete orphaned loop branch", "branch", branch, "err", err)
		} else {
			slog.Info("deleted orphaned loop branch", "branch", branch)
		}
	}
}
