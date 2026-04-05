package marathon

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// mockVMProvider implements VMProvider for testing.
type mockVMProvider struct {
	mu sync.Mutex

	provisionFn func(ctx context.Context, spec VMSpec) (*VMInfo, error)
	statusFn    func(ctx context.Context, vmID string) (*VMInfo, error)
	executeFn   func(ctx context.Context, vmID string, command string) ([]byte, error)
	terminateFn func(ctx context.Context, vmID string) error
	costSoFarFn func(ctx context.Context, vmID string) (float64, error)

	provisionCount int
	terminateCount int
	executeCount   int
	terminatedVMs  []string
	executedCmds   []string
}

func newMockVMProvider() *mockVMProvider {
	return &mockVMProvider{}
}

func (m *mockVMProvider) Provision(ctx context.Context, spec VMSpec) (*VMInfo, error) {
	m.mu.Lock()
	m.provisionCount++
	m.mu.Unlock()

	if m.provisionFn != nil {
		return m.provisionFn(ctx, spec)
	}
	return &VMInfo{
		ID:        "vm-test-001",
		PublicIP:  "10.0.0.1",
		Status:    "running",
		CreatedAt: time.Now(),
	}, nil
}

func (m *mockVMProvider) Status(ctx context.Context, vmID string) (*VMInfo, error) {
	if m.statusFn != nil {
		return m.statusFn(ctx, vmID)
	}
	return &VMInfo{ID: vmID, Status: "running"}, nil
}

func (m *mockVMProvider) Execute(ctx context.Context, vmID string, command string) ([]byte, error) {
	m.mu.Lock()
	m.executeCount++
	m.executedCmds = append(m.executedCmds, command)
	m.mu.Unlock()

	if m.executeFn != nil {
		return m.executeFn(ctx, vmID, command)
	}
	// Return a valid checkpoint JSON by default.
	cp := Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: 5,
		SpentUSD:        1.23,
	}
	data, _ := json.Marshal(cp)
	return data, nil
}

func (m *mockVMProvider) Terminate(ctx context.Context, vmID string) error {
	m.mu.Lock()
	m.terminateCount++
	m.terminatedVMs = append(m.terminatedVMs, vmID)
	m.mu.Unlock()

	if m.terminateFn != nil {
		return m.terminateFn(ctx, vmID)
	}
	return nil
}

func (m *mockVMProvider) CostSoFar(ctx context.Context, vmID string) (float64, error) {
	if m.costSoFarFn != nil {
		return m.costSoFarFn(ctx, vmID)
	}
	return 0.50, nil
}

func validMarathonConfig() Config {
	return Config{
		BudgetUSD:          10.0,
		Duration:           1 * time.Hour,
		CheckpointInterval: 5 * time.Minute,
		SessionCount:       2,
		RepoPath:           "/tmp/test-repo",
	}
}

func validVMSpec() VMSpec {
	return VMSpec{
		Provider:    "gcp",
		MachineType: "n2-standard-8",
		Region:      "us-central1-a",
		Image:       "ubuntu-2404-lts",
		DiskGB:      100,
	}
}

func TestCloudScheduler_Schedule(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 100 * time.Millisecond,
	})

	ctx := context.Background()
	taskID, err := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	if err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}
	if taskID == "" {
		t.Fatal("expected non-empty task ID")
	}

	// Poll until the task is no longer pending.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not progress within deadline")
		default:
		}
		task, err := cs.TaskStatus(taskID)
		if err != nil {
			t.Fatalf("TaskStatus error: %v", err)
		}
		if task.ID != taskID {
			t.Errorf("task ID mismatch: got %q, want %q", task.ID, taskID)
		}
		if task.State != CloudTaskPending {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestCloudScheduler_ScheduleInvalidConfig(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{})

	ctx := context.Background()

	// Invalid marathon config (no duration).
	_, err := cs.Schedule(ctx, Config{RepoPath: "/tmp/test"}, validVMSpec())
	if err == nil {
		t.Fatal("expected error for invalid marathon config")
	}

	// Missing VM provider.
	_, err = cs.Schedule(ctx, validMarathonConfig(), VMSpec{})
	if err == nil {
		t.Fatal("expected error for missing VM provider")
	}
}

