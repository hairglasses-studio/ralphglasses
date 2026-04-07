package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeAutomationWindowBounds_Anchor(t *testing.T) {
	now := time.Date(2026, 4, 6, 13, 15, 0, 0, time.UTC)
	policy := DefaultSubscriptionPolicy()
	policy.Enabled = true
	policy.Timezone = "UTC"
	policy.ResetAnchor = "2026-04-06T00:00:00Z"
	policy.ResetWindowHours = 24

	start, end, next, err := computeAutomationWindowBounds(policy, now)
	if err != nil {
		t.Fatalf("computeAutomationWindowBounds: %v", err)
	}
	if !start.Equal(time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("start = %v", start)
	}
	if !end.Equal(time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("end = %v", end)
	}
	if !next.Equal(end) {
		t.Fatalf("next = %v, want %v", next, end)
	}
}

func TestSubscriptionAutomationController_ParkAndResume(t *testing.T) {
	repo := t.TempDir()
	mgr := NewManager()

	now := time.Date(2026, 4, 6, 0, 30, 0, 0, time.UTC)
	launches := 0
	resumes := 0
	mgr.SetHooksForTesting(func(_ context.Context, opts LaunchOptions) (*Session, error) {
		launches++
		id := "launch-1"
		if opts.Resume != "" {
			resumes++
			id = "resume-1"
		}
		return &Session{
			ID:                id,
			Provider:          opts.Provider,
			ProviderSessionID: firstNonEmpty(opts.Resume, "provider-session-1"),
			RepoPath:          opts.RepoPath,
			RepoName:          filepath.Base(opts.RepoPath),
			Status:            StatusRunning,
			Prompt:            opts.Prompt,
			Model:             opts.Model,
			BudgetUSD:         opts.MaxBudgetUSD,
			MaxTurns:          opts.MaxTurns,
			LaunchedAt:        now,
			LastActivity:      now,
			doneCh:            make(chan struct{}),
			OutputCh:          make(chan string, 1),
		}, nil
	}, nil)

	ctrl := NewSubscriptionAutomationController(mgr, repo)
	ctrl.now = func() time.Time { return now }

	policy := DefaultSubscriptionPolicy()
	policy.Enabled = true
	policy.Timezone = "UTC"
	policy.ResetAnchor = "2026-04-06T00:00:00Z"
	policy.ResetWindowHours = 24
	policy.WindowBudgetUSD = 100
	policy.TargetUtilizationPct = 100
	if err := ctrl.SetPolicy(policy); err != nil {
		t.Fatalf("SetPolicy: %v", err)
	}
	if _, err := ctrl.Enqueue(AutomationQueueItem{
		Prompt:   "implement the scheduled feature work",
		Provider: ProviderCodex,
	}); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	ctrl.Tick(context.Background())
	if got := ctrl.Status().ActiveSessionID; got == "" {
		t.Fatal("expected active session after launch tick")
	}
	if launches != 1 || resumes != 0 {
		t.Fatalf("launches=%d resumes=%d", launches, resumes)
	}

	active, ok := mgr.Get("launch-1")
	if !ok {
		t.Fatal("expected launched session in manager")
	}
	active.Lock()
	active.Status = StatusErrored
	active.LastOutput = "Premium usage exhausted. Try again after 2 hours."
	active.OutputHistory = []string{active.LastOutput}
	active.Unlock()

	now = now.Add(10 * time.Minute)
	ctrl.Tick(context.Background())
	snapshot := ctrl.Status()
	if snapshot.WindowStatus != "parked" {
		t.Fatalf("window status = %q, want parked", snapshot.WindowStatus)
	}
	if snapshot.ParkedSessionID == "" {
		t.Fatal("expected parked session id after exhaustion")
	}
	if snapshot.ActiveSessionID != "" {
		t.Fatalf("active session should be cleared, got %q", snapshot.ActiveSessionID)
	}

	now = now.Add(3 * time.Hour)
	ctrl.Tick(context.Background())
	snapshot = ctrl.Status()
	if snapshot.ActiveSessionID == "" {
		t.Fatal("expected auto-resume to relaunch a running session")
	}
	if snapshot.ParkedSessionID != "" {
		t.Fatalf("expected parked session cleared after resume, got %q", snapshot.ParkedSessionID)
	}
	if launches != 2 || resumes != 1 {
		t.Fatalf("launches=%d resumes=%d", launches, resumes)
	}
}

