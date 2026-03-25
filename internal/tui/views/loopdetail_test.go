package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderLoopDetail_NilLoop(t *testing.T) {
	out := RenderLoopDetail(nil, 80, 40)
	if !strings.Contains(out, "No loop selected") {
		t.Errorf("nil loop: expected 'No loop selected', got: %q", out)
	}
}

func TestRenderLoopDetail_Fields(t *testing.T) {
	now := time.Now().Add(-5 * time.Minute)
	l := &session.LoopRun{
		ID:        "abc12345-xxxx",
		RepoName:  "my-repo",
		Status:    "running",
		CreatedAt: now,
		Iterations: []session.LoopIteration{
			{
				Number: 3,
				Status: "completed",
				Task:   session.LoopTask{Title: "Refactor auth module"},
			},
		},
	}

	out := RenderLoopDetail(l, 80, 40)

	checks := []struct {
		label string
		want  string
	}{
		{"loop ID prefix", "abc12345"},
		{"repo name", "my-repo"},
		{"status", "running"},
		{"iteration count", "1"},
		{"last iteration number", "3"},
		{"last task title", "Refactor auth module"},
		{"elapsed present", "m"},
		{"help hint", "Esc"},
	}

	for _, c := range checks {
		if !strings.Contains(out, c.want) {
			t.Errorf("%s: expected %q in output", c.label, c.want)
		}
	}
}

func TestRenderLoopDetail_WithError(t *testing.T) {
	l := &session.LoopRun{
		ID:        "err00001",
		RepoName:  "failing-repo",
		Status:    "failed",
		LastError: "verification command exited with code 1",
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}

	out := RenderLoopDetail(l, 80, 40)

	if !strings.Contains(out, "verification command exited with code 1") {
		t.Errorf("expected last error in output, got: %q", out)
	}
	if !strings.Contains(out, "failed") {
		t.Errorf("expected 'failed' status in output, got: %q", out)
	}
}
