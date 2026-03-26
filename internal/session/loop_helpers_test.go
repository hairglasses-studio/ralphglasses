package session

import (
	"testing"
)

func TestSanitizeLoopName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  My Loop  ", "my-loop"},
		{"my_loop_name", "my-loop-name"},
		{"UPPER CASE!", "upper-case"},
		{"", "loop"},
		{"  ", "loop"},
		{"special@chars#here", "special-chars-here"},
		{"---leading-trailing---", "leading-trailing"},
	}
	for _, tt := range tests {
		if got := sanitizeLoopName(tt.input); got != tt.want {
			t.Errorf("sanitizeLoopName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateForPrompt(t *testing.T) {
	tests := []struct {
		input string
		limit int
		want  string
	}{
		{"short", 100, "short"},
		{"hello world", 5, "he..."},
		{"abc", 0, "abc"},
		{"  padded  ", 100, "padded"},
		{"exactly10!", 10, "exactly10!"},
	}
	for _, tt := range tests {
		if got := truncateForPrompt(tt.input, tt.limit); got != tt.want {
			t.Errorf("truncateForPrompt(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"first\nsecond\nthird", "first"},
		{"only line", "only line"},
		{"", ""},
		{"\n\nthird", "third"},
	}
	for _, tt := range tests {
		if got := firstLine(tt.input); got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestJoinOrPlaceholder(t *testing.T) {
	if got := joinOrPlaceholder(nil, "default"); got != "default" {
		t.Errorf("nil items: got %q, want default", got)
	}
	if got := joinOrPlaceholder([]string{}, "default"); got != "default" {
		t.Errorf("empty items: got %q, want default", got)
	}
	if got := joinOrPlaceholder([]string{"a", "b"}, "default"); got != "a\nb" {
		t.Errorf("with items: got %q, want a\\nb", got)
	}
}

func TestFirstNonBlank(t *testing.T) {
	if got := firstNonBlank("", "  ", "hello"); got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
	if got := firstNonBlank("", ""); got != "" {
		t.Errorf("all blank: got %q, want empty", got)
	}
	if got := firstNonBlank("first", "second"); got != "first" {
		t.Errorf("got %q, want first", got)
	}
}

func TestConsecutiveLoopFailures(t *testing.T) {
	iters := []LoopIteration{
		{Status: "completed"},
		{Status: "failed"},
		{Status: "failed"},
		{Status: "failed"},
	}
	if got := consecutiveLoopFailures(iters); got != 3 {
		t.Errorf("got %d, want 3", got)
	}

	// No failures
	iters2 := []LoopIteration{{Status: "completed"}}
	if got := consecutiveLoopFailures(iters2); got != 0 {
		t.Errorf("got %d, want 0", got)
	}

	// Empty
	if got := consecutiveLoopFailures(nil); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestMapProvider(t *testing.T) {
	tests := []struct {
		input Provider
		want  string
	}{
		{ProviderGemini, "gemini"},
		{ProviderCodex, "openai"},
		{ProviderClaude, "claude"},
		{"", "claude"},
	}
	for _, tt := range tests {
		got := mapProvider(tt.input)
		if string(got) != tt.want {
			t.Errorf("mapProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSessionOutputSummary(t *testing.T) {
	s := &Session{
		OutputHistory: []string{"line1", "line2"},
		LastOutput:    "last",
		Error:         "err",
	}
	got := sessionOutputSummary(s)
	if got == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSessionOutputSummary_Empty(t *testing.T) {
	s := &Session{}
	got := sessionOutputSummary(s)
	if got != "" {
		t.Errorf("expected empty summary, got %q", got)
	}
}
