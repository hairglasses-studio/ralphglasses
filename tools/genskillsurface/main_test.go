package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckSkillSurfaces_MatchesCheckedInFiles(t *testing.T) {
	t.Parallel()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs(filepath.Join(cwd, "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := checkSkillSurfaces(repoRoot); err != nil {
		t.Fatal(err)
	}
}
