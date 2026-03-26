package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderLoopControlPanelEmpty(t *testing.T) {
	out := RenderLoopControlPanel(nil, 0, 120, 40)
	if !strings.Contains(out, "No active loops") {
		t.Error("empty panel should show 'No active loops'")
	}
	if !strings.Contains(out, "Loop Control Panel") {
		t.Error("should contain title")
	}
}

func TestRenderLoopControlPanelWithLoops(t *testing.T) {
	now := time.Now()
	ended := now.Add(-10 * time.Second)
	loops := []*session.LoopRun{
		func() *session.LoopRun {
			l := &session.LoopRun{
				ID:        "aaaa-bbbb-cccc-dddd",
				RepoName:  "testrepo",
				Status:    "running",
				CreatedAt: now.Add(-5 * time.Minute),
				Iterations: []session.LoopIteration{
					{
						Number:    1,
						Status:    "completed",
						Task:      session.LoopTask{Title: "Fix widget"},
						StartedAt: now.Add(-60 * time.Second),
						EndedAt:   &ended,
					},
				},
			}
			return l
		}(),
		func() *session.LoopRun {
			l := &session.LoopRun{
				ID:        "eeee-ffff-0000-1111",
				RepoName:  "otherrepo",
				Status:    "running",
				Paused:    true,
				CreatedAt: now.Add(-10 * time.Minute),
			}
			return l
		}(),
	}

	data := SnapshotLoopControl(loops)
	if len(data) != 2 {
		t.Fatalf("expected 2 items, got %d", len(data))
	}

	// First loop should have iteration data
	if data[0].LastIterTask != "Fix widget" {
		t.Errorf("expected task 'Fix widget', got %q", data[0].LastIterTask)
	}
	if data[0].NextEstimate == "—" {
		t.Error("running loop should not have '—' estimate")
	}

	// Second loop should be paused
	if data[1].NextEstimate != "paused" {
		t.Errorf("paused loop estimate = %q, want 'paused'", data[1].NextEstimate)
	}

	// Render with first selected
	out := RenderLoopControlPanel(data, 0, 120, 40)
	if !strings.Contains(out, "Loop Control Panel") {
		t.Error("should contain title")
	}
	if !strings.Contains(out, "testrepo") {
		t.Error("should contain repo name")
	}
	if !strings.Contains(out, "otherrepo") {
		t.Error("should contain second repo")
	}
	if !strings.Contains(out, "Fix widget") {
		t.Error("selected loop should show task detail")
	}
	if !strings.Contains(out, "running") {
		t.Error("should contain running status")
	}
	if !strings.Contains(out, "paused") {
		t.Error("should contain paused status")
	}
	if !strings.Contains(out, "force-step") {
		t.Error("help text should mention force-step")
	}

	// Render with second selected — should NOT show "Fix widget" as inline detail
	out2 := RenderLoopControlPanel(data, 1, 120, 40)
	if !strings.Contains(out2, "otherrepo") {
		t.Error("should contain second repo")
	}
}

func TestSnapshotLoopControlStoppedLoop(t *testing.T) {
	loops := []*session.LoopRun{
		{
			ID:        "stop-1234",
			RepoName:  "stopped-repo",
			Status:    "stopped",
			CreatedAt: time.Now().Add(-1 * time.Hour),
		},
	}
	data := SnapshotLoopControl(loops)
	if len(data) != 1 {
		t.Fatalf("expected 1, got %d", len(data))
	}
	if data[0].NextEstimate != "—" {
		t.Errorf("stopped loop should have '—', got %q", data[0].NextEstimate)
	}
}
