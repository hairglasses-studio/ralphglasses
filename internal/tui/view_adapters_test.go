package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestInitViewRegistry_ReturnsRegistry(t *testing.T) {
	reg := initViewRegistry()
	if reg == nil {
		t.Fatal("initViewRegistry returned nil")
	}
}

func TestInitViewRegistry_PopulatesViewDispatch(t *testing.T) {
	_ = initViewRegistry()
	if viewDispatch == nil {
		t.Fatal("viewDispatch should be populated after initViewRegistry")
	}
}

func TestViewDispatch_ContainsAllExpectedViews(t *testing.T) {
	_ = initViewRegistry()

	expected := []struct {
		mode ViewMode
		name string
	}{
		{ViewHelp, "ViewHelp"},
		{ViewLogs, "ViewLogs"},
		{ViewLoopList, "ViewLoopList"},
		{ViewSessions, "ViewSessions"},
		{ViewTeams, "ViewTeams"},
		{ViewRDCycle, "ViewRDCycle"},
		{ViewRepoDetail, "ViewRepoDetail"},
		{ViewSessionDetail, "ViewSessionDetail"},
		{ViewTeamDetail, "ViewTeamDetail"},
		{ViewFleet, "ViewFleet"},
		{ViewDiff, "ViewDiff"},
		{ViewTimeline, "ViewTimeline"},
	}

	for _, tc := range expected {
		rv, ok := viewDispatch[tc.mode]
		if !ok {
			t.Errorf("viewDispatch missing entry for %s", tc.name)
			continue
		}
		if rv.render == nil {
			t.Errorf("viewDispatch[%s].render is nil", tc.name)
		}
		if rv.handleKey == nil {
			t.Errorf("viewDispatch[%s].handleKey is nil", tc.name)
		}
	}
}

func TestViewDispatch_RenderFunctions_NoPanic(t *testing.T) {
	_ = initViewRegistry()
	m := NewModel("/tmp/test-adapters", nil)
	m.Width = 80
	m.Height = 30

	// These views can render without selection state or session data
	safeViews := []struct {
		mode ViewMode
		name string
	}{
		{ViewHelp, "ViewHelp"},
		{ViewLogs, "ViewLogs"},
		{ViewLoopList, "ViewLoopList"},
		{ViewSessions, "ViewSessions"},
		{ViewTeams, "ViewTeams"},
		{ViewRDCycle, "ViewRDCycle"},
		{ViewFleet, "ViewFleet"},
	}

	for _, tc := range safeViews {
		rv := viewDispatch[tc.mode]
		// Should not panic with default model
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("render for %s panicked: %v", tc.name, r)
				}
			}()
			_ = rv.render(&m, 80, 26)
		}()
	}
}

func TestViewDispatch_HandleKeyFunctions_NoPanic(t *testing.T) {
	_ = initViewRegistry()
	m := NewModel("/tmp/test-adapters", nil)
	m.Width = 80
	m.Height = 30

	msg := tea.KeyPressMsg{Code: 'q', Text: "q"}

	for mode, rv := range viewDispatch {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("handleKey for mode %d panicked: %v", mode, r)
				}
			}()
			_, _ = rv.handleKey(m, msg)
		}()
	}
}

func TestViewDispatch_RepoDetailRender_OutOfRange(t *testing.T) {
	_ = initViewRegistry()
	m := NewModel("/tmp/test-adapters", nil)
	m.Sel.RepoIdx = -1

	rv := viewDispatch[ViewRepoDetail]
	out := rv.render(&m, 80, 26)
	if out != "" {
		t.Errorf("expected empty string for out-of-range repo index, got %q", out)
	}
}

func TestViewDispatch_SessionDetailRender_NoManager(t *testing.T) {
	_ = initViewRegistry()
	m := NewModel("/tmp/test-adapters", nil)
	m.SessMgr = nil

	rv := viewDispatch[ViewSessionDetail]
	out := rv.render(&m, 80, 26)
	if out != "" {
		t.Errorf("expected empty string when SessMgr is nil, got %q", out)
	}
}

func TestViewDispatch_TeamDetailRender_NoManager(t *testing.T) {
	_ = initViewRegistry()
	m := NewModel("/tmp/test-adapters", nil)
	m.SessMgr = nil

	rv := viewDispatch[ViewTeamDetail]
	out := rv.render(&m, 80, 26)
	if out != "" {
		t.Errorf("expected empty string when SessMgr is nil, got %q", out)
	}
}

func TestViewDispatch_DiffRender_NoRepo(t *testing.T) {
	_ = initViewRegistry()
	m := NewModel("/tmp/test-adapters", nil)
	m.Sel.RepoIdx = -1

	rv := viewDispatch[ViewDiff]
	out := rv.render(&m, 80, 26)
	if out != "" {
		t.Errorf("expected empty string for out-of-range diff repo, got %q", out)
	}
}

func TestRegisteredView_Types(t *testing.T) {
	// Verify the type structure compiles and works
	rv := registeredView{
		render: func(m *Model, width, height int) string {
			return "test-render"
		},
		handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
			return m, nil
		},
	}

	m := NewModel("/tmp/test", nil)
	out := rv.render(&m, 80, 30)
	if out != "test-render" {
		t.Errorf("expected 'test-render', got %q", out)
	}

	result, cmd := rv.handleKey(m, tea.KeyPressMsg{})
	if result == nil {
		t.Error("handleKey returned nil model")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestViewDispatch_OverviewNotRegistered(t *testing.T) {
	_ = initViewRegistry()
	// ViewOverview is handled by the switch fallback, not viewDispatch
	if _, ok := viewDispatch[ViewOverview]; ok {
		t.Error("ViewOverview should not be in viewDispatch (handled by switch)")
	}
}

func TestInitViewRegistry_Idempotent(t *testing.T) {
	reg1 := initViewRegistry()
	count1 := len(viewDispatch)

	reg2 := initViewRegistry()
	count2 := len(viewDispatch)

	if reg1 == nil || reg2 == nil {
		t.Fatal("registries should not be nil")
	}
	if count1 != count2 {
		t.Errorf("viewDispatch count changed: %d vs %d", count1, count2)
	}
}
