package patterns

import (
	"fmt"
	"sync"
)

// ChainRole identifies a session's role in the review chain.
type ChainRole string

const (
	RoleAuthor   ChainRole = "author"
	RoleReviewer ChainRole = "reviewer"
)

// ChainLink represents one node in the review chain.
type ChainLink struct {
	SessionID string    `json:"session_id"`
	Role      ChainRole `json:"role"`
	Order     int       `json:"order"` // position in chain (0 = author)
}

// ReviewResult captures what one reviewer produced.
type ReviewResult struct {
	ReviewerID string         `json:"reviewer_id"`
	Step       int            `json:"step"`
	Response   ReviewResponse `json:"response"`
}

// ReviewChainPattern implements sequential review where each session reviews
// the prior session's output. The first link is the author; subsequent links
// are reviewers in order.
type ReviewChainPattern struct {
	mu      sync.RWMutex
	chain   []ChainLink
	results []ReviewResult
	current int    // index of the link currently active
	content string // the artifact being reviewed (updated after each step)
	taskID  string
	done    bool
}

// NewReviewChainPattern creates a review chain.
// The first sessionID is the author; the rest are reviewers in order.
func NewReviewChainPattern(taskID string, sessionIDs []string) (*ReviewChainPattern, error) {
	if len(sessionIDs) < 2 {
		return nil, ErrEmptyChain
	}
	chain := make([]ChainLink, len(sessionIDs))
	chain[0] = ChainLink{SessionID: sessionIDs[0], Role: RoleAuthor, Order: 0}
	for i := 1; i < len(sessionIDs); i++ {
		chain[i] = ChainLink{SessionID: sessionIDs[i], Role: RoleReviewer, Order: i}
	}
	return &ReviewChainPattern{
		chain:   chain,
		taskID:  taskID,
		current: 0,
	}, nil
}

// Chain returns a copy of the chain links.
func (rc *ReviewChainPattern) Chain() []ChainLink {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	out := make([]ChainLink, len(rc.chain))
	copy(out, rc.chain)
	return out
}

// CurrentLink returns the chain link that should act next.
func (rc *ReviewChainPattern) CurrentLink() (ChainLink, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.done {
		return ChainLink{}, fmt.Errorf("patterns: review chain is complete")
	}
	return rc.chain[rc.current], nil
}

// SetContent sets the initial content (from the author) to be reviewed.
func (rc *ReviewChainPattern) SetContent(content string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.content = content
	// Author has produced content; advance to first reviewer.
	if rc.current == 0 {
		rc.current = 1
	}
}

// Content returns the current artifact content.
func (rc *ReviewChainPattern) Content() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.content
}

// BuildReviewRequest creates a ReviewRequest for the current reviewer.
func (rc *ReviewChainPattern) BuildReviewRequest() (*ReviewRequest, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.done {
		return nil, fmt.Errorf("patterns: review chain is complete")
	}
	if rc.current == 0 {
		return nil, fmt.Errorf("patterns: author has not yet produced content")
	}
	return &ReviewRequest{
		TaskID:    rc.taskID,
		Content:   rc.content,
		ChainStep: rc.current,
	}, nil
}

// SubmitReview records a reviewer's response and advances the chain.
// If the reviewer modifies the content, pass the updated content.
func (rc *ReviewChainPattern) SubmitReview(resp ReviewResponse, updatedContent string) error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.done {
		return fmt.Errorf("patterns: review chain is complete")
	}
	if rc.current >= len(rc.chain) {
		return fmt.Errorf("patterns: chain index out of range")
	}
	rc.results = append(rc.results, ReviewResult{
		ReviewerID: rc.chain[rc.current].SessionID,
		Step:       rc.current,
		Response:   resp,
	})
	if updatedContent != "" {
		rc.content = updatedContent
	}
	rc.current++
	if rc.current >= len(rc.chain) {
		rc.done = true
	}
	return nil
}

// IsDone returns true when all reviewers have submitted.
func (rc *ReviewChainPattern) IsDone() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.done
}

// Results returns a copy of all review results so far.
func (rc *ReviewChainPattern) Results() []ReviewResult {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	out := make([]ReviewResult, len(rc.results))
	copy(out, rc.results)
	return out
}

// FinalVerdict returns the aggregate verdict. Approved only if all reviewers approved.
// Returns needs_changes if any reviewer requested changes, rejected if any rejected.
func (rc *ReviewChainPattern) FinalVerdict() (Verdict, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if !rc.done {
		return "", fmt.Errorf("patterns: chain not yet complete")
	}
	worst := VerdictApproved
	for _, r := range rc.results {
		switch r.Response.Verdict {
		case VerdictRejected:
			return VerdictRejected, nil
		case VerdictNeedsChanges:
			worst = VerdictNeedsChanges
		}
	}
	return worst, nil
}
