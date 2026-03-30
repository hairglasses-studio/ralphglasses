package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFleetDashboardModel_Init(t *testing.T) {
	m := NewFleetDashboard()
	cmd := m.Init()
	if cmd != nil {
		t.Fatal("Init should return nil cmd")
	}
}

func TestFleetDashboardModel_View_Empty(t *testing.T) {
	m := NewFleetDashboard()
	view := m.View()

	if !strings.Contains(view, "Fleet Dashboard") {
		t.Fatal("empty view should contain title")
	}
	if !strings.Contains(view, "No sessions") {
		t.Fatal("empty view should show 'No sessions'")
	}
	if !strings.Contains(view, "0 total") {
		t.Fatal("empty view should show 0 total sessions")
	}
}

func TestFleetDashboardModel_View_WithSessions(t *testing.T) {
	m := NewFleetDashboard()
	m.SetSessions([]FleetSession{
		{
			ID:       "sess-abcdef123456",
			Provider: "claude",
			Status:   "running",
			Cost:     1.25,
			Duration: 3*time.Minute + 20*time.Second,
			Task:     "implement feature X",
		},
		{
			ID:       "sess-ghij",
			Provider: "gemini",
			Status:   "idle",
			Cost:     0.50,
			Duration: 10 * time.Second,
			Task:     "review PR",
		},
		{
			ID:       "sess-fail",
			Provider: "codex",
			Status:   "failed",
			Cost:     0.10,
			Duration: 5 * time.Second,
			Task:     "broken task",
		},
	})

	view := m.View()

	// Title present
	if !strings.Contains(view, "Fleet Dashboard") {
		t.Fatal("view should contain title")
	}

	// Stats
	if !strings.Contains(view, "3 total") {
		t.Fatal("view should show 3 total sessions")
	}

	// Truncated ID (8 chars)
	if !strings.Contains(view, "sess-abc") {
		t.Fatal("view should contain truncated session ID 'sess-abc'")
	}
	// Short ID not truncated
	if !strings.Contains(view, "sess-ghi") {
		t.Fatal("view should contain short session ID")
	}

	// Provider names
	if !strings.Contains(view, "claude") {
		t.Fatal("view should contain provider 'claude'")
	}
	if !strings.Contains(view, "gemini") {
		t.Fatal("view should contain provider 'gemini'")
	}

	// Cost
	if !strings.Contains(view, "$1.25") {
		t.Fatalf("view should contain cost '$1.25', got:\n%s", view)
	}

	// Duration formatting
	if !strings.Contains(view, "3m20s") {
		t.Fatal("view should contain formatted duration '3m20s'")
	}

	// Task
	if !strings.Contains(view, "implement feature X") {
		t.Fatal("view should contain task description")
	}

	// Help text
	if !strings.Contains(view, "r:refresh") {
		t.Fatal("view should contain help text")
	}
}

func TestFleetDashboardModel_KeyBindings(t *testing.T) {
	sessions := []FleetSession{
		{ID: "s1", Provider: "claude", Status: "running", Cost: 1.0, Duration: time.Minute},
		{ID: "s2", Provider: "gemini", Status: "idle", Cost: 0.5, Duration: 30 * time.Second},
		{ID: "s3", Provider: "codex", Status: "failed", Cost: 0.1, Duration: 5 * time.Second},
	}

	t.Run("down moves cursor", func(t *testing.T) {
		m := NewFleetDashboard()
		m.SetSessions(sessions)

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.cursor != 1 {
			t.Fatalf("cursor = %d, want 1", m.cursor)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.cursor != 2 {
			t.Fatalf("cursor = %d, want 2", m.cursor)
		}

		// Should not go past last
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		if m.cursor != 2 {
			t.Fatalf("cursor = %d, want 2 (clamped)", m.cursor)
		}
	})

	t.Run("up moves cursor", func(t *testing.T) {
		m := NewFleetDashboard()
		m.SetSessions(sessions)
		m.cursor = 2

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if m.cursor != 1 {
			t.Fatalf("cursor = %d, want 1", m.cursor)
		}

		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0", m.cursor)
		}

		// Should not go below 0
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
		if m.cursor != 0 {
			t.Fatalf("cursor = %d, want 0 (clamped)", m.cursor)
		}
	})

	t.Run("enter selects session", func(t *testing.T) {
		m := NewFleetDashboard()
		m.SetSessions(sessions)
		m.cursor = 1

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("enter should produce a cmd")
		}
		msg := cmd()
		sel, ok := msg.(FleetDashboardSelectMsg)
		if !ok {
			t.Fatalf("expected FleetDashboardSelectMsg, got %T", msg)
		}
		if sel.SessionID != "s2" {
			t.Fatalf("SessionID = %q, want s2", sel.SessionID)
		}
	})

	t.Run("r triggers refresh", func(t *testing.T) {
		m := NewFleetDashboard()
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
		if cmd == nil {
			t.Fatal("r should produce a cmd")
		}
		msg := cmd()
		if _, ok := msg.(FleetDashboardRefreshMsg); !ok {
			t.Fatalf("expected FleetDashboardRefreshMsg, got %T", msg)
		}
	})

	t.Run("q quits", func(t *testing.T) {
		m := NewFleetDashboard()
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		if cmd == nil {
			t.Fatal("q should produce a quit cmd")
		}
	})
}

func TestFleetDashboardModel_SetSessions(t *testing.T) {
	m := NewFleetDashboard()

	// Set initial sessions
	m.SetSessions([]FleetSession{
		{ID: "s1", Provider: "claude", Status: "running"},
		{ID: "s2", Provider: "gemini", Status: "idle"},
		{ID: "s3", Provider: "codex", Status: "failed"},
	})

	view := m.View()
	if !strings.Contains(view, "3 total") {
		t.Fatal("should show 3 total sessions after SetSessions")
	}

	// Cursor should clamp when sessions shrink
	m.cursor = 2
	m.SetSessions([]FleetSession{
		{ID: "s1", Provider: "claude", Status: "running"},
	})
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 after shrink", m.cursor)
	}

	// Empty sessions
	m.SetSessions(nil)
	view = m.View()
	if !strings.Contains(view, "No sessions") {
		t.Fatal("should show 'No sessions' after setting nil")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0 for empty sessions", m.cursor)
	}
}
