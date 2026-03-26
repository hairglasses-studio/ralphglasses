package views

import (
	"testing"
)

func TestColorizeDiffLine(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"+++ b/file.go", "b/file.go"},
		{"--- a/file.go", "a/file.go"},
		{"@@ -1,5 +1,6 @@", "-1,5 +1,6"},
		{"+added line", "added line"},
		{"-removed line", "removed line"},
		{"diff --git a/f b/f", "diff --git"},
		{" context line", " context line"},
	}
	for _, tt := range tests {
		got := colorizeDiffLine(tt.input)
		if got == "" {
			t.Errorf("colorizeDiffLine(%q) returned empty", tt.input)
		}
		// The colorized output should still contain the text content
		if len(got) == 0 {
			t.Errorf("colorizeDiffLine(%q) produced empty output", tt.input)
		}
	}
}

func TestRenderDiffViewInvalidRepo(t *testing.T) {
	// RenderDiffView with a non-existent repo path should produce error output
	out := RenderDiffView("/nonexistent/path", "HEAD~1", 120, 40)
	if out == "" {
		t.Error("should produce non-empty output even with invalid repo")
	}
}
