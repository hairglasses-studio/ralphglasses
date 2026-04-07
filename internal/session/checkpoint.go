package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// checkpointExcludes lists pathspec patterns excluded from checkpoint staging
// to prevent secrets, credentials, databases, and bulky dependency dirs from
// being committed.
var checkpointExcludes = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"*.p12",
	"credentials.json",
	"credentials.yaml",
	"credentials.yml",
	"credentials.xml",
	"credentials.toml",
	"*-secret.json",
	"*-secret.yaml",
	"*-secret.yml",
	"*.secret",
	".env.secret",
	"*.sqlite",
	"*.db",
	"node_modules/",
	"vendor/",
}

// CreateCheckpoint creates a git checkpoint (commit + tag) for a session.
// Only commits if the working tree is dirty.
func CreateCheckpoint(repoPath string, count int, spendUSD float64, turnCount int) error {
	ctx, cancel := context.WithTimeout(context.Background(), gitCmdTimeout)
	defer cancel()
	gitEnv := append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	if os.Getenv("GIT_AUTHOR_NAME") == "" {
		gitEnv = append(gitEnv, "GIT_AUTHOR_NAME=ralphglasses")
	}
	if os.Getenv("GIT_AUTHOR_EMAIL") == "" {
		gitEnv = append(gitEnv, "GIT_AUTHOR_EMAIL=ralphglasses@local.invalid")
	}
	if os.Getenv("GIT_COMMITTER_NAME") == "" {
		gitEnv = append(gitEnv, "GIT_COMMITTER_NAME=ralphglasses")
	}
	if os.Getenv("GIT_COMMITTER_EMAIL") == "" {
		gitEnv = append(gitEnv, "GIT_COMMITTER_EMAIL=ralphglasses@local.invalid")
	}

	// Check if working tree is dirty
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = repoPath
	statusCmd.Env = gitEnv
	out, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if len(strings.TrimSpace(string(out))) == 0 {
		return nil // nothing to commit
	}

	// Stage all changes, excluding sensitive files and large artifacts.
	addArgs := []string{"add", "-A", "--"}
	for _, excl := range checkpointExcludes {
		addArgs = append(addArgs, ":(exclude)"+excl)
	}
	addCmd := exec.CommandContext(ctx, "git", addArgs...)
	addCmd.Dir = repoPath
	addCmd.Env = gitEnv
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Commit
	msg := fmt.Sprintf("session checkpoint #%d ($%.2f, %d turns)", count, spendUSD, turnCount)
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", msg)
	commitCmd.Dir = repoPath
	commitCmd.Env = gitEnv
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	// Tag
	ts := time.Now().Format("20060102-150405")
	tag := fmt.Sprintf("session-checkpoint-%d-%s", count, ts)
	tagCmd := exec.CommandContext(ctx, "git", "tag", tag)
	tagCmd.Dir = repoPath
	tagCmd.Env = gitEnv
	if err := tagCmd.Run(); err != nil {
		return fmt.Errorf("git tag: %w", err)
	}

	return nil
}
