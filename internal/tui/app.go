package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"

	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// NewModel creates the root model.
func NewModel(scanPath string, sessMgr *session.Manager) Model {
	table := views.NewOverviewTable()
	sessionTable := views.NewSessionsTable()
	teamTable := views.NewTeamsTable()
	loopListTable := views.NewLoopListTable()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.StatusRunning

	table.MultiSelect = true
	sessionTable.MultiSelect = true

	return Model{
		Nav: NavigationState{
			CurrentView: ViewOverview,
			Breadcrumb:  components.Breadcrumb{Parts: []string{"Repos"}},
		},
		Fleet: FleetNavState{
			Window: 1,
		},
		ScanPath:      scanPath,
		Table:         table,
		SessionTable:  sessionTable,
		TeamTable:     teamTable,
		LoopListTable: loopListTable,
		TabBar:       components.TabBar{Tabs: tabNames},
		LogView:        views.NewLogView(),
		HelpView:          views.NewHelpView(),
		RepoDetailView:    views.NewRepoDetailView(),
		LoopHealthView:    views.NewLoopHealthView(),
		SessionDetailView: views.NewSessionDetailView(),
		TeamDetailView:    views.NewTeamDetailView(),
		FleetView:         views.NewFleetView(),
		DiffViewport:      views.NewDiffViewport(),
		TimelineViewport:  views.NewTimelineViewport(),
		ProcMgr:        process.NewManager(),
		SessMgr:        sessMgr,
		Keys:           DefaultKeyMap(),
		Spinner:        s,
	}
}

// tabNames for the tab bar.
var tabNames = []string{
	fmt.Sprintf("1:%s Repos", styles.IconRepo),
	fmt.Sprintf("2:%s Sessions", styles.IconSession),
	fmt.Sprintf("3:%s Teams", styles.IconTeam),
	fmt.Sprintf("4:%s Fleet", styles.IconFleet),
}

