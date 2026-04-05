package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// handleSearchInput processes keys while the global search overlay is active.
func (m Model) handleSearchInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	result, confirmed := m.SearchInput.HandleKey(msg)

	if confirmed {
		// Navigate to the selected result's target view.
		m.InputMode = ModeNormal
		m.SearchInput.Deactivate()
		return m.navigateToSearchResult(result)
	}

	if !m.SearchInput.Active {
		// Escape was pressed — return to normal mode.
		m.InputMode = ModeNormal
		return m, nil
	}

	// Re-run search with the updated query.
	m.refreshSearchResults()

	return m, nil
}

// refreshSearchResults re-runs the global search and updates the SearchInput results.
func (m *Model) refreshSearchResults() {
	// Gather session snapshots (lock-free after snapshot).
	var sessions []views.SessionInfo
	if m.SessMgr != nil {
		for _, s := range m.SessMgr.List("") {
			s.Lock()
			info := views.SessionInfo{
				ID:       s.ID,
				RepoName: s.RepoName,
				Provider: string(s.Provider),
				Status:   string(s.Status),
				Prompt:   s.Prompt,
				TeamName: s.TeamName,
			}
			s.Unlock()
			sessions = append(sessions, info)
		}
	}

	// Gather cycles.
	cycles := m.buildRDCycleData()

	results := views.Search(m.SearchInput.Query, m.Repos, sessions, cycles)
	m.SearchInput.SetResults(results)
}

// navigateToSearchResult switches to the view appropriate for the selected result.
func (m Model) navigateToSearchResult(result components.SearchResult) (tea.Model, tea.Cmd) {
	switch result.Type {
	case components.SearchTypeRepo:
		idx := m.findRepoByPath(result.Path)
		if idx < 0 {
			idx = m.findRepoByName(result.Name)
		}
		if idx >= 0 {
			m.Sel.RepoIdx = idx
			m.switchTab(0, ViewOverview, "Repos")
			m.pushView(ViewRepoDetail, m.Repos[idx].Name)
		}

	case components.SearchTypeSession:
		m.Sel.SessionID = result.Path // Path holds the session ID
		m.switchTab(1, ViewSessions, "Sessions")
		m.pushView(ViewSessionDetail, result.Name)

	case components.SearchTypeCycle:
		m.pushView(ViewRDCycle, "R&D Cycle")

	case components.SearchTypeTeam:
		m.Sel.TeamName = result.Path
		m.switchTab(2, ViewTeams, "Teams")
		m.pushView(ViewTeamDetail, result.Name)
	}

	return m, nil
}
