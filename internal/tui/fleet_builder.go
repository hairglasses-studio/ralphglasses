package tui

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/notify"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

type fleetWindow struct {
	Label string
	Span  time.Duration
}

var fleetWindows = []fleetWindow{
	{Label: "15m", Span: 15 * time.Minute},
	{Label: "1h", Span: time.Hour},
	{Label: "6h", Span: 6 * time.Hour},
	{Label: "24h", Span: 24 * time.Hour},
	{Label: "all", Span: 0},
}

// countAlerts returns the count of active alerts across repos and sessions.
func (m *Model) countAlerts() int {
	count := 0
	for _, r := range m.Repos {
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			count++
		}
	}
	if m.SessMgr != nil {
		for _, s := range m.SessMgr.List("") {
			s.Lock()
			if s.Status == session.StatusErrored {
				count++
			}
			s.Unlock()
		}
	}
	return count
}

// buildFleetData aggregates data for the fleet dashboard.
func (m *Model) buildFleetData() views.FleetData {
	data := views.FleetData{
		TotalRepos: len(m.Repos),
		Providers:  make(map[string]views.ProviderStat),
	}
	data.SelectedSection = m.selectedFleetSection()
	data.SelectedCursor = m.FleetCursor
	data.CostWindowLabel = fleetWindows[m.clampFleetWindow()].Label

	// Repo stats
	for _, r := range m.Repos {
		status := r.StatusDisplay()
		switch status {
		case "running":
			data.RunningLoops++
		case "paused":
			data.PausedLoops++
		}
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			data.OpenCircuits++
			data.Alerts = append(data.Alerts, views.FleetAlert{
				Severity: "critical",
				Message:  fmt.Sprintf("Circuit breaker OPEN: %s — %s", r.Name, r.Circuit.Reason),
			})
		}
		if r.Status != nil {
			// Stale check (>1h since update)
			if !r.Status.Timestamp.IsZero() && time.Since(r.Status.Timestamp) > time.Hour && r.StatusDisplay() == "running" {
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "warning",
					Message:  fmt.Sprintf("Stale status (>1h): %s", r.Name),
				})
			}
			// Budget check (>=90%)
			if r.Status.BudgetStatus == "exceeded" || (r.Status.SessionSpendUSD > 0 && r.Status.BudgetStatus != "" && r.Status.BudgetStatus != "ok") {
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "warning",
					Message:  fmt.Sprintf("Budget concern: %s — %s", r.Name, r.Status.BudgetStatus),
				})
			}
		}
		if r.Circuit != nil && r.Circuit.ConsecutiveNoProgress >= 3 {
			data.Alerts = append(data.Alerts, views.FleetAlert{
				Severity: "warning",
				Message:  fmt.Sprintf("No progress (%dx): %s", r.Circuit.ConsecutiveNoProgress, r.Name),
			})
		}
	}
	data.Repos = append([]*model.Repo(nil), m.Repos...)

	// Event history
	if m.EventBus != nil {
		data.Events = m.EventBus.History("", 10)
		data.CostHistory = m.buildFleetCostHistory()
	}

	// Session stats
	if m.SessMgr != nil {
		sessions := m.SessMgr.List("")
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].LaunchedAt.After(sessions[j].LaunchedAt)
		})
		data.Sessions = sessions
		data.TotalSessions = len(sessions)
		teams := m.SessMgr.ListTeams()
		sort.Slice(teams, func(i, j int) bool {
			return teams[i].Name < teams[j].Name
		})
		data.Teams = teams

		providerTurns := make(map[string]int)
		providerSpend := make(map[string]float64)
		repoSpend := make(map[string]float64)
		repoBudget := make(map[string]float64)
		repoNames := make(map[string]string) // path → name

		for _, s := range sessions {
			s.Lock()
			provider := string(s.Provider)
			status := s.Status
			spent := s.SpentUSD
			budget := s.BudgetUSD
			repoName := s.RepoName
			repoPath := s.RepoPath
			id := s.ID
			turns := s.TurnCount
			s.Unlock()

			if status == session.StatusRunning || status == session.StatusLaunching {
				data.RunningSessions++
			}
			data.TotalSpendUSD += spent
			data.TotalTurns += turns
			if len(data.CostHistory) == 0 && len(s.CostHistory) > 0 {
				data.CostHistory = append(data.CostHistory, s.CostHistory...)
			}

			ps := data.Providers[provider]
			ps.Sessions++
			if status == session.StatusRunning || status == session.StatusLaunching {
				ps.Running++
			}
			ps.SpendUSD += spent
			data.Providers[provider] = ps

			// Track for cost-per-turn
			providerTurns[provider] += turns
			providerSpend[provider] += spent

			// Track for repo budget utilization
			repoSpend[repoPath] += spent
			if budget > 0 && budget > repoBudget[repoPath] {
				repoBudget[repoPath] = budget
			}
			repoNames[repoPath] = repoName

			// Top expensive sessions
			data.TopExpensive = append(data.TopExpensive, views.ExpensiveSession{
				ID:       id,
				Provider: provider,
				RepoName: repoName,
				SpendUSD: spent,
				Status:   string(status),
			})

			// Session alerts
			if status == session.StatusErrored {
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "info",
					Message:  fmt.Sprintf("Session errored: %s (%s)", shortID, repoName),
				})
			}
			if budget > 0 && spent/budget >= 0.90 {
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "warning",
					Message:  fmt.Sprintf("Session budget ≥90%%: %s ($%.2f/$%.2f)", shortID, spent, budget),
				})
			}
		}

		// Build cost-per-turn map
		data.CostPerTurn = make(map[string]float64)
		for provider, spend := range providerSpend {
			if providerTurns[provider] > 0 {
				data.CostPerTurn[provider] = spend / float64(providerTurns[provider])
			}
		}

		// Sort top expensive sessions and keep top 5
		sort.Slice(data.TopExpensive, func(i, j int) bool {
			return data.TopExpensive[i].SpendUSD > data.TopExpensive[j].SpendUSD
		})
		if len(data.TopExpensive) > 5 {
			data.TopExpensive = data.TopExpensive[:5]
		}

		// Build repo budgets list
		for path, budget := range repoBudget {
			if budget > 0 {
				data.RepoBudgets = append(data.RepoBudgets, views.RepoBudget{
					Name:      repoNames[path],
					SpendUSD:  repoSpend[path],
					BudgetUSD: budget,
				})
			}
		}
		sort.Slice(data.RepoBudgets, func(i, j int) bool {
			return data.RepoBudgets[i].SpendUSD > data.RepoBudgets[j].SpendUSD
		})
	}

	sort.Slice(data.Repos, func(i, j int) bool {
		return data.Repos[i].Name < data.Repos[j].Name
	})

	// HITL and autonomy data from session manager
	if m.SessMgr != nil {
		data.HITLSnapshot = m.SessMgr.HITLSnapshot()
		data.AutonomyLevel = m.SessMgr.GetAutonomyLevel()
	}

	// Gate reports from cache
	if m.GateCache != nil {
		data.GateReports = make(map[string]*e2e.GateReport)
		for repoPath, entry := range m.GateCache {
			// Use repo name instead of full path
			name := filepath.Base(repoPath)
			data.GateReports[name] = entry.Report
		}
	}

	// Send desktop notifications for critical alerts
	if m.NotifyEnabled {
		for _, alert := range data.Alerts {
			if alert.Severity == "critical" {
				go func(msg string) {
					if err := notify.Send("ralphglasses alert", msg); err != nil {
						slog.Warn("failed to send desktop notification", "error", err)
					}
				}(alert.Message)
			}
		}
	}

	return data
}

