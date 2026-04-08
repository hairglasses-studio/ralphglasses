package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderConfigReference_MatchesCheckedInDoc(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	expected, err := os.ReadFile(filepath.Join(repoRoot, "docs", "ralphrc-reference.md"))
	if err != nil {
		t.Fatalf("read docs/ralphrc-reference.md: %v", err)
	}

	rendered := renderConfigReference()
	if string(expected) != rendered {
		t.Fatalf("docs/ralphrc-reference.md drift detected; run `go run ./tools/genconfig/main.go > docs/ralphrc-reference.md`")
	}
}
