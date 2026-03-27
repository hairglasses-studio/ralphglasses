package tui

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

func TestNewModel(t *testing.T) {
	m := NewModel("/tmp/test", nil)
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
	m := NewModel("/tmp/test", nil)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init should return a command")
	}
}

func TestViewStackPushPop(t *testing.T) {
	m := NewModel("/tmp/test", nil)
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
	m := NewModel("/tmp/test", nil)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = m2.(Model)
	if m.Width != 120 || m.Height != 40 {
		t.Errorf("size = %dx%d", m.Width, m.Height)
	}
}

func TestScanResultMsg(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	repos := []*model.Repo{{Name: "repo1", Path: "/tmp/repo1"}, {Name: "repo2", Path: "/tmp/repo2"}}
	m2, _ := m.Update(scanResultMsg{repos: repos})
	m = m2.(Model)
	if len(m.Repos) != 2 {
		t.Errorf("repos = %d, want 2", len(m.Repos))
	}
}

func TestScanResultMsgError(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Update(scanResultMsg{err: fmt.Errorf("scan failed")})
	// Should not crash
}

func TestLogLinesMsg(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.Update(process.LogLinesMsg{Lines: []string{"hello", "world"}})
	m = m2.(Model)
	if len(m.LogView.Lines) != 2 {
		t.Errorf("log lines = %d, want 2", len(m.LogView.Lines))
	}
}

func TestHandleKeyQuit(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Error("q should produce quit command")
	}
}

func TestHandleKeyCommandMode(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = m2.(Model)
	if m.InputMode != ModeCommand {
		t.Errorf("mode = %d, want ModeCommand", m.InputMode)
	}
}

func TestHandleKeyFilterMode(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = m2.(Model)
	if m.InputMode != ModeFilter {
		t.Error("should be filter mode")
	}
}

func TestHandleKeyHelpToggle(t *testing.T) {
	m := NewModel("/tmp/test", nil)
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
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "alpha", Path: "/tmp/alpha"}, {Name: "beta", Path: "/tmp/beta"}}
	if idx := m.findRepoByName("beta"); idx != 1 {
		t.Errorf("findRepoByName(beta) = %d", idx)
	}
	if idx := m.findRepoByName("nonexistent"); idx != -1 {
		t.Errorf("findRepoByName(nonexistent) = %d", idx)
	}
}

func TestView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
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

func TestViewSmallTerminal(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	// Zero size
	m.Width = 0
	m.Height = 0
	view := m.View()
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("zero size should show small terminal message, got: %q", view)
	}

	// Width too small
	m.Width = 2
	m.Height = 40
	view = m.View()
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("narrow terminal should show small terminal message, got: %q", view)
	}

	// Height too small
	m.Width = 120
	m.Height = 2
	view = m.View()
	if !strings.Contains(view, "Terminal too small") {
		t.Errorf("short terminal should show small terminal message, got: %q", view)
	}

	// Just large enough
	m.Width = 3
	m.Height = 3
	view = m.View()
	if strings.Contains(view, "Terminal too small") {
		t.Error("3x3 terminal should render normally")
	}
}

func TestHandleKeyCtrlC(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl+c should produce quit command")
	}
}

func TestHandleKeyRefresh(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Error("r should produce scan command")
	}
}

func TestHandleKeyEscAtRoot(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	// At root, Esc does nothing (no crash)
	if m.CurrentView != ViewOverview {
		t.Error("Esc at root should stay at overview")
	}
}

func TestHandleKeyEscPopsView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.pushView(ViewHelp, "Help")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	if m.CurrentView != ViewOverview {
		t.Error("Esc should pop back to overview")
	}
}

