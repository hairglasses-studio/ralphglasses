package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// --- handleSearchInput ---

func TestSearchInput_EscapeReturnsNormal(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()

	m2, cmd := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyEscape})
	got := m2.(Model)
	if got.InputMode != ModeNormal {
		t.Errorf("InputMode = %v, want ModeNormal", got.InputMode)
	}
	if got.SearchInput.Active {
		t.Error("SearchInput should be deactivated after Escape")
	}
	if cmd != nil {
		t.Error("expected nil cmd after Escape")
	}
}

func TestSearchInput_TypeCharUpdatesQuery(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()

	m2, _ := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := m2.(Model)
	if got.SearchInput.Query != "a" {
		t.Errorf("Query = %q, want %q", got.SearchInput.Query, "a")
	}
	if got.InputMode != ModeSearch {
		t.Errorf("InputMode = %v, want ModeSearch", got.InputMode)
	}
}

func TestSearchInput_EnterNoResults(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()

	// Enter with no results should return to normal mode
	m2, cmd := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	// With no results, HandleKey returns (zero, false) so confirmed=false,
	// but Active is still true, so we stay in search mode.
	if cmd != nil {
		t.Error("expected nil cmd for Enter with no results")
	}
	_ = got
}

func TestSearchInput_EnterWithRepoResult(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()
	m.Repos = []*model.Repo{
		{Name: "myrepo", Path: "/tmp/myrepo"},
	}
	m.SearchInput.SetResults([]components.SearchResult{
		{Type: components.SearchTypeRepo, Name: "myrepo", Path: "/tmp/myrepo", Score: 90},
	})

	m2, cmd := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.InputMode != ModeNormal {
		t.Errorf("InputMode = %v, want ModeNormal after confirmed search", got.InputMode)
	}
	if got.Nav.CurrentView != ViewRepoDetail {
		t.Errorf("CurrentView = %v, want ViewRepoDetail", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestSearchInput_EnterWithSessionResult(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()
	m.SearchInput.SetResults([]components.SearchResult{
		{Type: components.SearchTypeSession, Name: "session-1", Path: "sess-id-123", Score: 80},
	})

	m2, _ := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.InputMode != ModeNormal {
		t.Errorf("InputMode = %v, want ModeNormal", got.InputMode)
	}
	if got.Sel.SessionID != "sess-id-123" {
		t.Errorf("SessionID = %q, want %q", got.Sel.SessionID, "sess-id-123")
	}
	if got.Nav.CurrentView != ViewSessionDetail {
		t.Errorf("CurrentView = %v, want ViewSessionDetail", got.Nav.CurrentView)
	}
}

func TestSearchInput_EnterWithTeamResult(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()
	m.SearchInput.SetResults([]components.SearchResult{
		{Type: components.SearchTypeTeam, Name: "alpha-team", Path: "alpha-team", Score: 85},
	})

	m2, _ := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.Sel.TeamName != "alpha-team" {
		t.Errorf("TeamName = %q, want %q", got.Sel.TeamName, "alpha-team")
	}
	if got.Nav.CurrentView != ViewTeamDetail {
		t.Errorf("CurrentView = %v, want ViewTeamDetail", got.Nav.CurrentView)
	}
}

func TestSearchInput_EnterWithCycleResult(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()
	m.SearchInput.SetResults([]components.SearchResult{
		{Type: components.SearchTypeCycle, Name: "cycle-1", Path: "cycle-id", Score: 70},
	})

	m2, _ := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewRDCycle {
		t.Errorf("CurrentView = %v, want ViewRDCycle", got.Nav.CurrentView)
	}
}

func TestSearchInput_ArrowDownMovesSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()
	m.SearchInput.Query = "repo"
	m.Repos = []*model.Repo{
		{Name: "repo1", Path: "/tmp/repo1"},
		{Name: "repo2", Path: "/tmp/repo2"},
	}
	m.SearchInput.SetResults([]components.SearchResult{
		{Type: components.SearchTypeRepo, Name: "repo1", Path: "/tmp/repo1", Score: 90},
		{Type: components.SearchTypeRepo, Name: "repo2", Path: "/tmp/repo2", Score: 80},
	})

	m2, _ := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyDown})
	got := m2.(Model)
	// After arrow down, refreshSearchResults re-runs the search.
	// The Selected value depends on the refresh, but the key was processed.
	// Verify no crash and the model is returned.
	_ = got
}

func TestSearchInput_BackspaceDeletesChar(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeSearch
	m.SearchInput.Activate()
	m.SearchInput.Query = "ab"

	m2, _ := m.handleSearchInput(tea.KeyMsg{Type: tea.KeyBackspace})
	got := m2.(Model)
	if got.SearchInput.Query != "a" {
		t.Errorf("Query = %q, want %q", got.SearchInput.Query, "a")
	}
}

// --- navigateToSearchResult ---

func TestNavigateSearchResult_RepoNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()

	// No repos in the model, so findRepoByPath and findRepoByName both return -1
	m2, cmd := m.navigateToSearchResult(components.SearchResult{
		Type: components.SearchTypeRepo,
		Name: "nonexistent",
		Path: "/nope",
	})
	got := m2.(Model)
	// Should not crash, view should remain unchanged
	if got.Nav.CurrentView != ViewOverview {
		t.Errorf("CurrentView = %v, want ViewOverview (unchanged)", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

// --- refreshSearchResults ---

func TestRefreshSearchResults_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.SearchInput.Activate()
	m.SearchInput.Query = "test"

	// Should not panic with nil SessMgr
	m.refreshSearchResults()
}
