package session

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitLogSince returns commits in [since, until] for the given repo.
func GitLogSince(repoPath string, since, until time.Time) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "log",
		"--format=%H|%s|%aI",
		"--since="+since.Format(time.RFC3339),
		"--until="+until.Format(time.RFC3339),
	)
	cmd.Dir = repoPath

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}

	var commits []map[string]string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		if len(parts) < 3 {
			continue
		}
		commits = append(commits, map[string]string{
			"hash":    parts[0],
			"subject": parts[1],
			"date":    parts[2],
		})
	}
	return commits, nil
}

// GitDiffWindow returns a diff/stat for commits in a time window.
// If statOnly is true, only --stat is returned.
// Returns: diffText, stat map, truncated flag, error.
func GitDiffWindow(repoPath string, since, until time.Time, statOnly bool, maxLines int) (string, map[string]int, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find the oldest commit in the window
	hashCmd := exec.CommandContext(ctx, "git", "log",
		"--format=%H",
		"--since="+since.Format(time.RFC3339),
		"--until="+until.Format(time.RFC3339),
		"--reverse",
	)
	hashCmd.Dir = repoPath

	hashOut, err := hashCmd.Output()
	if err != nil {
		return "", nil, false, fmt.Errorf("git log hashes: %w", err)
	}

	hashes := strings.Split(strings.TrimSpace(string(hashOut)), "\n")
	if len(hashes) == 0 || hashes[0] == "" {
		return "", map[string]int{"files_changed": 0, "insertions": 0, "deletions": 0}, false, nil
	}

	oldest := hashes[0]
	diffRef := oldest + "^..HEAD"

	// Get --numstat for structured stats
	statCtx, statCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer statCancel()

	statCmd := exec.CommandContext(statCtx, "git", "diff", diffRef, "--numstat")
	statCmd.Dir = repoPath
	statOut, _ := statCmd.Output()

	stat := map[string]int{"files_changed": 0, "insertions": 0, "deletions": 0}
	for line := range strings.SplitSeq(strings.TrimSpace(string(statOut)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		stat["files_changed"]++
		if ins, err := strconv.Atoi(fields[0]); err == nil {
			stat["insertions"] += ins
		}
		if del, err := strconv.Atoi(fields[1]); err == nil {
			stat["deletions"] += del
		}
	}

	if statOnly {
		return "", stat, false, nil
	}

	// Get full diff
	diffCtx, diffCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer diffCancel()

	diffCmd := exec.CommandContext(diffCtx, "git", "diff", diffRef)
	diffCmd.Dir = repoPath
	diffOut, err := diffCmd.Output()
	if err != nil {
		return "", stat, false, fmt.Errorf("git diff: %w", err)
	}

	diffText := string(diffOut)
	truncated := false
	if maxLines > 0 {
		lines := strings.Split(diffText, "\n")
		if len(lines) > maxLines {
			diffText = strings.Join(lines[:maxLines], "\n")
			truncated = true
		}
	}

	return diffText, stat, truncated, nil
}