func TestCommandModeInput(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	// Enter command mode
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = m2.(Model)
	if m.InputMode != ModeCommand {
		t.Fatal("should be in command mode")
	}

	// Type characters
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = m2.(Model)
	if m.CommandBuf != "q" {
		t.Errorf("CommandBuf = %q, want 'q'", m.CommandBuf)
	}

	// Backspace
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = m2.(Model)
	if m.CommandBuf != "" {
		t.Errorf("after backspace, CommandBuf = %q", m.CommandBuf)
	}

	// Escape exits command mode
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	if m.InputMode != ModeNormal {
		t.Error("Esc should exit command mode")
	}
}

func TestCommandModeExec(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	// Enter command mode and type "quit"
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(":")})
	m = m2.(Model)
	for _, ch := range "quit" {
		m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = m2.(Model)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error(":quit should produce quit command")
	}
}

func TestFilterModeInput(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	// Enter filter mode
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = m2.(Model)
	if m.InputMode != ModeFilter {
		t.Fatal("should be in filter mode")
	}
	if !m.Filter.Active {
		t.Error("filter should be active")
	}

	// Type characters
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = m2.(Model)
	if m.Filter.Text != "a" {
		t.Errorf("Filter.Text = %q, want 'a'", m.Filter.Text)
	}

	// Enter confirms filter
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	if m.InputMode != ModeNormal {
		t.Error("Enter should exit filter mode")
	}
}

func TestFilterModeEscClears(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = m2.(Model)
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = m2.(Model)
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	if m.Filter.Text != "" {
		t.Error("Esc should clear filter text")
	}
	if m.Filter.Active {
		t.Error("Esc should deactivate filter")
	}
}

func TestViewRepoDetail(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "test", Path: "/tmp/test"}}
	m.Width = 120
	m.Height = 40
	m.SelectedIdx = 0
	m.pushView(ViewRepoDetail, "test")
	view := m.View()
	if view == "" {
		t.Error("detail view should not be empty")
	}
}

func TestViewLogs(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.LogView.Width = 120
	m.LogView.Height = 40
	m.pushView(ViewLogs, "Logs")
	view := m.View()
	if view == "" {
		t.Error("log view should not be empty")
	}
}

func TestViewHelp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.pushView(ViewHelp, "Help")
	view := m.View()
	if view == "" {
		t.Error("help view should not be empty")
	}
}

func TestViewCommandMode(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.InputMode = ModeCommand
	m.CommandBuf = "scan"
	view := m.View()
	if view == "" {
		t.Error("command mode view should not be empty")
	}
}

func TestViewFilterMode(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.InputMode = ModeFilter
	m.Filter.Text = "test"
	view := m.View()
	if view == "" {
		t.Error("filter mode view should not be empty")
	}
}

func TestExecUnknownCommand(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m2, _ := m.execCommand(Command{Name: "bogus"})
	_ = m2 // should not panic
}

func TestExecScanCommand(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	_, cmd := m.execCommand(Command{Name: "scan"})
	if cmd == nil {
		t.Error(":scan should produce a command")
	}
}

func TestExecStopAllCommand(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.execCommand(Command{Name: "stopall"})
	_ = m2 // should not panic
}

func TestTickMsg(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "test", Path: "/tmp/test"}}
	m2, cmd := m.Update(tickMsg(time.Now()))
	m = m2.(Model)
	if cmd == nil {
		t.Error("tick should produce next tick command")
	}
	if m.LastRefresh.IsZero() {
		t.Error("LastRefresh should be set after tick")
	}
}

func TestOverviewKeyJK(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "a", Path: "/tmp/a"}, {Name: "b", Path: "/tmp/b"}}
	m.updateTable()

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_ = m2 // should not panic

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	_ = m2 // should not panic
}

func TestLogViewKeys(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.LogView.SetLines([]string{"line1", "line2", "line3"})
	m.LogView.Height = 10
	m.pushView(ViewLogs, "Logs")

	keys := []string{"j", "k", "G", "g", "f"}
	for _, k := range keys {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		m = m2.(Model)
	}
	// Should not panic
}

