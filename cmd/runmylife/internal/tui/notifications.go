package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type toastLevel int

const (
	toastSuccess toastLevel = iota
	toastWarning
	toastError
	toastInfo
)

type toast struct {
	message   string
	level     toastLevel
	expiresAt time.Time
}

type toastExpiredMsg struct{}

func newToast(message string, level toastLevel) toast {
	return toast{
		message:   message,
		level:     level,
		expiresAt: time.Now().Add(3 * time.Second),
	}
}

func toastTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return toastExpiredMsg{}
	})
}

func (t toast) render(width int) string {
	base := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)

	var style lipgloss.Style
	switch t.level {
	case toastSuccess:
		style = base.BorderForeground(colorSuccess).Foreground(colorSuccess)
	case toastWarning:
		style = base.BorderForeground(colorWarning).Foreground(colorWarning)
	case toastError:
		style = base.BorderForeground(colorDanger).Foreground(colorDanger)
	default:
		style = base.BorderForeground(colorSecondary).Foreground(colorSecondary)
	}

	rendered := style.Render(t.message)
	pad := width - lipgloss.Width(rendered)
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + rendered
}
