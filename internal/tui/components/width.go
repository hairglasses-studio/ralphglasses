package components

import "github.com/charmbracelet/x/ansi"

// VisualWidth counts the printable width of a string in terminal cells,
// correctly handling ANSI escape sequences, wide characters (CJK), and emoji.
// Delegates to charmbracelet/x/ansi.StringWidth.
func VisualWidth(s string) int {
	return ansi.StringWidth(s)
}

// VisualTruncate truncates s to at most maxW visual columns,
// preserving ANSI escape sequences and handling wide characters.
// Delegates to charmbracelet/x/ansi.Truncate.
func VisualTruncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	return ansi.Truncate(s, maxW, "")
}

// StripAnsi removes all ANSI escape sequences from a string.
// Delegates to charmbracelet/x/ansi.Strip.
func StripAnsi(s string) string {
	return ansi.Strip(s)
}
