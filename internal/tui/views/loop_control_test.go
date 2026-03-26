package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderLoopControl_Empty(t *testing.T) {
	data := LoopControlData{Loops: nil, Selected: 0}
	out := RenderLoopControl(data, 120, 40)
	if !strings.Contains(out, "No active loops") {
		t.Errorf("empty panel should show 'No active loops', got: %q", out)
	}
	if !strings.Contains(out, "Loop Control Panel") {
		t.Errorf("should show panel title, got: %q", out)
	}
	if !strings.Contains(out, "Esc back") {
		t.Errorf("should show help footer, got: %q", out)
	}
}

func TestRenderLoopControl_Populated(t *testing.T) {
	now := time.Now()

	running := &session.LoopRun{
		ID:        "aaaa1111bbbb2222",
		RepoName:  "my-running-repo",
		Status:    "running",
		CreatedAt: now.Add(-10 * time.Minute),
	}
	running.Iterations = []session.LoopIteration{
		{
			Number:    1,
			Status:    "done",
			StartedAt: now.Add(-5 * time.Minute),
			EndedAt:   func() *time.Time { t := now.Add(-3 * time.Minute); return &t }(),
			Task:      session.LoopTask{Title: "Fix the tests"},
		},
	}

	paused := &session.LoopRun{
		ID:        "cccc3333dddd4444",
		RepoName:  "my-paused-repo",
		Status:    "running",
		Paused:    true,
		CreatedAt: now.Add(-20 * time.Minute),
	}
	paused.Iterations = []session.LoopIteration{
		{
			Number:    1,
			Status:    "done",
			StartedAt: now.Add(-15 * time.Minute),
			EndedAt:   func() *time.Time { t := now.Add(-12 * time.Minute); return &t }(),
		},
	}

	data := LoopControlData{
		Loops:    []*session.LoopRun{running, paused},
		Selected: 0,
	}
	out := RenderLoopControl(data, 120, 40)

	if !strings.Contains(out, "aaaa1111") {
		t.Errorf("should show running loop ID prefix, got: %q", out)
	}
	if !strings.Contains(out, "my-running-repo") {
		t.Errorf("should show running repo name, got: %q", out)
	}
	if !strings.Contains(out, "paused") {
		t.Errorf("should show paused status for paused loop, got: %q", out)
	}
	if !strings.Contains(out, "cccc3333") {
		t.Errorf("should show paused loop ID prefix, got: %q", out)
	}
	// Selected loop (index 0) should have inline detail
	if !strings.Contains(out, "iterations:") {
		t.Errorf("selected loop should show inline detail with iterations, got: %q", out)
	}
	// Average duration should appear (2 minutes for the running loop)
	if !strings.Contains(out, "avg:") {
		t.Errorf("should show average iteration duration, got: %q", out)
	}
	if !strings.Contains(out, "j/k navigate") {
		t.Errorf("should show navigation help, got: %q", out)
	}
}

func TestRenderLoopControl_StoppedEstimate(t *testing.T) {
	now := time.Now()
	ended := now.Add(-30 * time.Second)

	stopped := &session.LoopRun{
		ID:        "eeee5555ffff6666",
		RepoName:  "stopped-repo",
		Status:    "stopped",
		CreatedAt: now.Add(-5 * time.Minute),
	}
	stopped.Iterations = []session.LoopIteration{
		{
			Number:    1,
			Status:    "done",
			StartedAt: now.Add(-4 * time.Minute),
			EndedAt:   &ended,
		},
	}

	data := LoopControlData{
		Loops:    []*session.LoopRun{stopped},
		Selected: 0,
	}
	out := RenderLoopControl(data, 120, 40)

	// Stopped loop should not show a "next" estimate
	if strings.Contains(out, "next:") {
		t.Errorf("stopped loop should not show next iteration estimate, got: %q", out)
	}
	// Should still show avg duration
	if !strings.Contains(out, "avg:") {
		t.Errorf("should show average iteration duration for stopped loop, got: %q", out)
	}
}

func TestAvgLoopIterDuration_NoCompleted(t *testing.T) {
	iters := []session.LoopIteration{
		{Number: 1, StartedAt: time.Now()}, // no EndedAt
	}
	if d := avgLoopIterDuration(iters); d != 0 {
		t.Errorf("expected 0 when no iterations have EndedAt, got %v", d)
	}
}

func TestAvgLoopIterDuration_Multiple(t *testing.T) {
	now := time.Now()
	e1 := now.Add(-8 * time.Minute)
	e2 := now.Add(-3 * time.Minute)
	iters := []session.LoopIteration{
		{Number: 1, StartedAt: now.Add(-10 * time.Minute), EndedAt: &e1}, // 2m
		{Number: 2, StartedAt: now.Add(-5 * time.Minute), EndedAt: &e2},  // 2m
	}
	d := avgLoopIterDuration(iters)
	if d < time.Minute*1 || d > time.Minute*3 {
		t.Errorf("expected ~2m average, got %v", d)
	}
}

func TestLoopControlTruncate(t *testing.T) {
	tests := []struct {
		input  string
		max    int
		expect string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello w…"},
		{"hi", 2, "h…"},
		{"x", 1, "…"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := loopControlTruncate(tt.input, tt.max)
		if got != tt.expect {
			t.Errorf("loopControlTruncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.expect)
		}
	}
}
