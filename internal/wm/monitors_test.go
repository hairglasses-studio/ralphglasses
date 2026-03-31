package wm

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/wm/hyprland"
	"github.com/hairglasses-studio/ralphglasses/internal/wm/sway"
)

// --- xrandr mock outputs ---

// sevenMonitorXrandr simulates a 7-monitor thin client (dual-GPU RTX 4090 setup).
const sevenMonitorXrandr = `Screen 0: minimum 8 x 8, current 15360 x 4320, maximum 32768 x 32768
DP-1 connected primary 3840x2160+0+0 (normal left inverted right x axis y axis) 600mm x 340mm
   3840x2160     60.00*+  30.00
   2560x1440     59.95
DP-2 connected 3840x2160+3840+0 (normal left inverted right x axis y axis) 600mm x 340mm
   3840x2160     60.00*+  30.00
DP-3 connected 2560x1440+7680+0 (normal left inverted right x axis y axis) 550mm x 310mm
   2560x1440     144.00*+ 120.00    60.00
HDMI-1 connected 1920x1080+10240+0 (normal left inverted right x axis y axis) 530mm x 300mm
   1920x1080     60.00*+  50.00
DP-4 connected 1920x1080+12160+0 (normal left inverted right x axis y axis) 530mm x 300mm
   1920x1080     60.00*+
DP-5 connected 1920x1080+14080+0 (normal left inverted right x axis y axis) 530mm x 300mm
   1920x1080     60.00*+
DP-6 connected 1920x1080+14080+1080 (normal left inverted right x axis y axis) 530mm x 300mm
   1920x1080     60.00*+
DP-7 disconnected (normal left inverted right x axis y axis)
`

// dualMonitorXrandr simulates a typical dual-monitor developer setup.
const dualMonitorXrandr = `Screen 0: minimum 8 x 8, current 5760 x 2160, maximum 32768 x 32768
eDP-1 connected primary 1920x1080+0+0 (normal left inverted right x axis y axis) 344mm x 194mm
   1920x1080     60.01*+  60.01    59.97
   1366x768      59.79    60.00
DP-1 connected 3840x2160+1920+0 (normal left inverted right x axis y axis) 600mm x 340mm
   3840x2160     60.00*+  30.00
   2560x1440     59.95
VGA-1 disconnected (normal left inverted right x axis y axis)
`

// singleMonitorXrandr simulates a single laptop display.
const singleMonitorXrandr = `Screen 0: minimum 8 x 8, current 1920 x 1080, maximum 32768 x 32768
eDP-1 connected primary 1920x1080+0+0 (normal left inverted right x axis y axis) 344mm x 194mm
   1920x1080     60.01*+
   1366x768      59.79
HDMI-1 disconnected (normal left inverted right x axis y axis)
DP-1 disconnected (normal left inverted right x axis y axis)
`

// noModeXrandr simulates a connected output with no active mode.
const noModeXrandr = `Screen 0: minimum 8 x 8, current 0 x 0, maximum 32768 x 32768
DP-1 connected (normal left inverted right x axis y axis)
   3840x2160     60.00 +
   2560x1440     59.95
`

