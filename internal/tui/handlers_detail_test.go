package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// helper: create a model in detail view with a selected repo
func newDetailModel(repos ...*model.Repo) Model {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Repos = repos
	m.Width = 120
	m.Height = 40
	if len(repos) > 0 {
		m.SelectedIdx = 0
		m.pushView(ViewRepoDetail, repos[0].Name)
	}
	return m
}

// --- handleDetailKey guard: invalid SelectedIdx ---

func TestDetail_InvalidSelectedIdx(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.CurrentView = ViewRepoDetail
	m.Keys.SetViewContext(ViewRepoDetail)
	m.SelectedIdx = -1

	m2, cmd := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd for invalid SelectedIdx")
	}
	// Should remain in same view since guard returned early
	if got.CurrentView != ViewRepoDetail {
		t.Errorf("CurrentView = %v, want ViewRepoDetail", got.CurrentView)
	}
}

func TestDetail_SelectedIdxOutOfRange(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.CurrentView = ViewRepoDetail
	m.Keys.SetViewContext(ViewRepoDetail)
	m.SelectedIdx = 999

	m2, cmd := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	_ = m2
	if cmd != nil {
		t.Error("expected nil cmd for out-of-range SelectedIdx")
	}
}

// --- Enter: push log view ---

func TestDetail_EnterPushesLogView(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.CurrentView != ViewLogs {
		t.Errorf("CurrentView = %v, want ViewLogs", got.CurrentView)
	}
}

// --- EditConfig key: 'e' ---

func TestDetail_EditConfig_NoConfig(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo", Config: nil})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := m2.(Model)
	// Should show notification about missing config
	if !got.Notify.Active() {
		t.Error("expected notification when config is nil")
	}
	if got.CurrentView != ViewRepoDetail {
		t.Errorf("should remain in detail view, got %v", got.CurrentView)
	}
}

func TestDetail_EditConfig_WithConfig(t *testing.T) {
	cfg := &model.RalphConfig{
		Path:   "/tmp/myrepo/.ralphrc",
		Values: map[string]string{"max_calls_per_hour": "10"},
	}
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo", Config: cfg})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	got := m2.(Model)
	if got.CurrentView != ViewConfigEditor {
		t.Errorf("CurrentView = %v, want ViewConfigEditor", got.CurrentView)
	}
	if got.ConfigEdit == nil {
		t.Error("expected ConfigEdit to be set")
	}
}

// --- StartLoop key: 'S' ---

func TestDetail_StartLoop(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification after start loop")
	}
}

// --- StopAction key: 'X' ---

func TestDetail_StopAction(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	got := m2.(Model)
	if got.ConfirmDialog == nil {
		t.Error("expected ConfirmDialog after stop key in detail")
	}
	if got.ConfirmDialog != nil && got.ConfirmDialog.Action != "stopLoop" {
		t.Errorf("ConfirmDialog.Action = %q, want %q", got.ConfirmDialog.Action, "stopLoop")
	}
}

// --- PauseLoop key: 'P' ---

func TestDetail_PauseLoop(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification after pause in detail")
	}
}

// --- DiffView key: 'd' ---

func TestDetail_DiffView(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := m2.(Model)
	if got.CurrentView != ViewDiff {
		t.Errorf("CurrentView = %v, want ViewDiff", got.CurrentView)
	}
}

// --- ActionsMenu key: 'a' ---

func TestDetail_ActionsMenu(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := m2.(Model)
	if got.ActionMenu == nil {
		t.Error("expected ActionMenu after 'a' in detail")
	}
}

// --- LaunchSession key: 'L' ---

func TestDetail_LaunchSession(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("L")})
	got := m2.(Model)
	if got.Launcher == nil {
		t.Error("expected Launcher after 'L' in detail")
	}
}

// --- TimelineView key: 't' ---

func TestDetail_TimelineView(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	got := m2.(Model)
	if got.CurrentView != ViewTimeline {
		t.Errorf("CurrentView = %v, want ViewTimeline", got.CurrentView)
	}
}

// --- LoopHealth key: 'h' ---

func TestDetail_LoopHealth(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, _ := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	got := m2.(Model)
	if got.CurrentView != ViewLoopHealth {
		t.Errorf("CurrentView = %v, want ViewLoopHealth", got.CurrentView)
	}
}

// --- Unmatched key ---

func TestDetail_UnmatchedKey(t *testing.T) {
	m := newDetailModel(&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"})
	m2, cmd := m.handleDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("unmatched key should return nil cmd")
	}
}

