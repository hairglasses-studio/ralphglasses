package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// --- handleFleetSessionStart ---

func TestFleetSessionStart_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.SessMgr = nil

	m2, cmd := handleFleetSessionStart(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when SessMgr is nil")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionStart_NoSelectedRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	// FleetView.SessionTable has no rows by default

	m2, cmd := handleFleetSessionStart(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when no row selected")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionStart_RepoNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.FleetView.SessionTable.SetRows([][]string{
		{"sess-1", "nonexistent-repo", "claude", "running"},
	})
	// No repos in model, so findRepoByName returns -1

	m2, cmd := handleFleetSessionStart(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when repo not found")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

// --- handleFleetSessionStop ---

func TestFleetSessionStop_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.SessMgr = nil

	m2, cmd := handleFleetSessionStop(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when SessMgr is nil")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionStop_NoSelectedRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()

	m2, cmd := handleFleetSessionStop(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when no row selected")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionStop_SessionNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.FleetView.SessionTable.SetRows([][]string{
		{"unknown-prefix", "repo1", "claude", "running"},
	})
	// No sessions exist, so findFullSessionID returns ""

	m2, cmd := handleFleetSessionStop(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when session not found")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

// --- handleFleetSessionPause ---

func TestFleetSessionPause_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.SessMgr = nil

	m2, cmd := handleFleetSessionPause(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when SessMgr is nil")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionPause_NoSelectedRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()

	m2, cmd := handleFleetSessionPause(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when no row selected")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionPause_RepoNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.FleetView.SessionTable.SetRows([][]string{
		{"sess-1", "nonexistent", "claude", "running"},
	})

	m2, cmd := handleFleetSessionPause(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when repo not found")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionPause_WithValidRepo(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Repos = []*model.Repo{
		{Name: "myrepo", Path: "/tmp/myrepo"},
	}
	m.FleetView.SessionTable.SetRows([][]string{
		{"sess-1", "myrepo", "claude", "running"},
	})

	// togglePause with no process should still not panic
	m2, _ := handleFleetSessionPause(&m, tea.KeyMsg{})
	_ = asModel(t, m2)
}

// --- handleFleetSessionResume ---

func TestFleetSessionResume_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.SessMgr = nil

	m2, cmd := handleFleetSessionResume(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when SessMgr is nil")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionResume_NoSelectedRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()

	m2, cmd := handleFleetSessionResume(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when no row selected")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionResume_RepoNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.FleetView.SessionTable.SetRows([][]string{
		{"sess-1", "nonexistent", "claude", "running"},
	})

	m2, cmd := handleFleetSessionResume(&m, tea.KeyMsg{})
	got := asModel(t, m2)
	if !got.Notify.Active() {
		t.Error("expected notification when repo not found")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestFleetSessionResume_WithValidRepo(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Repos = []*model.Repo{
		{Name: "myrepo", Path: "/tmp/myrepo"},
	}
	m.FleetView.SessionTable.SetRows([][]string{
		{"sess-1", "myrepo", "claude", "running"},
	})

	m2, _ := handleFleetSessionResume(&m, tea.KeyMsg{})
	_ = asModel(t, m2)
}

// --- FleetActionResultMsg ---

func TestFleetActionResultMsg_Fields(t *testing.T) {
	msg := FleetActionResultMsg{
		Action:    "start",
		SessionID: "abc123",
		RepoName:  "myrepo",
		Err:       nil,
	}
	if msg.Action != "start" {
		t.Errorf("Action = %q, want %q", msg.Action, "start")
	}
	if msg.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", msg.SessionID, "abc123")
	}
}