func (m *Model) clampFleetWindow() int {
	if m.FleetWindow < 0 || m.FleetWindow >= len(fleetWindows) {
		return 0
	}
	return m.FleetWindow
}

func (m *Model) selectedFleetSection() string {
	switch m.FleetSection {
	case 1:
		return "sessions"
	case 2:
		return "teams"
	default:
		return "repos"
	}
}

func (m *Model) fleetSectionLen(data views.FleetData) int {
	switch m.selectedFleetSection() {
	case "sessions":
		return len(data.Sessions)
	case "teams":
		return len(data.Teams)
	default:
		return len(data.Repos)
	}
}

func (m *Model) moveFleetCursor(data views.FleetData, delta int) {
	size := m.fleetSectionLen(data)
	if size == 0 {
		m.FleetCursor = 0
		return
	}
	m.FleetCursor += delta
	if m.FleetCursor < 0 {
		m.FleetCursor = 0
	}
	if m.FleetCursor >= size {
		m.FleetCursor = size - 1
	}
}

func (m *Model) cycleFleetSection(data views.FleetData, delta int) {
	m.FleetSection += delta
	if m.FleetSection < 0 {
		m.FleetSection = 2
	}
	if m.FleetSection > 2 {
		m.FleetSection = 0
	}
	m.moveFleetCursor(data, 0)
}

func (m *Model) openFleetSelection(data views.FleetData) (tea.Model, tea.Cmd) {
	switch m.selectedFleetSection() {
	case "sessions":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Sessions) {
			m.SelectedSession = data.Sessions[m.FleetCursor].ID
			m.pushView(ViewSessionDetail, "Session")
		}
	case "teams":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Teams) {
			m.SelectedTeam = data.Teams[m.FleetCursor].Name
			m.pushView(ViewTeamDetail, data.Teams[m.FleetCursor].Name)
		}
	default:
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Repos) {
			if idx := m.findRepoByPath(data.Repos[m.FleetCursor].Path); idx >= 0 {
				m.SelectedIdx = idx
				m.pushView(ViewRepoDetail, data.Repos[m.FleetCursor].Name)
			}
		}
	}
	return m, nil
}