func TestLoopPanelToggle(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	// l navigates to the loop list view
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = m2.(Model)
	if m.CurrentView != ViewLoopList {
		t.Errorf("l should navigate to ViewLoopList, got %v", m.CurrentView)
	}

	// Esc from loop list pops back to overview
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	if m.CurrentView != ViewOverview {
		t.Errorf("Esc should return to ViewOverview, got %v", m.CurrentView)
	}
}

func TestLoopPanelViewRender(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.StatusBar.Width = 120
	m.ShowLoopPanel = true
	m.LoopView = "  myrepo  running  iters:3  Fix the bug\n"

	view := m.View()
	if !strings.Contains(view, "Loop Status") {
		t.Error("view should contain 'Loop Status'")
	}
	if !strings.Contains(view, "Fix the bug") {
		t.Error("view should contain task title")
	}
	if !strings.Contains(view, "running") {
		t.Error("view should contain state label")
	}
}

func TestLoopPanelHidden(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.StatusBar.Width = 120
	m.ShowLoopPanel = false
	m.LoopView = "  myrepo  running  iters:3  Some task\n"

	view := m.View()
	if strings.Contains(view, "Loop Status") {
		t.Error("loop panel should not appear when ShowLoopPanel=false")
	}
}

// TestKeyDispatchCoversGlobalBindings uses reflection to enumerate every exported
// key.Binding field of KeyMap that belongs to the global dispatch set and asserts
// that KeyDispatch contains a matching entry for each one.
func TestKeyDispatchCoversGlobalBindings(t *testing.T) {
	km := DefaultKeyMap()
	rt := reflect.TypeOf(km)
	rv := reflect.ValueOf(km)
	bindingType := reflect.TypeOf(key.Binding{})

	// globalFields are the KeyMap fields that were in the original switch/case
	// block in handleKey and must therefore appear in KeyDispatch.
	globalFields := map[string]bool{
		"Quit": true, "CmdMode": true, "FilterMode": true, "Help": true,
		"Escape": true, "Refresh": true, "LoopPanel": true, "LoopControlPanel": true,
		"Tab1": true, "Tab2": true, "Tab3": true, "Tab4": true,
		"LoopListStart": true, "LoopListStop": true, "LoopListPause": true,
	}

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() || field.Type != bindingType || !globalFields[field.Name] {
			continue
		}
		expected := rv.Field(i).Interface().(key.Binding)
		t.Run(field.Name, func(t *testing.T) {
			for _, entry := range KeyDispatch {
				if reflect.DeepEqual(entry.Binding(&km), expected) {
					return
				}
			}
			t.Errorf("KeyDispatch has no entry for KeyMap.%s", field.Name)
		})
	}
}

