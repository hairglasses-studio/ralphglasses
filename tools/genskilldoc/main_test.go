package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderSkillMarkdown_MatchesCheckedInDoc(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	expected, err := os.ReadFile(filepath.Join(repoRoot, "docs", "SKILLS.md"))
	if err != nil {
		t.Fatalf("read docs/SKILLS.md: %v", err)
	}

	rendered := renderSkillMarkdown(repoRoot)
	if string(expected) != rendered {
		t.Fatalf("docs/SKILLS.md drift detected; run `go run ./tools/genskilldoc -output docs/SKILLS.md`")
	}
}