func TestParseXrandrOutput_SevenMonitors(t *testing.T) {
	monitors, err := ParseXrandrOutput(sevenMonitorXrandr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(monitors) != 7 {
		t.Fatalf("expected 7 monitors, got %d", len(monitors))
	}

	// Verify primary monitor.
	dp1 := monitors[0]
	if dp1.Name != "DP-1" {
		t.Errorf("expected first monitor DP-1, got %s", dp1.Name)
	}
	if !dp1.Primary {
		t.Error("DP-1 should be primary")
	}
	if dp1.Width != 3840 || dp1.Height != 2160 {
		t.Errorf("DP-1 resolution: expected 3840x2160, got %dx%d", dp1.Width, dp1.Height)
	}
	if dp1.OffsetX != 0 || dp1.OffsetY != 0 {
		t.Errorf("DP-1 offset: expected 0+0, got %d+%d", dp1.OffsetX, dp1.OffsetY)
	}
	if dp1.RefreshHz != 60.00 {
		t.Errorf("DP-1 refresh: expected 60.00, got %.2f", dp1.RefreshHz)
	}

	// Verify a secondary monitor.
	dp3 := monitors[2]
	if dp3.Name != "DP-3" {
		t.Errorf("expected third monitor DP-3, got %s", dp3.Name)
	}
	if dp3.Primary {
		t.Error("DP-3 should not be primary")
	}
	if dp3.Width != 2560 || dp3.Height != 1440 {
		t.Errorf("DP-3 resolution: expected 2560x1440, got %dx%d", dp3.Width, dp3.Height)
	}
	if dp3.RefreshHz != 144.00 {
		t.Errorf("DP-3 refresh: expected 144.00, got %.2f", dp3.RefreshHz)
	}

	// Verify the stacked monitor (non-zero Y offset).
	dp6 := monitors[6]
	if dp6.Name != "DP-6" {
		t.Errorf("expected seventh monitor DP-6, got %s", dp6.Name)
	}
	if dp6.OffsetY != 1080 {
		t.Errorf("DP-6 offset_y: expected 1080, got %d", dp6.OffsetY)
	}

	// Verify disconnected DP-7 is excluded.
	for _, m := range monitors {
		if m.Name == "DP-7" {
			t.Error("disconnected DP-7 should not be included")
		}
	}
}

func TestParseXrandrOutput_DualMonitor(t *testing.T) {
	monitors, err := ParseXrandrOutput(dualMonitorXrandr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}

	// Laptop display is primary.
	if !monitors[0].Primary {
		t.Error("eDP-1 should be primary")
	}
	if monitors[0].Width != 1920 {
		t.Errorf("eDP-1 width: expected 1920, got %d", monitors[0].Width)
	}

	// External display is larger.
	if monitors[1].Name != "DP-1" {
		t.Errorf("expected DP-1, got %s", monitors[1].Name)
	}
	if monitors[1].Area() != 3840*2160 {
		t.Errorf("DP-1 area: expected %d, got %d", 3840*2160, monitors[1].Area())
	}
}

func TestParseXrandrOutput_SingleMonitor(t *testing.T) {
	monitors, err := ParseXrandrOutput(singleMonitorXrandr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}
	if monitors[0].Name != "eDP-1" {
		t.Errorf("expected eDP-1, got %s", monitors[0].Name)
	}
}

func TestParseXrandrOutput_ConnectedNoMode(t *testing.T) {
	monitors, err := ParseXrandrOutput(noModeXrandr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}
	// Connected but no active mode: resolution should be zero.
	if monitors[0].Width != 0 || monitors[0].Height != 0 {
		t.Errorf("expected 0x0 for no-mode output, got %dx%d", monitors[0].Width, monitors[0].Height)
	}
}

func TestParseXrandrOutput_Empty(t *testing.T) {
	_, err := ParseXrandrOutput("")
	if err == nil {
		t.Error("expected error for empty output")
	}
}

func TestParseXrandrOutput_AllDisconnected(t *testing.T) {
	input := `Screen 0: minimum 8 x 8, current 0 x 0, maximum 32768 x 32768
DP-1 disconnected (normal left inverted right x axis y axis)
HDMI-1 disconnected (normal left inverted right x axis y axis)
`
	_, err := ParseXrandrOutput(input)
	if err == nil {
		t.Error("expected error when all monitors disconnected")
	}
	if !strings.Contains(err.Error(), "no connected monitors") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMonitor_Area(t *testing.T) {
	m := Monitor{Width: 3840, Height: 2160}
	if m.Area() != 8294400 {
		t.Errorf("expected 8294400, got %d", m.Area())
	}
}

// --- AssignWorkspaces tests ---

func TestAssignWorkspaces_EmptyInputs(t *testing.T) {
	// No monitors.
	plan := AssignWorkspaces(nil, []SessionInfo{{ID: "s1"}})
	if len(plan.Assignments) != 0 {
		t.Errorf("expected 0 assignments with no monitors, got %d", len(plan.Assignments))
	}

	// No sessions.
	plan = AssignWorkspaces([]Monitor{{Name: "DP-1", Width: 1920, Height: 1080, Connected: true}}, nil)
	if len(plan.Assignments) != 0 {
		t.Errorf("expected 0 assignments with no sessions, got %d", len(plan.Assignments))
	}
}

func TestAssignWorkspaces_SingleMonitorMultipleSessions(t *testing.T) {
	monitors := []Monitor{
		{Name: "eDP-1", Width: 1920, Height: 1080, Primary: true, Connected: true},
	}
	sessions := []SessionInfo{
		{ID: "s1", Name: "build", Priority: 0},
		{ID: "s2", Name: "test", Priority: 1},
		{ID: "s3", Name: "lint", Priority: 2},
	}

	plan := AssignWorkspaces(monitors, sessions)
	if len(plan.Assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(plan.Assignments))
	}

	// All should go to the single monitor.
	for i, a := range plan.Assignments {
		if a.MonitorName != "eDP-1" {
			t.Errorf("assignment[%d]: expected monitor eDP-1, got %s", i, a.MonitorName)
		}
	}

	// Highest priority session should be assigned first.
	if plan.Assignments[0].SessionID != "s1" {
		t.Errorf("expected s1 (priority 0) first, got %s", plan.Assignments[0].SessionID)
	}

	// Workspace names should be sequential on the same monitor.
	expected := []string{"ws-1-1", "ws-1-2", "ws-1-3"}
	for i, a := range plan.Assignments {
		if a.Workspace != expected[i] {
			t.Errorf("assignment[%d]: expected workspace %s, got %s", i, expected[i], a.Workspace)
		}
	}
}

func TestAssignWorkspaces_PriorityOnBestMonitor(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 3840, Height: 2160, Primary: true, Connected: true},  // primary + large
		{Name: "HDMI-1", Width: 1920, Height: 1080, Primary: false, Connected: true}, // small
	}
	sessions := []SessionInfo{
		{ID: "low", Name: "background", Priority: 10},
		{ID: "high", Name: "main-task", Priority: 0},
	}

	plan := AssignWorkspaces(monitors, sessions)
	if len(plan.Assignments) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(plan.Assignments))
	}

	// High-priority session should be on the primary/large monitor (DP-1).
	if plan.Assignments[0].SessionID != "high" {
		t.Errorf("expected high-priority session first, got %s", plan.Assignments[0].SessionID)
	}
	if plan.Assignments[0].MonitorName != "DP-1" {
		t.Errorf("high-priority session should be on DP-1 (primary/large), got %s", plan.Assignments[0].MonitorName)
	}

	// Low-priority session should be on the secondary monitor.
	if plan.Assignments[1].MonitorName != "HDMI-1" {
		t.Errorf("low-priority session should be on HDMI-1, got %s", plan.Assignments[1].MonitorName)
	}
}

