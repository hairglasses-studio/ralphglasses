package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRenderManpages_MatchesCheckedInTree(t *testing.T) {
	t.Parallel()

	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	expectedDir := filepath.Join(repoRoot, "man", "man1")
	generatedDir := filepath.Join(t.TempDir(), "man1")

	if err := renderManpages(generatedDir); err != nil {
		t.Fatalf("render manpages: %v", err)
	}

	expectedFiles := listManpages(t, expectedDir)
	generatedFiles := listManpages(t, generatedDir)
	if !slices.Equal(expectedFiles, generatedFiles) {
		t.Fatalf("man/man1 file set drift detected\nexpected: %v\ngenerated: %v", expectedFiles, generatedFiles)
	}

	for _, name := range expectedFiles {
		expected, err := os.ReadFile(filepath.Join(expectedDir, name))
		if err != nil {
			t.Fatalf("read expected %s: %v", name, err)
		}
		generated, err := os.ReadFile(filepath.Join(generatedDir, name))
		if err != nil {
			t.Fatalf("read generated %s: %v", name, err)
		}
		if string(expected) != string(generated) {
			t.Fatalf("manpage drift detected in %s; run `go run ./tools/gendoc/main.go`", name)
		}
	}
}

func listManpages(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	slices.Sort(names)
	return names
}
