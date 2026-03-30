package views

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// SparklineBar characters ordered from lowest to highest.
var sparklineBars = []rune{'\u2581', '\u2582', '\u2583', '\u2584', '\u2585', '\u2586', '\u2587', '\u2588'}

// AnomalyMarker holds the index and metadata of an anomalous data point.
type AnomalyMarker struct {
	Index   int
	ZScore  float64
	Message string
}

// SparklineModel is a reusable sparkline chart component.
// It renders []float64 data points as colored Unicode block characters
// with optional anomaly markers and configurable width.
type SparklineModel struct {
	Data      []float64
	Width     int // max columns; 0 = use len(Data)
	Anomalies []AnomalyMarker
	Label     string
}

// NewSparklineModel creates a sparkline with the given data points.
func NewSparklineModel(data []float64) SparklineModel {
	return SparklineModel{Data: data}
}

// SetWidth sets the maximum display width. Data is downsampled if it exceeds width.
func (s *SparklineModel) SetWidth(w int) {
	s.Width = w
}

// SetAnomalies marks specific indices as anomalous.
func (s *SparklineModel) SetAnomalies(anomalies []AnomalyMarker) {
	s.Anomalies = anomalies
}

// Render produces the colored sparkline string.
func (s SparklineModel) Render() string {
	if len(s.Data) == 0 {
		return styles.InfoStyle.Render("(no data)")
	}

	data := s.resample()
	mn, mx := minMax(data)
	spread := mx - mn

	// Build anomaly index set from original indices, mapped to resampled positions.
	anomalySet := s.anomalyIndexSet(len(data))

	var b strings.Builder
	for i, val := range data {
		barIdx := 3 // default mid-height for equal values
		if spread > 0 {
			normalized := (val - mn) / spread
			barIdx = int(math.Round(normalized * 7))
			if barIdx < 0 {
				barIdx = 0
			}
			if barIdx > 7 {
				barIdx = 7
			}
		}

		ch := string(sparklineBars[barIdx])

		if _, isAnomaly := anomalySet[i]; isAnomaly {
			// Anomaly: render with blinking-style highlight (bright white on red bg)
			anomalyStyle := lipgloss.NewStyle().
				Foreground(styles.ColorBrightWhite).
				Background(styles.ColorRed).
				Bold(true)
			b.WriteString(anomalyStyle.Render(ch))
		} else {
			// Color gradient: green (low) -> yellow (medium) -> red (high)
			b.WriteString(barColorStyle(barIdx).Render(ch))
		}
	}
	return b.String()
}

// barColorStyle returns a lipgloss style for the given bar level (0-7).
// Gradient: green (0-2) -> yellow (3-4) -> red (5-7).
func barColorStyle(level int) lipgloss.Style {
	switch {
	case level <= 2:
		return lipgloss.NewStyle().Foreground(styles.ColorGreen)
	case level <= 4:
		return lipgloss.NewStyle().Foreground(styles.ColorYellow)
	default:
		return lipgloss.NewStyle().Foreground(styles.ColorRed)
	}
}

// resample returns data downsampled to fit within s.Width columns.
// If Width is 0 or >= len(Data), returns data as-is.
func (s SparklineModel) resample() []float64 {
	n := len(s.Data)
	w := s.Width
	if w <= 0 || w >= n {
		return s.Data
	}

	result := make([]float64, w)
	bucketSize := float64(n) / float64(w)
	for i := 0; i < w; i++ {
		start := int(float64(i) * bucketSize)
		end := int(float64(i+1) * bucketSize)
		if end > n {
			end = n
		}
		if start >= end {
			if start < n {
				result[i] = s.Data[start]
			}
			continue
		}
		var sum float64
		for j := start; j < end; j++ {
			sum += s.Data[j]
		}
		result[i] = sum / float64(end-start)
	}
	return result
}

// anomalyIndexSet maps original anomaly indices to resampled positions.
func (s SparklineModel) anomalyIndexSet(resampledLen int) map[int]struct{} {
	set := make(map[int]struct{})
	if len(s.Anomalies) == 0 {
		return set
	}

	n := len(s.Data)
	w := resampledLen
	for _, a := range s.Anomalies {
		if a.Index < 0 || a.Index >= n {
			continue
		}
		if w >= n {
			set[a.Index] = struct{}{}
		} else {
			mapped := int(float64(a.Index) / float64(n) * float64(w))
			if mapped >= w {
				mapped = w - 1
			}
			set[mapped] = struct{}{}
		}
	}
	return set
}

// BurnRateTrend indicates the direction of cost change.
type BurnRateTrend int

const (
	TrendStable      BurnRateTrend = iota // ->
	TrendRising                            // upward
	TrendFalling                           // downward
)

// TrendIndicator returns a styled arrow string for the burn rate trend.
func TrendIndicator(trend string) string {
	switch trend {
	case "accelerating", "increasing":
		return lipgloss.NewStyle().Foreground(styles.ColorRed).Bold(true).Render("\u2191 rising")
	case "decelerating", "decreasing":
		return lipgloss.NewStyle().Foreground(styles.ColorGreen).Bold(true).Render("\u2193 falling")
	default:
		return lipgloss.NewStyle().Foreground(styles.ColorYellow).Render("\u2192 stable")
	}
}

// BudgetProjectionBar renders a horizontal bar showing budget consumption.
// pct is 0-100. width is the bar width in columns.
func BudgetProjectionBar(pct float64, width int) string {
	if width < 10 {
		width = 20
	}

	filled := int(math.Round(pct / 100 * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	empty := width - filled

	// Color the filled portion based on consumption level
	var fillStyle lipgloss.Style
	switch {
	case pct >= 90:
		fillStyle = lipgloss.NewStyle().Foreground(styles.ColorRed).Bold(true)
	case pct >= 75:
		fillStyle = lipgloss.NewStyle().Foreground(styles.ColorYellow)
	default:
		fillStyle = lipgloss.NewStyle().Foreground(styles.ColorGreen)
	}

	bar := fillStyle.Render(strings.Repeat("\u2588", filled)) +
		lipgloss.NewStyle().Foreground(styles.ColorDarkGray).Render(strings.Repeat("\u2591", empty))

	label := fmt.Sprintf(" %.1f%%", pct)
	return bar + fillStyle.Render(label)
}

// ExhaustionETA formats an exhaustion time as a human-readable ETA string.
// Returns "N/A" if t is nil, "EXHAUSTED" if already past.
func ExhaustionETA(t *time.Time) string {
	if t == nil {
		return styles.InfoStyle.Render("N/A (no budget limit)")
	}

	remaining := time.Until(*t)
	if remaining <= 0 {
		return styles.AlertCritical.Render("EXHAUSTED")
	}

	return styles.InfoStyle.Render(formatETADuration(remaining))
}

// formatETADuration formats a duration into a human-readable string like "2h 15m" or "3d 4h".
func formatETADuration(d time.Duration) string {
	if d < time.Minute {
		return "< 1m"
	}

	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