func TestAssignWorkspaces_RoundRobinDistribution(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 3840, Height: 2160, Primary: true, Connected: true},
		{Name: "DP-2", Width: 3840, Height: 2160, Primary: false, Connected: true},
		{Name: "DP-3", Width: 2560, Height: 1440, Primary: false, Connected: true},
	}
	// 6 sessions with equal priority should distribute evenly across 3 monitors.
	sessions := []SessionInfo{
		{ID: "s1", Priority: 0},
		{ID: "s2", Priority: 0},
		{ID: "s3", Priority: 0},
		{ID: "s4", Priority: 0},
		{ID: "s5", Priority: 0},
		{ID: "s6", Priority: 0},
	}

	plan := AssignWorkspaces(monitors, sessions)
	if len(plan.Assignments) != 6 {
		t.Fatalf("expected 6 assignments, got %d", len(plan.Assignments))
	}

	// Count assignments per monitor.
	counts := map[string]int{}
	for _, a := range plan.Assignments {
		counts[a.MonitorName]++
	}

	// Each monitor should get exactly 2 sessions.
	for _, mon := range monitors {
		if counts[mon.Name] != 2 {
			t.Errorf("monitor %s: expected 2 sessions, got %d", mon.Name, counts[mon.Name])
		}
	}
}

func TestAssignWorkspaces_SevenMonitorThinClient(t *testing.T) {
	// Parse the 7-monitor xrandr output and assign sessions.
	monitors, err := ParseXrandrOutput(sevenMonitorXrandr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sessions := []SessionInfo{
		{ID: "lead", Name: "orchestrator", Priority: 0},
		{ID: "w1", Name: "worker-claude", Priority: 1},
		{ID: "w2", Name: "worker-gemini", Priority: 1},
		{ID: "w3", Name: "worker-codex", Priority: 1},
		{ID: "bg1", Name: "background-lint", Priority: 5},
		{ID: "bg2", Name: "background-test", Priority: 5},
		{ID: "bg3", Name: "background-build", Priority: 5},
	}

	plan := AssignWorkspaces(monitors, sessions)
	if len(plan.Assignments) != 7 {
		t.Fatalf("expected 7 assignments, got %d", len(plan.Assignments))
	}

	// The lead session (priority 0) should land on the primary/largest monitor.
	lead := plan.Assignments[0]
	if lead.SessionID != "lead" {
		t.Errorf("expected lead session first, got %s", lead.SessionID)
	}
	// Primary monitor (DP-1) is 3840x2160 and primary, so it should be the top pick.
	if lead.MonitorName != "DP-1" {
		t.Errorf("lead should be on primary DP-1, got %s", lead.MonitorName)
	}

	// Every session should be assigned to a unique monitor (7 sessions, 7 monitors).
	seen := map[string]bool{}
	for _, a := range plan.Assignments {
		if seen[a.MonitorName] {
			t.Errorf("monitor %s assigned more than once with 7 monitors and 7 sessions", a.MonitorName)
		}
		seen[a.MonitorName] = true
	}
}

func TestAssignWorkspaces_MoreSessionsThanMonitors(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 3840, Height: 2160, Primary: true, Connected: true},
		{Name: "DP-2", Width: 1920, Height: 1080, Primary: false, Connected: true},
	}
	sessions := []SessionInfo{
		{ID: "s1", Priority: 0},
		{ID: "s2", Priority: 1},
		{ID: "s3", Priority: 2},
		{ID: "s4", Priority: 3},
		{ID: "s5", Priority: 4},
	}

	plan := AssignWorkspaces(monitors, sessions)
	if len(plan.Assignments) != 5 {
		t.Fatalf("expected 5 assignments, got %d", len(plan.Assignments))
	}

	// Count per monitor; distribution should be as balanced as possible (3/2 or 2/3).
	counts := map[string]int{}
	for _, a := range plan.Assignments {
		counts[a.MonitorName]++
	}
	for name, c := range counts {
		if c < 2 || c > 3 {
			t.Errorf("monitor %s: expected 2-3 sessions, got %d", name, c)
		}
	}
}

