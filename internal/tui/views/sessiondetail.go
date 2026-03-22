package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
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
	providerSessionID := s.ProviderSessionID
	spent := s.SpentUSD
	budget := s.BudgetUSD
	turns := s.TurnCount
	maxTurns := s.MaxTurns
	launched := s.LaunchedAt
	lastActivity := s.LastActivity
	exitReason := s.ExitReason
	lastOutput := s.LastOutput
	errMsg := s.Error
	lastEventType := s.LastEventType
	parseErrors := s.StreamParseErrors
	costHistory := make([]float64, len(s.CostHistory))
	copy(costHistory, s.CostHistory)
	outputHistory := make([]string, len(s.OutputHistory))
	copy(outputHistory, s.OutputHistory)
	s.Unlock()

	var b strings.Builder

	// Session Info
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Session %s", styles.IconSession, id)))
	b.WriteString("\n\n")

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Session Info", styles.IconSession)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ID:            %s\n", id))
	b.WriteString(fmt.Sprintf("  Provider:      %s %s\n",
		styles.ProviderIcon(provider),
		styles.ProviderStyle(provider).Render(provider)))
	if providerSessionID != "" {
		b.WriteString(fmt.Sprintf("  Provider ID:   %s\n", providerSessionID))
	}
	b.WriteString(fmt.Sprintf("  Repo:          %s\n", repo))
	b.WriteString(fmt.Sprintf("  Path:          %s\n", repoPath))
	b.WriteString(fmt.Sprintf("  Status:        %s %s\n",
		styles.StatusIcon(status),
		styles.StatusStyle(status).Render(status)))
	b.WriteString(fmt.Sprintf("  Model:         %s\n", model))
	if agent != "" {
		b.WriteString(fmt.Sprintf("  Agent:         %s %s\n", styles.IconAgent, agent))
	}
	if team != "" {
		b.WriteString(fmt.Sprintf("  Team:          %s %s\n", styles.IconTeam, team))
	}
	b.WriteString(fmt.Sprintf("  Launched:      %s %s\n", styles.IconClock, launched.Format("15:04:05")))
	if !lastActivity.IsZero() {
		staleness := time.Since(lastActivity)
		stalenessStr := lastActivity.Format("15:04:05")
		if staleness > 5*time.Minute && status == "running" {
			stalenessStr += styles.StatusFailed.Render(fmt.Sprintf(" (stale: %s)", formatStaleness(staleness)))
		}
		b.WriteString(fmt.Sprintf("  Last Activity: %s %s\n", styles.IconClock, stalenessStr))
	}
	b.WriteString(fmt.Sprintf("  Duration:      %s\n", formatDuration(launched)))
	if exitReason != "" {
		b.WriteString(fmt.Sprintf("  Exit Reason:   %s\n", exitReason))
	}
	if lastEventType != "" {
		b.WriteString(fmt.Sprintf("  Last Event:    %s\n", lastEventType))
	}
	if parseErrors > 0 {
		b.WriteString(fmt.Sprintf("  Parse Errors:  %d\n", parseErrors))
	}
	b.WriteString("\n")

	// Cost
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Cost", styles.IconBudget)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Spent:         $%.2f\n", spent))
	if budget > 0 {
		pct := (spent / budget) * 100
		bar := renderBudgetBar(pct, 30)
		b.WriteString(fmt.Sprintf("  Budget:        $%.2f\n", budget))
		b.WriteString(fmt.Sprintf("  Utilization:   %s %.0f%%\n", bar, pct))
	}

	// Turns with gauge
	turnStr := fmt.Sprintf("%d", turns)
	if maxTurns > 0 {
		gauge := components.InlineGauge(float64(turns), float64(maxTurns), 30)
		turnStr = fmt.Sprintf("%s %d/%d", gauge, turns, maxTurns)
	}
	b.WriteString(fmt.Sprintf("  Turns:         %s\n", turnStr))

	// Cost-per-turn
	if turns > 0 && spent > 0 {
		cpt := spent / float64(turns)
		b.WriteString(fmt.Sprintf("  $/turn:        $%.4f\n", cpt))
	}

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
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("%s Error", styles.IconErrored)))
		b.WriteString("\n")
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("  %s", errMsg)))
		b.WriteString("\n\n")
	}

	// Output history
	if len(outputHistory) > 0 || lastOutput != "" {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Output History", styles.IconLog)))
		b.WriteString("\n")
		lines := outputHistory
		if len(lines) == 0 && lastOutput != "" {
			lines = strings.Split(lastOutput, "\n")
		}
		maxLines := height - 32
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

	b.WriteString(styles.HelpStyle.Render("  Enter: output  o: live output  d: git diff  t: timeline  X: stop  Esc: back"))

	return b.String()
}

func formatStaleness(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
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
