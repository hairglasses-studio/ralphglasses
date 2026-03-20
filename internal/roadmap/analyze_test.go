package roadmap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyze(t *testing.T) {
	// Create a temp repo with some dirs matching roadmap keywords
	dir := t.TempDir()

	// Write roadmap
	rmPath := filepath.Join(dir, "ROADMAP.md")
	os.WriteFile(rmPath, []byte(testRoadmap), 0644)

	// Create some evidence dirs
	os.MkdirAll(filepath.Join(dir, "internal", "parser"), 0755)

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// Should have gaps (incomplete tasks with no evidence)
	if len(analysis.Gaps) == 0 && len(analysis.Stale) == 0 {
		t.Error("expected some gaps or stale items")
	}

	// Should have ready tasks (tasks with no unmet deps)
	if len(analysis.Ready) == 0 {
		t.Error("expected some ready tasks")
	}

	// Summary should be populated
	if analysis.Summary.TotalTasks != rm.Stats.Total {
		t.Errorf("summary total = %d, want %d", analysis.Summary.TotalTasks, rm.Stats.Total)
	}
}

func TestAnalyze_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	rmPath := filepath.Join(dir, "ROADMAP.md")
	os.WriteFile(rmPath, []byte(testRoadmap), 0644)

	rm, err := Parse(rmPath)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	analysis, err := Analyze(rm, dir)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}

	// All incomplete tasks should be gaps
	if analysis.Summary.GapCount == 0 {
		t.Error("expected gaps in empty repo")
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		desc string
		want int // minimum number of keywords
	}{
		{"implement `internal/parser` module", 1},
		{"add internal/tui/views handler", 1},
		{"basic description with no paths", 0},
	}
	for _, tt := range tests {
		kw := extractKeywords(tt.desc)
		if len(kw) < tt.want {
			t.Errorf("extractKeywords(%q) got %d keywords, want >= %d", tt.desc, len(kw), tt.want)
		}
	}
}
