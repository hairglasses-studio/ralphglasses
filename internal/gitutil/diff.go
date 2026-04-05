package gitutil

import (
	"os/exec"
	"strconv"
	"strings"
)

// GitDiffPaths runs git diff --name-only HEAD in the given directory and
// returns the list of changed file paths relative to the repo root.
func GitDiffPaths(dir string) ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}

	var paths []string
	for line := range strings.SplitSeq(trimmed, "\n") {
		if p := strings.TrimSpace(line); p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// GitDiffStats runs git diff --stat on a directory and parses the summary line,
// returning the number of files changed, lines added, and lines removed.
func GitDiffStats(dir string) (files, added, removed int) {
	cmd := exec.Command("git", "diff", "--stat", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return 0, 0, 0
	}
	// Summary line looks like: " 3 files changed, 10 insertions(+), 5 deletions(-)"
	summary := lines[len(lines)-1]
	for part := range strings.SplitSeq(summary, ",") {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(part, "file"):
			files = n
		case strings.Contains(part, "insertion"):
			added = n
		case strings.Contains(part, "deletion"):
			removed = n
		}
	}
	return files, added, removed
}

// ParseLines splits trimmed byte output into non-empty lines.
func ParseLines(b []byte) []string {
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
