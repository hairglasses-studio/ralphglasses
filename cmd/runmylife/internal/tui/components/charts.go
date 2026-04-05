package components

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/NimbleMarkets/ntcharts/barchart"
	"github.com/NimbleMarkets/ntcharts/sparkline"
)

// Sparkline renders an inline sparkline chart from data points.
func Sparkline(data []float64, width int, color lipgloss.Color) string {
	if len(data) == 0 || width < 3 {
		return ""
	}
	height := 3
	if width < 10 {
		height = 2
	}

	s := sparkline.New(width, height)
	s.Style = lipgloss.NewStyle().Foreground(color)
	s.PushAll(data)
	return s.View()
}

// BarChart renders a vertical bar chart with labeled bars.
func BarChart(labels []string, values []float64, width, height int, color lipgloss.Color) string {
	if len(labels) == 0 || len(values) == 0 {
		return ""
	}

	bc := barchart.New(width, height)
	bc.AxisStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	bc.LabelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	style := lipgloss.NewStyle().Foreground(color)
	for i, label := range labels {
		if i >= len(values) {
			break
		}
		bc.Push(barchart.BarData{
			Label: label,
			Values: []barchart.BarValue{{
				Name:  label,
				Value: values[i],
				Style: style,
			}},
		})
	}

	return bc.View()
}

// HabitHeatmap renders a simple text-based heatmap grid for habit completions.
// data[row][col] should be 0 (not done) or 1 (done).
func HabitHeatmap(habitNames []string, dayLabels []string, data [][]int) string {
	if len(habitNames) == 0 || len(dayLabels) == 0 {
		return ""
	}

	done := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("█")
	empty := lipgloss.NewStyle().Foreground(lipgloss.Color("#374151")).Render("░")
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	result := "  "
	for _, dl := range dayLabels {
		if len(dl) >= 2 {
			dl = dl[:2]
		}
		result += muted.Render(dl) + " "
	}
	result += "\n"

	for i, name := range habitNames {
		label := name
		if len(label) > 12 {
			label = label[:11] + "~"
		}
		result += muted.Render(padRight(label, 12)) + " "
		if i < len(data) {
			for _, v := range data[i] {
				if v > 0 {
					result += done + "  "
				} else {
					result += empty + "  "
				}
			}
		}
		result += "\n"
	}
	return result
}

func padRight(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
