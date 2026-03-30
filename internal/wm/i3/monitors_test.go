package i3

import (
	"encoding/json"
	"testing"
)

// --- i3 get_outputs JSON fixtures ---

// sevenMonitorJSON simulates a 7-monitor thin client setup.
var sevenMonitorJSON = mustMarshal([]i3Output{
	{Name: "DP-1", Active: true, Primary: true, CurrentWorkspace: "1",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 0, Y: 0, Width: 3840, Height: 2160}},
	{Name: "DP-2", Active: true, Primary: false, CurrentWorkspace: "2",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 3840, Y: 0, Width: 3840, Height: 2160}},
	{Name: "DP-3", Active: true, Primary: false, CurrentWorkspace: "3",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 7680, Y: 0, Width: 2560, Height: 1440}},
	{Name: "HDMI-1", Active: true, Primary: false, CurrentWorkspace: "4",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 10240, Y: 0, Width: 1920, Height: 1080}},
	{Name: "DP-4", Active: true, Primary: false, CurrentWorkspace: "5",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 12160, Y: 0, Width: 1920, Height: 1080}},
	{Name: "DP-5", Active: true, Primary: false, CurrentWorkspace: "6",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 14080, Y: 0, Width: 1920, Height: 1080}},
	{Name: "DP-6", Active: true, Primary: false, CurrentWorkspace: "7",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 14080, Y: 1080, Width: 1920, Height: 1080}},
	// Virtual output with zero dimensions (should be filtered out).
	{Name: "xroot-0", Active: false, Primary: false,
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 0, Y: 0, Width: 0, Height: 0}},
})

// dualMonitorJSON simulates a dual-monitor setup.
var dualMonitorJSON = mustMarshal([]i3Output{
	{Name: "eDP-1", Active: true, Primary: true, CurrentWorkspace: "1",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 0, Y: 0, Width: 1920, Height: 1080}},
	{Name: "DP-1", Active: true, Primary: false, CurrentWorkspace: "2",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 1920, Y: 0, Width: 3840, Height: 2160}},
})

// singleMonitorJSON simulates a single laptop display.
var singleMonitorJSON = mustMarshal([]i3Output{
	{Name: "eDP-1", Active: true, Primary: true, CurrentWorkspace: "1",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 0, Y: 0, Width: 1920, Height: 1080}},
})

// noPrimaryJSON simulates outputs where no monitor is marked primary.
var noPrimaryJSON = mustMarshal([]i3Output{
	{Name: "DP-1", Active: true, Primary: false, CurrentWorkspace: "1",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 0, Y: 0, Width: 1920, Height: 1080}},
	{Name: "DP-2", Active: true, Primary: false, CurrentWorkspace: "2",
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 1920, Y: 0, Width: 2560, Height: 1440}},
})

// inactiveOnlyJSON simulates all outputs being inactive (only virtual).
var inactiveOnlyJSON = mustMarshal([]i3Output{
	{Name: "xroot-0", Active: false, Primary: false,
		Rect: struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		}{X: 0, Y: 0, Width: 0, Height: 0}},
})

func mustMarshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

// --- ParseOutputsJSON tests ---

