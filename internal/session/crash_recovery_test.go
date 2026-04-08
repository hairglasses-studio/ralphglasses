package session

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func requireCrashRecoverySessionError(t *testing.T, bus *events.Bus, component string) events.Event {
	t.Helper()

	for _, event := range bus.History(events.SessionError, 20) {
		if event.Data["component"] == component {
			return event
		}
	}

	t.Fatalf("expected SessionError event for component %q", component)
	return events.Event{}
}

func TestCrashRecovery_ExecuteRecoveryBudgetExhaustedPublishesEvent(t *testing.T) {
	mgr := NewManager()
	bus := events.NewBus(20)
	orchestrator := NewCrashRecoveryOrchestrator(mgr, bus, nil)
	orchestrator.SetBudget(NewRecoveryBudgetEnvelope(0, 0))

	plan := &CrashRecoveryPlan{
		SessionsToResume: []RecoverableSession{{
			SessionID: "claude-1",
			RepoPath:  "/tmp/repo",
			RepoName:  "repo",
		}},
	}

	if err := orchestrator.ExecuteRecovery(context.Background(), plan, 1); err != nil {
		t.Fatalf("ExecuteRecovery: %v", err)
	}

	event := requireCrashRecoverySessionError(t, bus, "crash_recovery.execute")
	if got := event.Data["reason"]; got != "budget_exhausted" {
		t.Fatalf("reason = %v, want budget_exhausted", got)
	}
}

func TestCrashRecovery_ExecuteRecoveryResumeFailurePublishesEvent(t *testing.T) {
	mgr := NewManager()
	mgr.FleetPool.SetBudgetCap(0.01)
	bus := events.NewBus(20)
	orchestrator := NewCrashRecoveryOrchestrator(mgr, bus, nil)

	plan := &CrashRecoveryPlan{
		SessionsToResume: []RecoverableSession{{
			SessionID: "claude-2",
			RepoPath:  "/tmp/repo",
			RepoName:  "repo",
		}},
	}

	if err := orchestrator.ExecuteRecovery(context.Background(), plan, 1); err == nil {
		t.Fatal("expected ExecuteRecovery error")
	}

	event := requireCrashRecoverySessionError(t, bus, "crash_recovery.resume_session")
	if got := event.Data["reason"]; got != "resume_failed" {
		t.Fatalf("reason = %v, want resume_failed", got)
	}
	if got := event.Data["session_id"]; got != "claude-2" {
		t.Fatalf("session_id = %v, want claude-2", got)
	}
}