func (m *Model) stopFleetSelection(data views.FleetData) (tea.Model, tea.Cmd) {
	switch m.selectedFleetSection() {
	case "sessions":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Sessions) {
			s := data.Sessions[m.FleetCursor]
			shortID := s.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			m.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop Session",
				Message: fmt.Sprintf("Stop session %s?", shortID),
				Action:  "stopSession",
				Data:    s.ID,
				Active:  true,
				Width:   50,
			}
		}
	default:
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Repos) {
			if idx := m.findRepoByPath(data.Repos[m.FleetCursor].Path); idx >= 0 {
				m.ConfirmDialog = &components.ConfirmDialog{
					Title:   "Confirm Stop",
					Message: fmt.Sprintf("Stop loop for %s?", data.Repos[m.FleetCursor].Name),
					Action:  "stopLoop",
					Data:    idx,
					Active:  true,
					Width:   50,
				}
			}
		}
	}
	return m, nil
}

func (m *Model) diffFleetSelection(data views.FleetData) (tea.Model, tea.Cmd) {
	switch m.selectedFleetSection() {
	case "sessions":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Sessions) {
			if idx := m.findRepoByPath(data.Sessions[m.FleetCursor].RepoPath); idx >= 0 {
				m.SelectedIdx = idx
				m.pushView(ViewDiff, "Diff")
			}
		}
	case "teams":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Teams) {
			if idx := m.findRepoByPath(data.Teams[m.FleetCursor].RepoPath); idx >= 0 {
				m.SelectedIdx = idx
				m.pushView(ViewDiff, "Diff")
			}
		}
	default:
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Repos) {
			if idx := m.findRepoByPath(data.Repos[m.FleetCursor].Path); idx >= 0 {
				m.SelectedIdx = idx
				m.pushView(ViewDiff, "Diff")
			}
		}
	}
	return m, nil
}

func (m *Model) timelineFleetSelection(data views.FleetData) (tea.Model, tea.Cmd) {
	switch m.selectedFleetSection() {
	case "sessions":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Sessions) {
			if idx := m.findRepoByPath(data.Sessions[m.FleetCursor].RepoPath); idx >= 0 {
				m.SelectedIdx = idx
			}
			m.pushView(ViewTimeline, "Timeline")
		}
	case "teams":
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Teams) {
			if idx := m.findRepoByPath(data.Teams[m.FleetCursor].RepoPath); idx >= 0 {
				m.SelectedIdx = idx
			}
			m.pushView(ViewTimeline, "Timeline")
		}
	default:
		if m.FleetCursor >= 0 && m.FleetCursor < len(data.Repos) {
			if idx := m.findRepoByPath(data.Repos[m.FleetCursor].Path); idx >= 0 {
				m.SelectedIdx = idx
			}
			m.pushView(ViewTimeline, "Timeline")
		}
	}
	return m, nil
}

func (m *Model) buildFleetCostHistory() []float64 {
	if m.EventBus == nil {
		return nil
	}
	window := fleetWindows[m.clampFleetWindow()]
	var historyEvents []events.Event
	if window.Span == 0 {
		historyEvents = m.EventBus.History("", 200)
	} else {
		historyEvents = m.EventBus.HistorySince(time.Now().Add(-window.Span))
	}
	if len(historyEvents) == 0 {
		return nil
	}

	sessionSpend := make(map[string]float64)
	var history []float64
	for _, event := range historyEvents {
		if event.Type != events.CostUpdate {
			continue
		}
		spent, ok := event.Data["spent_usd"].(float64)
		if !ok {
			continue
		}
		sessionSpend[event.SessionID] = spent
		total := 0.0
		for _, value := range sessionSpend {
			total += value
		}
		history = append(history, total)
	}
	return history
}

// buildTimelineEntries collects session timeline data for the timeline view.
func (m *Model) buildTimelineEntries() []views.TimelineEntry {
	if m.SessMgr == nil {
		return nil
	}
	// If in repo detail context, filter to that repo
	var repoPath string
	if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
		repoPath = m.Repos[m.SelectedIdx].Path
	}
	sessions := m.SessMgr.List("")
	var entries []views.TimelineEntry
	for _, s := range sessions {
		s.Lock()
		if repoPath != "" && s.RepoPath != repoPath {
			s.Unlock()
			continue
		}
		entry := views.TimelineEntry{
			ID:        s.ID,
			Provider:  string(s.Provider),
			StartTime: s.LaunchedAt,
			Status:    string(s.Status),
		}
		if s.EndedAt != nil {
			t := *s.EndedAt
			entry.EndTime = &t
		}
		s.Unlock()
		entries = append(entries, entry)
	}
	return entries
}
