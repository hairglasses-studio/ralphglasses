package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// modelRenderFunc produces the rendered string for a view given the current Model.
type modelRenderFunc func(m *Model, width, height int) string

// modelKeyFunc handles a key event for a view, returning the updated model and command.
type modelKeyFunc func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)

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
			handleKey: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
				return m.handleHelpKey(msg)
			},
		},
		ViewLogs: {
			render: func(m *Model, _, _ int) string {
				return m.LogView.View()
			},
			handleKey: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
				return m.handleLogKey(msg)
			},
		},
		ViewLoopList: {
			render: func(m *Model, _, _ int) string {
				return m.LoopListTable.View() + "\n" +
					styles.HelpStyle.Render("  s start loop  x/d stop loop  p pause/resume  Enter detail  j/k navigate  Esc back")
			},
			handleKey: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
				return m.handleLoopListKey(msg)
			},
		},
		ViewSessions: {
			render: func(m *Model, _, _ int) string {
				return m.SessionTable.View()
			},
			handleKey: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
				return m.handleSessionsKey(msg)
			},
		},
		ViewTeams: {
			render: func(m *Model, _, _ int) string {
				return m.TeamTable.View()
			},
			handleKey: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			handleKey: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
				return m.handleRDCycleKey(msg)
			},
		},
	}

	return reg
}
