package session

import (
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/gitutil"
)

// ForbiddenSelfTestPaths are file path patterns that require human review
// when modified during a self-test loop.
var ForbiddenSelfTestPaths = []string{
	"internal/session/loop.go",
	"internal/session/manager.go",
	"internal/session/selftest.go",
	"internal/mcpserver/handler_loop.go",
	"internal/mcpserver/handler_selftest.go",
	"internal/e2e/selftest.go",
}

// gitDiffPaths runs git diff --name-only HEAD in the given directory and
// returns the list of changed file paths relative to the repo root.
func gitDiffPaths(dir string) ([]string, error) {
	return gitutil.GitDiffPaths(dir)
}

// ClassifyDiffPaths separates changed files into safe and needs-review categories.
func ClassifyDiffPaths(paths []string) (safe, needsReview []string) {
	for _, p := range paths {
		forbidden := false
		for _, pattern := range ForbiddenSelfTestPaths {
			if strings.Contains(p, pattern) || p == pattern {
				forbidden = true
				break
			}
		}
		if forbidden {
			needsReview = append(needsReview, p)
		} else {
			safe = append(safe, p)
		}
	}
	return
}
