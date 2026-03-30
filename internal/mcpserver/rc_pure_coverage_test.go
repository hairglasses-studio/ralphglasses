package mcpserver

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestWindowType_Rolling(t *testing.T) {
	got := windowType(24.0)
	if got != "rolling" {
		t.Errorf("windowType(24) = %q, want rolling", got)
	}
}

func TestWindowType_All(t *testing.T) {
	got := windowType(0)
	if got != "all" {
		t.Errorf("windowType(0) = %q, want all", got)
	}
}

func TestWindowType_Negative(t *testing.T) {
	got := windowType(-1)
	if got != "all" {
		t.Errorf("windowType(-1) = %q, want all", got)
	}
}

func TestSummarizeDefault_WithRepoAndData(t *testing.T) {
	e := events.Event{
		RepoName: "myrepo",
		Data:     map[string]any{"key": "value"},
	}
	got := summarizeDefault(e)
	if got == "" {
		t.Error("summarizeDefault with repo and data should return non-empty string")
	}
}

func TestSummarizeDefault_WithRepoOnly(t *testing.T) {
	e := events.Event{RepoName: "myrepo"}
	got := summarizeDefault(e)
	if got != "myrepo" {
		t.Errorf("summarizeDefault with repo = %q, want myrepo", got)
	}
}

func TestSummarizeDefault_WithDataOnly(t *testing.T) {
	e := events.Event{
		Data: map[string]any{"status": "ok"},
	}
	got := summarizeDefault(e)
	if got == "" {
		t.Error("summarizeDefault with data only should return non-empty string")
	}
}

func TestSummarizeDefault_Empty(t *testing.T) {
	e := events.Event{}
	got := summarizeDefault(e)
	if got != "" {
		t.Errorf("summarizeDefault empty event = %q, want empty", got)
	}
}

func TestSummarizeScanComplete_WithRepoCount(t *testing.T) {
	data := map[string]any{"repo_count": float64(5)}
	got := summarizeScanComplete(data)
	if got != "found 5 repos" {
		t.Errorf("summarizeScanComplete = %q, want 'found 5 repos'", got)
	}
}

func TestSummarizeScanComplete_WithCount(t *testing.T) {
	data := map[string]any{"count": float64(3)}
	got := summarizeScanComplete(data)
	if got != "found 3 repos" {
		t.Errorf("summarizeScanComplete = %q, want 'found 3 repos'", got)
	}
}

func TestSummarizeScanComplete_Empty(t *testing.T) {
	got := summarizeScanComplete(map[string]any{})
	if got != "scan finished" {
		t.Errorf("summarizeScanComplete empty = %q, want 'scan finished'", got)
	}
}

func TestSummarizeLoopIterated_StepAndStatus(t *testing.T) {
	data := map[string]any{"step": float64(2), "status": "passed"}
	got := summarizeLoopIterated(data)
	if got != "step 2: passed" {
		t.Errorf("summarizeLoopIterated = %q, want 'step 2: passed'", got)
	}
}

func TestSummarizeLoopIterated_StepOnly(t *testing.T) {
	data := map[string]any{"step": float64(3)}
	got := summarizeLoopIterated(data)
	if got != "step 3" {
		t.Errorf("summarizeLoopIterated step only = %q, want 'step 3'", got)
	}
}

func TestSummarizeLoopIterated_StatusOnly(t *testing.T) {
	data := map[string]any{"status": "running"}
	got := summarizeLoopIterated(data)
	if got != "running" {
		t.Errorf("summarizeLoopIterated status only = %q, want 'running'", got)
	}
}

func TestSummarizeLoopIterated_Empty(t *testing.T) {
	got := summarizeLoopIterated(map[string]any{})
	if got != "iteration" {
		t.Errorf("summarizeLoopIterated empty = %q, want 'iteration'", got)
	}
}
