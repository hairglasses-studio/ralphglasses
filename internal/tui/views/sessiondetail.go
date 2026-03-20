package views

import (
	"fmt"
	"strings"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RenderSessionDetail renders a detailed view of a single session.
func RenderSessionDetail(s *session.Session, width, height int) string {
	if s == nil {
		return styles.InfoStyle.Render("  No session selected")
	}

	s.Lock()
	id := s.ID
	provider := string(s.Provider)
	repo := s.RepoName
	repoPath := s.RepoPath
	status := string(s.Status)
	model := s.Model
	agent := s.AgentName
	team := s.TeamName
	spent := s.SpentUSD
	budget := s.BudgetUSD
	turns := s.TurnCount
	maxTurns := s.MaxTurns
	launched := s.LaunchedAt
	lastActivity := s.LastActivity
	exitReason := s.ExitReason
	lastOutput := s.LastOutput
	errMsg := s.Error
	costHistory := make([]float64, len(s.CostHistory))
	copy(costHistory, s.CostHistory)
	s.Unlock()

	var b strings.Builder

	// Session Info
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Session %s", id)))
	b.WriteString("\n\n")

	b.WriteString(styles.HeaderStyle.Render("Session Info"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ID:            %s\n", id))
	b.WriteString(fmt.Sprintf("  Provider:      %s\n", styles.ProviderStyle(provider).Render(provider)))
	b.WriteString(fmt.Sprintf("  Repo:          %s\n", repo))
	b.WriteString(fmt.Sprintf("  Path:          %s\n", repoPath))
	b.WriteString(fmt.Sprintf("  Status:        %s\n", styles.StatusStyle(status).Render(status)))
	b.WriteString(fmt.Sprintf("  Model:         %s\n", model))
	if agent != "" {
		b.WriteString(fmt.Sprintf("  Agent:         %s\n", agent))
	}
	if team != "" {
		b.WriteString(fmt.Sprintf("  Team:          %s\n", team))
	}
	b.WriteString(fmt.Sprintf("  Launched:      %s\n", launched.Format("15:04:05")))
	if !lastActivity.IsZero() {
		b.WriteString(fmt.Sprintf("  Last Activity: %s\n", lastActivity.Format("15:04:05")))
	}
	b.WriteString(fmt.Sprintf("  Duration:      %s\n", formatDuration(launched)))
	if exitReason != "" {
		b.WriteString(fmt.Sprintf("  Exit Reason:   %s\n", exitReason))
	}
	b.WriteString("\n")

	// Cost
	b.WriteString(styles.HeaderStyle.Render("Cost"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Spent:         $%.2f\n", spent))
	if budget > 0 {
		pct := (spent / budget) * 100
		b.WriteString(fmt.Sprintf("  Budget:        $%.2f\n", budget))
		bar := renderBudgetBar(pct, 30)
		b.WriteString(fmt.Sprintf("  Utilization:   %s %.0f%%\n", bar, pct))
	}
	turnStr := fmt.Sprintf("%d", turns)
	if maxTurns > 0 {
		turnStr = fmt.Sprintf("%d/%d", turns, maxTurns)
	}
	b.WriteString(fmt.Sprintf("  Turns:         %s\n", turnStr))

	// Cost sparkline
	if len(costHistory) > 1 {
		sparkWidth := 30
		if width > 0 && width-4 < sparkWidth {
			sparkWidth = width - 4
		}
		points := costHistory
		if len(points) > sparkWidth {
			points = points[len(points)-sparkWidth:]
		}
		sl := sparkline.New(sparkWidth, 2)
		for _, v := range points {
			sl.Push(v)
		}
		b.WriteString(fmt.Sprintf("  Cost trend:    %s\n", sl.View()))
	}
	b.WriteString("\n")

	// Error
	if errMsg != "" {
		b.WriteString(styles.StatusFailed.Render("Error"))
		b.WriteString("\n")
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("  %s", errMsg)))
		b.WriteString("\n\n")
	}

	// Output (last N lines)
	if lastOutput != "" {
		b.WriteString(styles.HeaderStyle.Render("Last Output"))
		b.WriteString("\n")
		lines := strings.Split(lastOutput, "\n")
		maxLines := height - 30 // leave room for header sections
		if maxLines < 5 {
			maxLines = 5
		}
		if len(lines) > maxLines {
			lines = lines[len(lines)-maxLines:]
		}
		for _, line := range lines {
			if width > 0 && len([]rune(line)) > width-4 {
				line = string([]rune(line)[:width-4])
			}
			b.WriteString(fmt.Sprintf("  %s\n", line))
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.HelpStyle.Render("  X: stop session  d: git diff  Esc: back"))

	return b.String()
}

// renderBudgetBar renders a progress bar using bubbles/progress.
func renderBudgetBar(pct float64, width int) string {
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}

	var color string
	switch {
	case pct >= 90:
		color = string(styles.ColorRed)
	case pct >= 70:
		color = string(styles.ColorYellow)
	default:
		color = string(styles.ColorGreen)
	}

	p := progress.New(progress.WithSolidFill(color), progress.WithoutPercentage())
	p.Width = width
	return p.ViewAs(pct / 100)
}
