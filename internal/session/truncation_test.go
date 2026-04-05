package session

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestTruncator_NoTruncation(t *testing.T) {
	tr := NewTruncator()
	input := "short output"
	result := tr.Truncate(input)

	if result.Truncated {
		t.Error("expected Truncated=false for short input")
	}
	if result.Output != input {
		t.Errorf("output changed: got %q, want %q", result.Output, input)
	}
	if result.OriginalSize != len(input) {
		t.Errorf("OriginalSize = %d, want %d", result.OriginalSize, len(input))
	}
}

func TestTruncator_TruncatesLargeOutput(t *testing.T) {
	tr := NewTruncatorWithSize(200)
	input := strings.Repeat("a", 500)
	result := tr.Truncate(input)

	if !result.Truncated {
		t.Error("expected Truncated=true for oversized input")
	}
	if result.OriginalSize != 500 {
		t.Errorf("OriginalSize = %d, want 500", result.OriginalSize)
	}
	marker := fmt.Sprintf(truncationMarkerFmt, 200)
	if !strings.Contains(result.Output, marker) {
		t.Errorf("output missing truncation marker, got: %s", result.Output)
	}
	// Truncated output must be shorter than or equal to the original.
	if len(result.Output) >= len(input) {
		t.Errorf("truncated output (%d bytes) not shorter than original (%d bytes)", len(result.Output), len(input))
	}
}

func TestTruncator_PreservesJSONStructure(t *testing.T) {
	tr := NewTruncatorWithSize(100)
	// Build a JSON object that exceeds 100 bytes.
	input := `{"key": "value", "nested": {"inner": [1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20]}}`
	result := tr.Truncate(input)

	if !result.Truncated {
		t.Fatal("expected truncation")
	}

	// The output should end with the truncation marker, but the JSON
	// portion before it should have balanced braces/brackets.
	marker := fmt.Sprintf(truncationMarkerFmt, 100)
	idx := strings.Index(result.Output, marker)
	if idx < 0 {
		t.Fatal("missing truncation marker")
	}
	jsonPart := strings.TrimSpace(result.Output[:idx])

	// Count open/close braces and brackets.
	opens := strings.Count(jsonPart, "{") + strings.Count(jsonPart, "[")
	closes := strings.Count(jsonPart, "}") + strings.Count(jsonPart, "]")
	if opens != closes {
		t.Errorf("unbalanced JSON structure: %d opens, %d closes in: %s", opens, closes, jsonPart)
	}
}

func TestTruncator_SetMaxOutputSize(t *testing.T) {
	tr := NewTruncator()
	if tr.MaxOutputSize() != DefaultMaxOutputSize {
		t.Errorf("initial max = %d, want %d", tr.MaxOutputSize(), DefaultMaxOutputSize)
	}

	tr.SetMaxOutputSize(1024)
	if tr.MaxOutputSize() != 1024 {
		t.Errorf("after SetMaxOutputSize(1024): got %d", tr.MaxOutputSize())
	}

	// Zero resets to default.
	tr.SetMaxOutputSize(0)
	if tr.MaxOutputSize() != DefaultMaxOutputSize {
		t.Errorf("after SetMaxOutputSize(0): got %d, want %d", tr.MaxOutputSize(), DefaultMaxOutputSize)
	}
}

func TestTruncator_ThreadSafety(t *testing.T) {
	tr := NewTruncatorWithSize(100)
	var wg sync.WaitGroup

	// Concurrent reads and writes.
	for i := range 50 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			tr.SetMaxOutputSize(100 + n)
		}(i)
		go func() {
			defer wg.Done()
			tr.Truncate(strings.Repeat("x", 200))
		}()
	}
	wg.Wait()
}

func TestCloseJSONStructure(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "balanced",
			input: `{"a": 1}`,
			want:  `{"a": 1}`,
		},
		{
			name:  "open object",
			input: `{"a": 1`,
			want:  `{"a": 1}`,
		},
		{
			name:  "open array in object",
			input: `{"a": [1, 2`,
			want:  `{"a": [1, 2]}`,
		},
		{
			name:  "nested open",
			input: `{"a": {"b": [1`,
			want:  `{"a": {"b": [1]}}`,
		},
		{
			name:  "string with bracket",
			input: `{"a": "hello [world"`,
			want:  `{"a": "hello [world"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := closeJSONStructure(tc.input)
			if got != tc.want {
				t.Errorf("closeJSONStructure(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