func TestAssignWorkspaces_StableSortOrder(t *testing.T) {
	monitors := []Monitor{
		{Name: "B-MON", Width: 1920, Height: 1080, Connected: true},
		{Name: "A-MON", Width: 1920, Height: 1080, Connected: true},
	}
	sessions := []SessionInfo{
		{ID: "x", Priority: 0},
		{ID: "y", Priority: 0},
	}

	// Run twice to verify determinism.
	plan1 := AssignWorkspaces(monitors, sessions)
	plan2 := AssignWorkspaces(monitors, sessions)

	for i := range plan1.Assignments {
		if plan1.Assignments[i].SessionID != plan2.Assignments[i].SessionID {
			t.Errorf("assignment[%d] not stable: %s vs %s",
				i, plan1.Assignments[i].SessionID, plan2.Assignments[i].SessionID)
		}
		if plan1.Assignments[i].MonitorName != plan2.Assignments[i].MonitorName {
			t.Errorf("assignment[%d] monitor not stable: %s vs %s",
				i, plan1.Assignments[i].MonitorName, plan2.Assignments[i].MonitorName)
		}
	}
}

func TestAssignWorkspaces_PreservesSessionName(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 1920, Height: 1080, Primary: true, Connected: true},
	}
	sessions := []SessionInfo{
		{ID: "s1", Name: "my-session", Priority: 0},
	}

	plan := AssignWorkspaces(monitors, sessions)
	if plan.Assignments[0].SessionName != "my-session" {
		t.Errorf("expected session name 'my-session', got %q", plan.Assignments[0].SessionName)
	}
}

func TestAssignWorkspaces_MonitorSortOrder(t *testing.T) {
	// Verify that a non-primary large monitor beats a primary small monitor
	// only when primary tie is broken by area.
	monitors := []Monitor{
		{Name: "SMALL", Width: 1024, Height: 768, Primary: true, Connected: true},
		{Name: "LARGE", Width: 3840, Height: 2160, Primary: false, Connected: true},
	}
	sessions := []SessionInfo{
		{ID: "s1", Priority: 0},
	}

	plan := AssignWorkspaces(monitors, sessions)
	// Primary wins over area, so SMALL (primary) should get the first session.
	if plan.Assignments[0].MonitorName != "SMALL" {
		t.Errorf("expected primary monitor SMALL to be preferred, got %s", plan.Assignments[0].MonitorName)
	}
}

// --- ParseSwayOutputs tests ---

