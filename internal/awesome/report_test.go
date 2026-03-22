package awesome

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateReport(t *testing.T) {
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
}

func TestFormatMarkdown(t *testing.T) {
	report := &Report{
		GeneratedAt: time.Now().UTC(),
		Source:      "test/repo",
		Summary:     AnalysisSummary{Total: 2, High: 1, Medium: 1},
		High: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-a", URL: "https://github.com/org/tool-a"}, Stars: 500, Language: "Go", CapabilityMatches: 5, Complexity: "drop-in"},
		},
		Medium: []AnalysisEntry{
			{AwesomeEntry: AwesomeEntry{Name: "tool-b", URL: "https://github.com/org/tool-b"}, Stars: 50, Language: "Rust", CapabilityMatches: 2, Complexity: "moderate"},
		},
	}

	md := FormatMarkdown(report)
	if !strings.Contains(md, "## HIGH VALUE") {
		t.Error("missing HIGH VALUE section")
	}
	if !strings.Contains(md, "tool-a") {
		t.Error("missing tool-a entry")
	}
	if !strings.Contains(md, "## MEDIUM VALUE") {
		t.Error("missing MEDIUM VALUE section")
	}
}
