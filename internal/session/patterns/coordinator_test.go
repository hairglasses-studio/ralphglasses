package patterns

import "testing"

func TestCentralCoordinator_SetPlan(t *testing.T) {
	cc := NewCentralCoordinator(nil, nil, DefaultErrorPolicy())
	cc.SetPlan([]CoordinatedTask{
		{ID: "t1", Description: "task 1"},
		{ID: "t2", Description: "task 2"},
	})

	completed, failed, pending := cc.PlanStatus()
	if completed != 0 || failed != 0 || pending != 2 {
		t.Errorf("PlanStatus = %d/%d/%d, want 0/0/2", completed, failed, pending)
	}
}

func TestCentralCoordinator_AssignAndComplete(t *testing.T) {
	cc := NewCentralCoordinator(nil, nil, DefaultErrorPolicy())
	cc.SetPlan([]CoordinatedTask{
		{ID: "t1", Description: "task 1"},
	})

	if err := cc.AssignTask("t1", "worker-1"); err != nil {
		t.Fatal(err)
	}
	if err := cc.ReportResult("t1", TaskResult{TaskID: "t1", Success: true, Output: "done"}); err != nil {
		t.Fatal(err)
	}

	completed, _, _ := cc.PlanStatus()
	if completed != 1 {
		t.Errorf("completed = %d, want 1", completed)
	}
	if !cc.AllComplete() {
		t.Error("expected AllComplete")
	}
}

func TestCentralCoordinator_RetryOnFailure(t *testing.T) {
	cc := NewCentralCoordinator(nil, nil, ErrorPolicy{MaxRetries: 2, AbortThreshold: 1.0, ReassignOnFail: true})
	cc.SetPlan([]CoordinatedTask{{ID: "t1", Description: "task 1"}})

	cc.AssignTask("t1", "w1")
	cc.ReportResult("t1", TaskResult{TaskID: "t1", Success: false, Error: "oops"})

	// Should be retried, not failed
	pending := cc.PendingTasks()
	if len(pending) != 1 || pending[0].Status != "retried" {
		t.Errorf("expected 1 retried task, got %v", pending)
	}

	// AssignedTo should be cleared for reassignment
	if pending[0].AssignedTo != "" {
		t.Errorf("expected cleared AssignedTo for reassignment, got %q", pending[0].AssignedTo)
	}
}

func TestCentralCoordinator_AbortThreshold(t *testing.T) {
	cc := NewCentralCoordinator(nil, nil, ErrorPolicy{MaxRetries: 0, AbortThreshold: 0.5})
	cc.SetPlan([]CoordinatedTask{
		{ID: "t1", Description: "task 1"},
		{ID: "t2", Description: "task 2"},
	})

	// Fail t1 (50% failure rate = threshold)
	cc.AssignTask("t1", "w1")
	cc.ReportResult("t1", TaskResult{TaskID: "t1", Success: false})

	if !cc.ShouldAbort() {
		t.Error("expected ShouldAbort after 50% failure")
	}
	if !cc.AllComplete() {
		t.Error("expected AllComplete when aborted")
	}
}

func TestCentralCoordinator_AssignNotFound(t *testing.T) {
	cc := NewCentralCoordinator(nil, nil, DefaultErrorPolicy())
	cc.SetPlan(nil)

	if err := cc.AssignTask("nonexistent", "w1"); err == nil {
		t.Error("expected error for nonexistent task")
	}
}
