package jobs

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func setupJobDB(t *testing.T) *sql.DB {
	t.Helper()
	db := testutil.TestDB(t)
	if err := EnsureTable(db); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	return db
}

func TestEnsureTable_Idempotent(t *testing.T) {
	db := testutil.TestDB(t)
	if err := EnsureTable(db); err != nil {
		t.Fatalf("first EnsureTable: %v", err)
	}
	if err := EnsureTable(db); err != nil {
		t.Fatalf("second EnsureTable: %v", err)
	}
}

func TestEnqueue_Basic(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	id, err := Enqueue(ctx, db, "test_job", `{"key":"val"}`, 0)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if id <= 0 {
		t.Errorf("id = %d, want > 0", id)
	}

	jobs, err := ListPending(ctx, db, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("pending = %d, want 1", len(jobs))
	}
	if jobs[0].Type != "test_job" {
		t.Errorf("type = %q, want %q", jobs[0].Type, "test_job")
	}
	if jobs[0].Payload != `{"key":"val"}` {
		t.Errorf("payload = %q", jobs[0].Payload)
	}
}

func TestEnqueue_Priority(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	Enqueue(ctx, db, "low_prio", "", 1)
	Enqueue(ctx, db, "high_prio", "", 10)
	Enqueue(ctx, db, "mid_prio", "", 5)

	jobs, err := ListPending(ctx, db, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	// Should be ordered by priority DESC
	if jobs[0].Type != "high_prio" {
		t.Errorf("first job = %q, want high_prio", jobs[0].Type)
	}
	if jobs[1].Type != "mid_prio" {
		t.Errorf("second job = %q, want mid_prio", jobs[1].Type)
	}
	if jobs[2].Type != "low_prio" {
		t.Errorf("third job = %q, want low_prio", jobs[2].Type)
	}
}

func TestRetryJob(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	id, _ := Enqueue(ctx, db, "retry_me", "", 0)
	// Mark as failed
	db.ExecContext(ctx, "UPDATE job_queue SET status = 'failed', error_message = 'oops' WHERE id = ?", id)

	err := RetryJob(ctx, db, id)
	if err != nil {
		t.Fatalf("RetryJob: %v", err)
	}

	jobs, _ := ListPending(ctx, db, 10)
	found := false
	for _, j := range jobs {
		if j.ID == id && j.Status == "pending" {
			found = true
		}
	}
	if !found {
		t.Error("retried job should be pending")
	}
}

func TestRetryJob_DeadStatus(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	id, _ := Enqueue(ctx, db, "dead_job", "", 0)
	db.ExecContext(ctx, "UPDATE job_queue SET status = 'dead' WHERE id = ?", id)

	err := RetryJob(ctx, db, id)
	if err != nil {
		t.Fatalf("RetryJob dead: %v", err)
	}

	jobs, _ := ListPending(ctx, db, 10)
	found := false
	for _, j := range jobs {
		if j.ID == id {
			found = true
		}
	}
	if !found {
		t.Error("dead job should be retryable to pending")
	}
}

func TestClearCompleted(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	id, _ := Enqueue(ctx, db, "done_job", "", 0)
	old := time.Now().Add(-48 * time.Hour).Format("2006-01-02T15:04:05")
	db.ExecContext(ctx, "UPDATE job_queue SET status = 'completed', completed_at = ? WHERE id = ?", old, id)

	deleted, err := ClearCompleted(ctx, db, 24*time.Hour)
	if err != nil {
		t.Fatalf("ClearCompleted: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

func TestGetStats(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	Enqueue(ctx, db, "a", "", 0)
	Enqueue(ctx, db, "b", "", 0)
	id3, _ := Enqueue(ctx, db, "c", "", 0)
	db.ExecContext(ctx, "UPDATE job_queue SET status = 'completed' WHERE id = ?", id3)

	pending, running, completed, failed, dead, err := GetStats(ctx, db)
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if pending != 2 {
		t.Errorf("pending = %d, want 2", pending)
	}
	if completed != 1 {
		t.Errorf("completed = %d, want 1", completed)
	}
	if running != 0 || failed != 0 || dead != 0 {
		t.Errorf("unexpected: running=%d failed=%d dead=%d", running, failed, dead)
	}
}

func TestListAll_FilterByStatus(t *testing.T) {
	db := setupJobDB(t)
	ctx := context.Background()

	Enqueue(ctx, db, "a", "", 0)
	id, _ := Enqueue(ctx, db, "b", "", 0)
	db.ExecContext(ctx, "UPDATE job_queue SET status = 'failed' WHERE id = ?", id)

	all, _ := ListAll(ctx, db, "", 50)
	if len(all) != 2 {
		t.Errorf("all jobs = %d, want 2", len(all))
	}

	failed, _ := ListAll(ctx, db, "failed", 50)
	if len(failed) != 1 {
		t.Errorf("failed jobs = %d, want 1", len(failed))
	}
}
