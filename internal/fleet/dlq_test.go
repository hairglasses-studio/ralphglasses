package fleet

import (
	"testing"
	"time"
)

func TestMoveToDLQ(t *testing.T) {
	q := NewWorkQueue()

	item := &WorkItem{
		ID:       "w1",
		Status:   WorkFailed,
		RepoName: "test-repo",
		Prompt:   "fix bug",
		Error:    "max retries exceeded",
	}
	q.Push(item)

	// Verify item is in main queue
	if _, ok := q.Get("w1"); !ok {
		t.Fatal("expected item in main queue before move")
	}

	// Move to DLQ
	ok := q.MoveToDLQ("w1")
	if !ok {
		t.Fatal("MoveToDLQ returned false")
	}

	// Verify item is no longer in main queue
	if _, ok := q.Get("w1"); ok {
		t.Error("item should not be in main queue after move to DLQ")
	}

	// Verify item is in DLQ
	dlqItems := q.ListDLQ()
	if len(dlqItems) != 1 {
		t.Fatalf("expected 1 DLQ item, got %d", len(dlqItems))
	}
	if dlqItems[0].ID != "w1" {
		t.Errorf("expected DLQ item ID 'w1', got %q", dlqItems[0].ID)
	}
	if dlqItems[0].CompletedAt == nil {
		t.Error("expected CompletedAt to be set on DLQ item")
	}
}

func TestMoveToDLQ_NotFound(t *testing.T) {
	q := NewWorkQueue()

	ok := q.MoveToDLQ("nonexistent")
	if ok {
		t.Error("MoveToDLQ should return false for nonexistent item")
	}
}

func TestRetryFromDLQ(t *testing.T) {
	q := NewWorkQueue()

	now := time.Now()
	retryAfter := now.Add(time.Minute)
	item := &WorkItem{
		ID:          "w1",
		Status:      WorkFailed,
		RepoName:    "test-repo",
		Prompt:      "fix bug",
		RetryCount:  3,
		MaxRetries:  3,
		Error:       "max retries exceeded",
		AssignedTo:  "worker-1",
		AssignedAt:  &now,
		CompletedAt: &now,
		RetryAfter:  &retryAfter,
	}
	q.Push(item)
	q.MoveToDLQ("w1")

	// Retry from DLQ
	if err := q.RetryFromDLQ("w1"); err != nil {
		t.Fatalf("RetryFromDLQ failed: %v", err)
	}

	// Verify item is back in main queue with reset state
	got, ok := q.Get("w1")
	if !ok {
		t.Fatal("expected item back in main queue after retry")
	}
	if got.Status != WorkPending {
		t.Errorf("expected status pending, got %q", got.Status)
	}
	if got.RetryCount != 0 {
		t.Errorf("expected retry count 0, got %d", got.RetryCount)
	}
	if got.AssignedTo != "" {
		t.Errorf("expected empty AssignedTo, got %q", got.AssignedTo)
	}
	if got.AssignedAt != nil {
		t.Error("expected nil AssignedAt")
	}
	if got.CompletedAt != nil {
		t.Error("expected nil CompletedAt")
	}
	if got.Error != "" {
		t.Errorf("expected empty Error, got %q", got.Error)
	}
	if got.RetryAfter != nil {
		t.Error("expected nil RetryAfter")
	}

	// Verify item is no longer in DLQ
	if q.DLQDepth() != 0 {
		t.Errorf("expected DLQ depth 0 after retry, got %d", q.DLQDepth())
	}
}

func TestRetryFromDLQ_NotFound(t *testing.T) {
	q := NewWorkQueue()

	err := q.RetryFromDLQ("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent DLQ item")
	}
}

func TestPurgeDLQ(t *testing.T) {
	q := NewWorkQueue()

	q.Push(&WorkItem{ID: "w1", Status: WorkFailed})
	q.Push(&WorkItem{ID: "w2", Status: WorkFailed})
	q.Push(&WorkItem{ID: "w3", Status: WorkFailed})
	q.MoveToDLQ("w1")
	q.MoveToDLQ("w2")
	q.MoveToDLQ("w3")

	if q.DLQDepth() != 3 {
		t.Fatalf("expected DLQ depth 3 before purge, got %d", q.DLQDepth())
	}

	n := q.PurgeDLQ()
	if n != 3 {
		t.Errorf("PurgeDLQ returned %d, want 3", n)
	}

	if q.DLQDepth() != 0 {
		t.Errorf("expected DLQ depth 0 after purge, got %d", q.DLQDepth())
	}

	// Verify items are also not in main queue
	if len(q.All()) != 0 {
		t.Errorf("expected empty main queue, got %d items", len(q.All()))
	}
}

func TestDLQDepth(t *testing.T) {
	q := NewWorkQueue()

	if q.DLQDepth() != 0 {
		t.Errorf("expected empty DLQ, got depth %d", q.DLQDepth())
	}

	q.Push(&WorkItem{ID: "w1", Status: WorkFailed})
	q.Push(&WorkItem{ID: "w2", Status: WorkFailed})
	q.MoveToDLQ("w1")

	if q.DLQDepth() != 1 {
		t.Errorf("expected DLQ depth 1, got %d", q.DLQDepth())
	}

	q.MoveToDLQ("w2")
	if q.DLQDepth() != 2 {
		t.Errorf("expected DLQ depth 2, got %d", q.DLQDepth())
	}
}

func TestDLQ_DoesNotAffectMainQueueCounts(t *testing.T) {
	q := NewWorkQueue()

	q.Push(&WorkItem{ID: "w1", Status: WorkPending})
	q.Push(&WorkItem{ID: "w2", Status: WorkFailed})
	q.Push(&WorkItem{ID: "w3", Status: WorkCompleted})

	// Move the failed item to DLQ
	q.MoveToDLQ("w2")

	counts := q.Counts()
	if counts[WorkPending] != 1 {
		t.Errorf("expected 1 pending, got %d", counts[WorkPending])
	}
	if counts[WorkCompleted] != 1 {
		t.Errorf("expected 1 completed, got %d", counts[WorkCompleted])
	}
	// Failed item was moved to DLQ, so main queue should have 0 failed
	if counts[WorkFailed] != 0 {
		t.Errorf("expected 0 failed in main queue after DLQ move, got %d", counts[WorkFailed])
	}

	// Total items in main queue should be 2
	if len(q.All()) != 2 {
		t.Errorf("expected 2 items in main queue, got %d", len(q.All()))
	}
}