func TestRefreshLoopViewWithManager(t *testing.T) {
	// Nil SessMgr should show idle message
	m := NewModel("/tmp/test", nil)
	m.refreshLoopView()
	if !strings.Contains(m.LoopView, "No active loops") {
		t.Errorf("nil SessMgr: expected idle message, got: %q", m.LoopView)
	}

	// Real manager: start a loop and verify panel content
	mgr := session.NewManager()
	m2 := NewModel("/tmp/test", mgr)

	dir, err := os.MkdirTemp("", "loop-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	_, err = mgr.StartLoop(context.Background(), dir, session.DefaultLoopProfile())
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	m2.refreshLoopView()
	if !strings.Contains(m2.LoopView, "pending") {
		t.Errorf("expected 'pending' in loop view, got: %q", m2.LoopView)
	}

	// ShowLoopPanel=true and View() renders "Loop Status"
	m2.ShowLoopPanel = true
	m2.Width = 120
	m2.Height = 40
	m2.StatusBar.Width = 120
	view := m2.View()
	if !strings.Contains(view, "Loop Status") {
		t.Error("view should contain 'Loop Status' when ShowLoopPanel=true")
	}
}

func TestLoopListKeyBindings(t *testing.T) {
	mgr := session.NewManager()
	m := NewModel("/tmp/test", mgr)
	m.Width = 120
	m.Height = 40
	m.StatusBar.Width = 120

	// Navigate to loop list view
	m.pushView(ViewLoopList, "Loops")

	// 's' should produce a loopListCmd (non-nil) even with no row selected
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	// cmd may be nil (no selection) or non-nil; no panic is the key requirement
	_ = cmd

	// 'x' should also not panic
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	_ = cmd

	// 'd' should also not panic
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	_ = cmd

	// Verify LoopListStart binding key matches 's'
	km := DefaultKeyMap()
	km.SetViewContext(ViewLoopList)
	if !km.LoopListStart.Enabled() {
		t.Error("LoopListStart should be enabled in ViewLoopList")
	}
	if !km.LoopListStop.Enabled() {
		t.Error("LoopListStop should be enabled in ViewLoopList")
	}

	// Verify bindings are disabled in ViewOverview
	km.SetViewContext(ViewOverview)
	if km.LoopListStart.Enabled() {
		t.Error("LoopListStart should be disabled in ViewOverview")
	}
	if km.LoopListStop.Enabled() {
		t.Error("LoopListStop should be disabled in ViewOverview")
	}

	// Verify LoopListStart and LoopListStop are in the KeyDispatch table
	foundStart, foundStop := false, false
	for _, entry := range KeyDispatch {
		b := entry.Binding(&km)
		keys := b.Keys()
		for _, k := range keys {
			if k == "s" {
				foundStart = true
			}
			if k == "x" || k == "d" {
				foundStop = true
			}
		}
	}
	if !foundStart {
		t.Error("KeyDispatch should contain an entry for LoopListStart ('s')")
	}
	if !foundStop {
		t.Error("KeyDispatch should contain an entry for LoopListStop ('x'/'d')")
	}

	// Verify loop list view shows help footer
	m2 := NewModel("/tmp/test", nil)
	m2.Width = 120
	m2.Height = 40
	m2.StatusBar.Width = 120
	m2.pushView(ViewLoopList, "Loops")
	v := m2.View()
	if !strings.Contains(v, "start loop") {
		t.Error("loop list view should show 'start loop' in footer hints")
	}
	if !strings.Contains(v, "stop loop") {
		t.Error("loop list view should show 'stop loop' in footer hints")
	}
}

func TestLoopListPauseKeyBinding(t *testing.T) {
	mgr := session.NewManager()
	m := NewModel("/tmp/test", mgr)
	m.Width = 120
	m.Height = 40
	m.StatusBar.Width = 120

	// Navigate to loop list view
	m.pushView(ViewLoopList, "Loops")

	// Verify LoopListPause is enabled in ViewLoopList
	km := DefaultKeyMap()
	km.SetViewContext(ViewLoopList)
	if !km.LoopListPause.Enabled() {
		t.Error("LoopListPause should be enabled in ViewLoopList")
	}

	// Verify LoopListPause is disabled in ViewOverview
	km.SetViewContext(ViewOverview)
	if km.LoopListPause.Enabled() {
		t.Error("LoopListPause should be disabled in ViewOverview")
	}

	// Create a loop and select it in the table
	tmpDir := t.TempDir()
	_, err := mgr.StartLoop(context.Background(), tmpDir, session.DefaultLoopProfile())
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// Populate the loop list table
	loops := mgr.ListLoops()
	m.LoopListTable.SetRows(views.LoopRunsToRows(loops, 0))

	// 'p' should not panic even with no selection
	m2 := NewModel("/tmp/test", mgr)
	m2.pushView(ViewLoopList, "Loops")
	_, cmd := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	_ = cmd // no panic is the key requirement

	// With a loop selected, 'p' should trigger pause and show notification
	m.LoopListTable.SetRows(views.LoopRunsToRows(loops, 0))
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = updated.(Model)
	// Handler should produce a loopListCmd (non-nil) and show a notification
	if cmd == nil {
		t.Error("pressing 'p' in loop list should produce a non-nil Cmd")
	}
	if !m.Notify.Active() {
		t.Error("pressing 'p' should trigger a notification (Paused/Resumed)")
	}

	// Verify footer shows pause/resume hint
	m.LoopListTable.SetRows(views.LoopRunsToRows(mgr.ListLoops(), 0))
	v := m.View()
	if !strings.Contains(v, "pause/resume") {
		t.Error("loop list view should show 'pause/resume' in footer hints")
	}

	// Verify LoopListPause is in the KeyDispatch table
	foundPause := false
	km2 := DefaultKeyMap()
	for _, entry := range KeyDispatch {
		b := entry.Binding(&km2)
		for _, k := range b.Keys() {
			if k == "p" {
				foundPause = true
			}
		}
	}
	if !foundPause {
		t.Error("KeyDispatch should contain an entry for LoopListPause ('p')")
	}
}

func TestProcessExitMsg_SetsRepoStatus(t *testing.T) {
	tests := []struct {
		name       string
		exitCode   int
		err        error
		wantStatus string
	}{
		{"non-zero exit sets crashed", 1, fmt.Errorf("signal: killed"), "crashed"},
		{"zero exit sets stopped", 0, nil, "stopped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("/tmp/test", nil)
			m.Repos = []*model.Repo{{Name: "myrepo", Path: "/tmp/myrepo"}}

			m2, _ := m.Update(process.ProcessExitMsg{
				RepoPath: "/tmp/myrepo",
				ExitCode: tt.exitCode,
				Error:    tt.err,
			})
			got := m2.(Model)

			if got.Repos[0].Status == nil {
				t.Fatal("expected Status to be set, got nil")
			}
			if got.Repos[0].Status.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Repos[0].Status.Status, tt.wantStatus)
			}
		})
	}
}