func TestCloudScheduler_ScheduleAndComplete(t *testing.T) {
	mock := newMockVMProvider()

	// Return valid results from execute.
	mock.executeFn = func(ctx context.Context, vmID string, command string) ([]byte, error) {
		cp := Checkpoint{
			Timestamp:       time.Now(),
			CyclesCompleted: 12,
			SpentUSD:        4.56,
		}
		data, _ := json.Marshal(cp)
		return data, nil
	}
	mock.costSoFarFn = func(ctx context.Context, vmID string) (float64, error) {
		return 1.20, nil
	}

	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	taskID, err := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	if err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for completion.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not complete within deadline")
		default:
		}

		task, err := cs.TaskStatus(taskID)
		if err != nil {
			t.Fatalf("TaskStatus error: %v", err)
		}

		if task.State == CloudTaskCompleted {
			if task.Result == nil {
				t.Fatal("completed task has nil result")
			}
			if task.Result.Stats.CyclesCompleted != 12 {
				t.Errorf("cycles: got %d, want 12", task.Result.Stats.CyclesCompleted)
			}
			if task.Result.Stats.TotalSpentUSD != 4.56 {
				t.Errorf("marathon cost: got %.2f, want 4.56", task.Result.Stats.TotalSpentUSD)
			}
			if task.Result.VMCostUSD != 1.20 {
				t.Errorf("vm cost: got %.2f, want 1.20", task.Result.VMCostUSD)
			}
			break
		}
		if task.State == CloudTaskFailed {
			t.Fatalf("task failed: %s", task.Error)
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Verify VM was terminated.
	mock.mu.Lock()
	if mock.terminateCount == 0 {
		t.Error("VM was not terminated after completion")
	}
	mock.mu.Unlock()
}

func TestCloudScheduler_Cancel(t *testing.T) {
	provisionDone := make(chan struct{})
	mock := newMockVMProvider()
	mock.provisionFn = func(ctx context.Context, spec VMSpec) (*VMInfo, error) {
		close(provisionDone)
		return &VMInfo{ID: "vm-cancel-test", Status: "running", CreatedAt: time.Now()}, nil
	}
	// Make execute block until context is cancelled.
	mock.executeFn = func(ctx context.Context, vmID string, command string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	taskID, err := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	if err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for provisioning to complete before cancelling.
	select {
	case <-provisionDone:
	case <-time.After(5 * time.Second):
		t.Fatal("provisioning did not complete")
	}

	// Poll until the task enters Running state.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not reach running state within deadline")
		default:
		}
		task, _ := cs.TaskStatus(taskID)
		if task.State == CloudTaskRunning {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	if err := cs.Cancel(ctx, taskID); err != nil {
		t.Fatalf("Cancel error: %v", err)
	}

	task, err := cs.TaskStatus(taskID)
	if err != nil {
		t.Fatalf("TaskStatus error: %v", err)
	}
	if task.State != CloudTaskCancelled {
		t.Errorf("state: got %q, want %q", task.State, CloudTaskCancelled)
	}

	// Verify VM termination was attempted.
	mock.mu.Lock()
	terminated := mock.terminateCount
	mock.mu.Unlock()
	if terminated == 0 {
		t.Error("VM was not terminated after cancel")
	}
}

func TestCloudScheduler_CancelNotFound(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{})

	err := cs.Cancel(context.Background(), "nonexistent-task")
	if err == nil {
		t.Fatal("expected error for nonexistent task")
	}
}

