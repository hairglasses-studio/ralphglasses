package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/tui/components"
)

type adhdData struct {
	OverwhelmScore float64
	TriageActive   bool
	FocusCategory  string
	FocusMinutes   int
	ShouldBreak    bool
	FocusNudge     string
	Achievements   []achievementItem
	SwitchCount    int
	SwitchCostMin  int
	FrequentSwitch string
}

type achievementItem struct {
	Title       string
	Description string
	AchievedAt  string
}

func loadADHDData(db *sql.DB) adhdData {
	ctx := context.Background()
	today := time.Now().Format("2006-01-02")

	d := adhdData{}

	// Overwhelm
	score, err := adhd.CheckOverwhelm(ctx, db)
	if err == nil && score != nil {
		d.OverwhelmScore = score.CompositeScore
		d.TriageActive = score.TriageActivated
	}

	// Active focus / hyperfocus
	alert, _ := adhd.DetectHyperfocus(ctx, db)
	if alert != nil {
		d.FocusCategory = alert.Category
		d.FocusMinutes = alert.Minutes
		d.ShouldBreak = alert.ShouldBreak
		d.FocusNudge = alert.GentleNudge
	}

	// Recent achievements
	celebrations := adhd.GetRecentCelebrations(ctx, db, 7)
	for _, c := range celebrations {
		d.Achievements = append(d.Achievements, achievementItem{
			Title:       c.Title,
			Description: c.Description,
			AchievedAt:  c.AchievedAt,
		})
	}

	// Context switches
	stats, _ := adhd.GetSwitchStats(ctx, db, today)
	if stats != nil {
		d.SwitchCount = stats.TotalSwitches
		d.SwitchCostMin = stats.TotalCostMinutes
		d.FrequentSwitch = stats.MostFrequentPair
	}

	return d
}

func renderADHD(d adhdData, width int) string {
	var b strings.Builder
	gaugeWidth := 25

	// Overwhelm gauge
	overwhelm := components.StyledIcon(components.IconBrain, colorPrimary) + subtitleStyle.Render("Overwhelm") + "\n"
	pct := d.OverwhelmScore
	overwhelm += "  " + components.Gauge(pct, 1.0, gaugeWidth,
		components.WithThresholds(0.4, 0.7),
		components.WithBraille(),
	)

	label := successStyle.Render(" manageable")
	if pct > 0.7 {
		label = alertStyle.Render(" HIGH")
	} else if pct > 0.4 {
		label = warningStyle.Render(" elevated")
	}
	overwhelm += label

	if d.TriageActive {
		overwhelm += "\n\n  " + lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(colorDanger).
			Padding(0, 1).
			Render(components.IconAlert + "TRIAGE MODE — focus on top 3 tasks only")
	}
	b.WriteString(cardStyle.Width(width).Render(overwhelm))
	b.WriteString("\n")

	// Active focus session
	if d.FocusCategory != "" {
		focus := components.StyledIcon(components.IconFocus, colorSeries1) + subtitleStyle.Render("Focus Session") + "\n"
		focus += fmt.Sprintf("  %s — %d minutes", d.FocusCategory, d.FocusMinutes)

		// Visual timer bar (proportional to 2hr max)
		maxMin := 120.0
		focus += "\n  " + components.Gauge(float64(d.FocusMinutes), maxMin, gaugeWidth,
			components.WithThresholds(0.75, 0.9),
			components.NoPct(),
		)

		if d.ShouldBreak {
			focus += "\n\n  " + lipgloss.NewStyle().
				Bold(true).
				Foreground(colorDanger).
				Render(components.IconBreak + d.FocusNudge)
		} else if d.FocusNudge != "" {
			focus += "\n  " + warningStyle.Render(d.FocusNudge)
		}
		b.WriteString(cardStyle.Width(width).Render(focus))
		b.WriteString("\n")
	}

	// Context switches gauge
	switches := components.StyledIcon(components.IconAlert, colorWarning) + subtitleStyle.Render("Context Switches Today") + "\n"
	// Gauge: 10+ switches is "high"
	switches += "  " + components.Gauge(float64(d.SwitchCount), 10, gaugeWidth,
		components.WithLabel(fmt.Sprintf("%d switches", d.SwitchCount)),
		components.WithThresholds(0.5, 0.8),
	)
	switches += fmt.Sprintf("\n  ~%d min lost", d.SwitchCostMin)
	if d.FrequentSwitch != "" {
		switches += fmt.Sprintf("\n  Most frequent: %s", mutedStyle.Render(d.FrequentSwitch))
	}
	b.WriteString(cardStyle.Width(width).Render(switches))
	b.WriteString("\n")

	// Achievements
	if len(d.Achievements) > 0 {
		ach := components.StyledIcon(components.IconAchieve, colorSuccess) + subtitleStyle.Render("Recent Achievements") + "\n"
		for _, a := range d.Achievements {
			ach += fmt.Sprintf("  %s %s", components.StyledIcon(components.IconStar, colorWarning), a.Title)
			if a.AchievedAt != "" {
				ach += "  " + mutedStyle.Render(a.AchievedAt)
			}
			ach += "\n"
			if a.Description != "" {
				ach += fmt.Sprintf("    %s\n", mutedStyle.Render(a.Description))
			}
		}
		b.WriteString(cardStyle.Width(width).Render(ach))
	} else {
		b.WriteString(mutedStyle.Render("  No recent achievements — keep going!"))
	}

	return b.String()
}
