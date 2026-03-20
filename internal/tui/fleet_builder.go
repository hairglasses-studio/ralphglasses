package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/notify"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

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
	data.Repos = m.Repos

	// Session stats
	if m.SessMgr != nil {
		sessions := m.SessMgr.List("")
		data.Sessions = sessions
		data.TotalSessions = len(sessions)

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
			// Aggregate cost histories for fleet sparkline
			if len(s.CostHistory) > 0 {
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
	}

	// Send desktop notifications for critical alerts
	if m.NotifyEnabled {
		for _, alert := range data.Alerts {
			if alert.Severity == "critical" {
				go notify.Send("ralphglasses alert", alert.Message)
			}
		}
	}

	return data
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