func TestTUICrashNotification(t *testing.T) {
	const repoPath = "/tmp/crash-repo"
	crashErr := fmt.Errorf("signal: killed")
	msg := process.ProcessExitMsg{
		RepoPath: repoPath,
		ExitCode: 1,
		Error:    crashErr,
	}

	// Assert message fields directly — Error being non-nil on the crash
	// branch was a previous failure point.
	if msg.RepoPath != repoPath {
		t.Fatalf("RepoPath = %q, want %q", msg.RepoPath, repoPath)
	}
	if msg.ExitCode == 0 {
		t.Fatal("ExitCode should be non-zero for a crash")
	}
	if msg.Error == nil {
		t.Fatal("Error must be non-nil on the crash branch")
	}

	// Feed the message into the TUI model and verify status transitions to "crashed".
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "crash-repo", Path: repoPath}}

	m2, _ := m.Update(msg)
	got := m2.(Model)

	if got.Repos[0].Status == nil {
		t.Fatal("expected Status to be set, got nil")
	}
	if got.Repos[0].Status.Status != "crashed" {
		t.Errorf("Status = %q, want %q", got.Repos[0].Status.Status, "crashed")
	}
}

func TestLoopDetailKeyBindings(t *testing.T) {
	mgr := session.NewManager()
	m := NewModel("/tmp/test", mgr)
	m.Width = 120
	m.Height = 40
	m.StatusBar.Width = 120

	// Create a loop to interact with
	tmpDir := t.TempDir()
	run, err := mgr.StartLoop(context.Background(), tmpDir, session.DefaultLoopProfile())
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// Navigate to loop detail
	m.SelectedLoop = run.ID
	m.pushView(ViewLoopDetail, "Loop Detail")

	// Verify keybindings are enabled
	km := DefaultKeyMap()
	km.SetViewContext(ViewLoopDetail)
	if !km.LoopDetailStep.Enabled() {
		t.Error("LoopDetailStep should be enabled in ViewLoopDetail")
	}
	if !km.LoopDetailToggle.Enabled() {
		t.Error("LoopDetailToggle should be enabled in ViewLoopDetail")
	}
	if km.Refresh.Enabled() {
		t.Error("Refresh should be disabled in ViewLoopDetail (r is used for toggle)")
	}
	if km.LoopListStart.Enabled() {
		t.Error("LoopListStart should be disabled in ViewLoopDetail")
	}

	// 's' should produce a tea.Cmd (StepLoop) without panicking
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	if cmd == nil {
		t.Error("pressing 's' in loop detail should produce a non-nil Cmd")
	}
	m = updated.(Model)

	// Execute the command and verify it produces LoopStepResultMsg
	msg := cmd()
	if stepMsg, ok := msg.(LoopStepResultMsg); !ok {
		t.Errorf("expected LoopStepResultMsg, got %T", msg)
	} else if stepMsg.LoopID != run.ID {
		t.Errorf("LoopStepResultMsg.LoopID = %q, want %q", stepMsg.LoopID, run.ID)
	}

	// 'r' should produce a tea.Cmd (toggle run/stop)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if cmd == nil {
		t.Error("pressing 'r' in loop detail should produce a non-nil Cmd")
	}
	// Execute and verify it produces LoopToggleResultMsg
	msg = cmd()
	if toggleMsg, ok := msg.(LoopToggleResultMsg); !ok {
		t.Errorf("expected LoopToggleResultMsg, got %T", msg)
	} else if toggleMsg.LoopID != run.ID {
		t.Errorf("LoopToggleResultMsg.LoopID = %q, want %q", toggleMsg.LoopID, run.ID)
	}

	// Verify loop detail view renders with keybinding help
	v := m.View()
	if !strings.Contains(v, "step") {
		t.Error("loop detail view should show 'step' in footer hints")
	}
	if !strings.Contains(v, "run/stop") {
		t.Error("loop detail view should show 'run/stop' in footer hints")
	}

	// 'p' should produce a tea.Cmd (pause/resume)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if cmd == nil {
		t.Error("pressing 'p' in loop detail should produce a non-nil Cmd")
	}
	// Execute and verify it produces LoopPauseResultMsg
	msg = cmd()
	if pauseMsg, ok := msg.(LoopPauseResultMsg); !ok {
		t.Errorf("expected LoopPauseResultMsg, got %T", msg)
	} else if pauseMsg.LoopID != run.ID {
		t.Errorf("LoopPauseResultMsg.LoopID = %q, want %q", pauseMsg.LoopID, run.ID)
	} else if !pauseMsg.Paused {
		t.Error("first pause toggle should set Paused=true")
	}

	// Verify LoopDetailPause is enabled in ViewLoopDetail
	if !km.LoopDetailPause.Enabled() {
		t.Error("LoopDetailPause should be enabled in ViewLoopDetail")
	}

	// Verify LoopDetailPause is disabled in ViewOverview
	km2 := DefaultKeyMap()
	km2.SetViewContext(ViewOverview)
	if km2.LoopDetailPause.Enabled() {
		t.Error("LoopDetailPause should be disabled in ViewOverview")
	}

	// Verify LoopStepResultMsg is handled correctly
	m2, _ := m.Update(LoopStepResultMsg{LoopID: run.ID})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("LoopStepResultMsg should trigger a notification")
	}

	// Verify LoopToggleResultMsg is handled correctly
	m3, _ := m.Update(LoopToggleResultMsg{LoopID: run.ID, Started: false})
	got3 := m3.(Model)
	if !got3.Notify.Active() {
		t.Error("LoopToggleResultMsg should trigger a notification")
	}

	// Verify LoopPauseResultMsg is handled correctly
	m4, _ := m.Update(LoopPauseResultMsg{LoopID: run.ID, Paused: true})
	got4 := m4.(Model)
	if !got4.Notify.Active() {
		t.Error("LoopPauseResultMsg should trigger a notification")
	}

	// Verify loop detail view includes pause/resume hint
	if !strings.Contains(v, "pause/resume") {
		t.Error("loop detail view should show 'pause/resume' in footer hints")
	}
}

