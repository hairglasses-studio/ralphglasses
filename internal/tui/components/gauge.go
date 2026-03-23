package components

import (
	"fmt"
	"math"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// Gauge fill characters
const (
	gaugeFilled = '▰'
	gaugeEmpty  = '▱'
)

// Sparkline block characters (8 levels, bottom to top)
var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Braille spinner frames
var brailleFrames = []rune{'⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'}

// InlineGauge renders a fixed-width bar: ▰▰▰▰▱▱▱▱ with color thresholds.
// current/max are the values; width is the number of bar characters.
func InlineGauge(current, max float64, width int) string {
	if width <= 0 {
		return ""
	}
	if max <= 0 {
		// No max known — render empty gauge
		return styles.InfoStyle.Render(string(repeatRune(gaugeEmpty, width)))
	}

	pct := current / max
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}

	filled := int(math.Round(pct * float64(width)))
	if filled > width {
		filled = width
	}

	bar := string(repeatRune(gaugeFilled, filled)) + string(repeatRune(gaugeEmpty, width-filled))

	// Color based on utilization
	switch {
	case pct >= 0.9:
		return styles.StatusFailed.Render(bar)
	case pct >= 0.7:
		return styles.WarningStyle.Render(bar)
	default:
		return styles.StatusRunning.Render(bar)
	}
}

// InlineSparkline renders a 1-line sparkline using Unicode block chars.
// data is the series of values; width is the max number of characters.
func InlineSparkline(data []float64, width int) string {
	if len(data) == 0 || width <= 0 {
		return ""
	}

	// Use the last `width` data points
	if len(data) > width {
		data = data[len(data)-width:]
	}

	// Find range
	minV, maxV := data[0], data[0]
	for _, v := range data {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	rng := maxV - minV
	if rng == 0 {
		rng = 1 // avoid division by zero
	}

	result := make([]rune, len(data))
	for i, v := range data {
		normalized := (v - minV) / rng
		idx := int(normalized * float64(len(sparkBlocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		result[i] = sparkBlocks[idx]
	}

	return styles.StatusRunning.Render(string(result))
}

// ActivityDot returns a braille spinner char cycling on frame (for active),
// or a dim dot (for inactive).
func ActivityDot(active bool, frame int) string {
	if !active {
		return styles.InfoStyle.Render("·")
	}
	idx := frame % len(brailleFrames)
	return styles.StatusRunning.Render(string(brailleFrames[idx]))
}

// GaugeWithLabel renders an inline gauge followed by a label like "N/M".
func GaugeWithLabel(current, max float64, barWidth int, label string) string {
	return fmt.Sprintf("%s %s", InlineGauge(current, max, barWidth), label)
}

// HealthSparkline renders a sparkline where each point is colored green or red
// based on whether it falls below or above the threshold.
// Values below threshold are "good" (green); above are "bad" (red).
func HealthSparkline(data []float64, threshold float64, width int) string {
	if len(data) == 0 || width <= 0 {
		return ""
	}

	// Use the last `width` data points
	if len(data) > width {
		data = data[len(data)-width:]
	}

	// Find range
	minV, maxV := data[0], data[0]
	for _, v := range data {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}

	rng := maxV - minV
	if rng == 0 {
		rng = 1
	}

	var result string
	for _, v := range data {
		normalized := (v - minV) / rng
		idx := int(normalized * float64(len(sparkBlocks)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		ch := string(sparkBlocks[idx])
		if v > threshold {
			result += styles.StatusFailed.Render(ch)
		} else {
			result += styles.StatusRunning.Render(ch)
		}
	}

	return result
}

func repeatRune(r rune, n int) []rune {
	if n <= 0 {
		return nil
	}
	out := make([]rune, n)
	for i := range out {
		out[i] = r
	}
	return out
}
