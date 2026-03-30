package gitutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// DefaultGitTimeout is the default timeout for git operations.
const DefaultGitTimeout = 30 * time.Second

// CreateTag creates a lightweight git tag at HEAD.
func CreateTag(ctx context.Context, repoPath, tagName string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultGitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "tag", tagName)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git tag %s: %w (%s)", tagName, err, out)
	}
	return nil
}

// MarathonTag returns a formatted marathon checkpoint tag name.
func MarathonTag(count int) string {
	ts := time.Now().Format("20060102-150405")
	return fmt.Sprintf("marathon-%d-%s", count, ts)
}
