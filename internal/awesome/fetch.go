package awesome

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DefaultSource is the awesome-claude-code repo.
const DefaultSource = "hesreallyhim/awesome-claude-code"

// Fetch retrieves and parses an awesome-list README into structured entries.
func Fetch(ctx context.Context, client *http.Client, repo string) (*Index, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if repo == "" {
		repo = DefaultSource
	}

	body, err := fetchREADME(ctx, client, repo)
	if err != nil {
		return nil, fmt.Errorf("fetch README for %s: %w", repo, err)
	}

	entries := parseAwesomeList(body)
	return &Index{
		Source:    repo,
		FetchedAt: time.Now().UTC(),
		Entries:   entries,
	}, nil
}

// fetchREADME downloads a repo's README.md via the GitHub API.
func fetchREADME(ctx context.Context, client *http.Client, repo string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/main/README.md", repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	addAuthHeader(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB max
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FetchRepoREADME fetches a single repo's README for analysis.
func FetchRepoREADME(ctx context.Context, client *http.Client, repoURL string) (string, error) {
	// Extract owner/repo from GitHub URL
	repo := extractRepoFromURL(repoURL)
	if repo == "" {
		return "", fmt.Errorf("cannot extract repo from URL: %s", repoURL)
	}
	return fetchREADME(ctx, client, repo)
}

// linkPattern matches markdown links: [text](url) — optionally followed by description.
var linkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)\s*[-–—]?\s*(.*)`)

// parseAwesomeList extracts entries from markdown text.
func parseAwesomeList(md string) []AwesomeEntry {
	var entries []AwesomeEntry
	var currentCategory string

	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)

		// Detect category headers (## or ###)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			cat := strings.TrimLeft(trimmed, "#")
			cat = strings.TrimSpace(cat)
			// Skip non-content headers
			lower := strings.ToLower(cat)
			if lower == "contents" || lower == "table of contents" || lower == "contributing" || lower == "license" {
				continue
			}
			currentCategory = cat
			continue
		}

		// Match list items with links
		if !strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "* ") {
			continue
		}
		item := strings.TrimLeft(trimmed, "-* ")
		matches := linkPattern.FindStringSubmatch(item)
		if matches == nil {
			continue
		}

		url := matches[2]
		// Only include GitHub links
		if !strings.Contains(url, "github.com") {
			continue
		}

		entries = append(entries, AwesomeEntry{
			Name:        matches[1],
			URL:         url,
			Description: strings.TrimSpace(matches[3]),
			Category:    currentCategory,
		})
	}

	return entries
}

// extractRepoFromURL extracts "owner/repo" from a GitHub URL.
func extractRepoFromURL(url string) string {
	// https://github.com/owner/repo or https://github.com/owner/repo/...
	url = strings.TrimRight(url, "/")
	idx := strings.Index(url, "github.com/")
	if idx < 0 {
		return ""
	}
	path := url[idx+len("github.com/"):]
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}
