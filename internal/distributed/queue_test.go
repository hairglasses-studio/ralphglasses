package distributed

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDistributedQueue_SubmitAndClaim(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())

	task := &DistributedTask{Type: "session", Prompt: "fix bug", Priority: 5}
	if err := q.Submit(task); err != nil {
		t.Fatal(err)
	}
	if task.ID == "" {
		t.Error("expected auto-generated ID")
	}

	stats := q.Stats()
	if stats.Pending != 1 {
		t.Errorf("pending = %d, want 1", stats.Pending)
	}

	claimed := q.Claim("worker-1")
	if claimed == nil {
		t.Fatal("expected task")
	}
	if claimed.ClaimedBy != "worker-1" {
		t.Errorf("claimed_by = %q, want worker-1", claimed.ClaimedBy)
	}
	if claimed.State != TaskClaimed {
		t.Errorf("state = %q, want claimed", claimed.State)
	}
}

func TestDistributedQueue_PriorityOrder(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())

	q.Submit(&DistributedTask{ID: "low", Type: "a", Priority: 1})
	q.Submit(&DistributedTask{ID: "high", Type: "b", Priority: 10})
	q.Submit(&DistributedTask{ID: "mid", Type: "c", Priority: 5})

	first := q.Claim("w1")
	if first.ID != "high" {
		t.Errorf("first claim = %q, want high", first.ID)
	}
	second := q.Claim("w2")
	if second.ID != "mid" {
		t.Errorf("second claim = %q, want mid", second.ID)
	}
}

func TestDistributedQueue_Complete(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())
	q.Submit(&DistributedTask{ID: "t1", Type: "session"})

	q.Claim("w1")
	result, _ := json.Marshal(map[string]string{"status": "ok"})
	if err := q.Complete("t1", result); err != nil {
		t.Fatal(err)
	}

	stats := q.Stats()
	if stats.Claimed != 0 {
		t.Errorf("claimed = %d, want 0 after complete", stats.Claimed)
	}
}

func TestDistributedQueue_FailAndRetry(t *testing.T) {
	q := NewDistributedQueue(QueueConfig{MaxRetries: 2, ClaimTimeout: time.Minute, MaxPending: 100, DeadLetterMax: 10})

	q.Submit(&DistributedTask{ID: "t1", Type: "session"})
	q.Claim("w1")
	q.Fail("t1", "timeout")

	// Should be requeued
	stats := q.Stats()
	if stats.Pending != 1 {
		t.Errorf("pending = %d, want 1 (requeued)", stats.Pending)
	}
	if stats.DLQ != 0 {
		t.Errorf("dlq = %d, want 0", stats.DLQ)
	}

	// Claim and fail again
	q.Claim("w2")
	q.Fail("t1", "timeout again")
	stats = q.Stats()
	if stats.Pending != 1 {
		t.Errorf("pending = %d after 2nd fail, want 1", stats.Pending)
	}

	// Third failure exceeds max retries -> DLQ
	q.Claim("w3")
	q.Fail("t1", "final failure")
	stats = q.Stats()
	if stats.DLQ != 1 {
		t.Errorf("dlq = %d, want 1 after exhausting retries", stats.DLQ)
	}
	if stats.Pending != 0 {
		t.Errorf("pending = %d, want 0", stats.Pending)
	}
}

func TestDistributedQueue_ClaimEmpty(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())
	if q.Claim("w1") != nil {
		t.Error("expected nil from empty queue")
	}
}

func TestDistributedQueue_QueueFull(t *testing.T) {
	q := NewDistributedQueue(QueueConfig{MaxPending: 2, MaxRetries: 1, ClaimTimeout: time.Minute, DeadLetterMax: 10})

	q.Submit(&DistributedTask{ID: "t1", Type: "a"})
	q.Submit(&DistributedTask{ID: "t2", Type: "b"})
	err := q.Submit(&DistributedTask{ID: "t3", Type: "c"})
	if err == nil {
		t.Error("expected queue full error")
	}
}

