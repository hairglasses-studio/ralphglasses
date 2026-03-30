package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	lipglossv1 "github.com/charmbracelet/lipgloss"

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
	// Use v1 lipgloss for spinner style (bubbles/spinner is still on v1; migrates in phase 1B)
	s.Style = lipglossv1.NewStyle().Foreground(lipglossv1.Color(styles.ColorGreenStr))

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
		DiffViewport:         views.NewDiffViewport(),
		TimelineViewport:     views.NewTimelineViewport(),
		LoopDetailView:       views.NewLoopDetailView(),
		LoopControlView:      views.NewLoopControlView(),
		ObservationViewport:  views.NewObservationViewport(),
		RDCycleView:              views.NewRDCycleView(),
		TeamOrchestrationView:    views.NewTeamOrchestrationView(),
		SearchInput:   components.NewSearchInput(),
		SearchView:    views.NewSearchView(),
		LastRefresh:    components.NowFunc(),
		StartedAt:      components.NowFunc(),
		ProcMgr:        process.NewManager(),
		SessMgr:        sessMgr,
		Keys:           DefaultKeyMap(),
		Spinner:        s,
		viewRegistry:   initViewRegistry(),
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

	// Main content — try registry first, fall back to switch
	if rv, ok := viewDispatch[m.Nav.CurrentView]; ok {
		mCopy := m
		b.WriteString(rv.render(&mCopy, m.Width, m.Height-4))
	} else {
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
	case ViewTeamOrchestration:
		if m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.Sel.TeamName); ok {
				leadSession, _ := m.SessMgr.Get(team.LeadID)
				m.TeamOrchestrationView.SetTeam(team, leadSession)
				m.TeamOrchestrationView.SetDimensions(m.Width, m.Height-4)
				b.WriteString(m.TeamOrchestrationView.Render())
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
				m.LoopDetailView.SetData(l)
				m.LoopDetailView.SetDimensions(m.Width, m.Height-4)
				b.WriteString(m.LoopDetailView.Render())
			} else {
				b.WriteString(styles.InfoStyle.Render("  Loop not found"))
			}
		}
	case ViewLoopControl:
		m.LoopControlView.SetData(m.LoopControlData, m.LoopControlIdx)
		m.LoopControlView.SetDimensions(m.Width, m.Height-4)
		b.WriteString(m.LoopControlView.Render())
	case ViewObservation:
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			repo := m.Repos[m.Sel.RepoIdx]
			data := views.ObservationViewData{
				RepoName:     repo.Name,
				Observations: m.getObservations(repo.Path),
			}
			m.ObservationViewport.SetData(data)
			m.ObservationViewport.SetDimensions(m.Width, m.Height-4)
			b.WriteString(m.ObservationViewport.Render())
		}
	case ViewEventLog:
		if m.EventLog != nil {
			b.WriteString(m.EventLog.View())
		} else {
			b.WriteString(styles.InfoStyle.Render("  No event log available"))
		}
	case ViewRDCycle:
		cycles := m.buildRDCycleData()
		m.RDCycleView.SetCycles(cycles)
		m.RDCycleView.SetDimensions(m.Width, m.Height-4)
		b.WriteString(m.RDCycleView.Render())
	}
	} // end else (fallback switch)

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

	// Global search overlay
	if m.SearchInput != nil && m.SearchInput.Active {
		b.WriteString("\n")
		b.WriteString(m.SearchInput.View(m.Width))
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
	case ModeSearch:
		b.WriteString(styles.CommandStyle.Render("search: "))
		b.WriteString(m.SearchInput.Query)
		b.WriteString(styles.CommandStyle.Render("█"))
	default:
		// Mode indicator in status bar
		m.StatusBar.Mode = "NORMAL"
		m.StatusBar.Filter = m.Filter.Text
		m.StatusBar.SpinnerFrame = m.Spinner.View()
		m.StatusBar.LastRefresh = m.LastRefresh
		b.WriteString(m.StatusBar.View())
	}

	return b.String()
}

// buildRDCycleData gathers R&D cycle data from all repos.
func (m Model) buildRDCycleData() []*session.CycleRun {
	var all []*session.CycleRun
	for _, r := range m.Repos {
		cycles, err := session.ListCycles(r.Path)
		if err != nil {
			continue
		}
		all = append(all, cycles...)
	}
	// Sort by UpdatedAt descending (ListCycles already returns sorted per-repo,
	// but we need a global sort across repos).
	if len(all) > 1 {
		for i := 1; i < len(all); i++ {
			for j := i; j > 0 && all[j].UpdatedAt.After(all[j-1].UpdatedAt); j-- {
				all[j], all[j-1] = all[j-1], all[j]
			}
		}
	}
	return all
}