// View renders the TUI.
func (m Model) View() string {
	if m.Width < 3 || m.Height < 3 {
		return "Terminal too small. Please resize."
	}

	var b strings.Builder

	// Title bar
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" %s ralphglasses ", styles.IconGlasses)))
	b.WriteString("  ")
	b.WriteString(m.Nav.Breadcrumb.View())
	b.WriteString("\n")

	// Tab bar
	b.WriteString(m.TabBar.View())
	b.WriteString("\n\n")

	// Main content
	switch m.Nav.CurrentView {
	case ViewOverview:
		b.WriteString(m.Table.View())
	case ViewRepoDetail:
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
			m.RepoDetailView.SetDimensions(m.Width, m.Height-4)
			b.WriteString(m.RepoDetailView.Render())
		}
	case ViewLogs:
		b.WriteString(m.LogView.View())
	case ViewConfigEditor:
		if m.ConfigEdit != nil {
			b.WriteString(m.ConfigEdit.View())
		}
	case ViewHelp:
		m.HelpView.SetData(m.Keys.HelpGroups())
		m.HelpView.SetDimensions(m.Width, m.Height-4)
		b.WriteString(m.HelpView.Render())
	case ViewSessions:
		b.WriteString(m.SessionTable.View())
	case ViewSessionDetail:
		if m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.Sel.SessionID); ok {
				m.SessionDetailView.SetData(s)
				m.SessionDetailView.SetDimensions(m.Width, m.Height-4)
				b.WriteString(m.SessionDetailView.Render())
			} else {
				b.WriteString(styles.InfoStyle.Render("  Session not found"))
			}
		}
	case ViewTeams:
		b.WriteString(m.TeamTable.View())
	case ViewTeamDetail:
		if m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.Sel.TeamName); ok {
				leadSession, _ := m.SessMgr.Get(team.LeadID)
				m.TeamDetailView.SetData(team, leadSession)
				m.TeamDetailView.SetDimensions(m.Width, m.Height-4)
				b.WriteString(m.TeamDetailView.Render())
			} else {
				b.WriteString(styles.InfoStyle.Render("  Team not found"))
			}
		}
	case ViewFleet:
		data := m.buildFleetData()
		m.FleetView.SetData(data)
		m.FleetView.SetDimensions(m.Width, m.Height-4)
		b.WriteString(m.FleetView.Render())
	case ViewDiff:
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			m.DiffViewport.SetData(m.Repos[m.Sel.RepoIdx].Path, "")
			m.DiffViewport.SetDimensions(m.Width, m.Height-4)
			b.WriteString(m.DiffViewport.Render())
		}
	case ViewTimeline:
		entries := m.buildTimelineEntries()
		repoName := "All Sessions"
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			repoName = m.Repos[m.Sel.RepoIdx].Name
		}
		m.TimelineViewport.SetData(entries, repoName)
		m.TimelineViewport.SetDimensions(m.Width, m.Height-4)
		b.WriteString(m.TimelineViewport.Render())
	case ViewLoopHealth:
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			repo := m.Repos[m.Sel.RepoIdx]
			healthData := views.LoopHealthData{
				RepoName:     repo.Name,
				Observations: m.getObservations(repo.Path),
			}
			if entry := m.getGateEntry(repo.Path); entry != nil {
				healthData.GateReport = entry.Report
				healthData.Summary = entry.Summary
			}
			m.LoopHealthView.SetData(healthData)
			m.LoopHealthView.SetDimensions(m.Width, m.Height-4)
			b.WriteString(m.LoopHealthView.Render())
		}
	case ViewLoopList:
		b.WriteString(m.LoopListTable.View())
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("  s start loop  x/d stop loop  p pause/resume  Enter detail  j/k navigate  Esc back"))
	case ViewLoopDetail:
		if m.SessMgr != nil && m.Sel.LoopID != "" {
			if l, ok := m.SessMgr.GetLoop(m.Sel.LoopID); ok {
				b.WriteString(views.RenderLoopDetail(l, m.Width, m.Height))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Loop not found"))
			}
		}
	case ViewLoopControl:
		b.WriteString(views.RenderLoopControlPanel(m.LoopControlData, m.LoopControlIdx, m.Width, m.Height))
	case ViewObservation:
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			repo := m.Repos[m.Sel.RepoIdx]
			data := views.ObservationViewData{
				RepoName:     repo.Name,
				Observations: m.getObservations(repo.Path),
			}
			b.WriteString(views.RenderObservationView(data, m.Width, m.Height))
		}
	case ViewEventLog:
		if m.EventLog != nil {
			b.WriteString(m.EventLog.View())
		} else {
			b.WriteString(styles.InfoStyle.Render("  No event log available"))
		}
	}

	// Loop panel overlay
	if m.ShowLoopPanel {
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Loop Status "))
		b.WriteString("\n")
		b.WriteString(m.LoopView)
	}

	// Modal overlays
	if m.Modals.ConfirmDialog != nil && m.Modals.ConfirmDialog.Active {
		b.WriteString("\n")
		b.WriteString(m.Modals.ConfirmDialog.View())
	}
	if m.Modals.ActionMenu != nil && m.Modals.ActionMenu.Active {
		b.WriteString("\n")
		b.WriteString(m.Modals.ActionMenu.View())
	}
	if m.Modals.Launcher != nil && m.Modals.Launcher.Active {
		b.WriteString("\n")
		b.WriteString(m.Modals.Launcher.View())
	}

	// Notification overlay
	if notif := m.Notify.View(); notif != "" {
		b.WriteString("\n")
		b.WriteString(notif)
	}

	// Session output streaming view (split pane)
	if m.Stream.Active && m.Stream.OutputView != nil {
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Live Output "))
		b.WriteString("\n")
		b.WriteString(m.Stream.OutputView.View())
	}

	b.WriteString("\n")

	// Input line
	switch m.InputMode {
	case ModeCommand:
		b.WriteString(styles.CommandStyle.Render(":"))
		b.WriteString(m.CommandBuf)
		b.WriteString(styles.CommandStyle.Render("█"))
	case ModeFilter:
		b.WriteString(styles.CommandStyle.Render("/"))
		b.WriteString(m.Filter.Text)
		b.WriteString(styles.CommandStyle.Render("█"))
	default:
		// Mode indicator in status bar
		m.StatusBar.Mode = "NORMAL"
		m.StatusBar.Filter = m.Filter.Text
		m.StatusBar.SpinnerFrame = m.Spinner.View()
		b.WriteString(m.StatusBar.View())
	}

	return b.String()
}
