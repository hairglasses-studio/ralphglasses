package components

import "unicode/utf8"

// VisualWidth counts the printable rune width of a string,
// skipping ANSI escape sequences. CJK double-width characters are
// not accounted for (all runes count as 1) since the TUI uses
// Latin/symbol chars exclusively.
func VisualWidth(s string) int {
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

// VisualTruncate truncates s to at most maxW visual columns,
// preserving ANSI escape sequences so colors survive truncation.
// If truncation occurs, an ellipsis is appended (consuming 1 column).
func VisualTruncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}

	vis := 0
	inEsc := false
	var result []byte
	truncated := false
	i := 0

	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])

		if r == '\x1b' {
			inEsc = true
			result = append(result, s[i:i+size]...)
			i += size
			continue
		}
		if inEsc {
			result = append(result, s[i:i+size]...)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			i += size
			continue
		}

		// Printable character
		if vis >= maxW {
			truncated = true
			break
		}
		// Reserve space for ellipsis if this isn't the last possible char
		if vis == maxW-1 && i+size < len(s) && !isOnlyAnsiRemaining(s[i+size:]) {
			result = append(result, '\xe2', '\x80', '\xa6') // "…"
			truncated = true
			break
		}
		result = append(result, s[i:i+size]...)
		vis++
		i += size
	}

	_ = truncated

	// Close any open ANSI sequences with a reset
	if inEsc || hasOpenAnsi(s[:i]) {
		result = append(result, '\x1b', '[', '0', 'm')
	}

	return string(result)
}

// isOnlyAnsiRemaining returns true if the rest of the string contains
// only ANSI escape sequences and no printable characters.
func isOnlyAnsiRemaining(s string) bool {
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
		return false // found a printable character
	}
	return true
}

// hasOpenAnsi checks if the string contains an ANSI start without a reset.
func hasOpenAnsi(s string) bool {
	lastStart := -1
	lastReset := -1
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			lastStart = i
		}
		if i+3 < len(s) && s[i] == '\x1b' && s[i+1] == '[' && s[i+2] == '0' && s[i+3] == 'm' {
			lastReset = i
		}
	}
	return lastStart > lastReset
}