func TestCloudScheduler_ProvisionFailure(t *testing.T) {
	mock := newMockVMProvider()
	mock.provisionFn = func(ctx context.Context, spec VMSpec) (*VMInfo, error) {
		return nil, fmt.Errorf("quota exceeded")
	}

	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	taskID, err := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	if err != nil {
		t.Fatalf("Schedule error: %v", err)
	}

	// Wait for failure.
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not fail within deadline")
		default:
		}

		task, err := cs.TaskStatus(taskID)
		if err != nil {
			t.Fatalf("TaskStatus error: %v", err)
		}
		if task.State == CloudTaskFailed {
			if task.Error == "" {
				t.Error("expected error message on failed task")
			}
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestCloudScheduler_ConcurrencyLimit(t *testing.T) {
	mock := newMockVMProvider()
	// Make execute block forever so tasks stay running.
	mock.executeFn = func(ctx context.Context, vmID string, command string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		MaxConcurrent: 1,
		PollInterval:  50 * time.Millisecond,
	})

	ctx := t.Context()

	// First task should succeed.
	id1, err := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	if err != nil {
		t.Fatalf("first Schedule error: %v", err)
	}

	// Poll until task is in an active state (provisioning/running).
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not reach active state within deadline")
		default:
		}
		task, _ := cs.TaskStatus(id1)
		if task.State == CloudTaskProvisioning || task.State == CloudTaskRunning {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Second task should be rejected.
	_, err = cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	if err == nil {
		t.Fatal("expected concurrency limit error")
	}
}

func TestCloudScheduler_ListTasks(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()

	// Schedule two tasks.
	id1, _ := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())
	id2, _ := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())

	// Poll until both tasks reach a terminal state.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("tasks did not complete within deadline")
		default:
		}
		all := cs.ListTasks([]CloudTaskState{CloudTaskCompleted, CloudTaskFailed})
		if len(all) >= 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	all := cs.ListTasks(nil)
	if len(all) != 2 {
		t.Errorf("ListTasks(nil): got %d tasks, want 2", len(all))
	}

	// Check IDs are present.
	ids := make(map[string]bool)
	for _, t := range all {
		ids[t.ID] = true
	}
	if !ids[id1] || !ids[id2] {
		t.Errorf("ListTasks missing expected IDs: got %v", ids)
	}
}

func TestCloudScheduler_ListTasksFilterByState(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	cs.Schedule(ctx, validMarathonConfig(), validVMSpec())

	// Poll until task reaches a terminal state.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not complete within deadline")
		default:
		}
		done := cs.ListTasks([]CloudTaskState{CloudTaskCompleted, CloudTaskFailed})
		if len(done) > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	completed := cs.ListTasks([]CloudTaskState{CloudTaskCompleted})
	pending := cs.ListTasks([]CloudTaskState{CloudTaskPending})

	if len(completed) == 0 {
		t.Error("expected at least one completed task")
	}
	if len(pending) != 0 {
		t.Errorf("expected no pending tasks, got %d", len(pending))
	}
}

func TestCloudScheduler_TotalCostUSD(t *testing.T) {
	mock := newMockVMProvider()
	mock.costSoFarFn = func(ctx context.Context, vmID string) (float64, error) {
		return 2.00, nil
	}
	mock.executeFn = func(ctx context.Context, vmID string, command string) ([]byte, error) {
		cp := Checkpoint{CyclesCompleted: 3, SpentUSD: 1.50}
		data, _ := json.Marshal(cp)
		return data, nil
	}

	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	cs.Schedule(ctx, validMarathonConfig(), validVMSpec())

	// Poll until task reaches a terminal state.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not complete within deadline")
		default:
		}
		done := cs.ListTasks([]CloudTaskState{CloudTaskCompleted, CloudTaskFailed})
		if len(done) > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	total := cs.TotalCostUSD()
	// Expected: VM cost (2.00) + marathon cost (1.50) = 3.50
	if total < 3.49 || total > 3.51 {
		t.Errorf("TotalCostUSD: got %.2f, want ~3.50", total)
	}
}

