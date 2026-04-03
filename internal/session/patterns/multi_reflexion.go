// D3.2: Multi-Agent Reflexion — parallel multi-perspective critique with
// judge aggregation.
//
// Informed by Multi-Agent Reflexion (ArXiv 2512.20845): multiple perspectives
// critique the same artifact simultaneously; a judge aggregates via majority vote.
package patterns

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// ReflexionPerspective defines a review viewpoint.
type ReflexionPerspective struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	SystemPrompt string `json:"system_prompt"` // perspective-specific prompt prefix
}

// PerspectiveReview captures one perspective's review of an artifact.
type PerspectiveReview struct {
	Perspective string   `json:"perspective"`
	Verdict     string   `json:"verdict"` // "approve", "request_changes", "comment"
	Comments    []string `json:"comments"`
	Score       float64  `json:"score"` // 0.0-1.0
}

// AggregatedReview is the combined result of multi-perspective review.
type AggregatedReview struct {
	Verdict     string   `json:"verdict"`      // majority vote result
	Score       float64  `json:"score"`        // average score
	Comments    []string `json:"comments"`     // merged unique comments
	Unanimous   bool     `json:"unanimous"`    // all perspectives agree
	ReviewCount int      `json:"review_count"` // how many perspectives reviewed
}

// MultiReflexion manages parallel multi-perspective review of an artifact.
type MultiReflexion struct {
	mu           sync.Mutex
	perspectives []ReflexionPerspective
	reviews      map[string]PerspectiveReview // perspective name -> review
}

// NewMultiReflexion creates a multi-perspective review session.
func NewMultiReflexion(perspectives []ReflexionPerspective) *MultiReflexion {
	return &MultiReflexion{
		perspectives: perspectives,
		reviews:      make(map[string]PerspectiveReview),
	}
}

// Perspectives returns the registered perspectives.
func (mr *MultiReflexion) Perspectives() []ReflexionPerspective {
	return mr.perspectives
}

// AddReview records a perspective's review. Returns error if the perspective
// is not registered.
func (mr *MultiReflexion) AddReview(perspectiveName string, review PerspectiveReview) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	found := false
	for _, p := range mr.perspectives {
		if p.Name == perspectiveName {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown perspective: %s", perspectiveName)
	}

	review.Perspective = perspectiveName
	mr.reviews[perspectiveName] = review
	return nil
}

// AllReviewed returns true if all perspectives have submitted reviews.
func (mr *MultiReflexion) AllReviewed() bool {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return len(mr.reviews) >= len(mr.perspectives)
}

// Aggregate combines all reviews via majority vote on verdict, average score,
// and merged comments.
func (mr *MultiReflexion) Aggregate() AggregatedReview {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if len(mr.reviews) == 0 {
		return AggregatedReview{Verdict: "pending"}
	}

	// Count verdicts for majority vote
	verdictCounts := make(map[string]int)
	var totalScore float64
	commentSet := make(map[string]bool)
	var allComments []string

	for _, r := range mr.reviews {
		verdictCounts[r.Verdict]++
		totalScore += r.Score
		for _, c := range r.Comments {
			if !commentSet[c] {
				commentSet[c] = true
				allComments = append(allComments, c)
			}
		}
	}

	// Majority vote
	verdict := majorityVote(verdictCounts)
	avgScore := totalScore / float64(len(mr.reviews))

	// Check unanimity
	unanimous := len(verdictCounts) == 1

	return AggregatedReview{
		Verdict:     verdict,
		Score:       avgScore,
		Comments:    allComments,
		Unanimous:   unanimous,
		ReviewCount: len(mr.reviews),
	}
}

// Reset clears all reviews for a new artifact review cycle.
func (mr *MultiReflexion) Reset() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.reviews = make(map[string]PerspectiveReview)
}

// FormatReport generates a markdown summary of all reviews and the aggregated result.
func (mr *MultiReflexion) FormatReport() string {
	mr.mu.Lock()
	reviews := make(map[string]PerspectiveReview, len(mr.reviews))
	for k, v := range mr.reviews {
		reviews[k] = v
	}
	mr.mu.Unlock()

	agg := mr.Aggregate()

	var sb strings.Builder
	sb.WriteString("## Multi-Perspective Review\n\n")
	sb.WriteString(fmt.Sprintf("**Verdict:** %s (%.0f%% score, %s)\n\n",
		agg.Verdict, agg.Score*100,
		map[bool]string{true: "unanimous", false: "split"}[agg.Unanimous]))

	// Sort perspectives for deterministic output
	names := make([]string, 0, len(reviews))
	for name := range reviews {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		r := reviews[name]
		sb.WriteString(fmt.Sprintf("### %s: %s (%.0f%%)\n", name, r.Verdict, r.Score*100))
		for _, c := range r.Comments {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
		sb.WriteString("\n")
	}

	if len(agg.Comments) > 0 {
		sb.WriteString("### Merged Action Items\n")
		for _, c := range agg.Comments {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	return sb.String()
}

// majorityVote returns the verdict with the most votes. On tie, prefers
// "request_changes" over "approve" (conservative).
func majorityVote(counts map[string]int) string {
	if len(counts) == 0 {
		return "pending"
	}

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count == sorted[j].count {
			// Tie-break: prefer conservative verdict
			priority := map[string]int{"request_changes": 0, "comment": 1, "approve": 2}
			return priority[sorted[i].key] < priority[sorted[j].key]
		}
		return sorted[i].count > sorted[j].count
	})

	return sorted[0].key
}
