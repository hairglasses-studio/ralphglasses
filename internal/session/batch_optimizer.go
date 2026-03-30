package session

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// BatchStatus represents the lifecycle of a batch group.
type BatchStatus string

const (
	BatchStatusPending    BatchStatus = "pending"
	BatchStatusReady      BatchStatus = "ready"
	BatchStatusSubmitted  BatchStatus = "submitted"
	BatchStatusCompleted  BatchStatus = "completed"
)

// BatchRequest represents a single prompt request that can be batched.
type BatchRequest struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Provider  Provider  `json:"provider"`
	Prompt    string    `json:"prompt"`
	Model     string    `json:"model,omitempty"`
	RepoPath  string    `json:"repo_path,omitempty"`
	Priority  int       `json:"priority"` // higher = more urgent
	AddedAt   time.Time `json:"added_at"`

	promptHash string // cached hash for dedup
}

// BatchGroup is a set of compatible requests grouped for batch submission.
type BatchGroup struct {
	ID        string         `json:"id"`
	Provider  Provider       `json:"provider"`
	Model     string         `json:"model,omitempty"`
	Status    BatchStatus    `json:"status"`
	Requests  []BatchRequest `json:"requests"`
	CreatedAt time.Time      `json:"created_at"`
}

// BatchOptimizerConfig configures the batch optimizer behavior.
type BatchOptimizerConfig struct {
	// MaxGroupSize is the maximum number of requests per batch group.
	MaxGroupSize int `json:"max_group_size"`
	// MaxWaitTime is how long to wait for more requests before submitting.
	MaxWaitTime time.Duration `json:"max_wait_time"`
	// DedupThreshold is the Jaccard similarity above which prompts are
	// considered duplicates and merged. Uses the existing JaccardSimilarity.
	DedupThreshold float64 `json:"dedup_threshold"`
	// MinBatchSize is the minimum requests needed to form a batch group.
	MinBatchSize int `json:"min_batch_size"`
}

// DefaultBatchOptimizerConfig returns sensible defaults.
func DefaultBatchOptimizerConfig() BatchOptimizerConfig {
	return BatchOptimizerConfig{
		MaxGroupSize:   10,
		MaxWaitTime:    30 * time.Second,
		DedupThreshold: 0.85,
		MinBatchSize:   2,
	}
}

// BatchOptimizer groups similar tasks for batch API submission, deduplicates
// redundant prompts, and merges compatible requests. All heuristic-based.
type BatchOptimizer struct {
	mu     sync.Mutex
	config BatchOptimizerConfig
	groups map[string]*BatchGroup // groupID -> group
	dedup  map[string]string      // prompt hash -> first request ID
	stats  BatchOptimizerStats

	nextGroupID int
}

// BatchOptimizerStats tracks optimization metrics.
type BatchOptimizerStats struct {
	TotalRequests    int `json:"total_requests"`
	DedupedRequests  int `json:"deduped_requests"`
	GroupsCreated    int `json:"groups_created"`
	GroupsSubmitted  int `json:"groups_submitted"`
	RequestsMerged   int `json:"requests_merged"`
}

// NewBatchOptimizer creates a new batch optimizer.
func NewBatchOptimizer(config BatchOptimizerConfig) *BatchOptimizer {
	return &BatchOptimizer{
		config: config,
		groups: make(map[string]*BatchGroup),
		dedup:  make(map[string]string),
	}
}

// AddRequest adds a request to the optimizer. It returns the batch group ID
// the request was assigned to, or empty string if the request was deduplicated.
// A second return value indicates whether the request was a duplicate.
func (bo *BatchOptimizer) AddRequest(req BatchRequest) (groupID string, isDup bool) {
	if req.AddedAt.IsZero() {
		req.AddedAt = time.Now()
	}
	req.promptHash = hashPrompt(req.Prompt)

	bo.mu.Lock()
	defer bo.mu.Unlock()

	bo.stats.TotalRequests++

	// Check exact duplicate by hash.
	if existingID, ok := bo.dedup[req.promptHash]; ok {
		bo.stats.DedupedRequests++
		_ = existingID
		return "", true
	}

	// Check near-duplicate by Jaccard similarity.
	if bo.isDuplicatePrompt(req.Prompt) {
		bo.stats.DedupedRequests++
		return "", true
	}

	bo.dedup[req.promptHash] = req.ID

	// Find a compatible group or create one.
	gID := bo.findCompatibleGroup(req)
	if gID == "" {
		gID = bo.createGroup(req)
	} else {
		bo.groups[gID].Requests = append(bo.groups[gID].Requests, req)
		bo.stats.RequestsMerged++
	}

	return gID, false
}

