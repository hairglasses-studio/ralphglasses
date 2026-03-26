package awesome

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateReport(t *testing.T) {
	t.Parallel()
	analysis := &Analysis{
		Source:   "test/repo",
		Analyzed: time.Now().UTC(),
		Entries: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "high1"}, Rating: RatingHigh, Stars: 500},
			{AwesomeEntry: AwesomeEntry{Name: "med1"}, Rating: RatingMedium, Stars: 100},
			{AwesomeEntry: AwesomeEntry{Name: "low1"}, Rating: RatingLow, Stars: 10},
			{AwesomeEntry: AwesomeEntry{Name: "none1"}, Rating: RatingNone, Stars: 1},
		},
		Summary: AnalysisSummary{Total: 4, High: 1, Medium: 1, Low: 1, None: 1},
	}

	report := GenerateReport(analysis)
	if len(report.High) != 1 {
		t.Errorf("high = %d, want 1", len(report.High))
	}
	if len(report.Medium) != 1 {
		t.Errorf("medium = %d, want 1", len(report.Medium))
	}
	if len(report.Low) != 1 {
		t.Errorf("low = %d, want 1", len(report.Low))
	}
	if report.Source != "test/repo" {
		t.Errorf("source = %q, want test/repo", report.Source)
	}
	if report.Summary.Total != 4 {
		t.Errorf("summary total = %d, want 4", report.Summary.Total)
	}
}

func TestGenerateReport_Empty(t *testing.T) {
	t.Parallel()
	analysis := &Analysis{
		Source:   "test/empty",
		Analyzed: time.Now().UTC(),
	}

	report := GenerateReport(analysis)
	if len(report.High) != 0 {
		t.Errorf("high = %d, want 0", len(report.High))
	}
	if len(report.Medium) != 0 {
		t.Errorf("medium = %d, want 0", len(report.Medium))
	}
	if len(report.Low) != 0 {
		t.Errorf("low = %d, want 0", len(report.Low))
	}
}

func TestGenerateReport_NoneNotIncluded(t *testing.T) {
	t.Parallel()
	analysis := &Analysis{
		Source:   "test/repo",
		Analyzed: time.Now().UTC(),
		Entries: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "none1"}, Rating: RatingNone},
			{AwesomeEntry: AwesomeEntry{Name: "none2"}, Rating: RatingNone},
		},
		Summary: AnalysisSummary{Total: 2, None: 2},
	}

	report := GenerateReport(analysis)
	if len(report.High) != 0 || len(report.Medium) != 0 || len(report.Low) != 0 {
		t.Error("NONE entries should not appear in any report section")
	}
}

func TestFormatMarkdown(t *testing.T) {
	t.Parallel()
	report := &Report{
		GeneratedAt: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC),
		Source:      "test/repo",
		Summary:     AnalysisSummary{Total: 3, High: 1, Medium: 1, Low: 1, None: 0},
		High: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-a", URL: "https://github.com/org/tool-a"}, Stars: 500, Language: "Go", CapabilityMatches: 5, Complexity: "drop-in", Rationale: "5 caps, 500 stars"},
		},
		Medium: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-b", URL: "https://github.com/org/tool-b"}, Stars: 50, Language: "Rust", CapabilityMatches: 2, Complexity: "moderate", Rationale: "2 caps, 50 stars"},
		},
		Low: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-c", URL: "https://github.com/org/tool-c"}, Stars: 10, Language: "Go", CapabilityMatches: 0, Rationale: "10 stars, Go"},
		},
	}

	md := FormatMarkdown(report)

	checks := []struct {
		name    string
		content string
	}{
		{"title", "# Awesome Claude Code — Analysis Report"},
		{"source", "**Source**: test/repo"},
		{"generated date", "2025-01-15T12:00:00Z"},
		{"total", "**Total**: 3 repos"},
		{"high section", "## HIGH VALUE"},
		{"high entry", "tool-a"},
		{"medium section", "## MEDIUM VALUE"},
		{"medium entry", "tool-b"},
		{"low section", "## LOW VALUE"},
		{"low entry", "tool-c"},
		{"table header high", "| Repo | Stars | Lang | Matches | Complexity | Rationale |"},
		{"separator", "---"},
	}

	for _, c := range checks {
		if !strings.Contains(md, c.content) {
			t.Errorf("missing %s: %q not found in output", c.name, c.content)
		}
	}
}

func TestFormatMarkdown_Empty(t *testing.T) {
	t.Parallel()
	report := &Report{
		GeneratedAt: time.Now().UTC(),
		Source:      "test/empty",
		Summary:     AnalysisSummary{},
	}

	md := FormatMarkdown(report)
	if !strings.Contains(md, "# Awesome Claude Code") {
		t.Error("missing title in empty report")
	}
	if strings.Contains(md, "## HIGH VALUE") {
		t.Error("should not have HIGH VALUE section when empty")
	}
	if strings.Contains(md, "## MEDIUM VALUE") {
		t.Error("should not have MEDIUM VALUE section when empty")
	}
	if strings.Contains(md, "## LOW VALUE") {
		t.Error("should not have LOW VALUE section when empty")
	}
}

func TestFormatMarkdown_OnlyLow(t *testing.T) {
	t.Parallel()
	report := &Report{
		GeneratedAt: time.Now().UTC(),
		Source:      "test/repo",
		Summary:     AnalysisSummary{Total: 1, Low: 1},
		Low: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-c", URL: "https://github.com/org/tool-c"}, Stars: 5, Language: "Go"},
		},
	}

	md := FormatMarkdown(report)
	if strings.Contains(md, "## HIGH VALUE") {
		t.Error("should not have HIGH VALUE section")
	}
	if strings.Contains(md, "## MEDIUM VALUE") {
		t.Error("should not have MEDIUM VALUE section")
	}
	if !strings.Contains(md, "## LOW VALUE") {
		t.Error("missing LOW VALUE section")
	}
}
