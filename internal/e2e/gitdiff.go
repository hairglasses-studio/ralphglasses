package e2e

import (
	"os/exec"
	"strconv"
	"strings"
)

// GitDiffStats runs git diff --stat on a directory and returns file/line counts.
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
	summary := lines[len(lines)-1]
	for _, part := range strings.Split(summary, ",") {
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
