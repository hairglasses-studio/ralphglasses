package session

import (
	"sync"
	"testing"
	"time"
)

func TestTaskReviewType(t *testing.T) {
	if string(TaskReview) != "review" {
		t.Fatalf("TaskReview = %q, want %q", TaskReview, "review")
	}
	if !validTaskTypes[TaskReview] {
		t.Fatal("TaskReview should be in validTaskTypes after init")
	}
}

func TestNewTaskSpec(t *testing.T) {
	ts := NewTaskSpec(TaskFeature, "add caching")
	if ts.Type != TaskFeature {
		t.Errorf("Type = %q, want %q", ts.Type, TaskFeature)
	}
	if ts.Name != "add caching" {
		t.Errorf("Name = %q, want %q", ts.Name, "add caching")
	}
	if ts.Priority != PriorityP2 {
		t.Errorf("Priority = %q, want %q", ts.Priority, PriorityP2)
	}
	if ts.EstimatedComplexity != ComplexityM {
		t.Errorf("EstimatedComplexity = %q, want %q", ts.EstimatedComplexity, ComplexityM)
	}
}

func TestTypedTaskSpecValidate_Valid(t *testing.T) {
	ts := &TypedTaskSpec{
		TaskSpec: TaskSpec{
			ID:                  "ws21-001",
			Name:                "implement caching",
			Description:         "add LRU cache layer",
			Type:                TaskFeature,
			Priority:            PriorityP1,
			EstimatedComplexity: ComplexityL,
		},
		ProviderHint:  HintClaude,
		EstimatedCost: 0.50,
		Constraints: []TaskConstraint{
			{Key: "max_turns", Value: "20", Required: true},
		},
	}
	if err := ts.Validate(); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestTypedTaskSpecValidate_InvalidProviderHint(t *testing.T) {
	ts := &TypedTaskSpec{
		TaskSpec: TaskSpec{
			ID:                  "ws21-002",
			Name:                "test",
			Description:         "desc",
			Type:                TaskTest,
			Priority:            PriorityP2,
			EstimatedComplexity: ComplexityS,
		},
		ProviderHint: ProviderHint("gpt5"),
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for invalid provider hint")
	}
	if got := err.Error(); !contains(got, "invalid provider_hint") {
		t.Errorf("error %q should mention invalid provider_hint", got)
	}
}

func TestTypedTaskSpecValidate_NegativeCost(t *testing.T) {
	ts := &TypedTaskSpec{
		TaskSpec: TaskSpec{
			ID:                  "ws21-003",
			Name:                "test",
			Description:         "desc",
			Type:                TaskBugfix,
			Priority:            PriorityP0,
			EstimatedComplexity: ComplexityS,
		},
		EstimatedCost: -1.0,
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for negative cost")
	}
	if got := err.Error(); !contains(got, "non-negative") {
		t.Errorf("error %q should mention non-negative", got)
	}
}

func TestTypedTaskSpecValidate_EmptyConstraintKey(t *testing.T) {
	ts := &TypedTaskSpec{
		TaskSpec: TaskSpec{
			ID:                  "ws21-004",
			Name:                "test",
			Description:         "desc",
			Type:                TaskRefactor,
			Priority:            PriorityP1,
			EstimatedComplexity: ComplexityM,
		},
		Constraints: []TaskConstraint{
			{Key: "", Value: "yes"},
		},
	}
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error for empty constraint key")
	}
	if got := err.Error(); !contains(got, "constraint[0] key is required") {
		t.Errorf("error %q should mention constraint key", got)
	}
}

func TestTypedTaskSpecValidate_InheritsBaseErrors(t *testing.T) {
	ts := &TypedTaskSpec{} // missing all base fields
	err := ts.Validate()
	if err == nil {
		t.Fatal("expected error from base TaskSpec validation")
	}
	if got := err.Error(); !contains(got, "id is required") {
		t.Errorf("error %q should propagate base validation errors", got)
	}
}

func TestMatchesProvider(t *testing.T) {
	tests := []struct {
		name     string
		hint     ProviderHint
		provider string
		want     bool
	}{
		{"no hint matches anything", HintAny, "claude", true},
		{"claude hint matches claude", HintClaude, "claude", true},
		{"claude hint matches Claude (case)", HintClaude, "Claude", true},
		{"claude hint rejects gemini", HintClaude, "gemini", false},
		{"gemini hint matches gemini", HintGemini, "gemini", true},
		{"codex hint matches codex", HintCodex, "codex", true},
		{"codex hint rejects claude", HintCodex, "claude", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := &TypedTaskSpec{ProviderHint: tc.hint}
			if got := ts.MatchesProvider(tc.provider); got != tc.want {
				t.Errorf("MatchesProvider(%q) = %v, want %v", tc.provider, got, tc.want)
			}
		})
	}
}

