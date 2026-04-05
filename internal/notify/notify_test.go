package notify

import (
	"os"
	"strings"
	"testing"
)

func TestSend_SmokeTest(t *testing.T) {
	t.Parallel()
	if os.Getenv("NOTIFY_LIVE_TEST") == "" {
		t.Skip("set NOTIFY_LIVE_TEST=1 to send real desktop notifications")
	}
	err := Send("test title", "test body")
	if err != nil {
		t.Logf("Send returned error (may be expected in CI): %v", err)
	}
}

func TestSend_EmptyStrings(t *testing.T) {
	t.Parallel()
	if os.Getenv("NOTIFY_LIVE_TEST") == "" {
		t.Skip("set NOTIFY_LIVE_TEST=1 to send real desktop notifications")
	}
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
	for i := range 1200 {
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

func TestEscapeOSA_EmptyString(t *testing.T) {
	t.Parallel()
	got := escapeOSA("")
	if got != "" {
		t.Errorf("escapeOSA(%q) = %q; want empty string", "", got)
	}
}

func TestEscapeOSA_Backslashes(t *testing.T) {
	t.Parallel()
	input := `a\b\\c`
	got := escapeOSA(input)
	want := `a\\b\\\\c`
	if got != want {
		t.Errorf("escapeOSA(%q) = %q; want %q", input, got, want)
	}
}

func TestEscapeOSA_DoubleQuotes(t *testing.T) {
	t.Parallel()
	input := `say "hello" now`
	got := escapeOSA(input)
	want := `say \"hello\" now`
	if got != want {
		t.Errorf("escapeOSA(%q) = %q; want %q", input, got, want)
	}
}

func TestEscapeOSA_MixedSpecialChars(t *testing.T) {
	t.Parallel()
	input := `path \"C:\Users\test"`
	got := escapeOSA(input)
	// \ -> \\, " -> \"
	want := `path \\\"C:\\Users\\test\"`
	if got != want {
		t.Errorf("escapeOSA(%q) = %q; want %q", input, got, want)
	}
}

func TestEscapeOSA_OnlySpecialChars(t *testing.T) {
	t.Parallel()
	input := `""\`
	got := escapeOSA(input)
	want := `\"\"\\`
	if got != want {
		t.Errorf("escapeOSA(%q) = %q; want %q", input, got, want)
	}
}

func TestSendForOS_DefaultNoop(t *testing.T) {
	t.Parallel()
	err := sendForOS("freebsd", "title", "body")
	if err != nil {
		t.Errorf("sendForOS(freebsd) returned error: %v", err)
	}
}

func TestSendForOS_LinuxNoNotifySend(t *testing.T) {
	t.Parallel()
	// On macOS, notify-send won't be found, so this exercises the LookPath failure path.
	// On Linux without notify-send installed, same.
	err := sendForOS("linux", "title", "body")
	// Either nil (notify-send not found) or an error from running it
	_ = err
}

func TestSendForOS_UnknownOS(t *testing.T) {
	t.Parallel()
	err := sendForOS("plan9", "title", "body")
	if err != nil {
		t.Errorf("sendForOS(plan9) should be no-op, got error: %v", err)
	}
}

func TestSend_NoPanic(t *testing.T) {
	t.Parallel()
	if os.Getenv("NOTIFY_LIVE_TEST") == "" {
		t.Skip("set NOTIFY_LIVE_TEST=1 to send real desktop notifications")
	}
	inputs := []struct{ title, body string }{
		{"", ""},
		{"title", ""},
		{"", "body"},
		{`say "hi"`, `path\to\file`},
		{strings.Repeat("x", 10000), strings.Repeat("y", 10000)},
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Send(%q, %q) panicked: %v", in.title, in.body, r)
				}
			}()
			_ = Send(in.title, in.body)
		}()
	}
}