// --- updateTable ---

func TestUpdateTableBasic(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha", Status: &model.LoopStatus{Status: "running"}},
		{Name: "beta", Path: "/tmp/beta"},
	}
	m.updateTable()
	if m.StatusBar.RepoCount != 2 {
		t.Errorf("RepoCount = %d, want 2", m.StatusBar.RepoCount)
	}
}

func TestUpdateTableWithSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	mgr := session.NewManager()
	s1 := &session.Session{
		ID: "s1", Provider: "claude", Status: session.StatusRunning,
		SpentUSD: 2.0, BudgetUSD: 10.0, LaunchedAt: time.Now(),
	}
	s2 := &session.Session{
		ID: "s2", Provider: "gemini", Status: session.StatusCompleted,
		SpentUSD: 1.0, BudgetUSD: 5.0, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s1)
	mgr.AddSessionForTesting(s2)
	m.SessMgr = mgr
	m.Repos = []*model.Repo{
		{Name: "r", Path: "/tmp/r", Circuit: &model.CircuitBreakerState{State: "OPEN"}},
	}
	m.updateTable()
	if m.StatusBar.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", m.StatusBar.SessionCount)
	}
	if m.StatusBar.TotalSpendUSD != 3.0 {
		t.Errorf("TotalSpendUSD = %.2f, want 3.0", m.StatusBar.TotalSpendUSD)
	}
	if m.StatusBar.FleetBudgetPct == 0 {
		t.Error("FleetBudgetPct should be non-zero")
	}
	if m.StatusBar.AlertCount != 1 {
		t.Errorf("AlertCount = %d, want 1", m.StatusBar.AlertCount)
	}
	if m.StatusBar.HighestAlertSeverity != "critical" {
		t.Errorf("HighestAlertSeverity = %q, want critical", m.StatusBar.HighestAlertSeverity)
	}
}

