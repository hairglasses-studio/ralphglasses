package roadmap

import (
	"fmt"
	"strings"
)

// Expansion contains proposed roadmap additions.
type Expansion struct {
	Style     string     `json:"style"`
	Proposals []Proposal `json:"proposals"`
	Markdown  string     `json:"markdown"`
}

// Proposal is a single suggested addition.
type Proposal struct {
	Type        string `json:"type"` // gap_fill, stale_update, research_driven, missing_pattern
	Phase       string `json:"phase"`
	Section     string `json:"section,omitempty"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

// Expand generates proposed roadmap expansions from analysis and research.
func Expand(rm *Roadmap, analysis *Analysis, research *ResearchResults, style string) (*Expansion, error) {
	if style == "" {
		style = "balanced"
	}

	exp := &Expansion{Style: style}

	// Gap fills — break down not_started items into subtasks
	if analysis != nil {
		for _, gap := range analysis.Gaps {
			if gap.Status != "not_started" {
				continue
			}
			p := Proposal{
				Type:        "gap_fill",
				Phase:       gap.Phase,
				Section:     gap.Section,
				Description: fmt.Sprintf("Subtask breakdown for: %s", gap.Description),
				Markdown:    generateSubtasks(gap),
			}
			exp.Proposals = append(exp.Proposals, p)
		}

		// Stale updates — suggest marking completed items
		for _, stale := range analysis.Stale {
			p := Proposal{
				Type:        "stale_update",
				Phase:       stale.Phase,
				Description: fmt.Sprintf("Mark as complete: %s (evidence: %s)", stale.Description, strings.Join(stale.Evidence, ", ")),
				Markdown:    fmt.Sprintf("- [x] %s %s", stale.TaskID, stale.Description),
			}
			exp.Proposals = append(exp.Proposals, p)
		}
	}

	// Research-driven proposals
	if research != nil {
		for _, finding := range research.Findings {
			if !finding.IsNewDep || finding.Stars < 50 {
				continue
			}
			p := Proposal{
				Type:        "research_driven",
				Description: fmt.Sprintf("Consider integrating %s: %s", finding.Name, finding.Description),
				Markdown: fmt.Sprintf("- [ ] Evaluate [%s](%s) — %s (%d stars)",
					finding.Name, finding.URL, finding.Description, finding.Stars),
			}
			exp.Proposals = append(exp.Proposals, p)
		}
	}

	// Missing patterns — check for common sections
	if rm != nil {
		exp.Proposals = append(exp.Proposals, checkMissingPatterns(rm)...)
	}

	// Apply style filter
	exp.Proposals = filterByStyle(exp.Proposals, style)

	// Generate combined markdown
	exp.Markdown = renderExpansionMarkdown(exp.Proposals)

	return exp, nil
}

func generateSubtasks(gap GapItem) string {
	var b strings.Builder
	id := gap.TaskID
	if id == "" {
		id = "X.X"
	}

	b.WriteString(fmt.Sprintf("### %s — %s\n", gap.Section, gap.Description))
	b.WriteString(fmt.Sprintf("- [ ] %s.1 — Research approach and dependencies\n", id))
	b.WriteString(fmt.Sprintf("- [ ] %s.2 — Implement core logic\n", id))
	b.WriteString(fmt.Sprintf("- [ ] %s.3 — Add unit tests\n", id))
	b.WriteString(fmt.Sprintf("- [ ] %s.4 — Integration test and verify\n", id))
	b.WriteString(fmt.Sprintf("- **Acceptance:** %s working and tested\n", gap.Description))
	return b.String()
}

func checkMissingPatterns(rm *Roadmap) []Proposal {
	var proposals []Proposal

	// Collect all task descriptions
	var allText strings.Builder
	for _, p := range rm.Phases {
		allText.WriteString(strings.ToLower(p.Name))
		for _, s := range p.Sections {
			allText.WriteString(strings.ToLower(s.Name))
			for _, t := range s.Tasks {
				allText.WriteString(strings.ToLower(t.Description))
			}
		}
	}
	text := allText.String()

	patterns := []struct {
		keyword  string
		name     string
		markdown string
	}{
		{"test coverage", "Testing", "- [ ] Add comprehensive test suite with coverage targets\n- [ ] Set up CI test reporting"},
		{"ci/cd", "CI/CD", "- [ ] Set up GitHub Actions CI pipeline\n- [ ] Add linting, testing, and build steps"},
		{"documentation", "Documentation", "- [ ] Add API documentation\n- [ ] Add usage examples and guides"},
		{"monitoring", "Observability", "- [ ] Add structured logging\n- [ ] Add metrics and health checks"},
	}

	for _, p := range patterns {
		if !strings.Contains(text, p.keyword) {
			proposals = append(proposals, Proposal{
				Type:        "missing_pattern",
				Description: fmt.Sprintf("No %s section found in roadmap", p.name),
				Markdown:    fmt.Sprintf("### %s\n%s", p.name, p.markdown),
			})
		}
	}

	return proposals
}

func filterByStyle(proposals []Proposal, style string) []Proposal {
	switch style {
	case "conservative":
		// Only stale updates and gap fills
		var filtered []Proposal
		for _, p := range proposals {
			if p.Type == "stale_update" || p.Type == "gap_fill" {
				filtered = append(filtered, p)
			}
		}
		return filtered
	case "aggressive":
		// Everything
		return proposals
	default: // balanced
		// Skip low-value missing patterns
		var filtered []Proposal
		for _, p := range proposals {
			if p.Type == "missing_pattern" {
				continue
			}
			filtered = append(filtered, p)
		}
		return filtered
	}
}

func renderExpansionMarkdown(proposals []Proposal) string {
	if len(proposals) == 0 {
		return "<!-- No expansion proposals -->"
	}

	var b strings.Builder
	b.WriteString("## Proposed Roadmap Expansions\n\n")

	byType := make(map[string][]Proposal)
	for _, p := range proposals {
		byType[p.Type] = append(byType[p.Type], p)
	}

	typeNames := map[string]string{
		"gap_fill":        "Gap Fills (subtask breakdowns)",
		"stale_update":    "Stale Updates (mark completed)",
		"research_driven": "Research-Driven (new integrations)",
		"missing_pattern": "Missing Patterns (standard sections)",
	}

	for _, typ := range []string{"stale_update", "gap_fill", "research_driven", "missing_pattern"} {
		items, ok := byType[typ]
		if !ok {
			continue
		}
		b.WriteString(fmt.Sprintf("### %s\n\n", typeNames[typ]))
		for _, item := range items {
			b.WriteString(item.Markdown)
			b.WriteString("\n\n")
		}
	}

	return b.String()
}
