package roadmap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ResearchResults contains findings from external sources.
type ResearchResults struct {
	Query    string    `json:"query"`
	Findings []Finding `json:"findings"`
	Searched int       `json:"searched"`
}

// Finding is a single research result.
type Finding struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	Stars       int     `json:"stars"`
	Language    string  `json:"language,omitempty"`
	Relevance   float64 `json:"relevance"`
	IsNewDep    bool    `json:"is_new_dep"`
}

// Research searches GitHub for relevant repos based on project context.
func Research(ctx context.Context, client *http.Client, repoPath string, topics string, limit int) (*ResearchResults, error) {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if limit <= 0 {
		limit = 10
	}

	// Infer topics from project if not provided
	if topics == "" {
		topics = inferTopics(repoPath)
	}
	if topics == "" {
		return nil, fmt.Errorf("no topics specified and could not infer from project")
	}

	// Read go.mod for existing deps
	existingDeps := readGoModDeps(repoPath)

	// Build search queries
	queries := buildSearchQueries(topics)

	results := &ResearchResults{
		Query: topics,
	}

	seen := make(map[string]bool)

	for _, q := range queries {
		if len(results.Findings) >= limit {
			break
		}

		findings, err := searchGitHub(ctx, client, q)
		if err != nil {
			continue // Skip failed queries
		}
		results.Searched++

		for _, f := range findings {
			if seen[f.Name] {
				continue
			}
			seen[f.Name] = true

			// Check if already a dependency
			f.IsNewDep = !existingDeps[f.Name]

			results.Findings = append(results.Findings, f)
			if len(results.Findings) >= limit {
				break
			}
		}
	}

	return results, nil
}

func inferTopics(repoPath string) string {
	var topics []string

	// Read go.mod module name
	gomodPath := filepath.Join(repoPath, "go.mod")
	if f, err := os.Open(gomodPath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if after, ok := strings.CutPrefix(line, "module "); ok {
				parts := strings.Split(after, "/")
				if len(parts) > 0 {
					topics = append(topics, parts[len(parts)-1])
				}
				break
			}
		}
		f.Close()
	}

	// Read first lines of README or CLAUDE.md for context words
	for _, name := range []string{"CLAUDE.md", "README.md"} {
		p := filepath.Join(repoPath, name)
		if f, err := os.Open(p); err == nil {
			scanner := bufio.NewScanner(f)
			lines := 0
			for scanner.Scan() && lines < 10 {
				line := strings.ToLower(scanner.Text())
				for _, kw := range []string{"mcp", "tui", "agent", "cli", "automation", "ai", "llm"} {
					if strings.Contains(line, kw) {
						topics = append(topics, kw)
					}
				}
				lines++
			}
			f.Close()
			break
		}
	}

	return strings.Join(dedupStrings(topics), " ")
}

func readGoModDeps(repoPath string) map[string]bool {
	deps := make(map[string]bool)
	gomodPath := filepath.Join(repoPath, "go.mod")
	f, err := os.Open(gomodPath)
	if err != nil {
		return deps
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "require") || strings.HasPrefix(line, ")") || line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 1 {
			deps[parts[0]] = true
		}
	}
	return deps
}

func buildSearchQueries(topics string) []string {
	words := strings.Fields(topics)
	queries := []string{topics + " language:go"}
	if len(words) > 1 {
		for _, w := range words {
			queries = append(queries, w+" language:go")
		}
	}
	return queries
}

// githubSearchResponse is the GitHub API search result structure.
type githubSearchResponse struct {
	Items []struct {
		FullName    string `json:"full_name"`
		HTMLURL     string `json:"html_url"`
		Description string `json:"description"`
		Stars       int    `json:"stargazers_count"`
		Language    string `json:"language"`
	} `json:"items"`
}

