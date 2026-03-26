package awesome

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorePath(t *testing.T) {
	t.Parallel()
	got := StorePath("/home/user/repo")
	want := filepath.Join("/home/user/repo", ".ralph/awesome")
	if got != want {
		t.Errorf("StorePath = %q, want %q", got, want)
	}
}

func TestStoreRoundTrip_Index(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	idx := &Index{
		Source:    "test/repo",
		FetchedAt: time.Now().UTC(),
		Entries: []AwesomeEntry{
			{Name: "tool", URL: "https://github.com/org/tool", Description: "A tool", Category: "Tools"},
		},
	}

	if err := SaveIndex(dir, idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	loaded, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}

	if loaded.Source != idx.Source {
		t.Errorf("source = %q, want %q", loaded.Source, idx.Source)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(loaded.Entries))
	}
	if loaded.Entries[0].Name != "tool" {
		t.Errorf("entry name = %q", loaded.Entries[0].Name)
	}
}

func TestStoreRoundTrip_Analysis(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := &Analysis{
		Source:   "test/repo",
		Analyzed: time.Now().UTC(),
		Entries: []AnalysisEntry{
			{
				AwesomeEntry:      AwesomeEntry{Name: "tool", URL: "https://github.com/org/tool"},
				Stars:             100,
				Language:          "Go",
				CapabilityMatches: 3,
				Rating:            RatingHigh,
			},
		},
		Summary: AnalysisSummary{Total: 1, High: 1},
	}

	if err := SaveAnalysis(dir, a); err != nil {
		t.Fatalf("SaveAnalysis: %v", err)
	}

	loaded, err := LoadAnalysis(dir)
	if err != nil {
		t.Fatalf("LoadAnalysis: %v", err)
	}

	if loaded.Summary.High != 1 {
		t.Errorf("high = %d, want 1", loaded.Summary.High)
	}
	if loaded.Entries[0].Rating != RatingHigh {
		t.Errorf("rating = %q", loaded.Entries[0].Rating)
	}
}

func TestStoreRotation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	idx1 := &Index{Source: "v1", FetchedAt: time.Now().UTC(), Entries: []AwesomeEntry{{Name: "a", URL: "u1"}}}
	idx2 := &Index{Source: "v2", FetchedAt: time.Now().UTC(), Entries: []AwesomeEntry{{Name: "b", URL: "u2"}}}

	if err := SaveIndex(dir, idx1); err != nil {
		t.Fatal(err)
	}
	if err := SaveIndex(dir, idx2); err != nil {
		t.Fatal(err)
	}

	// Current should be v2
	current, err := LoadIndex(dir)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if current.Source != "v2" {
		t.Errorf("current source = %q, want v2", current.Source)
	}

	// Previous should be v1
	prev, err := LoadPrevIndex(dir)
	if err != nil {
		t.Fatalf("LoadPrevIndex: %v", err)
	}
	if prev.Source != "v1" {
		t.Errorf("prev source = %q, want v1", prev.Source)
	}
}

func TestSaveReport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "# Test Report\n\nSome content here"

	if err := SaveReport(dir, content); err != nil {
		t.Fatalf("SaveReport: %v", err)
	}

	// Verify the file was written
	path := filepath.Join(StorePath(dir), "report.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("report content = %q, want %q", string(data), content)
	}
}

func TestLoadIndex_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadIndex(dir)
	if err == nil {
		t.Error("expected error loading non-existent index")
	}
}

func TestLoadPrevIndex_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadPrevIndex(dir)
	if err == nil {
		t.Error("expected error loading non-existent prev index")
	}
}

func TestLoadAnalysis_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadAnalysis(dir)
	if err == nil {
		t.Error("expected error loading non-existent analysis")
	}
}

func TestSaveIndex_CreatesDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Use a nested path that doesn't exist yet
	nested := filepath.Join(dir, "deep", "nested")

	idx := &Index{
		Source:    "test/repo",
		FetchedAt: time.Now().UTC(),
		Entries:   []AwesomeEntry{{Name: "a", URL: "u1"}},
	}

	if err := SaveIndex(nested, idx); err != nil {
		t.Fatalf("SaveIndex: %v", err)
	}

	loaded, err := LoadIndex(nested)
	if err != nil {
		t.Fatalf("LoadIndex: %v", err)
	}
	if loaded.Source != "test/repo" {
		t.Errorf("source = %q", loaded.Source)
	}
}
