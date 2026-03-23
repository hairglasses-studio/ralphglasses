package fleet

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"
)

// WorkQueue is a thread-safe priority queue of work items with JSON persistence.
type WorkQueue struct {
	mu    sync.Mutex
	items map[string]*WorkItem // keyed by ID
}

// NewWorkQueue creates an empty work queue.
func NewWorkQueue() *WorkQueue {
	return &WorkQueue{
		items: make(map[string]*WorkItem),
	}
}

// Push adds a work item to the queue.
func (q *WorkQueue) Push(item *WorkItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.items[item.ID] = item
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
