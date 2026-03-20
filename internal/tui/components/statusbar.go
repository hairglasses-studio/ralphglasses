package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// StatusBar renders the bottom status bar.
type StatusBar struct {
	Width         int
	Mode          string // "NORMAL", "COMMAND", "FILTER"
	Filter        string
	RepoCount     int
	RunningCount  int
	SessionCount  int
	TotalSpendUSD float64
	AlertCount    int
	LastRefresh   time.Time
	SpinnerFrame  string
}

// View renders the status bar.
func (s *StatusBar) View() string {
	left := fmt.Sprintf(" %s  repos:%d  running:%d  sessions:%d  spend:$%.2f",
		styles.CommandStyle.Render(s.Mode), s.RepoCount, s.RunningCount,
		s.SessionCount, s.TotalSpendUSD)

	if s.RunningCount > 0 && s.SpinnerFrame != "" {
		left += " " + s.SpinnerFrame
	}

	if s.AlertCount > 0 {
		left += fmt.Sprintf("  %s",
			styles.StatusFailed.Render(fmt.Sprintf("alerts:%d", s.AlertCount)))
	}

	right := fmt.Sprintf("refreshed %s ", formatAgo(s.LastRefresh))

	if s.Filter != "" {
		left += fmt.Sprintf("  /%s", s.Filter)
	}

	padding := s.Width - lipglossWidth(left) - lipglossWidth(right)
	if padding < 1 {
		padding = 1
	}

	return styles.StatusBarStyle.Width(s.Width).Render(
		left + strings.Repeat(" ", padding) + right,
	)
}

func formatAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm ago", int(d.Minutes()))
}

func lipglossWidth(s string) int {
	// Rough width — count runes, ignoring ANSI.
	// Good enough for padding calculations.
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}
