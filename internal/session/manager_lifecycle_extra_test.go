package session

import (
	"context"
	"errors"
	"testing"
)

func TestStop_NotFound(t *testing.T) {
	m := NewManager()
	err := m.Stop("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestStop_NotRunning(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:     "completed-sess",
		Status: StatusCompleted,
	})

	err := m.Stop("completed-sess")
	if err == nil {
		t.Error("expected error for completed session")
	}
	if !errors.Is(err, ErrSessionNotRunning) {
		t.Errorf("expected ErrSessionNotRunning, got %v", err)
	}
}

func TestStop_RunningNoProcess(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:     "running-no-proc",
		Status: StatusRunning,
	})

	// Stop a running session with no cmd/process - should mark as stopped
	err := m.Stop("running-no-proc")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got, _ := m.Get("running-no-proc")
	if got.Status != StatusStopped {
		t.Errorf("status = %q, want stopped", got.Status)
	}
}

func TestStop_LaunchingSession(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:     "launching-sess",
		Status: StatusLaunching,
	})

	err := m.Stop("launching-sess")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	got, _ := m.Get("launching-sess")
	if got.Status != StatusStopped {
		t.Errorf("status = %q, want stopped", got.Status)
	}
}

func TestStopAll_Mixed(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:     "running-1",
		Status: StatusRunning,
	})
	m.AddSessionForTesting(&Session{
		ID:     "completed-1",
		Status: StatusCompleted,
	})

	m.StopAll()

	got, _ := m.Get("running-1")
	if got.Status != StatusStopped {
		t.Errorf("status = %q, want stopped", got.Status)
	}

	got2, _ := m.Get("completed-1")
	if got2.Status != StatusCompleted {
		t.Errorf("completed session status = %q, want completed", got2.Status)
	}
}

func TestStopAll_Empty(t *testing.T) {
	m := NewManager()
	// Should not panic
	m.StopAll()
}

func TestMigrateSession_NotFound(t *testing.T) {
	m := NewManager()
	_, err := m.MigrateSession(context.Background(), "nope", ProviderGemini)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMigrateSession_AlreadyOnProvider(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:       "already-claude",
		Status:   StatusRunning,
		Provider: ProviderClaude,
	})

	_, err := m.MigrateSession(context.Background(), "already-claude", ProviderClaude)
	if !errors.Is(err, ErrAlreadyOnProvider) {
		t.Errorf("expected ErrAlreadyOnProvider, got %v", err)
	}
}

func TestMigrateSession_NotRunning(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:       "done-sess",
		Status:   StatusCompleted,
		Provider: ProviderClaude,
	})

	_, err := m.MigrateSession(context.Background(), "done-sess", ProviderGemini)
	if !errors.Is(err, ErrSessionNotRunning) {
		t.Errorf("expected ErrSessionNotRunning, got %v", err)
	}
}

func TestMigrateSession_StoppedSession(t *testing.T) {
	m := NewManager()
	m.AddSessionForTesting(&Session{
		ID:       "stopped-sess",
		Status:   StatusStopped,
		Provider: ProviderClaude,
	})

	_, err := m.MigrateSession(context.Background(), "stopped-sess", ProviderGemini)
	if !errors.Is(err, ErrSessionNotRunning) {
		t.Errorf("expected ErrSessionNotRunning, got %v", err)
	}
}