func TestParseOutputsJSON_SevenMonitors(t *testing.T) {
	monitors, err := ParseOutputsJSON(sevenMonitorJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 8 outputs in fixture, but xroot-0 (zero dimensions) is filtered.
	if len(monitors) != 7 {
		t.Fatalf("expected 7 monitors, got %d", len(monitors))
	}

	// Verify primary.
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
	if dp1.X != 0 || dp1.Y != 0 {
		t.Errorf("DP-1 position: expected 0,0, got %d,%d", dp1.X, dp1.Y)
	}
	if !dp1.Active {
		t.Error("DP-1 should be active")
	}

	// Verify stacked monitor.
	dp6 := monitors[6]
	if dp6.Name != "DP-6" {
		t.Errorf("expected seventh monitor DP-6, got %s", dp6.Name)
	}
	if dp6.Y != 1080 {
		t.Errorf("DP-6 Y: expected 1080, got %d", dp6.Y)
	}

	// Verify virtual output excluded.
	for _, m := range monitors {
		if m.Name == "xroot-0" {
			t.Error("virtual xroot-0 should be filtered out")
		}
	}
}

func TestParseOutputsJSON_DualMonitor(t *testing.T) {
	monitors, err := ParseOutputsJSON(dualMonitorJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
	if !monitors[0].Primary {
		t.Error("eDP-1 should be primary")
	}
	if monitors[1].Width != 3840 {
		t.Errorf("DP-1 width: expected 3840, got %d", monitors[1].Width)
	}
}

func TestParseOutputsJSON_SingleMonitor(t *testing.T) {
	monitors, err := ParseOutputsJSON(singleMonitorJSON)
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

func TestParseOutputsJSON_InvalidJSON(t *testing.T) {
	_, err := ParseOutputsJSON([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseOutputsJSON_NoMonitors(t *testing.T) {
	_, err := ParseOutputsJSON(inactiveOnlyJSON)
	if err == nil {
		t.Error("expected error when only virtual outputs present")
	}
}

func TestParseOutputsJSON_EmptyArray(t *testing.T) {
	_, err := ParseOutputsJSON([]byte(`[]`))
	if err == nil {
		t.Error("expected error for empty outputs array")
	}
}

// --- Active monitor filtering tests ---

func TestFilterActive(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Active: true, Connected: true},
		{Name: "DP-2", Active: false, Connected: false},
		{Name: "DP-3", Active: true, Connected: true},
	}
	active := filterActive(monitors)
	if len(active) != 2 {
		t.Fatalf("expected 2 active monitors, got %d", len(active))
	}
	if active[0].Name != "DP-1" {
		t.Errorf("expected DP-1, got %s", active[0].Name)
	}
	if active[1].Name != "DP-3" {
		t.Errorf("expected DP-3, got %s", active[1].Name)
	}
}

func TestFilterActive_NoneActive(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Active: false, Connected: false},
	}
	active := filterActive(monitors)
	if len(active) != 0 {
		t.Errorf("expected 0 active monitors, got %d", len(active))
	}
}

// --- Primary monitor selection tests ---

func TestFindPrimary(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Primary: false, Active: true},
		{Name: "DP-2", Primary: true, Active: true},
		{Name: "DP-3", Primary: false, Active: true},
	}
	p, err := findPrimary(monitors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "DP-2" {
		t.Errorf("expected DP-2 as primary, got %s", p.Name)
	}
}

func TestFindPrimary_NoPrimaryFlag(t *testing.T) {
	monitors, err := ParseOutputsJSON(noPrimaryJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p, err := findPrimary(monitors)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to first active monitor.
	if p.Name != "DP-1" {
		t.Errorf("expected DP-1 as fallback primary, got %s", p.Name)
	}
}

func TestFindPrimary_Empty(t *testing.T) {
	_, err := findPrimary(nil)
	if err == nil {
		t.Error("expected error for empty monitor list")
	}
}

// --- MonitorByName tests ---

func TestFindByName(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1"},
		{Name: "HDMI-1"},
		{Name: "DP-2"},
	}

	m, err := findByName(monitors, "HDMI-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Name != "HDMI-1" {
		t.Errorf("expected HDMI-1, got %s", m.Name)
	}
}

func TestFindByName_NotFound(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1"},
	}
	_, err := findByName(monitors, "VGA-1")
	if err == nil {
		t.Error("expected error for missing monitor")
	}
}

// --- ArrangeForFleet tests ---

func TestArrangeForFleet_EmptyInputs(t *testing.T) {
	// No monitors.
	a := ArrangeForFleet(nil, 3)
	if len(a) != 0 {
		t.Errorf("expected 0 assignments with no monitors, got %d", len(a))
	}

	// Zero sessions.
	a = ArrangeForFleet([]Monitor{{Name: "DP-1", Width: 1920, Height: 1080}}, 0)
	if len(a) != 0 {
		t.Errorf("expected 0 assignments with 0 sessions, got %d", len(a))
	}

	// Negative sessions.
	a = ArrangeForFleet([]Monitor{{Name: "DP-1", Width: 1920, Height: 1080}}, -1)
	if len(a) != 0 {
		t.Errorf("expected 0 assignments with negative sessions, got %d", len(a))
	}
}

