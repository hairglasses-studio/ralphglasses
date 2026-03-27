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
		ScanPath:      scanPath,
		CurrentView:   ViewOverview,
		Table:         table,
		SessionTable:  sessionTable,
		TeamTable:     teamTable,
		LoopListTable: loopListTable,
		TabBar:       components.TabBar{Tabs: tabNames},
		LogView:      views.NewLogView(),
		ProcMgr:      process.NewManager(),
		SessMgr:      sessMgr,
		Breadcrumb:   components.Breadcrumb{Parts: []string{"Repos"}},
		Keys:         DefaultKeyMap(),
		Spinner:      s,
		FleetWindow:  1,
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
	b.WriteString(m.Breadcrumb.View())
	b.WriteString("\n")

	// Tab bar
	b.WriteString(m.TabBar.View())
	b.WriteString("\n\n")

	// Main content
	switch m.CurrentView {
	case ViewOverview:
		b.WriteString(m.Table.View())
	case ViewRepoDetail:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
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
			b.WriteString(views.RenderRepoDetail(repo, m.Width, detailHealth))
		}
	case ViewLogs:
		b.WriteString(m.LogView.View())
	case ViewConfigEditor:
		if m.ConfigEdit != nil {
			b.WriteString(m.ConfigEdit.View())
		}
	case ViewHelp:
		b.WriteString(views.RenderHelp(m.Keys.HelpGroups(), m.Width, m.Height))
	case ViewSessions:
		b.WriteString(m.SessionTable.View())
	case ViewSessionDetail:
		if m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
				b.WriteString(views.RenderSessionDetail(s, m.Width, m.Height))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Session not found"))
			}
		}
	case ViewTeams:
		b.WriteString(m.TeamTable.View())
	case ViewTeamDetail:
		if m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.SelectedTeam); ok {
				leadSession, _ := m.SessMgr.Get(team.LeadID)
				b.WriteString(views.RenderTeamDetail(team, leadSession, m.Width))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Team not found"))
			}
		}
	case ViewFleet:
		data := m.buildFleetData()
		b.WriteString(views.RenderFleetDashboard(data, m.Width, m.Height))
	case ViewDiff:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			b.WriteString(views.RenderDiffView(m.Repos[m.SelectedIdx].Path, "", m.Width, m.Height))
		}
	case ViewTimeline:
		entries := m.buildTimelineEntries()
		repoName := "All Sessions"
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repoName = m.Repos[m.SelectedIdx].Name
		}
		b.WriteString(views.RenderTimeline(entries, repoName, m.Width, m.Height))
	case ViewLoopHealth:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
			healthData := views.LoopHealthData{
				RepoName:     repo.Name,
				Observations: m.getObservations(repo.Path),
			}
			if entry := m.getGateEntry(repo.Path); entry != nil {
				healthData.GateReport = entry.Report
				healthData.Summary = entry.Summary
			}
			b.WriteString(views.RenderLoopHealth(healthData, m.Width, m.Height))
		}
	case ViewLoopList:
		b.WriteString(m.LoopListTable.View())
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("  s start loop  x/d stop loop  p pause/resume  Enter detail  j/k navigate  Esc back"))
	case ViewLoopDetail:
		if m.SessMgr != nil && m.SelectedLoop != "" {
			if l, ok := m.SessMgr.GetLoop(m.SelectedLoop); ok {
				b.WriteString(views.RenderLoopDetail(l, m.Width, m.Height))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Loop not found"))
			}
		}
	case ViewLoopControl:
		b.WriteString(views.RenderLoopControlPanel(m.LoopControlData, m.LoopControlIdx, m.Width, m.Height))
	case ViewObservation:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
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
	if m.ConfirmDialog != nil && m.ConfirmDialog.Active {
		b.WriteString("\n")
		b.WriteString(m.ConfirmDialog.View())
	}
	if m.ActionMenu != nil && m.ActionMenu.Active {
		b.WriteString("\n")
		b.WriteString(m.ActionMenu.View())
	}
	if m.Launcher != nil && m.Launcher.Active {
		b.WriteString("\n")
		b.WriteString(m.Launcher.View())
	}

	// Notification overlay
	if notif := m.Notify.View(); notif != "" {
		b.WriteString("\n")
		b.WriteString(notif)
	}

	// Session output streaming view (split pane)
	if m.StreamingOutput && m.SessionOutputView != nil {
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Live Output "))
		b.WriteString("\n")
		b.WriteString(m.SessionOutputView.View())
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
