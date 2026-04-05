package godview

import (
	"fmt"
	"strings"
	"time"
)

// ProgressBar renders a Unicode progress bar of the given width.
func ProgressBar(pct float64, width int) string {
	if width < 1 {
		width = 5
	}
	filled := int(pct / 100.0 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// FormatCost formats a USD amount compactly.
func FormatCost(usd float64) string {
	if usd == 0 {
		return "--"
	}
	if usd < 0.01 {
		return fmt.Sprintf("$%.4f", usd)
	}
	if usd < 1.0 {
		return fmt.Sprintf("$%.3f", usd)
	}
	if usd < 100.0 {
		return fmt.Sprintf("$%.2f", usd)
	}
	return fmt.Sprintf("$%.0f", usd)
}

// FormatRate formats a cost rate per hour.
func FormatRate(usdPerHr float64) string {
	if usdPerHr == 0 {
		return "--/hr"
	}
	return fmt.Sprintf("$%.1f/hr", usdPerHr)
}

// Truncate truncates a string to n characters with ellipsis.
func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// PadRight pads a string to width, truncating if needed.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return Truncate(s, width)
	}
	return s + strings.Repeat(" ", width-len(s))
}

// PadLeft pads a string to width from the left.
func PadLeft(s string, width int) string {
	if len(s) >= width {
		return Truncate(s, width)
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// TimeAgo returns a compact human-readable time ago string.
func TimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// StatusIcon returns a Unicode status indicator.
func StatusIcon(status string) string {
	switch status {
	case "running":
		return "▶"
	case "completed", "done", "converged":
		return "✓"
	case "failed", "errored", "error":
		return "✗"
	case "idle", "pending", "unknown":
		return "○"
	case "warn", "degraded":
		return "⚠"
	case "stopped":
		return "■"
	default:
		return "·"
	}
}