func TestArrangeForFleet_SingleMonitorFallback(t *testing.T) {
	monitors := []Monitor{
		{Name: "eDP-1", Width: 1920, Height: 1080, Primary: true, Active: true, Connected: true},
	}

	assignments := ArrangeForFleet(monitors, 4)
	if len(assignments) != 4 {
		t.Fatalf("expected 4 assignments, got %d", len(assignments))
	}

	// All sessions should go to the single monitor.
	for i, a := range assignments {
		if a.Monitor.Name != "eDP-1" {
			t.Errorf("assignment[%d]: expected monitor eDP-1, got %s", i, a.Monitor.Name)
		}
		if a.SessionIndex != i {
			t.Errorf("assignment[%d]: expected session index %d, got %d", i, i, a.SessionIndex)
		}
		expected := "fleet-" + itoa(i+1)
		if a.Workspace != expected {
			t.Errorf("assignment[%d]: expected workspace %s, got %s", i, expected, a.Workspace)
		}
	}
}

func TestArrangeForFleet_DistributesAcrossMonitors(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 3840, Height: 2160, Primary: true, Active: true, Connected: true},
		{Name: "DP-2", Width: 3840, Height: 2160, Active: true, Connected: true},
		{Name: "DP-3", Width: 2560, Height: 1440, Active: true, Connected: true},
	}

	assignments := ArrangeForFleet(monitors, 6)
	if len(assignments) != 6 {
		t.Fatalf("expected 6 assignments, got %d", len(assignments))
	}

	// Count assignments per monitor.
	counts := map[string]int{}
	for _, a := range assignments {
		counts[a.Monitor.Name]++
	}

	// Each monitor should get exactly 2 sessions.
	for _, m := range monitors {
		if counts[m.Name] != 2 {
			t.Errorf("monitor %s: expected 2 sessions, got %d", m.Name, counts[m.Name])
		}
	}
}

func TestArrangeForFleet_PrimaryGetsFirstSession(t *testing.T) {
	monitors := []Monitor{
		{Name: "HDMI-1", Width: 1920, Height: 1080, Active: true, Connected: true},
		{Name: "DP-1", Width: 3840, Height: 2160, Primary: true, Active: true, Connected: true},
	}

	assignments := ArrangeForFleet(monitors, 1)
	if len(assignments) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(assignments))
	}

	// Primary monitor (DP-1) should get the first session despite being second in input.
	if assignments[0].Monitor.Name != "DP-1" {
		t.Errorf("expected first session on primary DP-1, got %s", assignments[0].Monitor.Name)
	}
}

func TestArrangeForFleet_LargerMonitorPreferred(t *testing.T) {
	monitors := []Monitor{
		{Name: "SMALL", Width: 1920, Height: 1080, Active: true, Connected: true},
		{Name: "LARGE", Width: 3840, Height: 2160, Active: true, Connected: true},
	}

	assignments := ArrangeForFleet(monitors, 1)
	// No primary flag, so larger monitor wins.
	if assignments[0].Monitor.Name != "LARGE" {
		t.Errorf("expected LARGE monitor preferred, got %s", assignments[0].Monitor.Name)
	}
}

func TestArrangeForFleet_SevenMonitorThinClient(t *testing.T) {
	monitors, err := ParseOutputsJSON(sevenMonitorJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assignments := ArrangeForFleet(monitors, 7)
	if len(assignments) != 7 {
		t.Fatalf("expected 7 assignments, got %d", len(assignments))
	}

	// With 7 sessions and 7 monitors, every monitor gets exactly one session.
	seen := map[string]bool{}
	for _, a := range assignments {
		if seen[a.Monitor.Name] {
			t.Errorf("monitor %s assigned more than once", a.Monitor.Name)
		}
		seen[a.Monitor.Name] = true
	}

	// First session should be on primary (DP-1).
	if assignments[0].Monitor.Name != "DP-1" {
		t.Errorf("first session should be on primary DP-1, got %s", assignments[0].Monitor.Name)
	}
}

func TestArrangeForFleet_MoreSessionsThanMonitors(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 3840, Height: 2160, Primary: true, Active: true, Connected: true},
		{Name: "DP-2", Width: 1920, Height: 1080, Active: true, Connected: true},
	}

	assignments := ArrangeForFleet(monitors, 5)
	if len(assignments) != 5 {
		t.Fatalf("expected 5 assignments, got %d", len(assignments))
	}

	counts := map[string]int{}
	for _, a := range assignments {
		counts[a.Monitor.Name]++
	}
	// Should be balanced: 3/2 or 2/3.
	for name, c := range counts {
		if c < 2 || c > 3 {
			t.Errorf("monitor %s: expected 2-3 sessions, got %d", name, c)
		}
	}
}

