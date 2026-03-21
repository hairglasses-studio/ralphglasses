package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RenderTeamDetail renders a detailed view of a single team.
func RenderTeamDetail(team *session.TeamStatus, leadSession *session.Session, width int) string {
	if team == nil {
		return styles.InfoStyle.Render("  No team selected")
	}

	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Team: %s", styles.IconTeam, team.Name)))
	b.WriteString("\n\n")

	// Team Info
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Team Info", styles.IconTeam)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Name:     %s\n", team.Name))
	b.WriteString(fmt.Sprintf("  Repo:     %s\n", filepath.Base(team.RepoPath)))
	b.WriteString(fmt.Sprintf("  Status:   %s %s\n",
		styles.StatusIcon(string(team.Status)),
		styles.StatusStyle(string(team.Status)).Render(string(team.Status))))
	if !team.CreatedAt.IsZero() {
		b.WriteString(fmt.Sprintf("  Created:  %s\n", team.CreatedAt.Format("15:04:05")))
	}

	// Task completion progress
	completed := 0
	for _, task := range team.Tasks {
		if task.Status == "completed" {
			completed++
		}
	}
	total := len(team.Tasks)
	if total > 0 {
		label := fmt.Sprintf("%d/%d tasks", completed, total)
		b.WriteString(fmt.Sprintf("  Progress: %s\n",
			components.GaugeWithLabel(float64(completed), float64(total), 20, label)))
	}
	b.WriteString("\n")

	// Lead Session
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Lead Session", styles.IconSession)))
	b.WriteString("\n")
	if leadSession != nil {
		leadSession.Lock()
		leadID := leadSession.ID
		if len(leadID) > 8 {
			leadID = leadID[:8]
		}
		provider := string(leadSession.Provider)
		model := leadSession.Model
		spent := leadSession.SpentUSD
		status := string(leadSession.Status)
		leadSession.Unlock()

		b.WriteString(fmt.Sprintf("  ID:       %s\n", leadID))
		b.WriteString(fmt.Sprintf("  Provider: %s %s\n",
			styles.ProviderIcon(provider),
			styles.ProviderStyle(provider).Render(provider)))
		b.WriteString(fmt.Sprintf("  Model:    %s\n", model))
		b.WriteString(fmt.Sprintf("  Spent:    $%.2f\n", spent))
		b.WriteString(fmt.Sprintf("  Status:   %s %s\n",
			styles.StatusIcon(status),
			styles.StatusStyle(status).Render(status)))
	} else {
		b.WriteString(fmt.Sprintf("  ID: %s (not found)\n", team.LeadID))
	}
	b.WriteString("\n")

	// Tasks
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Tasks", styles.IconConfig)))
	b.WriteString("\n")
	if len(team.Tasks) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No tasks"))
		b.WriteString("\n")
	} else {
		for i, task := range team.Tasks {
			indicator := taskIndicator(task.Status)
			desc := task.Description
			if width > 0 && len([]rune(desc)) > width-10 {
				desc = string([]rune(desc)[:width-10])
			}
			providerStr := ""
			if task.Provider != "" {
				providerStr = fmt.Sprintf(" %s %s",
					styles.ProviderIcon(string(task.Provider)),
					styles.ProviderStyle(string(task.Provider)).Render(string(task.Provider)))
			}
			b.WriteString(fmt.Sprintf("  %d. %s %s%s\n", i+1, indicator, desc, providerStr))
		}
	}
	b.WriteString("\n")

	b.WriteString(styles.HelpStyle.Render("  Enter: lead session  d: diff  t: timeline  Esc: back"))

	return b.String()
}

func taskIndicator(status string) string {
	switch status {
	case "completed":
		return styles.StatusRunning.Render(styles.IconCompleted)
	case "in-progress":
		return styles.WarningStyle.Render(styles.IconLaunching)
	default:
		return styles.InfoStyle.Render(styles.IconIdle)
	}
}
