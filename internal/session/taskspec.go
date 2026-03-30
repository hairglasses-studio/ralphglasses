package session

import (
	"container/heap"
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskReview is the "review" task type, added to complement the original six
// task types defined in task_spec.go.
const TaskReview TaskType = "review"

func init() {
	validTaskTypes[TaskReview] = true
}

// ProviderHint suggests which provider is best suited for this task.
// Empty means no preference.
type ProviderHint string

const (
	HintClaude ProviderHint = "claude"
	HintGemini ProviderHint = "gemini"
	HintCodex  ProviderHint = "codex"
	HintAny    ProviderHint = "" // no preference
)

// TaskConstraint expresses a hard or soft constraint on task execution.
type TaskConstraint struct {
	Key      string `json:"key"`                // e.g. "max_turns", "sandbox", "branch"
	Value    string `json:"value"`              // constraint value
	Required bool   `json:"required,omitempty"` // hard constraint if true
}

// TypedTaskSpec extends TaskSpec with provider routing, cost estimation,
// duration hints, and execution constraints. It is the unit of work for
// the TaskQueue.
type TypedTaskSpec struct {
	TaskSpec

	ProviderHint  ProviderHint     `json:"provider_hint,omitempty"`
	EstimatedCost float64          `json:"estimated_cost_usd,omitempty"` // USD
	Constraints   []TaskConstraint `json:"constraints,omitempty"`
}

// NewTaskSpec creates a TypedTaskSpec with required fields pre-populated and
// sensible defaults for priority (P2) and complexity (M).
func NewTaskSpec(taskType TaskType, title string) *TypedTaskSpec {
	return &TypedTaskSpec{
		TaskSpec: TaskSpec{
			Name:                title,
			Type:                taskType,
			Priority:            PriorityP2,
			EstimatedComplexity: ComplexityM,
		},
	}
}

// Validate extends TaskSpec.Validate with additional checks for the typed
// fields. ProviderHint, EstimatedCost, and Constraints are optional but are
// validated when present.
func (ts *TypedTaskSpec) Validate() error {
	if err := ts.TaskSpec.Validate(); err != nil {
		return err
	}

	var errs []string

	if ts.ProviderHint != "" {
		switch ts.ProviderHint {
		case HintClaude, HintGemini, HintCodex:
			// ok
		default:
			errs = append(errs, fmt.Sprintf("invalid provider_hint %q", ts.ProviderHint))
		}
	}

	if ts.EstimatedCost < 0 {
		errs = append(errs, "estimated_cost_usd must be non-negative")
	}

	for i, c := range ts.Constraints {
		if strings.TrimSpace(c.Key) == "" {
			errs = append(errs, fmt.Sprintf("constraint[%d] key is required", i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("typed task spec validation: %s", strings.Join(errs, "; "))
	}
	return nil
}

// MatchesProvider reports whether the task is compatible with the given
// provider string. Returns true if the task has no provider hint or if
// the hint matches the given provider (case-insensitive).
func (ts *TypedTaskSpec) MatchesProvider(provider string) bool {
	if ts.ProviderHint == "" || ts.ProviderHint == HintAny {
		return true
	}
	return strings.EqualFold(string(ts.ProviderHint), provider)
}

// complexityDurations maps complexity to a base duration estimate.
var complexityDurations = map[Complexity]time.Duration{
	ComplexityS:  2 * time.Minute,
	ComplexityM:  10 * time.Minute,
	ComplexityL:  30 * time.Minute,
	ComplexityXL: 60 * time.Minute,
}

// typeDurationMultiplier adjusts duration by task type.
var typeDurationMultiplier = map[TaskType]float64{
	TaskBugfix:   0.8,
	TaskFeature:  1.2,
	TaskRefactor: 1.0,
	TaskTest:     0.7,
	TaskDocs:     0.5,
	TaskResearch: 1.5,
	TaskReview:   0.6,
}

// EstimateDuration returns a rough time estimate for completing this task,
// based on its complexity and type. Returns 10 minutes as a fallback.
func (ts *TypedTaskSpec) EstimateDuration() time.Duration {
	base, ok := complexityDurations[ts.EstimatedComplexity]
	if !ok {
		base = 10 * time.Minute
	}
	mult, ok := typeDurationMultiplier[ts.Type]
	if !ok {
		mult = 1.0
	}
	return time.Duration(float64(base) * mult)
}

// priorityRank maps Priority to an integer for ordering (lower = higher priority).
var priorityRank = map[Priority]int{
	PriorityP0: 0,
	PriorityP1: 1,
	PriorityP2: 2,
	PriorityP3: 3,
}

// ---------- TaskQueue (priority-ordered, concurrency-safe) ----------

// taskHeap is the underlying heap for TaskQueue.
type taskHeap []*TypedTaskSpec

func (h taskHeap) Len() int { return len(h) }
func (h taskHeap) Less(i, j int) bool {
	ri := priorityRank[h[i].Priority]
	rj := priorityRank[h[j].Priority]
	if ri != rj {
		return ri < rj // lower rank number = higher priority
	}
	// tie-break: lower estimated cost first
	return h[i].EstimatedCost < h[j].EstimatedCost
}
func (h taskHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *taskHeap) Push(x interface{}) { *h = append(*h, x.(*TypedTaskSpec)) }
func (h *taskHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*h = old[:n-1]
	return item
}

// TaskQueue is a concurrency-safe priority queue of TypedTaskSpec items.
// Tasks are dequeued in priority order (P0 first, then P1, etc.).
type TaskQueue struct {
	mu sync.Mutex
	h  taskHeap
}

// NewTaskQueue creates an empty TaskQueue.
func NewTaskQueue() *TaskQueue {
	q := &TaskQueue{}
	heap.Init(&q.h)
	return q
}

// Push adds a task to the queue. Returns an error if the task fails validation.
func (q *TaskQueue) Push(task *TypedTaskSpec) error {
	if task == nil {
		return fmt.Errorf("cannot push nil task")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	heap.Push(&q.h, task)
	return nil
}

// Pop removes and returns the highest-priority task. Returns nil if empty.
func (q *TaskQueue) Pop() *TypedTaskSpec {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() == 0 {
		return nil
	}
	return heap.Pop(&q.h).(*TypedTaskSpec)
}

// Peek returns the highest-priority task without removing it. Returns nil if empty.
func (q *TaskQueue) Peek() *TypedTaskSpec {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.h.Len() == 0 {
		return nil
	}
	return q.h[0]
}

// Len returns the number of tasks in the queue.
func (q *TaskQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.h.Len()
}