func TestUpdateTableNoBudget(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	mgr := session.NewManager()
	s := &session.Session{
		ID: "s1", Provider: "claude", Status: session.StatusRunning,
		SpentUSD: 1.0, BudgetUSD: 0, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	m.updateTable()
	if m.StatusBar.FleetBudgetPct != 0 {
		t.Errorf("FleetBudgetPct = %.2f, want 0 when no budget", m.StatusBar.FleetBudgetPct)
	}
}

func TestUpdateTableAlertSeverityInfo(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	mgr := session.NewManager()
	s := &session.Session{
		ID: "s1", Provider: "claude", Status: session.StatusErrored, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	m.updateTable()
	if m.StatusBar.HighestAlertSeverity != "info" {
		t.Errorf("HighestAlertSeverity = %q, want info (no open circuits)", m.StatusBar.HighestAlertSeverity)
	}
}

// --- updateSessionTable ---

func TestUpdateSessionTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s := &session.Session{
		ID: "s1", Provider: "claude", RepoName: "repo",
		Status: session.StatusRunning, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	m.updateSessionTable()
	// Should not panic and rows should be set
}

func TestUpdateSessionTableNilManager(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	m.updateSessionTable() // should not panic
}

// --- updateTeamTable ---

func TestUpdateTeamTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	mgr.AddTeamForTesting(&session.TeamStatus{
		Name: "team-1", RepoPath: "/tmp/repo", Status: session.StatusRunning,
	})
	m.SessMgr = mgr
	m.updateTeamTable()
	// Should not panic
}

func TestUpdateTeamTableNilManager(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	m.updateTeamTable() // should not panic
}

// --- findFullSessionID ---

func TestFindFullSessionIDNilManager(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	if got := m.findFullSessionID("abc"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestFindFullSessionIDFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	mgr.AddSessionForTesting(&session.Session{
		ID: "session-12345678-abcd", Provider: "claude", Status: session.StatusRunning,
		LaunchedAt: time.Now(),
	})
	m.SessMgr = mgr
	got := m.findFullSessionID("session-1")
	if got != "session-12345678-abcd" {
		t.Errorf("findFullSessionID = %q, want session-12345678-abcd", got)
	}
}

func TestFindFullSessionIDNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	mgr.AddSessionForTesting(&session.Session{
		ID: "session-12345678", Provider: "claude", Status: session.StatusRunning,
		LaunchedAt: time.Now(),
	})
	m.SessMgr = mgr
	got := m.findFullSessionID("zzzzz")
	if got != "" {
		t.Errorf("expected empty for no match, got %q", got)
	}
}

// --- View rendering branches ---

func TestViewTerminalTooSmall(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 2
	m.Height = 2
	got := m.View()
	if !strings.Contains(got, "too small") {
		t.Error("expected 'too small' message")
	}
}

func TestViewOverview(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewOverview
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewSessions
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewTeams(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewTeams
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewFleet(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewFleet
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewTimeline(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewTimeline
	m.SelectedIdx = -1
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewRepoDetailValid(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Repos = []*model.Repo{{Name: "alpha", Path: "/tmp/alpha"}}
	m.SelectedIdx = 0
	m.CurrentView = ViewRepoDetail
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewRepoDetailWithGate(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Repos = []*model.Repo{{Name: "alpha", Path: "/tmp/alpha"}}
	m.SelectedIdx = 0
	m.CurrentView = ViewRepoDetail
	m.GateCache = map[string]*GateCacheEntry{
		"/tmp/alpha": {
			Report: &e2e.GateReport{Overall: e2e.VerdictPass},
		},
	}
	m.ObsCache = map[string][]session.LoopObservation{
		"/tmp/alpha": {{TotalCostUSD: 1.0}},
	}
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewSessionDetailNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewSessionDetail
	mgr := session.NewManager()
	m.SessMgr = mgr
	m.SelectedSession = "nonexistent"
	got := m.View()
	if !strings.Contains(got, "not found") {
		t.Error("expected 'not found' for missing session")
	}
}

func TestViewTeamDetailNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewTeamDetail
	mgr := session.NewManager()
	m.SessMgr = mgr
	m.SelectedTeam = "nonexistent"
	got := m.View()
	if !strings.Contains(got, "not found") {
		t.Error("expected 'not found' for missing team")
	}
}

func TestViewLoopList(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.CurrentView = ViewLoopList
	got := m.View()
	if !strings.Contains(got, "pause/resume") {
		t.Error("expected loop list footer hints")
	}
}

func TestViewDiff(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Repos = []*model.Repo{{Name: "r", Path: "/tmp/test"}}
	m.SelectedIdx = 0
	m.CurrentView = ViewDiff
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

func TestViewLoopHealth(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Repos = []*model.Repo{{Name: "r", Path: "/tmp/r"}}
	m.SelectedIdx = 0
	m.CurrentView = ViewLoopHealth
	got := m.View()
	if got == "" {
		t.Error("View should not be empty")
	}
}

// --- activeTable ---

func TestActiveTableSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	tbl := m.activeTable()
	if tbl != m.SessionTable {
		t.Error("expected SessionTable for ViewSessions")
	}
}

func TestActiveTableTeams(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeams
	tbl := m.activeTable()
	if tbl != m.TeamTable {
		t.Error("expected TeamTable for ViewTeams")
	}
}

func TestActiveTableLoopList(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewLoopList
	tbl := m.activeTable()
	if tbl != m.LoopListTable {
		t.Error("expected LoopListTable for ViewLoopList")
	}
}

func TestActiveTableDefault(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewHelp // not a table view
	tbl := m.activeTable()
	if tbl != m.Table {
		t.Error("expected default Table for non-table views")
	}
}
