package awesome

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultCapabilities are keywords to match against target repo features.
var DefaultCapabilities = []string{
	"mcp", "tui", "agent", "session", "provider", "claude", "gemini", "codex",
	"loop", "budget", "cost", "circuit breaker", "roadmap", "workflow",
	"hook", "skill", "team", "fleet", "multi-agent", "orchestrat",
	"bubbletea", "lipgloss", "charmbracelet", "fsnotify", "cobra",
	"worktree", "parallel", "concurrent", "security", "config",
}

// AnalyzeOptions controls the analysis behavior.
type AnalyzeOptions struct {
	Capabilities []string // keywords to match (uses DefaultCapabilities if nil)
	MaxWorkers   int      // concurrent README fetches (default 5)
}

// Analyze performs deep analysis on a set of entries by fetching READMEs
// and scoring relevance to ralph capabilities.
func Analyze(ctx context.Context, client *http.Client, entries []AwesomeEntry, opts AnalyzeOptions) (*Analysis, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	caps := opts.Capabilities
	if caps == nil {
		caps = DefaultCapabilities
	}
	workers := opts.MaxWorkers
	if workers <= 0 {
		workers = 5
	}

	results := make([]AnalysisEntry, len(entries))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup

	for i, entry := range entries {
		wg.Add(1)
		go func(i int, entry AwesomeEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ae := analyzeOne(ctx, client, entry, caps)
			results[i] = ae
		}(i, entry)
	}

	wg.Wait()

	analysis := &Analysis{
		Analyzed: time.Now().UTC(),
		Entries:  results,
	}

	// Compute summary
	for _, e := range results {
		analysis.Summary.Total++
		switch e.Rating {
		case RatingHigh:
			analysis.Summary.High++
		case RatingMedium:
			analysis.Summary.Medium++
		case RatingLow:
			analysis.Summary.Low++
		case RatingNone:
			analysis.Summary.None++
		}
	}

	return analysis, nil
}

func analyzeOne(ctx context.Context, client *http.Client, entry AwesomeEntry, caps []string) AnalysisEntry {
	ae := AnalysisEntry{
		AwesomeEntry: entry,
	}

	repo := extractRepoFromURL(entry.URL)
	if repo == "" {
		ae.Rating = RatingNone
		ae.Rationale = "cannot extract repo from URL"
		return ae
	}

	// Fetch repo metadata from GitHub API
	meta, err := fetchRepoMeta(ctx, client, repo)
	if err == nil {
		ae.Stars = meta.Stars
		ae.Language = meta.Language
	}

	// Fetch README for feature extraction
	readme, err := fetchREADME(ctx, client, repo)
	if err != nil {
		// Fall back to description only
		readme = entry.Description
	}

	// Extract features and match capabilities
	readmeLower := strings.ToLower(readme)
	descLower := strings.ToLower(entry.Description)
	combined := readmeLower + " " + descLower

	var matches []string
	for _, cap := range caps {
		if strings.Contains(combined, cap) {
			matches = append(matches, cap)
		}
	}
	ae.CapabilityMatches = len(matches)
	ae.Features = matches

	// Rate value
	ae.Rating = rateValue(ae.Stars, ae.Language, ae.CapabilityMatches)
	ae.Complexity = rateComplexity(ae.Language, readmeLower)
	ae.Rationale = buildRationale(ae)

	return ae
}

// repoMeta is minimal GitHub repo metadata.
type repoMeta struct {
	Stars    int    `json:"stargazers_count"`
	Language string `json:"language"`
}

func fetchRepoMeta(ctx context.Context, client *http.Client, repo string) (*repoMeta, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s", repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	addAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var meta repoMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// rateValue assigns HIGH/MEDIUM/LOW/NONE based on stars, language, and capability matches.
func rateValue(stars int, language string, matches int) Rating {
	isGo := strings.EqualFold(language, "Go")

	switch {
	case matches >= 3 && (stars > 100 || isGo):
		return RatingHigh
	case matches >= 1 && matches <= 2:
		return RatingMedium
	case stars > 500 && isGo:
		return RatingMedium
	case isGo && matches == 0:
		return RatingLow
	case !isGo && matches >= 1:
		return RatingMedium
	case matches == 0 && (!isGo || stars < 10):
		return RatingNone
	default:
		return RatingLow
	}
}

// rateComplexity estimates integration effort.
func rateComplexity(language, readme string) string {
	isGo := strings.EqualFold(language, "Go")

	// Drop-in signals
	dropInSignals := []string{"mcp server", "mcp tool", "hook", "skill", "plugin", "cli tool", "npm install -g"}
	for _, sig := range dropInSignals {
		if strings.Contains(readme, sig) && (isGo || strings.Contains(readme, "npm")) {
			return "drop-in"
		}
	}

	if isGo {
		return "moderate"
	}
	return "moderate"
}

func buildRationale(ae AnalysisEntry) string {
	var parts []string
	if ae.CapabilityMatches > 0 {
		parts = append(parts, fmt.Sprintf("%d capability matches", ae.CapabilityMatches))
	}
	if ae.Stars > 0 {
		parts = append(parts, fmt.Sprintf("%d stars", ae.Stars))
	}
	if ae.Language != "" {
		parts = append(parts, ae.Language)
	}
	if len(parts) == 0 {
		return "insufficient data"
	}
	return strings.Join(parts, ", ")
}