func searchGitHub(ctx context.Context, client *http.Client, query string) ([]Finding, error) {
	url := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&per_page=5",
		strings.ReplaceAll(query, " ", "+"))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github API %d: %s", resp.StatusCode, body)
	}

	var result githubSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var findings []Finding
	queryKW := extractQueryKeywords(stripSearchModifiers(query))
	for _, item := range result.Items {
		itemText := item.FullName + " " + item.Description
		itemKW := extractQueryKeywords(itemText)
		findings = append(findings, Finding{
			Name:        item.FullName,
			URL:         item.HTMLURL,
			Description: item.Description,
			Stars:       item.Stars,
			Language:    item.Language,
			Relevance:   weightedRelevance(queryKW, itemKW, item.Stars),
		})
	}
	return findings, nil
}

// stopWords is the set of common English words filtered from keyword extraction.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "being": true, "have": true, "has": true,
	"had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
	"could": true, "should": true, "may": true, "might": true, "must": true,
	"shall": true, "can": true, "need": true, "dare": true, "ought": true, "used": true,
	"to": true, "of": true, "in": true, "for": true, "on": true, "with": true,
	"at": true, "by": true, "from": true, "as": true, "into": true, "through": true,
	"during": true, "before": true, "after": true, "above": true, "below": true,
	"between": true, "out": true, "off": true, "over": true, "under": true,
	"again": true, "further": true, "then": true, "once": true, "and": true,
	"but": true, "or": true, "nor": true, "not": true, "so": true, "yet": true,
	"both": true, "either": true, "neither": true, "each": true, "every": true,
	"all": true, "any": true, "few": true, "more": true, "most": true, "other": true,
	"some": true, "such": true, "no": true, "only": true, "own": true, "same": true,
	"than": true, "too": true, "very": true, "just": true, "because": true,
	"if": true, "when": true, "where": true, "how": true, "what": true, "which": true,
	"who": true, "whom": true, "this": true, "that": true, "these": true,
	"those": true, "it": true, "its": true,
}

// extractQueryKeywords splits text into lowercase keywords, filtering stop words
// and short tokens.
func extractQueryKeywords(text string) map[string]bool {
	kw := make(map[string]bool)
	for w := range strings.FieldsSeq(strings.ToLower(text)) {
		// Strip common punctuation and path separators
		w = strings.Trim(w, ".,;:!?\"'`()[]{}/-")
		if len(w) < 2 || stopWords[w] {
			continue
		}
		kw[w] = true
	}
	return kw
}

// jaccardSimilarity computes |intersection| / |union| of two keyword sets.
// Returns 0.0 if both sets are empty.
func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0.0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0.0
	}
	return float64(intersection) / float64(union)
}

// stripSearchModifiers removes GitHub search qualifiers (e.g., "language:go")
// from a query string so they don't pollute keyword extraction. (QW-10)
func stripSearchModifiers(query string) string {
	var words []string
	for w := range strings.FieldsSeq(query) {
		if !strings.Contains(w, ":") {
			words = append(words, w)
		}
	}
	return strings.Join(words, " ")
}

// weightedRelevance computes a relevance score that varies meaningfully across
// results by combining keyword overlap (Jaccard) with a coverage ratio and a
// small star-count signal. This replaces flat Jaccard which clustered at 0.5. (QW-10)
func weightedRelevance(queryKW, itemKW map[string]bool, stars int) float64 {
	jaccard := jaccardSimilarity(queryKW, itemKW)

	// Coverage: fraction of query keywords found in item (recall-oriented)
	coverage := 0.0
	if len(queryKW) > 0 {
		matched := 0
		for k := range queryKW {
			if itemKW[k] {
				matched++
			}
		}
		coverage = float64(matched) / float64(len(queryKW))
	}

	// Star signal: log-scale popularity boost (capped at 0.15)
	starBoost := 0.0
	if stars > 0 {
		// log2(stars)/20 gives ~0.05 at 10 stars, ~0.10 at 100, ~0.15 at 1000+
		starBoost = math.Log2(float64(stars)) / 20.0
		if starBoost > 0.15 {
			starBoost = 0.15
		}
	}

	// Combined: 40% jaccard + 45% coverage + 15% star boost
	score := 0.40*jaccard + 0.45*coverage + starBoost
	if score > 1.0 {
		score = 1.0
	}
	return score
}

func dedupStrings(ss []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
