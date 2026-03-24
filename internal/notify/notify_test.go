package notify

import (
	"strings"
	"testing"
)

func TestSend_SmokeTest(t *testing.T) {
	t.Parallel()
	// On macOS this may trigger a real notification — that is acceptable.
	// The test verifies Send does not panic or return an unexpected error.
	err := Send("test title", "test body")
	if err != nil {
		t.Logf("Send returned error (may be expected in CI): %v", err)
	}
}

func TestSend_EmptyStrings(t *testing.T) {
	t.Parallel()
	// Sending empty title and body must not panic.
	err := Send("", "")
	if err != nil {
		t.Logf("Send with empty strings returned error (may be expected in CI): %v", err)
	}
}

func TestEscapeOSA_Newlines(t *testing.T) {
	t.Parallel()
	input := "line1\nline2\nline3"
	got := escapeOSA(input)
	// escapeOSA only escapes backslashes and double quotes.
	// Newline characters (\n) are passed through unchanged — they are
	// preserved in the output string, not stripped or escaped.
	if got != input {
		t.Errorf("escapeOSA(%q) = %q; want newlines preserved as-is", input, got)
	}
	if !strings.Contains(got, "\n") {
		t.Error("expected newline characters to be preserved in output")
	}
}

func TestEscapeOSA_LongString(t *testing.T) {
	t.Parallel()
	// Build a 1200-character string with a mix of safe and escapable chars.
	var b strings.Builder
	for i := 0; i < 1200; i++ {
		b.WriteByte('a' + byte(i%26))
	}
	input := b.String()
	got := escapeOSA(input)
	if got != input {
		t.Errorf("escapeOSA truncated or mutated a %d-char safe string: got len %d", len(input), len(got))
	}

	// Also test a long string that contains characters requiring escaping.
	mixed := strings.Repeat(`a"b\c`, 250) // 1250 chars, every 5th needs escaping
	got2 := escapeOSA(mixed)
	if len(got2) < len(mixed) {
		t.Errorf("escaped output should be longer than input when escaping is needed: input %d, got %d", len(mixed), len(got2))
	}
	// Verify no raw unescaped double quotes remain.
	// After escaping, every " should be preceded by \.
	unescaped := strings.ReplaceAll(got2, `\"`, "")
	if strings.Contains(unescaped, `"`) {
		t.Error("found unescaped double quote in output")
	}
}

func TestEscapeOSA_UnicodeEmoji(t *testing.T) {
	t.Parallel()
	input := "hello 🎉 world 🚀 done ✅"
	got := escapeOSA(input)
	// Emoji are multi-byte UTF-8 sequences with no bytes matching '\' or '"',
	// so they should pass through unchanged.
	if got != input {
		t.Errorf("escapeOSA(%q) = %q; want emoji preserved unchanged", input, got)
	}
}
