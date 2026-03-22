package awesome

import (
	"fmt"
	"strings"
	"time"
)

// GenerateReport groups analysis entries by rating and produces a Report.
func GenerateReport(analysis *Analysis) *Report {
	r := &Report{
		GeneratedAt: time.Now().UTC(),
		Source:      analysis.Source,
		Summary:     analysis.Summary,
	}

	for _, e := range analysis.Entries {
		switch e.Rating {
		case RatingHigh:
			r.High = append(r.High, e)
		case RatingMedium:
			r.Medium = append(r.Medium, e)
		case RatingLow:
			r.Low = append(r.Low, e)
		}
	}

	return r
}

// FormatMarkdown renders a report as markdown text.
func FormatMarkdown(r *Report) string {
	var b strings.Builder

	b.WriteString("# Awesome Claude Code — Analysis Report\n\n")
	b.WriteString(fmt.Sprintf("**Source**: %s  \n", r.Source))
	b.WriteString(fmt.Sprintf("**Generated**: %s  \n", r.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Total**: %d repos — %d HIGH, %d MEDIUM, %d LOW, %d NONE\n\n",
		r.Summary.Total, r.Summary.High, r.Summary.Medium, r.Summary.Low, r.Summary.None))

	b.WriteString("---\n\n")

	if len(r.High) > 0 {
		b.WriteString("## HIGH VALUE\n\n")
		b.WriteString("| Repo | Stars | Lang | Matches | Complexity | Rationale |\n")
		b.WriteString("|------|-------|------|---------|------------|----------|\n")
		for _, e := range r.High {
			b.WriteString(fmt.Sprintf("| [%s](%s) | %d | %s | %d | %s | %s |\n",
				e.Name, e.URL, e.Stars, e.Language, e.CapabilityMatches, e.Complexity, e.Rationale))
		}
		b.WriteString("\n")
	}

	if len(r.Medium) > 0 {
		b.WriteString("## MEDIUM VALUE\n\n")
		b.WriteString("| Repo | Stars | Lang | Matches | Complexity | Rationale |\n")
		b.WriteString("|------|-------|------|---------|------------|----------|\n")
		for _, e := range r.Medium {
			b.WriteString(fmt.Sprintf("| [%s](%s) | %d | %s | %d | %s | %s |\n",
				e.Name, e.URL, e.Stars, e.Language, e.CapabilityMatches, e.Complexity, e.Rationale))
		}
		b.WriteString("\n")
	}

	if len(r.Low) > 0 {
		b.WriteString("## LOW VALUE\n\n")
		b.WriteString("| Repo | Stars | Lang | Matches | Rationale |\n")
		b.WriteString("|------|-------|------|---------|----------|\n")
		for _, e := range r.Low {
			b.WriteString(fmt.Sprintf("| [%s](%s) | %d | %s | %d | %s |\n",
				e.Name, e.URL, e.Stars, e.Language, e.CapabilityMatches, e.Rationale))
		}
		b.WriteString("\n")
	}

	return b.String()
}
