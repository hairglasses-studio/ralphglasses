package tui

import (
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// newTestModel creates a Model with deterministic state for golden file tests.
func newTestModel(t *testing.T) Model {
	t.Helper()
	m := NewModel(t.TempDir(), nil)
	m.Width = 120
	m.Height = 40
	m.LastRefresh = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return m
}

// newTestModelWithRepos creates a test model pre-loaded with mock repos.
func newTestModelWithRepos(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m.Repos = []*model.Repo{
		{Name: "ralphglasses", Path: "/tmp/ralphglasses", Status: &model.LoopStatus{Status: "running"}},
		{Name: "mcpkit", Path: "/tmp/mcpkit", Status: &model.LoopStatus{Status: "idle"}},
		{Name: "hg-mcp", Path: "/tmp/hg-mcp", Status: &model.LoopStatus{Status: "completed"}},
	}
	m.updateTable()
	return m
}

// --- Golden file snapshot tests ---

func TestTeatest_OverviewEmpty(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

func TestTeatest_OverviewWithRepos(t *testing.T) {
	m := newTestModelWithRepos(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

func TestTeatest_HelpView(t *testing.T) {
	m := newTestModel(t)
	m.Nav.CurrentView = ViewHelp
	m.Nav.Breadcrumb.Push("Help")
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

func TestTeatest_SmallTerminal(t *testing.T) {
	m := NewModel(t.TempDir(), nil)
	m.Width = 2
	m.Height = 2
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(2, 2))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	out, err := io.ReadAll(tm.FinalOutput(t, teatest.WithFinalTimeout(3*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	teatest.RequireEqualOutput(t, out)
}

// --- Interactive flow tests ---

func TestTeatest_NavigateToHelp(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Press ? to open help, then quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	// Verify final model is in help view
	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := final.(Model)
	if fm.Nav.CurrentView != ViewHelp {
		t.Errorf("expected ViewHelp, got %d", fm.Nav.CurrentView)
	}
}

func TestTeatest_TabSwitching(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Switch to Sessions (2), then Teams (3), then quit
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := final.(Model)
	if fm.Nav.ActiveTab != 2 {
		t.Errorf("expected tab 2 (Teams), got %d", fm.Nav.ActiveTab)
	}
}

func TestTeatest_WindowResize(t *testing.T) {
	m := newTestModel(t)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(120, 40))

	// Resize terminal, then quit
	tm.Send(tea.WindowSizeMsg{Width: 200, Height: 60})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})

	final := tm.FinalModel(t, teatest.WithFinalTimeout(3*time.Second))
	fm := final.(Model)
	if fm.Width != 200 || fm.Height != 60 {
		t.Errorf("expected 200x60, got %dx%d", fm.Width, fm.Height)
	}
}
