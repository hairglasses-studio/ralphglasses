package components

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// GaugeOpt configures gauge rendering.
type GaugeOpt func(*gaugeConfig)

type gaugeConfig struct {
	label      string
	warnAt     float64
	dangerAt   float64
	colorLow   lipgloss.Color
	colorMid   lipgloss.Color
	colorHigh  lipgloss.Color
	showPct    bool
	braille    bool
}

func defaultGaugeConfig() gaugeConfig {
	return gaugeConfig{
		warnAt:   0.7,
		dangerAt: 0.9,
		colorLow: lipgloss.Color("#10B981"),  // green
		colorMid: lipgloss.Color("#F59E0B"),  // amber
		colorHigh: lipgloss.Color("#EF4444"), // red
		showPct:  true,
	}
}

// WithLabel adds a label before the gauge.
func WithLabel(label string) GaugeOpt {
	return func(c *gaugeConfig) { c.label = label }
}

// WithThresholds sets warn/danger thresholds (0.0-1.0).
func WithThresholds(warn, danger float64) GaugeOpt {
	return func(c *gaugeConfig) {
		c.warnAt = warn
		c.dangerAt = danger
	}
}

// WithGradient sets custom colors for low/mid/high states.
func WithGradient(low, mid, high lipgloss.Color) GaugeOpt {
	return func(c *gaugeConfig) {
		c.colorLow = low
		c.colorMid = mid
		c.colorHigh = high
	}
}

// WithBraille uses braille characters for 2x resolution.
func WithBraille() GaugeOpt {
	return func(c *gaugeConfig) { c.braille = true }
}

// NoPct hides the percentage label.
func NoPct() GaugeOpt {
	return func(c *gaugeConfig) { c.showPct = false }
}

// Gauge renders a styled progress gauge bar.
func Gauge(value, max float64, width int, opts ...GaugeOpt) string {
	cfg := defaultGaugeConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	if max <= 0 {
		max = 1
	}
	pct := value / max
	if pct > 1 {
		pct = 1
	}
	if pct < 0 {
		pct = 0
	}

	// Pick color based on thresholds
	color := cfg.colorLow
	if pct >= cfg.dangerAt {
		color = cfg.colorHigh
	} else if pct >= cfg.warnAt {
		color = cfg.colorMid
	}

	barWidth := width
	var bar string
	if cfg.braille {
		// Braille gives 2x horizontal resolution
		totalDots := barWidth * 2
		filledDots := int(math.Round(pct * float64(totalDots)))
		bar = brailleBar(filledDots, totalDots, barWidth)
	} else {
		filled := int(math.Round(pct * float64(barWidth)))
		empty := barWidth - filled
		bar = strings.Repeat("█", filled) + strings.Repeat("░", empty)
	}

	style := lipgloss.NewStyle().Foreground(color)
	result := style.Render(bar)

	if cfg.showPct {
		result += fmt.Sprintf(" %3.0f%%", pct*100)
	}
	if cfg.label != "" {
		result = cfg.label + " " + result
	}
	return result
}

// InvertedGauge renders a gauge where LOW values are danger (e.g., energy).
func InvertedGauge(value, max float64, width int, opts ...GaugeOpt) string {
	// Swap thresholds: low values = danger, high = good
	adjusted := append([]GaugeOpt{
		WithThresholds(0.6, 0.3), // invert: warn when below 60%, danger below 30%
		WithGradient(
			lipgloss.Color("#EF4444"), // low = red
			lipgloss.Color("#F59E0B"), // mid = amber
			lipgloss.Color("#10B981"), // high = green
		),
	}, opts...)
	return invertedGauge(value, max, width, adjusted)
}

func invertedGauge(value, max float64, width int, opts []GaugeOpt) string {
	cfg := defaultGaugeConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	if max <= 0 {
		max = 1
	}
	pct := value / max

	// Inverted: low = danger, high = good
	color := cfg.colorLow // high = green (swapped)
	if pct <= cfg.dangerAt {
		color = cfg.colorHigh // low = red (swapped)
	} else if pct <= cfg.warnAt {
		color = cfg.colorMid
	}

	filled := int(math.Round(pct * float64(width)))
	empty := width - filled
	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	style := lipgloss.NewStyle().Foreground(color)
	result := style.Render(bar)

	if cfg.showPct {
		result += fmt.Sprintf(" %3.0f%%", pct*100)
	}
	if cfg.label != "" {
		result = cfg.label + " " + result
	}
	return result
}

// brailleBar renders a progress bar using braille characters for 2x resolution.
func brailleBar(filledDots, totalDots, charWidth int) string {
	// Braille pattern: ⣿ (full), ⡇ (left half), ⠀ (empty)
	var b strings.Builder
	for i := 0; i < charWidth; i++ {
		dotIdx := i * 2
		if dotIdx+1 < filledDots {
			b.WriteRune('⣿') // both dots filled
		} else if dotIdx < filledDots {
			b.WriteRune('⡇') // left dot only
		} else {
			b.WriteRune('⠀') // empty
		}
	}
	return b.String()
}