func TestDistributedQueue_ReclaimStale(t *testing.T) {
	q := NewDistributedQueue(QueueConfig{MaxPending: 100, MaxRetries: 2, ClaimTimeout: 1 * time.Millisecond, DeadLetterMax: 10})

	q.Submit(&DistributedTask{ID: "t1", Type: "session"})
	q.Claim("w1")

	time.Sleep(5 * time.Millisecond)
	reclaimed := q.ReclaimStale()
	if reclaimed != 1 {
		t.Errorf("reclaimed = %d, want 1", reclaimed)
	}

	stats := q.Stats()
	if stats.Pending != 1 || stats.Claimed != 0 {
		t.Errorf("after reclaim: pending=%d claimed=%d, want 1/0", stats.Pending, stats.Claimed)
	}
}

func TestDistributedQueue_Callbacks(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())

	var completed, dead int
	q.OnComplete(func(*DistributedTask) { completed++ })
	q.OnDead(func(*DistributedTask) { dead++ })

	q.Submit(&DistributedTask{ID: "t1", Type: "a"})
	q.Claim("w1")
	q.Complete("t1", nil)
	if completed != 1 {
		t.Errorf("completed callbacks = %d, want 1", completed)
	}

	q2 := NewDistributedQueue(QueueConfig{MaxPending: 100, MaxRetries: 0, ClaimTimeout: time.Minute, DeadLetterMax: 10})
	q2.OnDead(func(*DistributedTask) { dead++ })
	q2.Submit(&DistributedTask{ID: "t2", Type: "b"})
	q2.Claim("w2")
	q2.Fail("t2", "err")
	q.Claim("w2")
	q.Fail("t2", "err")
	if dead != 1 {
		t.Errorf("dead callbacks = %d, want 1", dead)
	}
}

func TestScheduler_RegisterAndAvailable(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())
	s := NewScheduler(q)

	s.RegisterWorker(WorkerCapability{
		NodeID: "w1", Providers: []string{"claude", "gemini"}, MaxSessions: 5, Active: 2, HealthScore: 0.9,
	})
	s.RegisterWorker(WorkerCapability{
		NodeID: "w2", Providers: []string{"claude"}, MaxSessions: 3, Active: 3, HealthScore: 0.7,
	})

	avail := s.AvailableWorkers("claude")
	if len(avail) != 1 { // w2 is at capacity
		t.Errorf("available = %d, want 1 (w2 at capacity)", len(avail))
	}
	if avail[0].NodeID != "w1" {
		t.Errorf("available[0] = %q, want w1", avail[0].NodeID)
	}
}

func TestScheduler_AssignNext(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())
	s := NewScheduler(q)

	s.RegisterWorker(WorkerCapability{
		NodeID: "w1", Providers: []string{"claude"}, MaxSessions: 5, HealthScore: 0.9,
	})

	q.Submit(&DistributedTask{ID: "t1", Type: "session", Provider: "claude", Priority: 5})

	task, worker := s.AssignNext()
	if task == nil {
		t.Fatal("expected assignment")
	}
	if worker != "w1" {
		t.Errorf("worker = %q, want w1", worker)
	}
}

func TestScheduler_FleetCapacity(t *testing.T) {
	q := NewDistributedQueue(DefaultQueueConfig())
	s := NewScheduler(q)

	s.RegisterWorker(WorkerCapability{NodeID: "w1", MaxSessions: 10, Active: 3})
	s.RegisterWorker(WorkerCapability{NodeID: "w2", MaxSessions: 5, Active: 5})

	cap := s.FleetCapacity()
	if cap.TotalWorkers != 2 {
		t.Errorf("workers = %d, want 2", cap.TotalWorkers)
	}
	if cap.TotalCapacity != 15 {
		t.Errorf("capacity = %d, want 15", cap.TotalCapacity)
	}
	if cap.AvailableSlots != 7 {
		t.Errorf("available = %d, want 7", cap.AvailableSlots)
	}
}
