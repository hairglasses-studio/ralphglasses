package roadmap

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResearchProposal is an actionable item extracted from research findings.
type ResearchProposal struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Domain      string   `json:"domain"`
	Complexity  int      `json:"complexity"` // 1-4
	Source      string   `json:"source"`     // e.g. "research-daemon"
	Keywords    []string `json:"keywords,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ResearchDiscovery is a JSONL-compatible record appended to ralph-discoveries.jsonl.
type ResearchDiscovery struct {
	Topic      string    `json:"topic"`
	Domain     string    `json:"domain"`
	Summary    string    `json:"summary"`
	Complexity int       `json:"complexity"`
	Actionable bool      `json:"actionable"`
	Source     string    `json:"source"`
	Timestamp  time.Time `json:"timestamp"`
}

// ExtractActionableItems scans research content for actionable patterns
// and returns proposals suitable for roadmap insertion.
func ExtractActionableItems(topic, domain, content string, complexity int) []ResearchProposal {
	var proposals []ResearchProposal

	keywords := extractQueryKeywords(content)
	actionIndicators := []string{
		"should implement", "recommended to", "consider adding",
		"migration needed", "upgrade required", "breaking change",
		"performance improvement", "security vulnerability",
		"deprecated", "end of life", "new release",
	}

	lower := strings.ToLower(content)
	for _, indicator := range actionIndicators {
		if strings.Contains(lower, indicator) {
			kw := make([]string, 0, len(keywords))
			for k := range keywords {
				kw = append(kw, k)
				if len(kw) >= 10 {
					break
				}
			}
			proposals = append(proposals, ResearchProposal{
				Title:       fmt.Sprintf("[research] %s: %s", domain, topic),
				Description: fmt.Sprintf("Research on %q found actionable signal: %s", topic, indicator),
				Domain:      domain,
				Complexity:  complexity,
				Source:      "research-daemon",
				Keywords:    kw,
				CreatedAt:   time.Now(),
			})
			break // one proposal per topic is sufficient
		}
	}

	return proposals
}

// AppendDiscovery writes a discovery record to the ralph-discoveries.jsonl file.
func AppendDiscovery(docsRoot string, disc ResearchDiscovery) error {
	path := filepath.Join(docsRoot, "knowledge", "ralph-discoveries.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create knowledge dir: %w", err)
	}

	data, err := json.Marshal(disc)
	if err != nil {
		return fmt.Errorf("marshal discovery: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open discoveries: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", data); err != nil {
		return fmt.Errorf("write discovery: %w", err)
	}

	slog.Debug("roadmap: appended discovery", "topic", disc.Topic, "domain", disc.Domain)
	return nil
}

// AppendToNextMarathonSeeds adds a research-derived seed to the marathon
// planning file for future sprint inclusion.
func AppendToNextMarathonSeeds(docsRoot string, proposal ResearchProposal) error {
	path := filepath.Join(docsRoot, "strategy", "next-marathon-seeds.md")

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open seeds file: %w", err)
	}
	defer f.Close()

	line := fmt.Sprintf("- [ ] %s (domain: %s, complexity: %d, source: %s, added: %s)\n",
		proposal.Title, proposal.Domain, proposal.Complexity,
		proposal.Source, proposal.CreatedAt.Format("2006-01-02"))
	if _, err := f.WriteString(line); err != nil {
		return fmt.Errorf("write seed: %w", err)
	}

	slog.Debug("roadmap: appended marathon seed", "title", proposal.Title)
	return nil
}

// ProposeToMetaRoadmap appends a research-derived item to META-ROADMAP.md
// under a "Research-Derived" section. Only used for complexity 4 items
// that have been approved via DecisionLog.
func ProposeToMetaRoadmap(docsRoot string, proposal ResearchProposal) error {
	metaPath := filepath.Join(docsRoot, "strategy", "META-ROADMAP.md")

	existing, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read META-ROADMAP: %w", err)
	}

	// Find or create the "Research-Derived" section.
	marker := "## Research-Derived Tasks"
	content := string(existing)
	entry := fmt.Sprintf("- [ ] %s — %s (complexity: %d, domain: %s, added: %s)\n",
		proposal.Title, proposal.Description, proposal.Complexity,
		proposal.Domain, proposal.CreatedAt.Format("2006-01-02"))

	if strings.Contains(content, marker) {
		// Append after the marker line.
		idx := strings.Index(content, marker)
		lineEnd := strings.Index(content[idx:], "\n")
		if lineEnd < 0 {
			lineEnd = len(content[idx:])
		}
		insertAt := idx + lineEnd + 1
		content = content[:insertAt] + entry + content[insertAt:]
	} else {
		// Append a new section at the end.
		content += "\n" + marker + "\n\n" + entry
	}

	if err := os.WriteFile(metaPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write META-ROADMAP: %w", err)
	}

	slog.Info("roadmap: proposed to META-ROADMAP", "title", proposal.Title)
	return nil
}
