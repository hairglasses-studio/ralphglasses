package session

import (
	"strings"
	"testing"
)

func TestIsExtraUsageExhausted_OutputHistory(t *testing.T) {
	s := &Session{
		OutputHistory: []string{
			"processing...",
			"You have run out of extra usage for this billing period.",
			"done",
		},
	}
	if !isExtraUsageExhausted(s) {
		t.Error("expected true when OutputHistory contains extra usage message")
	}
}

func TestIsExtraUsageExhausted_Error(t *testing.T) {
	s := &Session{
		Error: "Out of Extra Usage credits",
	}
	if !isExtraUsageExhausted(s) {
		t.Error("expected true when Error contains extra usage message")
	}
}

func TestIsExtraUsageExhausted_LastOutput(t *testing.T) {
	s := &Session{
		LastOutput: "You are out of extra usage for your plan",
	}
	if !isExtraUsageExhausted(s) {
		t.Error("expected true when LastOutput contains extra usage message")
	}
}

func TestIsExtraUsageExhausted_None(t *testing.T) {
	s := &Session{
		OutputHistory: []string{"all good", "still running"},
		Error:         "some other error",
		LastOutput:    "output completed",
	}
	if isExtraUsageExhausted(s) {
		t.Error("expected false when no extra usage message present")
	}
}

func TestIsExtraUsageExhausted_Empty(t *testing.T) {
	s := &Session{}
	if isExtraUsageExhausted(s) {
		t.Error("expected false for empty session")
	}
}

func TestIsExtraUsageExhausted_CaseInsensitive(t *testing.T) {
	s := &Session{
		Error: "OUT OF EXTRA USAGE",
	}
	if !isExtraUsageExhausted(s) {
		t.Error("expected case-insensitive match")
	}
}

func TestAppendSessionOutput_Basic(t *testing.T) {
	s := &Session{
		OutputCh: make(chan string, 10),
	}
	appendSessionOutput(s, "hello world", nil)

	if s.TotalOutputCount != 1 {
		t.Errorf("TotalOutputCount = %d, want 1", s.TotalOutputCount)
	}
	if s.LastOutput != "hello world" {
		t.Errorf("LastOutput = %q, want 'hello world'", s.LastOutput)
	}
	if len(s.OutputHistory) != 1 {
		t.Fatalf("OutputHistory len = %d, want 1", len(s.OutputHistory))
	}
	if s.OutputHistory[0] != "hello world" {
		t.Errorf("OutputHistory[0] = %q", s.OutputHistory[0])
	}
}

func TestAppendSessionOutput_Truncation(t *testing.T) {
	s := &Session{
		OutputCh: make(chan string, 10),
	}
	// Generate a string longer than 4000 characters
	long := strings.Repeat("x", 5000)
	appendSessionOutput(s, long, nil)

	if len(s.LastOutput) != 4000 {
		t.Errorf("LastOutput len = %d, want 4000 (truncated)", len(s.LastOutput))
	}
	// truncateStr keeps the tail
	if s.LastOutput[0] != 'x' {
		t.Error("expected truncated output to contain 'x' characters")
	}
}

func TestAppendSessionOutput_HistoryOverflow(t *testing.T) {
	s := &Session{
		OutputCh: make(chan string, 200),
	}
	// Add 110 entries — should be capped to 100
	for i := 0; i < 110; i++ {
		appendSessionOutput(s, "line", nil)
	}
	if len(s.OutputHistory) != 100 {
		t.Errorf("OutputHistory len = %d, want 100 (capped)", len(s.OutputHistory))
	}
	if s.TotalOutputCount != 110 {
		t.Errorf("TotalOutputCount = %d, want 110", s.TotalOutputCount)
	}
}

func TestAppendSessionOutput_ChannelFull(t *testing.T) {
	// Channel with no capacity — should not block
	s := &Session{
		OutputCh: make(chan string),
	}
	appendSessionOutput(s, "should not block", nil)
	if s.TotalOutputCount != 1 {
		t.Errorf("TotalOutputCount = %d, want 1", s.TotalOutputCount)
	}
}

func TestTruncateStr_ExtendedCases(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},          // shorter than max
		{"hello world", 5, "world"},     // keeps tail
		{"", 5, ""},                     // empty
		{"ab", 1, "b"},                  // single char tail
		{"abcdef", 3, "def"},            // exact truncation
		{strings.Repeat("x", 100), 50, strings.Repeat("x", 50)}, // large input
	}
	for _, tt := range tests {
		got := truncateStr(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}
