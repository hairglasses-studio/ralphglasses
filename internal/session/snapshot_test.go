package session

import (
	"testing"
	"time"
)

func TestSnapshotSaveLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	now := time.Now()
	events := []SessionEvent{
		{Type: EventCreated, Timestamp: now, SessionID: "snap-1"},
		{Type: EventStarted, Timestamp: now.Add(time.Second)},
		{Type: EventCostUpdated, SpentUSD: 0.25},
		{Type: EventTurnCompleted, TurnCount: 3},
	}
	state, _ := FoldEvents(SessionState{}, events)

	snap := NewSnapshot("snap-1", state, events)
	snap.Iteration = 5
	snap.IterationPhase = "execute"
	snap.RecentErrors = []LoopError{
		{Iteration: 4, Phase: "plan", Message: "timeout"},
	}

	if err := SaveSnapshot(dir, snap); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadSnapshot(dir, "snap-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.SessionID != "snap-1" {
		t.Errorf("expected snap-1, got %s", loaded.SessionID)
	}
	if loaded.State.TurnCount != 3 {
		t.Errorf("expected 3 turns, got %d", loaded.State.TurnCount)
	}
	if loaded.Iteration != 5 {
		t.Errorf("expected iteration 5, got %d", loaded.Iteration)
	}
	if loaded.IterationPhase != "execute" {
		t.Errorf("expected 'execute', got %s", loaded.IterationPhase)
	}
	if len(loaded.Events) != 4 {
		t.Errorf("expected 4 events, got %d", len(loaded.Events))
	}
	if len(loaded.RecentErrors) != 1 {
		t.Errorf("expected 1 error, got %d", len(loaded.RecentErrors))
	}
}

func TestSnapshotNotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := LoadSnapshot(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for missing snapshot")
	}
}

func TestSnapshotDelete(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	snap := NewSnapshot("del-1", SessionState{}, nil)
	if err := SaveSnapshot(dir, snap); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := DeleteSnapshot(dir, "del-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := LoadSnapshot(dir, "del-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestSnapshotDeleteNonexistent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := DeleteSnapshot(dir, "ghost"); err != nil {
		t.Errorf("delete nonexistent should not error: %v", err)
	}
}

func TestResumeFromSnapshot(t *testing.T) {
	t.Parallel()
	now := time.Now()
	events := []SessionEvent{
		{Type: EventCreated, Timestamp: now, SessionID: "resume-1"},
		{Type: EventStarted, Timestamp: now.Add(time.Second)},
		{Type: EventCostUpdated, SpentUSD: 1.50},
		{Type: EventTurnCompleted, TurnCount: 10},
		{Type: EventPaused},
	}
	state, _ := FoldEvents(SessionState{}, events)
	snap := &SessionSnapshot{
		Version:   snapshotVersion,
		SessionID: "resume-1",
		State:     state,
		Events:    events,
	}

	resumed, effects := ResumeFromSnapshot(snap)
	if resumed.Status != "paused" {
		t.Errorf("expected paused, got %s", resumed.Status)
	}
	if resumed.SpentUSD != 1.50 {
		t.Errorf("expected 1.50, got %f", resumed.SpentUSD)
	}
	if resumed.TurnCount != 10 {
		t.Errorf("expected 10, got %d", resumed.TurnCount)
	}
	if len(effects) == 0 {
		t.Error("expected side effects from resume fold")
	}
}

func TestSnapshotAtomicWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Save twice to verify overwrite works.
	snap1 := NewSnapshot("atomic-1", SessionState{SpentUSD: 1.0}, nil)
	if err := SaveSnapshot(dir, snap1); err != nil {
		t.Fatalf("first save: %v", err)
	}
	snap2 := NewSnapshot("atomic-1", SessionState{SpentUSD: 2.0}, nil)
	if err := SaveSnapshot(dir, snap2); err != nil {
		t.Fatalf("second save: %v", err)
	}

	loaded, err := LoadSnapshot(dir, "atomic-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.State.SpentUSD != 2.0 {
		t.Errorf("expected 2.0 after overwrite, got %f", loaded.State.SpentUSD)
	}
}
