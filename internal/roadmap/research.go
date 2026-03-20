package roadmap

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
			if strings.HasPrefix(line, "module ") {
				parts := strings.Split(strings.TrimPrefix(line, "module "), "/")
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
	for _, item := range result.Items {
		findings = append(findings, Finding{
			Name:        item.FullName,
			URL:         item.HTMLURL,
			Description: item.Description,
			Stars:       item.Stars,
			Language:    item.Language,
			Relevance:   0.5, // Base relevance, could be refined
		})
	}
	return findings, nil
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