func TestEstimateDuration(t *testing.T) {
	tests := []struct {
		name       string
		taskType   TaskType
		complexity Complexity
		want       time.Duration
	}{
		{"small bugfix", TaskBugfix, ComplexityS, time.Duration(float64(2*time.Minute) * 0.8)},
		{"medium feature", TaskFeature, ComplexityM, time.Duration(float64(10*time.Minute) * 1.2)},
		{"large research", TaskResearch, ComplexityL, time.Duration(float64(30*time.Minute) * 1.5)},
		{"XL refactor", TaskRefactor, ComplexityXL, time.Duration(float64(60*time.Minute) * 1.0)},
		{"small review", TaskReview, ComplexityS, time.Duration(float64(2*time.Minute) * 0.6)},
		{"small docs", TaskDocs, ComplexityM, time.Duration(float64(10*time.Minute) * 0.5)},
		{"small test", TaskTest, ComplexityS, time.Duration(float64(2*time.Minute) * 0.7)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := &TypedTaskSpec{
				TaskSpec: TaskSpec{
					Type:                tc.taskType,
					EstimatedComplexity: tc.complexity,
				},
			}
			got := ts.EstimateDuration()
			if got != tc.want {
				t.Errorf("EstimateDuration() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEstimateDuration_UnknownDefaults(t *testing.T) {
	ts := &TypedTaskSpec{
		TaskSpec: TaskSpec{
			Type:                TaskType("unknown"),
			EstimatedComplexity: Complexity("unknown"),
		},
	}
	got := ts.EstimateDuration()
	// unknown complexity -> 10m base, unknown type -> 1.0 multiplier
	if got != 10*time.Minute {
		t.Errorf("EstimateDuration() = %v, want %v", got, 10*time.Minute)
	}
}

// ---------- TaskQueue tests ----------

func TestTaskQueue_PriorityOrdering(t *testing.T) {
	q := NewTaskQueue()

	tasks := []*TypedTaskSpec{
		{TaskSpec: TaskSpec{ID: "p3", Name: "low", Description: "d", Type: TaskDocs, Priority: PriorityP3, EstimatedComplexity: ComplexityS}},
		{TaskSpec: TaskSpec{ID: "p0", Name: "critical", Description: "d", Type: TaskBugfix, Priority: PriorityP0, EstimatedComplexity: ComplexityS}},
		{TaskSpec: TaskSpec{ID: "p2", Name: "normal", Description: "d", Type: TaskFeature, Priority: PriorityP2, EstimatedComplexity: ComplexityM}},
		{TaskSpec: TaskSpec{ID: "p1", Name: "high", Description: "d", Type: TaskTest, Priority: PriorityP1, EstimatedComplexity: ComplexityS}},
	}

	for _, task := range tasks {
		if err := q.Push(task); err != nil {
			t.Fatalf("Push: %v", err)
		}
	}

	if q.Len() != 4 {
		t.Fatalf("Len() = %d, want 4", q.Len())
	}

	wantOrder := []string{"p0", "p1", "p2", "p3"}
	for i, wantID := range wantOrder {
		got := q.Pop()
		if got == nil {
			t.Fatalf("Pop() returned nil at position %d", i)
		}
		if got.ID != wantID {
			t.Errorf("Pop()[%d].ID = %q, want %q", i, got.ID, wantID)
		}
	}

	if got := q.Pop(); got != nil {
		t.Errorf("Pop() on empty queue = %+v, want nil", got)
	}
}

func TestTaskQueue_CostTieBreaker(t *testing.T) {
	q := NewTaskQueue()

	// Same priority, different costs — cheaper should come first.
	expensive := &TypedTaskSpec{
		TaskSpec:      TaskSpec{ID: "expensive", Name: "e", Description: "d", Type: TaskFeature, Priority: PriorityP1, EstimatedComplexity: ComplexityM},
		EstimatedCost: 5.0,
	}
	cheap := &TypedTaskSpec{
		TaskSpec:      TaskSpec{ID: "cheap", Name: "c", Description: "d", Type: TaskFeature, Priority: PriorityP1, EstimatedComplexity: ComplexityS},
		EstimatedCost: 0.5,
	}

	_ = q.Push(expensive)
	_ = q.Push(cheap)

	first := q.Pop()
	if first.ID != "cheap" {
		t.Errorf("expected cheap task first, got %q", first.ID)
	}
	second := q.Pop()
	if second.ID != "expensive" {
		t.Errorf("expected expensive task second, got %q", second.ID)
	}
}

func TestTaskQueue_Peek(t *testing.T) {
	q := NewTaskQueue()

	if got := q.Peek(); got != nil {
		t.Errorf("Peek() on empty queue = %+v, want nil", got)
	}

	task := &TypedTaskSpec{
		TaskSpec: TaskSpec{ID: "peek-me", Name: "p", Description: "d", Type: TaskDocs, Priority: PriorityP1, EstimatedComplexity: ComplexityS},
	}
	_ = q.Push(task)

	peeked := q.Peek()
	if peeked == nil || peeked.ID != "peek-me" {
		t.Errorf("Peek() = %v, want task peek-me", peeked)
	}
	// Peek should not remove
	if q.Len() != 1 {
		t.Errorf("Len() after Peek = %d, want 1", q.Len())
	}
}

func TestTaskQueue_PushNil(t *testing.T) {
	q := NewTaskQueue()
	err := q.Push(nil)
	if err == nil {
		t.Fatal("expected error pushing nil")
	}
}

func TestTaskQueue_ConcurrentAccess(t *testing.T) {
	q := NewTaskQueue()
	var wg sync.WaitGroup

	// Push 100 tasks concurrently
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ts := &TypedTaskSpec{
				TaskSpec: TaskSpec{
					ID:                  "concurrent",
					Name:                "task",
					Description:         "d",
					Type:                TaskTest,
					Priority:            PriorityP2,
					EstimatedComplexity: ComplexityS,
				},
			}
			_ = q.Push(ts)
		}(i)
	}
	wg.Wait()

	if q.Len() != 100 {
		t.Fatalf("Len() = %d, want 100", q.Len())
	}

	// Pop all concurrently
	var popped int64
	var mu sync.Mutex
	for range 100 {
		wg.Go(func() {
			if q.Pop() != nil {
				mu.Lock()
				popped++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if popped != 100 {
		t.Errorf("popped %d tasks, want 100", popped)
	}
	if q.Len() != 0 {
		t.Errorf("Len() after drain = %d, want 0", q.Len())
	}
}
