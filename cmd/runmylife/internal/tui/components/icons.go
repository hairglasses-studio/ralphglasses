package components

import "github.com/charmbracelet/lipgloss"

// Nerd Font icons organized by domain.
// Fallback: basic Unicode when Nerd Fonts unavailable.
const (
	// Calendar
	IconCalendar = "󰃭 "
	IconClock    = "󰥔 "

	// Tasks
	IconCheck   = "󰄬 "
	IconUncheck = "󰄮 "
	IconTask    = "󰝖 "

	// Finance
	IconDollar = " "
	IconTrendUp   = " "
	IconTrendDown = " "
	IconBudget    = "󰊗 "

	// Wellness
	IconHeart  = "󰋑 "
	IconMood   = "󰞅 "
	IconSleep  = "󰒲 "

	// Energy
	IconBolt = "󱐌 "

	// ADHD
	IconBrain    = "󱐋 "
	IconFocus    = "󰓎 "
	IconAlert    = " "
	IconBreak    = "󰾴 "
	IconAchieve  = "󰆥 "

	// Habits
	IconRepeat = "󰑖 "
	IconStreak = "󰈸 "

	// Weather
	IconSun   = " "
	IconCloud = " "
	IconRain  = " "

	// General
	IconStar    = " "
	IconWarning = " "
	IconInfo    = " "
	IconArrowR  = " "
	IconReply   = "󰑐 "
)

// StyledIcon returns an icon with the given foreground color.
func StyledIcon(icon string, color lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(color).Render(icon)
}
