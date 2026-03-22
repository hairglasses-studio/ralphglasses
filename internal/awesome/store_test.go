package awesome

import (
	"testing"
	"time"
)

func TestStoreRoundTrip_Index(t *testing.T) {
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
	dir := t.TempDir()
	idx1 := &Index{Source: "v1", FetchedAt: time.Now().UTC(), Entries: []AwesomeEntry{{Name: "a", URL: "u1"}}}
	idx2 := &Index{Source: "v2", FetchedAt: time.Now().UTC(), Entries: []AwesomeEntry{{Name: "b", URL: "u2"}}}

	SaveIndex(dir, idx1)
	SaveIndex(dir, idx2)

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
