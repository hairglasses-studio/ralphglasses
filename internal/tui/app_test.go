package tui

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

func TestNewModel(t *testing.T) {
	m := NewModel("/tmp/test")
	if m.ScanPath != "/tmp/test" {
		t.Errorf("ScanPath = %q", m.ScanPath)
	}
	if m.CurrentView != ViewOverview {
		t.Error("should start at overview")
	}
	if m.Table == nil {
		t.Error("Table should not be nil")
	}
	if m.ProcMgr == nil {
		t.Error("ProcMgr should not be nil")
	}
}

func TestInit(t *testing.T) {
	m := NewModel("/tmp/test")
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return a command")
	}
}

func TestViewStackPushPop(t *testing.T) {
	m := NewModel("/tmp/test")
	m.pushView(ViewRepoDetail, "my-repo")
	if m.CurrentView != ViewRepoDetail {
		t.Error("should be at repo detail")
	}
	m.pushView(ViewLogs, "Logs")
	if len(m.ViewStack) != 2 {
		t.Errorf("stack len = %d", len(m.ViewStack))
	}
	m2, _ := m.popView()
	m = m2.(Model)
	if m.CurrentView != ViewRepoDetail {
		t.Error("should be back at repo detail")
	}
	m2, _ = m.popView()
	m = m2.(Model)
	if m.CurrentView != ViewOverview {
		t.Error("should be back at overview")
	}
}

func TestWindowSizeMsg(t *testing.T) {
	m := NewModel("/tmp/test")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(Model)
	if m.Width != 120 || m.Height != 40 {
		t.Errorf("size = %dx%d", m.Width, m.Height)
	}
}

func TestScanResultMsg(t *testing.T) {
	m := NewModel("/tmp/test")
	repos := []*model.Repo{{Name: "repo1", Path: "/tmp/repo1"}, {Name: "repo2", Path: "/tmp/repo2"}}
	m2, _ := m.Update(scanResultMsg{repos: repos})
	m = m2.(Model)
	if len(m.Repos) != 2 {
		t.Errorf("repos = %d, want 2", len(m.Repos))
	}
}

func TestScanResultMsgError(t *testing.T) {
	m := NewModel("/tmp/test")
	m.Update(scanResultMsg{err: fmt.Errorf("scan failed")})
	// Should not crash
}

func TestLogLinesMsg(t *testing.T) {
	m := NewModel("/tmp/test")
	m2, _ := m.Update(process.LogLinesMsg{Lines: []string{"hello", "world"}})
	m = m2.(Model)
	if len(m.LogView.Lines) != 2 {
		t.Errorf("log lines = %d, want 2", len(m.LogView.Lines))
	}
}

func TestHandleKeyQuit(t *testing.T) {
	m := NewModel("/tmp/test")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("q should produce quit command")
	}
}

func TestHandleKeyCommandMode(t *testing.T) {
	m := NewModel("/tmp/test")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = m2.(Model)
	if m.InputMode != ModeCommand {
		t.Errorf("mode = %d, want ModeCommand", m.InputMode)
	}
}

func TestHandleKeyFilterMode(t *testing.T) {
	m := NewModel("/tmp/test")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = m2.(Model)
	if m.InputMode != ModeFilter {
		t.Error("should be filter mode")
	}
}

func TestHandleKeyHelpToggle(t *testing.T) {
	m := NewModel("/tmp/test")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = m2.(Model)
	if m.CurrentView != ViewHelp {
		t.Error("? should push help view")
	}
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = m2.(Model)
	if m.CurrentView != ViewOverview {
		t.Error("? again should pop back")
	}
}

func TestFindRepoByName(t *testing.T) {
	m := NewModel("/tmp/test")
	m.Repos = []*model.Repo{{Name: "alpha", Path: "/tmp/alpha"}, {Name: "beta", Path: "/tmp/beta"}}
	if idx := m.findRepoByName("beta"); idx != 1 {
		t.Errorf("findRepoByName(beta) = %d", idx)
	}
	if idx := m.findRepoByName("nonexistent"); idx != -1 {
		t.Errorf("findRepoByName(nonexistent) = %d", idx)
	}
}

func TestView(t *testing.T) {
	m := NewModel("/tmp/test")
	m.Width = 120
	m.Height = 40
	m.Table.Width = 120
	m.Table.Height = 40
	m.StatusBar.Width = 120
	m.LastRefresh = time.Now()
	if m.View() == "" {
		t.Error("view should not be empty")
	}
}