func TestSubscriptionAutomationController_EnqueueDueSchedules(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".ralph", "schedules"), 0o755); err != nil {
		t.Fatalf("mkdir schedules: %v", err)
	}

	schedule := cycleSchedule{
		ScheduleID:  "sched-1",
		CronExpr:    "0 * * * *",
		CycleConfig: `{"prompt":"run scheduled cycle","priority":9}`,
		CreatedAt:   "2026-04-06T00:00:00Z",
	}
	data, _ := json.Marshal(schedule)
	if err := os.WriteFile(filepath.Join(repo, ".ralph", "schedules", "sched-1.json"), data, 0o644); err != nil {
		t.Fatalf("write schedule: %v", err)
	}

	ctrl := NewSubscriptionAutomationController(NewManager(), repo)
	now := time.Date(2026, 4, 6, 1, 5, 0, 0, time.UTC)
	ctrl.now = func() time.Time { return now }
	ctrl.policy.Enabled = true
	ctrl.policy.Timezone = "UTC"
	ctrl.state.ScheduleLastEnqueued = map[string]string{}

	ctrl.mu.Lock()
	ctrl.enqueueDueSchedulesLocked(now)
	got := append([]AutomationQueueItem(nil), ctrl.queue...)
	ctrl.mu.Unlock()

	if len(got) != 1 {
		t.Fatalf("queue length = %d, want 1", len(got))
	}
	if got[0].ScheduleID != "sched-1" {
		t.Fatalf("schedule_id = %q", got[0].ScheduleID)
	}
	if got[0].Prompt != "run scheduled cycle" {
		t.Fatalf("prompt = %q", got[0].Prompt)
	}
	if got[0].Priority != 9 {
		t.Fatalf("priority = %d, want 9", got[0].Priority)
	}
}

func TestSubscriptionAutomationController_WriteStatusFilePreservesBudgetStatus(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".ralph"), 0o755); err != nil {
		t.Fatalf("mkdir .ralph: %v", err)
	}

	existing := map[string]any{
		"status":        "idle",
		"window_status": "idle",
		"budget_status": "ok",
	}
	data, _ := json.Marshal(existing)
	if err := os.WriteFile(filepath.Join(repo, ".ralph", "status.json"), data, 0o644); err != nil {
		t.Fatalf("write status.json: %v", err)
	}

	ctrl := NewSubscriptionAutomationController(NewManager(), repo)
	now := time.Date(2026, 4, 6, 1, 5, 0, 0, time.UTC)
	ctrl.now = func() time.Time { return now }

	ctrl.mu.Lock()
	err := ctrl.writeStatusFileLocked(AutomationStatusSnapshot{
		WindowStatus:          "parked",
		CurrentSpendUSD:       3.25,
		QueueDepth:            1,
		TargetUtilizationPct:  95,
		ProjectedSpendAtReset: 4.5,
	})
	ctrl.mu.Unlock()
	if err != nil {
		t.Fatalf("writeStatusFileLocked: %v", err)
	}

	status, err := os.ReadFile(filepath.Join(repo, ".ralph", "status.json"))
	if err != nil {
		t.Fatalf("read status.json: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(status, &got); err != nil {
		t.Fatalf("unmarshal status.json: %v", err)
	}
	if got["status"] != "parked" {
		t.Fatalf("status = %v, want parked", got["status"])
	}
	if got["window_status"] != "parked" {
		t.Fatalf("window_status = %v, want parked", got["window_status"])
	}
	if got["budget_status"] != "ok" {
		t.Fatalf("budget_status = %v, want ok", got["budget_status"])
	}
}