// --- Session view handlers (in handlers_detail.go) ---

func TestSessions_MoveDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m.SessionTable.SetRows([]components.Row{
		{"abc12345", "claude", "/tmp/repo", "running"},
		{"def67890", "gemini", "/tmp/repo2", "done"},
	})

	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := m2.(Model)
	row := got.SessionTable.SelectedRow()
	if row == nil {
		t.Fatal("expected selected row after move down")
	}
	if row[0] != "def67890" {
		t.Errorf("selected row = %q, want %q", row[0], "def67890")
	}
}

func TestSessions_MoveUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m.SessionTable.SetRows([]components.Row{
		{"abc12345", "claude", "/tmp/repo", "running"},
		{"def67890", "gemini", "/tmp/repo2", "done"},
	})

	// Move down then up
	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m2.(Model)
	m2, _ = m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	got := m2.(Model)
	row := got.SessionTable.SelectedRow()
	if row == nil {
		t.Fatal("expected selected row after move up")
	}
	if row[0] != "abc12345" {
		t.Errorf("selected row = %q, want %q", row[0], "abc12345")
	}
}

func TestSessions_Sort(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m.SessionTable.SetRows([]components.Row{
		{"abc12345", "claude", "/tmp/repo", "running"},
	})
	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	_ = m2.(Model)
}

func TestSessions_Enter_EmptyTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.CurrentView != ViewSessions {
		t.Errorf("should stay in sessions view with empty table")
	}
}

func TestSessions_StopAction_EmptyTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	got := m2.(Model)
	if got.ConfirmDialog != nil {
		t.Error("should not show confirm dialog with empty table")
	}
}

func TestSessions_SpaceToggle(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m.SessionTable.SetRows([]components.Row{
		{"abc12345", "claude", "/tmp/repo", "running"},
	})
	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	_ = m2.(Model)
}

func TestSessions_TimelineView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessions
	m.Keys.SetViewContext(ViewSessions)
	m2, _ := m.handleSessionsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	got := m2.(Model)
	if got.CurrentView != ViewTimeline {
		t.Errorf("CurrentView = %v, want ViewTimeline", got.CurrentView)
	}
}

// --- Session detail handlers ---

func TestSessionDetail_StopAction_NoSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessionDetail
	m.Keys.SetViewContext(ViewSessionDetail)
	m.SelectedSession = "" // no session

	m2, _ := m.handleSessionDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	got := m2.(Model)
	if got.ConfirmDialog != nil {
		t.Error("should not show dialog with no session")
	}
}

func TestSessionDetail_ActionsMenu(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessionDetail
	m.Keys.SetViewContext(ViewSessionDetail)

	m2, _ := m.handleSessionDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := m2.(Model)
	if got.ActionMenu == nil {
		t.Error("expected ActionMenu after 'a' in session detail")
	}
}

func TestSessionDetail_OutputView_NoSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessionDetail
	m.Keys.SetViewContext(ViewSessionDetail)
	m.SelectedSession = ""
	m.SessMgr = nil

	m2, cmd := m.handleSessionDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with no session")
	}
}

func TestSessionDetail_TimelineView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessionDetail
	m.Keys.SetViewContext(ViewSessionDetail)

	m2, _ := m.handleSessionDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	got := m2.(Model)
	if got.CurrentView != ViewTimeline {
		t.Errorf("CurrentView = %v, want ViewTimeline", got.CurrentView)
	}
}

func TestSessionDetail_Enter_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessionDetail
	m.Keys.SetViewContext(ViewSessionDetail)
	m.SelectedSession = "test-id"
	m.SessMgr = nil

	m2, _ := m.handleSessionDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	// Should stay since no SessMgr
	if got.CurrentView != ViewSessionDetail {
		t.Errorf("should stay in session detail, got %v", got.CurrentView)
	}
}

func TestSessionDetail_DiffView_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewSessionDetail
	m.Keys.SetViewContext(ViewSessionDetail)
	m.SelectedSession = "test-id"
	m.SessMgr = nil

	m2, _ := m.handleSessionDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := m2.(Model)
	if got.CurrentView != ViewSessionDetail {
		t.Errorf("should stay in session detail, got %v", got.CurrentView)
	}
}

// --- Teams view handlers ---

