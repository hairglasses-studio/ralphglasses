package main

import (
	"path/filepath"
	"testing"
)

func TestCheckSkillSurfaces_MatchesCheckedInFiles(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	if err := checkSkillSurfaces(repoRoot); err != nil {
		t.Fatal(err)
	}
}