func TestCloudScheduler_BuildMarathonCommand(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		MarathonBinary: "/usr/local/bin/ralphglasses",
	})

	task := &CloudTask{
		Marathon: Config{
			BudgetUSD:    25.0,
			Duration:     2 * time.Hour,
			SessionCount: 4,
			RepoPath:     "/home/user/repo",
		},
	}

	cmd := cs.buildMarathonCommand(task)
	expected := "/usr/local/bin/ralphglasses marathon --budget 25.00 --duration 2h0m0s --sessions 4 --repo /home/user/repo"
	if cmd != expected {
		t.Errorf("command mismatch:\n  got:  %s\n  want: %s", cmd, expected)
	}
}

func TestCloudScheduler_VMTerminatedOnFailure(t *testing.T) {
	mock := newMockVMProvider()
	// Marathon execution fails.
	mock.executeFn = func(ctx context.Context, vmID string, command string) ([]byte, error) {
		return nil, fmt.Errorf("marathon crashed")
	}

	cs := NewCloudScheduler(mock, CloudSchedulerConfig{
		PollInterval: 50 * time.Millisecond,
	})

	ctx := context.Background()
	taskID, _ := cs.Schedule(ctx, validMarathonConfig(), validVMSpec())

	// Wait for failure state AND VM termination.
	// The defer in runTask terminates the VM after failTask sets state,
	// so we must poll for both conditions to avoid a race.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("task did not fail and terminate VM within deadline")
		default:
		}

		task, _ := cs.TaskStatus(taskID)
		mock.mu.Lock()
		terminated := mock.terminateCount
		mock.mu.Unlock()

		if task.State == CloudTaskFailed && terminated > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestCloudScheduler_DefaultConfig(t *testing.T) {
	mock := newMockVMProvider()
	cs := NewCloudScheduler(mock, CloudSchedulerConfig{})

	if cs.cfg.PollInterval != 30*time.Second {
		t.Errorf("default PollInterval: got %v, want 30s", cs.cfg.PollInterval)
	}
	if cs.cfg.MarathonBinary != "ralphglasses" {
		t.Errorf("default MarathonBinary: got %q, want %q", cs.cfg.MarathonBinary, "ralphglasses")
	}
	if cs.cfg.ResultCommand == "" {
		t.Error("default ResultCommand should not be empty")
	}
}

func TestParseStatsFromOutput_Checkpoint(t *testing.T) {
	cp := Checkpoint{
		CyclesCompleted: 7,
		SpentUSD:        3.21,
	}
	data, _ := json.Marshal(cp)

	stats, err := parseStatsFromOutput(data)
	if err != nil {
		t.Fatalf("parseStatsFromOutput error: %v", err)
	}
	if stats.CyclesCompleted != 7 {
		t.Errorf("cycles: got %d, want 7", stats.CyclesCompleted)
	}
	if stats.TotalSpentUSD != 3.21 {
		t.Errorf("spent: got %.2f, want 3.21", stats.TotalSpentUSD)
	}
}

func TestParseStatsFromOutput_RawStats(t *testing.T) {
	s := Stats{
		CyclesCompleted: 10,
		TotalSpentUSD:   5.00,
		SessionsRun:     3,
	}
	data, _ := json.Marshal(s)

	stats, err := parseStatsFromOutput(data)
	if err != nil {
		t.Fatalf("parseStatsFromOutput error: %v", err)
	}
	// Parsed as checkpoint first (will have zero values), then falls through.
	// Since Checkpoint JSON also unmarshals Stats fields at top level, we accept either.
	if stats.CyclesCompleted == 0 && stats.TotalSpentUSD == 0 {
		t.Error("expected non-zero stats from raw stats JSON")
	}
}

func TestParseStatsFromOutput_InvalidJSON(t *testing.T) {
	_, err := parseStatsFromOutput([]byte("not json"))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
