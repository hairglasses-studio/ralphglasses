package components

import (
	"fmt"
	"math"
	"strings"
)

// sparkChars are sparkline characters from lowest to highest.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// Sparkline renders a series of float64 values as a single-line ASCII sparkline.
func Sparkline(values []float64, width int) string {
	if len(values) == 0 {
		return ""
	}

	// If more values than width, take the last `width` values.
	if len(values) > width {
		values = values[len(values)-width:]
	}

	min, max := values[0], values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	span := max - min
	if span == 0 {
		// All values the same — use middle char.
		return strings.Repeat(string(sparkChars[len(sparkChars)/2]), len(values))
	}

	var b strings.Builder
	for _, v := range values {
		normalized := (v - min) / span
		idx := int(math.Round(normalized * float64(len(sparkChars)-1)))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sparkChars) {
			idx = len(sparkChars) - 1
		}
		b.WriteRune(sparkChars[idx])
	}
	return b.String()
}

// SparklineWithLabel renders "label: ▁▂▃▅▇ (latest_value unit)".
func SparklineWithLabel(label string, values []float64, width int, unit string) string {
	if len(values) == 0 {
		return label + ": (no data)"
	}
	spark := Sparkline(values, width)
	latest := values[len(values)-1]
	return fmt.Sprintf("%s: %s (%.1f%s)", label, spark, latest, unit)
}
