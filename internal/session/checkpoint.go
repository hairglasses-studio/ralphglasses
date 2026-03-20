package session

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// CreateCheckpoint creates a git checkpoint (commit + tag) for a session.
// Only commits if the working tree is dirty.
func CreateCheckpoint(repoPath string, count int, spendUSD float64, turnCount int) error {
	// Check if working tree is dirty
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = repoPath
	out, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		return nil // nothing to commit
	}

	// Stage all changes
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = repoPath
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Commit
	msg := fmt.Sprintf("session checkpoint #%d ($%.2f, %d turns)", count, spendUSD, turnCount)
	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = repoPath
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Tag
	ts := time.Now().Format("20060102-150405")
	tag := fmt.Sprintf("session-checkpoint-%d-%s", count, ts)
	tagCmd := exec.Command("git", "tag", tag)
	tagCmd.Dir = repoPath
	if err := tagCmd.Run(); err != nil {
		return fmt.Errorf("git tag: %w", err)
	}

	return nil
}
