package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// modelRenderFunc produces the rendered string for a view given the current Model.
type modelRenderFunc func(m *Model, width, height int) string

// modelKeyFunc handles a key event for a view, returning the updated model and command.
type modelKeyFunc func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd)

// registeredView bundles the render and key-handling functions for a single view.
type registeredView struct {
	render    modelRenderFunc
	handleKey modelKeyFunc
}

// viewDispatch maps ViewMode to its render + key handler pair.
// This is the internal lookup table — the public views.Registry provides
// the ViewHandler interface for future use; this map is used directly
// by View() and handleKey() because those operate on Model values.
var viewDispatch map[ViewMode]registeredView

// initViewRegistry creates the views.Registry (stored on Model for future use)
// and populates the package-level viewDispatch map used by View() and handleKey().
func initViewRegistry() *views.Registry {
	reg := views.NewRegistry()

	viewDispatch = map[ViewMode]registeredView{
		ViewHelp: {
			render: func(m *Model, width, height int) string {
				m.HelpView.SetData(m.Keys.HelpGroups())
				m.HelpView.SetDimensions(width, height)
				return m.HelpView.Render()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleHelpKey(msg)
			},
		},
		ViewLogs: {
			render: func(m *Model, _, _ int) string {
				return m.LogView.View()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleLogKey(msg)
			},
		},
		ViewLoopList: {
			render: func(m *Model, _, _ int) string {
				return m.LoopListTable.View() + "\n" +
					styles.HelpStyle.Render("  s start loop  x/d stop loop  p pause/resume  Enter detail  j/k navigate  Esc back")
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleLoopListKey(msg)
			},
		},
		ViewSessions: {
			render: func(m *Model, _, _ int) string {
				return m.SessionTable.View()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleSessionsKey(msg)
			},
		},
		ViewTeams: {
			render: func(m *Model, _, _ int) string {
				return m.TeamTable.View()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleTeamsKey(msg)
			},
		},
		ViewRDCycle: {
			render: func(m *Model, width, height int) string {
				cycles := m.buildRDCycleData()
				m.RDCycleView.SetCycles(cycles)
				m.RDCycleView.SetDimensions(width, height)
				return m.RDCycleView.Render()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleRDCycleKey(msg)
			},
		},
		ViewRepoDetail: {
			render: func(m *Model, width, height int) string {
				if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
					repo := m.Repos[m.Sel.RepoIdx]
					var detailHealth *views.RepoDetailHealth
					if entry := m.getGateEntry(repo.Path); entry != nil {
						detailHealth = &views.RepoDetailHealth{
							Observations: m.getObservations(repo.Path),
							GateReport:   entry.Report,
						}
						if m.SessMgr != nil {
							detailHealth.ProviderProfiles = m.SessMgr.ProviderProfiles()
						}
					}
					m.RepoDetailView.SetData(repo, detailHealth)
					m.RepoDetailView.SetDimensions(width, height)
					return m.RepoDetailView.Render()
				}
				return ""
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleDetailKey(msg)
			},
		},
		ViewSessionDetail: {
			render: func(m *Model, width, height int) string {
				if m.SessMgr != nil {
					if s, ok := m.SessMgr.Get(m.Sel.SessionID); ok {
						m.SessionDetailView.SetData(s)
						m.SessionDetailView.SetDimensions(width, height)
						return m.SessionDetailView.Render()
					}
					return styles.InfoStyle.Render("  Session not found")
				}
				return ""
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleSessionDetailKey(msg)
			},
		},
		ViewTeamDetail: {
			render: func(m *Model, width, height int) string {
				if m.SessMgr != nil {
					if team, ok := m.SessMgr.GetTeam(m.Sel.TeamName); ok {
						leadSession, _ := m.SessMgr.Get(team.LeadID)
						m.TeamDetailView.SetData(team, leadSession)
						m.TeamDetailView.SetDimensions(width, height)
						return m.TeamDetailView.Render()
					}
					return styles.InfoStyle.Render("  Team not found")
				}
				return ""
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleTeamDetailKey(msg)
			},
		},
		ViewFleet: {
			render: func(m *Model, width, height int) string {
				data := m.buildFleetData()
				m.FleetView.SetData(data)
				m.FleetView.SetDimensions(width, height)
				return m.FleetView.Render()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleFleetKey(msg)
			},
		},
		ViewDiff: {
			render: func(m *Model, width, height int) string {
				if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
					m.DiffViewport.SetData(m.Repos[m.Sel.RepoIdx].Path, "")
					m.DiffViewport.SetDimensions(width, height)
					return m.DiffViewport.Render()
				}
				return ""
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleDiffKey(msg)
			},
		},
		ViewTimeline: {
			render: func(m *Model, width, height int) string {
				entries := m.buildTimelineEntries()
				repoName := "All Sessions"
				if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
					repoName = m.Repos[m.Sel.RepoIdx].Name
				}
				m.TimelineViewport.SetData(entries, repoName)
				m.TimelineViewport.SetDimensions(width, height)
				return m.TimelineViewport.Render()
			},
			handleKey: func(m Model, msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				return m.handleTimelineKey(msg)
			},
		},
	}

	return reg
}
