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
func VisualTruncate(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}

	vis := 0
	inEsc := false
	var result []byte
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

		// Printable character — check budget
		if vis >= maxW {
			break
		}
		result = append(result, s[i:i+size]...)
		vis++
		i += size
	}

	return string(result)
}

// StripAnsi removes all ANSI escape sequences from a string.
func StripAnsi(s string) string {
	var result []byte
	inEsc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEsc = false
			}
			continue
		}
		result = append(result, s[i])
	}
	return string(result)
}
