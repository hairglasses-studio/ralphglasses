package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// DelegationEntry represents a single delegated task in the delegation feed.
type DelegationEntry struct {
	Timestamp time.Time
	AgentName string
	Task      string
	Status    string // pending, in-progress, completed, failed
}

// TeamOrchestrationView renders a split-pane team orchestration dashboard
// with a top panel showing the team composition tree and a bottom panel
// showing the live delegation feed.
type TeamOrchestrationView struct {
	Viewport    *ViewportView
	team        *session.TeamStatus
	leadSession *session.Session
	delegations []DelegationEntry
	width       int
	height      int
}

// NewTeamOrchestrationView creates a new TeamOrchestrationView.
func NewTeamOrchestrationView() *TeamOrchestrationView {
	return &TeamOrchestrationView{
		Viewport: NewViewportView(),
	}
}

// SetTeam updates the team and lead session data.
func (v *TeamOrchestrationView) SetTeam(team *session.TeamStatus, leadSession *session.Session) {
	v.team = team
	v.leadSession = leadSession
	v.regenerate()
}

// SetDelegations updates the delegation feed entries.
func (v *TeamOrchestrationView) SetDelegations(delegations []DelegationEntry) {
	v.delegations = delegations
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *TeamOrchestrationView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *TeamOrchestrationView) Render() string {
	return v.Viewport.Render()
}

func (v *TeamOrchestrationView) regenerate() {
	content := RenderTeamOrchestration(v.team, v.leadSession, v.delegations, v.width, v.height)
	v.Viewport.SetContent(content)
}

// RenderTeamOrchestration renders the full team orchestration dashboard.
func RenderTeamOrchestration(team *session.TeamStatus, leadSession *session.Session, delegations []DelegationEntry, width, height int) string {
	if team == nil {
		return styles.InfoStyle.Render("  No team selected")
	}

	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Team Orchestration: %s", styles.IconTeam, team.Name)))
	b.WriteString("\n\n")

	// Top panel: Team Composition Tree
	b.WriteString(renderCompositionTree(team, leadSession, width))
	b.WriteString("\n")

	// Bottom panel: Delegation Feed
	b.WriteString(renderDelegationFeed(delegations, width))
	b.WriteString("\n")

	b.WriteString(styles.HelpStyle.Render("  j/k: scroll  G/g: bottom/top  Ctrl+D/U: page  Esc: back"))

	return b.String()
}

// renderCompositionTree renders the team hierarchy as an indented tree.
func renderCompositionTree(team *session.TeamStatus, leadSession *session.Session, width int) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Team Composition", styles.IconTeam)))
	b.WriteString("\n")

	// Team root node
	statusIcon := styles.StatusIcon(string(team.Status))
	statusStyle := styles.StatusStyle(string(team.Status))
	b.WriteString(fmt.Sprintf("  %s %s  %s\n",
		statusIcon,
		styles.TitleStyle.Render(team.Name),
		statusStyle.Render(string(team.Status))))

	if len(team.Tasks) == 0 && leadSession == nil {
		b.WriteString("  " + styles.InfoStyle.Render("No agents assigned") + "\n")
		return b.String()
	}

	// Lead session as an agent entry
	if leadSession != nil {
		leadSession.Lock()
		leadProvider := string(leadSession.Provider)
		leadStatus := string(leadSession.Status)
		leadModel := leadSession.Model
		leadSession.Unlock()

		prefix := "  \u2502\n"
		if len(team.Tasks) == 0 {
			prefix = "  \u2502\n"
		}
		b.WriteString(prefix)

		connector := "\u2514\u2500\u2500"
		if len(team.Tasks) > 0 {
			connector = "\u251c\u2500\u2500"
		}
		b.WriteString(fmt.Sprintf("  %s %s %s  role:%s  %s  %s\n",
			connector,
			styles.ProviderIcon(leadProvider),
			styles.TitleStyle.Render("lead"),
			styles.InfoStyle.Render("orchestrator"),
			styles.ProviderStyle(leadProvider).Render(leadProvider),
			styles.StatusStyle(leadStatus).Render(leadStatus)))
		if leadModel != "" {
			indent := "  \u2502   "
			if len(team.Tasks) == 0 {
				indent = "      "
			}
			b.WriteString(fmt.Sprintf("%smodel: %s\n", indent, styles.InfoStyle.Render(leadModel)))
		}
	}

	// Task agents as tree entries
	for i, task := range team.Tasks {
		isLast := i == len(team.Tasks)-1

		b.WriteString("  \u2502\n")

		connector := "\u251c\u2500\u2500"
		if isLast {
			connector = "\u2514\u2500\u2500"
		}

		agentName := fmt.Sprintf("agent-%d", i+1)
		provider := string(task.Provider)
		if provider == "" {
			provider = "unassigned"
		}

		taskStatus := task.Status
		if taskStatus == "" {
			taskStatus = "pending"
		}

		taskIcon := taskOrchestrationIcon(taskStatus)

		desc := task.Description
		maxDesc := 50
		if width > 100 {
			maxDesc = width - 40
		}
		if len(desc) > maxDesc {
			desc = desc[:maxDesc-1] + "\u2026"
		}

		b.WriteString(fmt.Sprintf("  %s %s %s  %s  %s  %s\n",
			connector,
			taskIcon,
			styles.TitleStyle.Render(agentName),
			styles.ProviderIcon(provider)+" "+styles.ProviderStyle(provider).Render(provider),
			styles.StatusStyle(taskStatus).Render(taskStatus),
			desc))
	}

	return b.String()
}

// renderDelegationFeed renders the live delegation feed.
func renderDelegationFeed(delegations []DelegationEntry, width int) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Delegation Feed", styles.IconSession)))
	b.WriteString("\n")

	if len(delegations) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No delegations yet"))
		b.WriteString("\n")
		return b.String()
	}

	// Header row
	b.WriteString(styles.InfoStyle.Render("  Time      Agent            Task                                Status"))
	b.WriteString("\n")

	for _, d := range delegations {
		ts := d.Timestamp.Format("15:04:05")

		agentName := d.AgentName
		if len(agentName) > 14 {
			agentName = agentName[:13] + "\u2026"
		}

		task := d.Task
		maxTask := 36
		if width > 120 {
			maxTask = width - 60
		}
		if len(task) > maxTask {
			task = task[:maxTask-1] + "\u2026"
		}

		status := d.Status
		if status == "" {
			status = "pending"
		}

		statusIcon := taskOrchestrationIcon(status)

		b.WriteString(fmt.Sprintf("  %s  %-14s  %-*s  %s %s\n",
			styles.InfoStyle.Render(ts),
			agentName,
			maxTask,
			task,
			statusIcon,
			styles.StatusStyle(status).Render(status)))
	}

	return b.String()
}

// taskOrchestrationIcon returns a status icon for orchestration task status.
func taskOrchestrationIcon(status string) string {
	switch status {
	case "completed":
		return styles.StatusRunning.Render(styles.IconCompleted)
	case "in-progress":
		return styles.WarningStyle.Render(styles.IconLaunching)
	case "failed":
		return styles.StatusFailed.Render(styles.IconCritical)
	default:
		return styles.InfoStyle.Render(styles.IconIdle)
	}
}