// GetReadyGroups returns batch groups that have reached the minimum size
// or have exceeded the max wait time.
func (bo *BatchOptimizer) GetReadyGroups() []*BatchGroup {
	bo.mu.Lock()
	defer bo.mu.Unlock()

	now := time.Now()
	var ready []*BatchGroup

	for _, g := range bo.groups {
		if g.Status != BatchStatusPending {
			continue
		}
		sizeReady := len(g.Requests) >= bo.config.MinBatchSize
		timeReady := now.Sub(g.CreatedAt) >= bo.config.MaxWaitTime
		full := len(g.Requests) >= bo.config.MaxGroupSize

		if sizeReady || timeReady || full {
			g.Status = BatchStatusReady
			ready = append(ready, g)
		}
	}

	// Sort by priority (highest first), then by creation time.
	sort.Slice(ready, func(i, j int) bool {
		pi := maxPriority(ready[i].Requests)
		pj := maxPriority(ready[j].Requests)
		if pi != pj {
			return pi > pj
		}
		return ready[i].CreatedAt.Before(ready[j].CreatedAt)
	})

	return ready
}

// MarkSubmitted marks a batch group as submitted.
func (bo *BatchOptimizer) MarkSubmitted(groupID string) {
	bo.mu.Lock()
	defer bo.mu.Unlock()

	if g, ok := bo.groups[groupID]; ok {
		g.Status = BatchStatusSubmitted
		bo.stats.GroupsSubmitted++
	}
}

// MarkCompleted marks a batch group as completed and removes it from tracking.
func (bo *BatchOptimizer) MarkCompleted(groupID string) {
	bo.mu.Lock()
	defer bo.mu.Unlock()

	if g, ok := bo.groups[groupID]; ok {
		g.Status = BatchStatusCompleted
		// Clean up dedup entries for this group's requests.
		for _, req := range g.Requests {
			delete(bo.dedup, req.promptHash)
		}
		delete(bo.groups, groupID)
	}
}

// Stats returns current optimization statistics.
func (bo *BatchOptimizer) Stats() BatchOptimizerStats {
	bo.mu.Lock()
	defer bo.mu.Unlock()
	return bo.stats
}

// PendingCount returns the number of requests in pending groups.
func (bo *BatchOptimizer) PendingCount() int {
	bo.mu.Lock()
	defer bo.mu.Unlock()

	count := 0
	for _, g := range bo.groups {
		if g.Status == BatchStatusPending {
			count += len(g.Requests)
		}
	}
	return count
}

// GroupCount returns the total number of active groups.
func (bo *BatchOptimizer) GroupCount() int {
	bo.mu.Lock()
	defer bo.mu.Unlock()
	return len(bo.groups)
}

// findCompatibleGroup returns the ID of a group that can accept this request,
// or empty string if none found. Must be called with mu held.
func (bo *BatchOptimizer) findCompatibleGroup(req BatchRequest) string {
	for id, g := range bo.groups {
		if g.Status != BatchStatusPending {
			continue
		}
		if g.Provider != req.Provider {
			continue
		}
		if g.Model != req.Model {
			continue
		}
		if len(g.Requests) >= bo.config.MaxGroupSize {
			continue
		}
		return id
	}
	return ""
}

// createGroup creates a new batch group for the request. Must be called with mu held.
func (bo *BatchOptimizer) createGroup(req BatchRequest) string {
	bo.nextGroupID++
	id := formatGroupID(bo.nextGroupID)

	bo.groups[id] = &BatchGroup{
		ID:        id,
		Provider:  req.Provider,
		Model:     req.Model,
		Status:    BatchStatusPending,
		Requests:  []BatchRequest{req},
		CreatedAt: time.Now(),
	}
	bo.stats.GroupsCreated++
	return id
}

// isDuplicatePrompt checks if a prompt is similar to any existing tracked prompt.
// Must be called with mu held.
func (bo *BatchOptimizer) isDuplicatePrompt(prompt string) bool {
	for _, g := range bo.groups {
		for _, req := range g.Requests {
			if JaccardSimilarity(prompt, req.Prompt) >= bo.config.DedupThreshold {
				return true
			}
		}
	}
	return false
}

// hashPrompt returns a hex-encoded SHA-256 prefix of the prompt.
func hashPrompt(prompt string) string {
	normalized := strings.TrimSpace(strings.ToLower(prompt))
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:12])
}

// formatGroupID produces a batch group identifier.
func formatGroupID(seq int) string {
	return "batch-" + strings.Replace(
		time.Now().Format("20060102-150405"), "-", "", 1,
	) + "-" + itoa(seq)
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// maxPriority returns the highest priority in a request slice.
func maxPriority(reqs []BatchRequest) int {
	best := 0
	for _, r := range reqs {
		if r.Priority > best {
			best = r.Priority
		}
	}
	return best
}