func TestParseSwayOutputs(t *testing.T) {
	outputs := []sway.Output{
		{
			Name:             "DP-1",
			Make:             "Dell Inc.",
			Model:            "U2723QE",
			Active:           true,
			CurrentWorkspace: "1",
			Rect:             sway.Rect{X: 0, Y: 0, Width: 3840, Height: 2160},
			CurrentMode:      sway.OutputMode{Width: 3840, Height: 2160, Refresh: 60000},
		},
		{
			Name:             "DP-2",
			Make:             "Dell Inc.",
			Model:            "U2723QE",
			Active:           true,
			CurrentWorkspace: "2",
			Rect:             sway.Rect{X: 3840, Y: 0, Width: 3840, Height: 2160},
			CurrentMode:      sway.OutputMode{Width: 3840, Height: 2160, Refresh: 60000},
		},
		{
			Name:   "HDMI-A-1",
			Active: false,
		},
	}

	monitors := ParseSwayOutputs(outputs)

	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}

	// Check first monitor
	if monitors[0].Name != "DP-1" {
		t.Errorf("expected DP-1, got %s", monitors[0].Name)
	}
	if monitors[0].Width != 3840 || monitors[0].Height != 2160 {
		t.Errorf("expected 3840x2160, got %dx%d", monitors[0].Width, monitors[0].Height)
	}
	if monitors[0].OffsetX != 0 {
		t.Errorf("expected offset 0, got %d", monitors[0].OffsetX)
	}
	if monitors[0].RefreshHz != 60.0 {
		t.Errorf("expected 60.0 Hz, got %f", monitors[0].RefreshHz)
	}

	// Check second monitor offset
	if monitors[1].OffsetX != 3840 {
		t.Errorf("expected offset 3840, got %d", monitors[1].OffsetX)
	}

	// Inactive monitor should be excluded
	for _, m := range monitors {
		if m.Name == "HDMI-A-1" {
			t.Error("inactive monitor HDMI-A-1 should be excluded")
		}
	}
}

func TestParseSwayOutputs_Empty(t *testing.T) {
	monitors := ParseSwayOutputs(nil)
	if len(monitors) != 0 {
		t.Errorf("expected 0 monitors for nil input, got %d", len(monitors))
	}
}

// --- ParseHyprlandMonitors tests ---

func TestParseHyprlandMonitors(t *testing.T) {
	monitors := ParseHyprlandMonitors([]hyprland.Monitor{
		{ID: 0, Name: "DP-1", Width: 3840, Height: 2160, X: 0, Y: 0, RefreshRate: 60.0, Focused: true},
		{ID: 1, Name: "HDMI-A-1", Width: 1920, Height: 1080, X: 3840, Y: 0, RefreshRate: 60.0, Focused: false},
	})

	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}

	// First monitor should be primary (focused).
	if monitors[0].Name != "DP-1" {
		t.Errorf("expected DP-1, got %s", monitors[0].Name)
	}
	if !monitors[0].Primary {
		t.Error("DP-1 should be primary (focused)")
	}
	if monitors[0].Width != 3840 || monitors[0].Height != 2160 {
		t.Errorf("expected 3840x2160, got %dx%d", monitors[0].Width, monitors[0].Height)
	}
	if monitors[0].OffsetX != 0 || monitors[0].OffsetY != 0 {
		t.Errorf("expected offset 0+0, got %d+%d", monitors[0].OffsetX, monitors[0].OffsetY)
	}
	if monitors[0].RefreshHz != 60.0 {
		t.Errorf("expected 60.0 Hz, got %f", monitors[0].RefreshHz)
	}
	if !monitors[0].Connected {
		t.Error("monitors should always be connected")
	}

	// Second monitor should not be primary.
	if monitors[1].Name != "HDMI-A-1" {
		t.Errorf("expected HDMI-A-1, got %s", monitors[1].Name)
	}
	if monitors[1].Primary {
		t.Error("HDMI-A-1 should not be primary")
	}
	if monitors[1].OffsetX != 3840 {
		t.Errorf("expected offset_x 3840, got %d", monitors[1].OffsetX)
	}
	if monitors[1].Width != 1920 || monitors[1].Height != 1080 {
		t.Errorf("expected 1920x1080, got %dx%d", monitors[1].Width, monitors[1].Height)
	}
}

func TestParseHyprlandMonitors_Empty(t *testing.T) {
	monitors := ParseHyprlandMonitors(nil)
	if len(monitors) != 0 {
		t.Errorf("expected 0 monitors for nil input, got %d", len(monitors))
	}
}

func TestParseHyprlandMonitors_SingleMonitor(t *testing.T) {
	monitors := ParseHyprlandMonitors([]hyprland.Monitor{
		{ID: 0, Name: "eDP-1", Width: 2560, Height: 1600, X: 0, Y: 0, RefreshRate: 165.0, Focused: true},
	})
	if len(monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(monitors))
	}
	if monitors[0].RefreshHz != 165.0 {
		t.Errorf("expected 165.0 Hz, got %f", monitors[0].RefreshHz)
	}
}

func TestParseSwayOutputs_AllInactive(t *testing.T) {
	outputs := []sway.Output{
		{Name: "DP-1", Active: false},
		{Name: "DP-2", Active: false},
	}
	monitors := ParseSwayOutputs(outputs)
	if len(monitors) != 0 {
		t.Errorf("expected 0 monitors when all inactive, got %d", len(monitors))
	}
}
