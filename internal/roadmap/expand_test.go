package roadmap

import (
	"strings"
	"testing"
)

func TestExpand_WithAnalysis(t *testing.T) {
	t.Parallel()

	rm := &Roadmap{
		Title: "Test Project",
		Phases: []Phase{
			{
				Name: "Phase 1",
				Sections: []Section{
					{
						Name: "Core",
						Tasks: []Task{
							{ID: "1.1", Description: "Implement parser", Done: false},
							{ID: "1.2", Description: "Add tests", Done: true},
						},
					},
				},
				Stats: Stats{Total: 2, Completed: 1, Remaining: 1},
			},
		},
		Stats: Stats{Total: 2, Completed: 1, Remaining: 1},
	}

	analysis := &Analysis{
		Gaps: []GapItem{
			{TaskID: "1.1", Description: "Implement parser", Phase: "Phase 1", Section: "Core", Status: "not_started"},
		},
		Stale: []StaleItem{
			{TaskID: "1.3", Description: "Config loader", Phase: "Phase 1", Evidence: []string{"exists: internal/config"}},
		},
	}

	exp, err := Expand(rm, analysis, nil, "aggressive")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	if exp.Style != "aggressive" {
		t.Errorf("style = %q, want %q", exp.Style, "aggressive")
	}

	// Should have gap_fill + stale_update + missing_pattern proposals
	if len(exp.Proposals) == 0 {
		t.Fatal("expected proposals")
	}

	var hasGapFill, hasStaleUpdate bool
	for _, p := range exp.Proposals {
		switch p.Type {
		case "gap_fill":
			hasGapFill = true
		case "stale_update":
			hasStaleUpdate = true
		}
	}
	if !hasGapFill {
		t.Error("expected gap_fill proposal")
	}
	if !hasStaleUpdate {
		t.Error("expected stale_update proposal")
	}

	if exp.Markdown == "" {
		t.Error("expected non-empty markdown")
	}
}

func TestExpand_WithResearch(t *testing.T) {
	t.Parallel()

	rm := &Roadmap{Title: "Test"}
	research := &ResearchResults{
		Findings: []Finding{
			{Name: "cool/lib", URL: "https://github.com/cool/lib", Description: "Useful", Stars: 100, IsNewDep: true},
			{Name: "low/stars", URL: "https://github.com/low/stars", Description: "Not popular", Stars: 10, IsNewDep: true},
			{Name: "existing/dep", URL: "https://github.com/existing/dep", Description: "Already used", Stars: 500, IsNewDep: false},
		},
	}

	exp, err := Expand(rm, nil, research, "aggressive")
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}

	var researchCount int
	for _, p := range exp.Proposals {
		if p.Type == "research_driven" {
			researchCount++
		}
	}
	// Only cool/lib qualifies: IsNewDep=true AND Stars>=50
	if researchCount != 1 {
		t.Errorf("expected 1 research_driven proposal, got %d", researchCount)
	}
}

func TestExpand_NilInputs(t *testing.T) {
	t.Parallel()

	exp, err := Expand(nil, nil, nil, "")
	if err != nil {
		t.Fatalf("Expand nil: %v", err)
	}
	if exp.Style != "balanced" {
		t.Errorf("default style = %q, want balanced", exp.Style)
	}
}

func TestExpand_EmptyRoadmap(t *testing.T) {
	t.Parallel()

	rm := &Roadmap{Title: "Empty"}
	exp, err := Expand(rm, &Analysis{}, nil, "balanced")
	if err != nil {
		t.Fatalf("Expand empty: %v", err)
	}
	// No gaps or stale, balanced filters out missing_pattern
	if len(exp.Proposals) != 0 {
		t.Errorf("expected 0 proposals for empty analysis with balanced style, got %d", len(exp.Proposals))
	}
}

func TestFilterByStyle(t *testing.T) {
	t.Parallel()

	proposals := []Proposal{
		{Type: "gap_fill", Description: "gap"},
		{Type: "stale_update", Description: "stale"},
		{Type: "research_driven", Description: "research"},
		{Type: "missing_pattern", Description: "pattern"},
	}

	tests := []struct {
		name  string
		style string
		want  int
	}{
		{"conservative", "conservative", 2},  // gap_fill + stale_update only
		{"balanced", "balanced", 3},           // everything except missing_pattern
		{"aggressive", "aggressive", 4},       // everything
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterByStyle(proposals, tt.style)
			if len(filtered) != tt.want {
				t.Errorf("filterByStyle(%q) = %d proposals, want %d", tt.style, len(filtered), tt.want)
			}
		})
	}
}

func TestGenerateSubtasks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		gap  GapItem
		want []string
	}{
		{
			name: "with_id",
			gap:  GapItem{TaskID: "2.1", Section: "Core", Description: "Add parser"},
			want: []string{"2.1.1", "2.1.2", "2.1.3", "2.1.4", "Acceptance"},
		},
		{
			name: "without_id",
			gap:  GapItem{Section: "Core", Description: "Add parser"},
			want: []string{"X.X.1", "X.X.2", "X.X.3", "X.X.4", "Acceptance"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateSubtasks(tt.gap)
			for _, s := range tt.want {
				if !strings.Contains(result, s) {
					t.Errorf("generateSubtasks missing %q in output:\n%s", s, result)
				}
			}
		})
	}
}

func TestCheckMissingPatterns(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rm       *Roadmap
		minCount int
	}{
		{
			name:     "empty_roadmap_all_missing",
			rm:       &Roadmap{},
			minCount: 4, // test coverage, ci/cd, documentation, monitoring
		},
		{
			name: "has_test_coverage",
			rm: &Roadmap{
				Phases: []Phase{
					{
						Name: "Phase 1",
						Sections: []Section{
							{Name: "Testing", Tasks: []Task{{Description: "test coverage targets"}}},
						},
					},
				},
			},
			minCount: 3, // ci/cd, documentation, monitoring remain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposals := checkMissingPatterns(tt.rm)
			if len(proposals) < tt.minCount {
				t.Errorf("checkMissingPatterns returned %d proposals, want >= %d", len(proposals), tt.minCount)
			}
			for _, p := range proposals {
				if p.Type != "missing_pattern" {
					t.Errorf("expected type missing_pattern, got %q", p.Type)
				}
			}
		})
	}
}

func TestRenderExpansionMarkdown_Empty(t *testing.T) {
	t.Parallel()

	result := renderExpansionMarkdown(nil)
	if !strings.Contains(result, "No expansion proposals") {
		t.Errorf("expected no-proposals comment, got %q", result)
	}
}

func TestRenderExpansionMarkdown_WithProposals(t *testing.T) {
	t.Parallel()

	proposals := []Proposal{
		{Type: "stale_update", Markdown: "- [x] task done", Description: "mark done"},
		{Type: "gap_fill", Markdown: "- [ ] subtask", Description: "fill gap"},
	}

	result := renderExpansionMarkdown(proposals)
	if !strings.Contains(result, "Proposed Roadmap Expansions") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "Stale Updates") {
		t.Error("missing stale updates section")
	}
	if !strings.Contains(result, "Gap Fills") {
		t.Error("missing gap fills section")
	}
}