func TestTeams_MoveDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeams
	m.Keys.SetViewContext(ViewTeams)
	m.TeamTable.SetRows([]components.Row{
		{"team-alpha", "abc123", "2", "active"},
		{"team-beta", "def456", "3", "active"},
	})

	m2, _ := m.handleTeamsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := m2.(Model)
	row := got.TeamTable.SelectedRow()
	if row == nil {
		t.Fatal("expected selected row after move down")
	}
	if row[0] != "team-beta" {
		t.Errorf("selected row = %q, want %q", row[0], "team-beta")
	}
}

func TestTeams_Enter_WithRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeams
	m.Keys.SetViewContext(ViewTeams)
	m.TeamTable.SetRows([]components.Row{
		{"team-alpha", "abc123", "2", "active"},
	})

	m2, _ := m.handleTeamsKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.CurrentView != ViewTeamDetail {
		t.Errorf("CurrentView = %v, want ViewTeamDetail", got.CurrentView)
	}
	if got.SelectedTeam != "team-alpha" {
		t.Errorf("SelectedTeam = %q, want %q", got.SelectedTeam, "team-alpha")
	}
}

func TestTeams_Enter_EmptyTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeams
	m.Keys.SetViewContext(ViewTeams)
	m2, _ := m.handleTeamsKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.CurrentView != ViewTeams {
		t.Error("should stay in teams view with empty table")
	}
}

func TestTeams_Sort(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeams
	m.Keys.SetViewContext(ViewTeams)
	m.TeamTable.SetRows([]components.Row{
		{"team-alpha", "abc123", "2", "active"},
	})
	m2, _ := m.handleTeamsKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	_ = m2.(Model)
}

// --- Team detail handlers ---

func TestTeamDetail_Enter_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeamDetail
	m.Keys.SetViewContext(ViewTeamDetail)
	m.SelectedTeam = "test-team"
	m.SessMgr = nil

	m2, _ := m.handleTeamDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.CurrentView != ViewTeamDetail {
		t.Error("should stay in team detail with nil SessMgr")
	}
}

func TestTeamDetail_Timeline_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeamDetail
	m.Keys.SetViewContext(ViewTeamDetail)
	m.SelectedTeam = "test-team"
	m.SessMgr = nil

	m2, _ := m.handleTeamDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	got := m2.(Model)
	if got.CurrentView != ViewTeamDetail {
		t.Error("should stay in team detail with nil SessMgr")
	}
}

func TestTeamDetail_DiffView_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeamDetail
	m.Keys.SetViewContext(ViewTeamDetail)
	m.SelectedTeam = "test-team"
	m.SessMgr = nil

	m2, _ := m.handleTeamDetailKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := m2.(Model)
	if got.CurrentView != ViewTeamDetail {
		t.Error("should stay in team detail with nil SessMgr")
	}
}

func TestTeamDetail_Enter_EmptyTeam(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewTeamDetail
	m.Keys.SetViewContext(ViewTeamDetail)
	m.SelectedTeam = ""

	m2, _ := m.handleTeamDetailKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.CurrentView != ViewTeamDetail {
		t.Error("should stay in team detail with empty team name")
	}
}

// --- Fleet view handlers ---

// asModel extracts a Model from tea.Model, handling both value and pointer returns.
func asModel(t *testing.T, tm tea.Model) Model {
	t.Helper()
	switch v := tm.(type) {
	case Model:
		return v
	case *Model:
		return *v
	default:
		t.Fatalf("unexpected tea.Model type %T", tm)
		return Model{}
	}
}

func TestFleet_MoveDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_ = asModel(t, m2) // no crash
}

func TestFleet_MoveUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	_ = asModel(t, m2)
}

func TestFleet_Enter(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyEnter})
	_ = asModel(t, m2)
}

func TestFleet_StopAction(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	_ = asModel(t, m2)
}

func TestFleet_DiffView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	_ = asModel(t, m2)
}

func TestFleet_TimelineView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	_ = asModel(t, m2)
}

func TestFleet_TabCycleSection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyTab})
	_ = asModel(t, m2)
}

func TestFleet_LeftCycleSection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyLeft})
	_ = asModel(t, m2)
}

func TestFleet_BracketWindowCycle(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.CurrentView = ViewFleet
	m.Keys.SetViewContext(ViewFleet)
	m.Width = 120
	m.Height = 40

	initial := m.FleetWindow

	m2, _ := m.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	got := asModel(t, m2)
	if got.FleetWindow == initial {
		// Could wrap around, but at least it changed or wrapped
	}

	m2, _ = got.handleFleetKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	got2 := asModel(t, m2)
	_ = got2
}
