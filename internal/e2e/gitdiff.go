package e2e

import (
	"github.com/hairglasses-studio/ralphglasses/internal/gitutil"
)

// GitDiffPaths runs git diff --name-only HEAD in the given directory and
// returns the list of changed file paths relative to the repo root.
func GitDiffPaths(dir string) ([]string, error) {
	return gitutil.GitDiffPaths(dir)
}

// GitDiffStats runs git diff --stat on a directory and returns file/line counts.
func GitDiffStats(dir string) (files, added, removed int) {
	return gitutil.GitDiffStats(dir)
}
