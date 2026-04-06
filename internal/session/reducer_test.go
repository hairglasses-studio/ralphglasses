package session

import (
	"testing"
	"time"
)

func TestReduce_Created(t *testing.T) {
	t.Parallel()
	state := SessionState{}
	event := SessionEvent{
		Type:      EventCreated,
		Timestamp: time.Now(),
		SessionID: "test-123",
	}
	next, effects := Reduce(state, event)
	if next.ID != "test-123" {
		t.Errorf("expected ID test-123, got %s", next.ID)
	}
	if next.Status != StatusLaunching {
		t.Errorf("expected launching, got %s", next.Status)
	}
	if next.EventCount != 1 {
		t.Errorf("expected event count 1, got %d", next.EventCount)
	}
	if len(effects) != 1 || effects[0].Type != EffectPersist {
		t.Errorf("expected persist effect, got %v", effects)
	}
}

func TestReduce_StartedSetsRunning(t *testing.T) {
	t.Parallel()
	state := SessionState{Status: StatusLaunching}
	next, effects := Reduce(state, SessionEvent{Type: EventStarted, Timestamp: time.Now()})
	if next.Status != StatusRunning {
		t.Errorf("expected running, got %s", next.Status)
	}
	if next.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}
	hasEmit := false
	for _, e := range effects {
		if e.Type == EffectEmitEvent {
			hasEmit = true
		}
	}
	if !hasEmit {
		t.Error("expected emit effect for session.started")
	}
}

func TestReduce_PauseResume(t *testing.T) {
	t.Parallel()
	state := SessionState{Status: StatusRunning}

	paused, _ := Reduce(state, SessionEvent{Type: EventPaused})
	if paused.Status != "paused" {
		t.Errorf("expected paused, got %s", paused.Status)
	}

	resumed, _ := Reduce(paused, SessionEvent{Type: EventResumed})
	if resumed.Status != StatusRunning {
		t.Errorf("expected running after resume, got %s", resumed.Status)
	}
}

func TestReduce_BudgetExceeded(t *testing.T) {
	t.Parallel()
	state := SessionState{Status: StatusRunning}
	next, effects := Reduce(state, SessionEvent{
		Type:      EventBudgetExceeded,
		Timestamp: time.Now(),
	})
	if next.Status != StatusStopped {
		t.Errorf("expected stopped, got %s", next.Status)
	}
	if next.ExitReason != "budget_exceeded" {
		t.Errorf("expected budget_exceeded exit reason, got %s", next.ExitReason)
	}
	hasKill := false
	for _, e := range effects {
		if e.Type == EffectKill {
			hasKill = true
		}
	}
	if !hasKill {
		t.Error("expected kill effect on budget exceeded")
	}
}

func TestReduce_CostNotifyAt90Percent(t *testing.T) {
	t.Parallel()
	state := SessionState{Status: StatusRunning, BudgetUSD: 10.0}
	next, effects := Reduce(state, SessionEvent{Type: EventCostUpdated, SpentUSD: 9.5})
	if next.SpentUSD != 9.5 {
		t.Errorf("expected 9.5, got %f", next.SpentUSD)
	}
	hasNotify := false
	for _, e := range effects {
		if e.Type == EffectNotify {
			hasNotify = true
		}
	}
	if !hasNotify {
		t.Error("expected notify effect at 90% budget")
	}
}

func TestReduce_CrashEscalates(t *testing.T) {
	t.Parallel()
	state := SessionState{Status: StatusRunning}
	next, effects := Reduce(state, SessionEvent{
		Type:      EventCrashed,
		Error:     "segfault",
		Timestamp: time.Now(),
	})
	if next.Status != StatusErrored {
		t.Errorf("expected errored, got %s", next.Status)
	}
	hasEscalate := false
	for _, e := range effects {
		if e.Type == EffectEscalate {
			hasEscalate = true
		}
	}
	if !hasEscalate {
		t.Error("expected escalate effect on crash")
	}
}

func TestFoldEvents_FullLifecycle(t *testing.T) {
	t.Parallel()
	now := time.Now()
	events := []SessionEvent{
		{Type: EventCreated, Timestamp: now, SessionID: "sess-1"},
		{Type: EventStarted, Timestamp: now.Add(time.Second)},
		{Type: EventTurnCompleted, TurnCount: 1},
		{Type: EventCostUpdated, SpentUSD: 0.05},
		{Type: EventOutputReceived, Output: "done"},
		{Type: EventTurnCompleted, TurnCount: 2},
		{Type: EventCostUpdated, SpentUSD: 0.10},
		{Type: EventCompleted, Timestamp: now.Add(time.Minute), ExitReason: "success"},
	}

	final, effects := FoldEvents(SessionState{}, events)
	if final.ID != "sess-1" {
		t.Errorf("expected sess-1, got %s", final.ID)
	}
	if final.Status != StatusCompleted {
		t.Errorf("expected completed, got %s", final.Status)
	}
	if final.TurnCount != 2 {
		t.Errorf("expected 2 turns, got %d", final.TurnCount)
	}
	if final.SpentUSD != 0.10 {
		t.Errorf("expected 0.10 spent, got %f", final.SpentUSD)
	}
	if final.EventCount != 8 {
		t.Errorf("expected 8 events, got %d", final.EventCount)
	}
	if final.LastOutput != "done" {
		t.Errorf("expected output 'done', got %q", final.LastOutput)
	}
	if len(effects) == 0 {
		t.Error("expected side effects from fold")
	}
}

func TestReduce_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	state := SessionState{Status: StatusRunning, SpentUSD: 1.0}
	original := state
	_, _ = Reduce(state, SessionEvent{Type: EventCostUpdated, SpentUSD: 5.0})

	if state.SpentUSD != original.SpentUSD {
		t.Error("reduce mutated input state")
	}
}

func BenchmarkReduce(b *testing.B) {
	state := SessionState{Status: StatusRunning, BudgetUSD: 100}
	event := SessionEvent{Type: EventCostUpdated, SpentUSD: 50.0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Reduce(state, event)
	}
}
