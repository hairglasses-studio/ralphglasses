package fleet

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// WorkQueue is a thread-safe priority queue of work items with JSON persistence.
type WorkQueue struct {
	mu    sync.Mutex
	items map[string]*WorkItem // keyed by ID
	dlq   map[string]*WorkItem // dead letter queue for permanently failed items
}

// NewWorkQueue creates an empty work queue.
func NewWorkQueue() *WorkQueue {
	return &WorkQueue{
		items: make(map[string]*WorkItem),
		dlq:   make(map[string]*WorkItem),
	}
}

// Push adds a work item to the queue.
func (q *WorkQueue) Push(item *WorkItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items[item.ID] = item
}

// PushValidated adds a work item to the queue after validating its RepoPath.
// Returns an error if RepoPath is non-empty and does not exist on disk.
func (q *WorkQueue) PushValidated(item *WorkItem) error {
	if item.RepoPath != "" {
		if _, err := os.Stat(item.RepoPath); err != nil {
			return fmt.Errorf("invalid repo path %q: %w", item.RepoPath, err)
		}
	}
	q.Push(item)
	return nil
}

// Get retrieves a work item by ID.
func (q *WorkQueue) Get(id string) (*WorkItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	item, ok := q.items[id]
	return item, ok
}

// Update replaces a work item in the queue.
func (q *WorkQueue) Update(item *WorkItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items[item.ID] = item
}

// AssignBest finds the best pending work item for a worker using a scoring function.
// The scorer returns -1 to skip an item, or a non-negative score (higher = better).
// Returns nil if no suitable work is available.
func (q *WorkQueue) AssignBest(scorer func(*WorkItem) int, workerID string) *WorkItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	var best *WorkItem
	bestScore := -1

	for _, item := range q.items {
		if item.Status != WorkPending {
			continue
		}
		score := scorer(item)
		if score < 0 {
			continue
		}
		if score > bestScore {
			best = item
			bestScore = score
		}
	}

	if best != nil {
		best.Status = WorkAssigned
		best.AssignedTo = workerID
		now := time.Now()
		best.AssignedAt = &now
	}

	return best
}

// ReclaimTimedOut returns assigned work items to pending if they've exceeded the timeout.
func (q *WorkQueue) ReclaimTimedOut(timeout time.Duration) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	for _, item := range q.items {
		if item.Status == WorkAssigned && item.AssignedAt != nil {
			if now.Sub(*item.AssignedAt) > timeout {
				item.Status = WorkPending
				item.AssignedTo = ""
				item.AssignedAt = nil
			}
		}
	}
}

// ReapStale moves pending items older than maxAge to the dead letter queue.
// Items with a non-empty RepoPath that no longer exists on disk are also reaped.
// Returns the number of reaped items.
func (q *WorkQueue) ReapStale(maxAge time.Duration) int {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now()
	var staleIDs []string

	for id, item := range q.items {
		if item.Status != WorkPending {
			continue
		}

		aged := now.Sub(item.SubmittedAt) > maxAge

		pathGone := false
		if item.RepoPath != "" {
			if _, err := os.Stat(item.RepoPath); os.IsNotExist(err) {
				pathGone = true
			}
		}

		if aged || pathGone {
			staleIDs = append(staleIDs, id)
		}
	}

	for _, id := range staleIDs {
		item := q.items[id]
		now := time.Now()
		item.CompletedAt = &now
		item.Error = "reaped: stale task"
		q.dlq[id] = item
		delete(q.items, id)
	}

	return len(staleIDs)
}

// Counts returns the number of items in each status.
func (q *WorkQueue) Counts() map[WorkItemStatus]int {
	q.mu.Lock()
	defer q.mu.Unlock()

	counts := make(map[WorkItemStatus]int)
	for _, item := range q.items {
		counts[item.Status]++
	}
	return counts
}

// Pending returns all pending items sorted by priority (descending).
func (q *WorkQueue) Pending() []*WorkItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	var result []*WorkItem
	for _, item := range q.items {
		if item.Status == WorkPending {
			result = append(result, item)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Priority > result[j].Priority
	})

	return result
}

// All returns all items in the queue.
func (q *WorkQueue) All() []*WorkItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]*WorkItem, 0, len(q.items))
	for _, item := range q.items {
		result = append(result, item)
	}
	return result
}

// SaveTo persists the queue to a JSON file.
func (q *WorkQueue) SaveTo(path string) error {
	q.mu.Lock()
	items := make([]*WorkItem, 0, len(q.items))
	for _, item := range q.items {
		items = append(items, item)
	}
	q.mu.Unlock()

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadFrom restores the queue from a JSON file.
func (q *WorkQueue) LoadFrom(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var items []*WorkItem
	if err := json.Unmarshal(data, &items); err != nil {
		return err
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	for _, item := range items {
		q.items[item.ID] = item
	}
	return nil
}

// MoveToDLQ moves a work item from the main queue to the dead letter queue.
// Returns false if the item was not found in the main queue.
func (q *WorkQueue) MoveToDLQ(itemID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, ok := q.items[itemID]
	if !ok {
		return false
	}
	now := time.Now()
	item.CompletedAt = &now
	q.dlq[itemID] = item
	delete(q.items, itemID)
	return true
}

// ListDLQ returns all items in the dead letter queue.
func (q *WorkQueue) ListDLQ() []*WorkItem {
	q.mu.Lock()
	defer q.mu.Unlock()

	result := make([]*WorkItem, 0, len(q.dlq))
	for _, item := range q.dlq {
		result = append(result, item)
	}
	return result
}

// RetryFromDLQ moves an item from the dead letter queue back to the main queue
// with its retry count and status reset for re-processing.
func (q *WorkQueue) RetryFromDLQ(itemID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, ok := q.dlq[itemID]
	if !ok {
		return fmt.Errorf("item %s not found in DLQ", itemID)
	}

	item.Status = WorkPending
	item.RetryCount = 0
	item.AssignedTo = ""
	item.AssignedAt = nil
	item.CompletedAt = nil
	item.Error = ""
	item.RetryAfter = nil

	q.items[itemID] = item
	delete(q.dlq, itemID)
	return nil
}

// PurgeDLQ removes all items from the dead letter queue.
func (q *WorkQueue) PurgeDLQ() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	n := len(q.dlq)
	q.dlq = make(map[string]*WorkItem)
	return n
}

// DLQDepth returns the number of items in the dead letter queue.
func (q *WorkQueue) DLQDepth() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.dlq)
}

// ReapPhantomRepos moves pending items to the DLQ when RepoName == "001" or
// filepath.Base(RepoPath) == "001". These are known placeholder entries that
// should never be dispatched to workers.
// Returns the number of reaped items.
func (q *WorkQueue) ReapPhantomRepos() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	var phantomIDs []string
	for id, item := range q.items {
		if item.Status != WorkPending {
			continue
		}
		if item.RepoName == "001" || filepath.Base(item.RepoPath) == "001" {
			phantomIDs = append(phantomIDs, id)
		}
	}

	for _, id := range phantomIDs {
		item := q.items[id]
		now := time.Now()
		item.CompletedAt = &now
		item.Error = "reaped: phantom repo placeholder"
		q.dlq[id] = item
		delete(q.items, id)
	}

	return len(phantomIDs)
}
