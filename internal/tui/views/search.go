package views

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// SearchView wraps the search overlay in a View-compatible struct.
type SearchView struct {
	Viewport *ViewportView
	width    int
	height   int
}

// NewSearchView creates a new SearchView.
func NewSearchView() *SearchView {
	return &SearchView{
		Viewport: NewViewportView(),
	}
}

// SetDimensions updates the available width and height.
func (v *SearchView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
}

// Render returns the scrollable viewport content.
func (v *SearchView) Render() string {
	return v.Viewport.Render()
}

// SessionInfo is a lightweight snapshot of a session for search indexing.
// It avoids holding the session mutex during search.
type SessionInfo struct {
	ID       string
	RepoName string
	Provider string
	Status   string
	Prompt   string
	TeamName string
}

// Search performs a fuzzy search across repos, sessions, and cycles.
// It returns up to 20 results sorted by descending score.
func Search(query string, repos []*model.Repo, sessions []SessionInfo, cycles []*session.CycleRun) []components.SearchResult {
	if query == "" {
		return nil
	}

	q := strings.ToLower(query)
	var results []components.SearchResult

	// Search repos
	for _, r := range repos {
		score := scoreMatch(q, r.Name, r.Path, r.StatusDisplay())
		if score > 0 {
			results = append(results, components.SearchResult{
				Type:       components.SearchTypeRepo,
				Name:       r.Name,
				Path:       r.Path,
				Score:      score,
				ViewTarget: 1, // ViewRepoDetail
			})
		}
	}

	// Search sessions
	for _, s := range sessions {
		score := scoreMatch(q, s.RepoName, s.ID, s.Provider, s.Status, s.Prompt, s.TeamName)
		if score > 0 {
			results = append(results, components.SearchResult{
				Type:       components.SearchTypeSession,
				Name:       s.RepoName + " (" + s.ID[:min(8, len(s.ID))] + ")",
				Path:       s.ID,
				Score:      score,
				ViewTarget: 6, // ViewSessionDetail
			})
		}
	}

	// Search cycles
	for _, c := range cycles {
		score := scoreMatch(q, c.Name, c.Objective, string(c.Phase), c.RepoPath)
		if score > 0 {
			results = append(results, components.SearchResult{
				Type:       components.SearchTypeCycle,
				Name:       c.Name,
				Path:       c.ID,
				Score:      score,
				ViewTarget: 18, // ViewRDCycle
			})
		}
	}

	// Sort by score descending (insertion sort — small list)
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	// Cap at 20
	if len(results) > 20 {
		results = results[:20]
	}

	return results
}

// scoreMatch returns the highest match score of query against any of the fields.
// Returns 0 if no field matches.
func scoreMatch(query string, fields ...string) float64 {
	best := 0.0
	for _, field := range fields {
		s := scoreField(query, strings.ToLower(field))
		if s > best {
			best = s
		}
	}
	return best
}

// scoreField scores a single query-vs-field comparison.
func scoreField(query, field string) float64 {
	if field == "" || query == "" {
		return 0
	}

	// Exact match
	if field == query {
		return 100
	}

	// Exact match on basename (for paths)
	base := strings.ToLower(filepath.Base(field))
	if base == query {
		return 95
	}

	// Prefix match
	if strings.HasPrefix(field, query) || strings.HasPrefix(base, query) {
		return 80
	}

	// Contains match
	if strings.Contains(field, query) {
		return 60
	}

	// Word boundary match: query matches the start of any word in the field
	words := splitWords(field)
	for _, w := range words {
		if strings.HasPrefix(w, query) {
			return 70
		}
	}

	// Fuzzy subsequence match
	if fuzzyMatch(query, field) {
		// Score based on how compact the match is
		return 40
	}

	return 0
}

// fuzzyMatch returns true if every rune of query appears in field in order.
func fuzzyMatch(query, field string) bool {
	qi := 0
	qr := []rune(query)
	for _, r := range field {
		if qi < len(qr) && r == qr[qi] {
			qi++
		}
	}
	return qi == len(qr)
}

// splitWords splits a string on non-alphanumeric boundaries.
func splitWords(s string) []string {
	var words []string
	var current strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}