func TestArrangeForFleet_WorkspaceNaming(t *testing.T) {
	monitors := []Monitor{
		{Name: "DP-1", Width: 1920, Height: 1080, Primary: true, Active: true, Connected: true},
	}

	assignments := ArrangeForFleet(monitors, 3)
	expected := []string{"fleet-1", "fleet-2", "fleet-3"}
	for i, a := range assignments {
		if a.Workspace != expected[i] {
			t.Errorf("assignment[%d]: expected workspace %s, got %s", i, expected[i], a.Workspace)
		}
	}
}

func TestArrangeForFleet_Deterministic(t *testing.T) {
	monitors := []Monitor{
		{Name: "B-MON", Width: 1920, Height: 1080, Active: true, Connected: true},
		{Name: "A-MON", Width: 1920, Height: 1080, Active: true, Connected: true},
	}

	a1 := ArrangeForFleet(monitors, 4)
	a2 := ArrangeForFleet(monitors, 4)

	for i := range a1 {
		if a1[i].Monitor.Name != a2[i].Monitor.Name {
			t.Errorf("assignment[%d] monitor not stable: %s vs %s", i, a1[i].Monitor.Name, a2[i].Monitor.Name)
		}
		if a1[i].SessionIndex != a2[i].SessionIndex {
			t.Errorf("assignment[%d] session index not stable: %d vs %d", i, a1[i].SessionIndex, a2[i].SessionIndex)
		}
	}
}

// --- IPC integration tests (using mock server) ---

func TestListMonitors_IPC(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeGetOutputs {
			t.Errorf("expected message type %d, got %d", MsgTypeGetOutputs, msgType)
		}
		return dualMonitorJSON
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	monitors, err := c.ListMonitors()
	if err != nil {
		t.Fatalf("ListMonitors: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
	if monitors[0].Name != "eDP-1" {
		t.Errorf("expected eDP-1, got %s", monitors[0].Name)
	}
}

func TestActiveMonitors_IPC(t *testing.T) {
	// Include one inactive output in the response.
	outputs := []i3Output{
		{Name: "DP-1", Active: true, Primary: true,
			Rect: struct {
				X      int `json:"x"`
				Y      int `json:"y"`
				Width  int `json:"width"`
				Height int `json:"height"`
			}{Width: 1920, Height: 1080}},
		{Name: "DP-2", Active: false, Primary: false,
			Rect: struct {
				X      int `json:"x"`
				Y      int `json:"y"`
				Width  int `json:"width"`
				Height int `json:"height"`
			}{Width: 1920, Height: 1080}},
	}
	data := mustMarshal(outputs)

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		return data
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	// ListMonitors returns both (DP-2 has non-zero dimensions).
	all, err := c.ListMonitors()
	if err != nil {
		t.Fatalf("ListMonitors: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 monitors from ListMonitors, got %d", len(all))
	}

	// ActiveMonitors filters to only active.
	active, err := c.ActiveMonitors()
	if err != nil {
		t.Fatalf("ActiveMonitors: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active monitor, got %d", len(active))
	}
	if active[0].Name != "DP-1" {
		t.Errorf("expected DP-1, got %s", active[0].Name)
	}
}

func TestPrimaryMonitor_IPC(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		return sevenMonitorJSON
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	p, err := c.PrimaryMonitor()
	if err != nil {
		t.Fatalf("PrimaryMonitor: %v", err)
	}
	if p.Name != "DP-1" {
		t.Errorf("expected primary DP-1, got %s", p.Name)
	}
}

func TestMonitorByName_IPC(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		return sevenMonitorJSON
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	m, err := c.MonitorByName("DP-3")
	if err != nil {
		t.Fatalf("MonitorByName: %v", err)
	}
	if m.Width != 2560 || m.Height != 1440 {
		t.Errorf("DP-3 resolution: expected 2560x1440, got %dx%d", m.Width, m.Height)
	}

	_, err = c.MonitorByName("NONEXISTENT")
	if err == nil {
		t.Error("expected error for nonexistent monitor")
	}
}

// itoa is a simple int-to-string for test assertions without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
