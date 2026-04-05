package i3

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Monitor represents a physical display connected via i3.
type Monitor struct {
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Primary   bool   `json:"primary"`
	Connected bool   `json:"connected"`
	Active    bool   `json:"active"`
}

// MonitorAssignment maps a monitor to a fleet session and workspace.
type MonitorAssignment struct {
	Monitor      Monitor `json:"monitor"`
	SessionIndex int     `json:"session_index"`
	Workspace    string  `json:"workspace"`
}

// i3Output is the raw JSON structure returned by i3's get_outputs IPC message.
type i3Output struct {
	Name             string `json:"name"`
	Active           bool   `json:"active"`
	Primary          bool   `json:"primary"`
	CurrentWorkspace string `json:"current_workspace"`
	Rect             struct {
		X      int `json:"x"`
		Y      int `json:"y"`
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"rect"`
}

// ParseOutputsJSON parses the JSON output from i3's get_outputs IPC message
// and returns the list of monitors. Virtual outputs (like "xroot-0") that
// have zero dimensions are excluded.
func ParseOutputsJSON(data []byte) ([]Monitor, error) {
	var outputs []i3Output
	if err := json.Unmarshal(data, &outputs); err != nil {
		return nil, fmt.Errorf("parse i3 outputs: %w", err)
	}

	var monitors []Monitor
	for _, o := range outputs {
		// Skip virtual outputs with no dimensions.
		if o.Rect.Width == 0 && o.Rect.Height == 0 {
			continue
		}
		monitors = append(monitors, Monitor{
			Name:      o.Name,
			Width:     o.Rect.Width,
			Height:    o.Rect.Height,
			X:         o.Rect.X,
			Y:         o.Rect.Y,
			Primary:   o.Primary,
			Connected: o.Active, // i3 only lists connected outputs that are active
			Active:    o.Active,
		})
	}

	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors found in i3 outputs")
	}
	return monitors, nil
}

// ListMonitors returns all monitors reported by i3 via the IPC get_outputs message.
func (c *Client) ListMonitors() ([]Monitor, error) {
	data, err := c.sendMessage(MsgTypeGetOutputs, nil)
	if err != nil {
		return nil, fmt.Errorf("i3 get_outputs: %w", err)
	}
	return ParseOutputsJSON(data)
}

// ActiveMonitors returns only the active and connected monitors.
func (c *Client) ActiveMonitors() ([]Monitor, error) {
	all, err := c.ListMonitors()
	if err != nil {
		return nil, err
	}
	return filterActive(all), nil
}

// filterActive returns monitors that are both active and connected.
func filterActive(monitors []Monitor) []Monitor {
	var active []Monitor
	for _, m := range monitors {
		if m.Active && m.Connected {
			active = append(active, m)
		}
	}
	return active
}

// PrimaryMonitor returns the primary monitor, or an error if none is marked primary.
// If no monitor has the primary flag, the first active monitor is returned.
func (c *Client) PrimaryMonitor() (*Monitor, error) {
	all, err := c.ListMonitors()
	if err != nil {
		return nil, err
	}
	return findPrimary(all)
}

// findPrimary returns the primary monitor from the list, falling back to the first
// active monitor if none is marked primary.
func findPrimary(monitors []Monitor) (*Monitor, error) {
	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors available")
	}
	for i := range monitors {
		if monitors[i].Primary {
			return &monitors[i], nil
		}
	}
	// Fallback: first active monitor.
	for i := range monitors {
		if monitors[i].Active {
			return &monitors[i], nil
		}
	}
	// Last resort: first monitor.
	return &monitors[0], nil
}

// MonitorByName returns the monitor with the given output name,
// or an error if not found.
func (c *Client) MonitorByName(name string) (*Monitor, error) {
	all, err := c.ListMonitors()
	if err != nil {
		return nil, err
	}
	return findByName(all, name)
}

// findByName finds a monitor by name in the given list.
func findByName(monitors []Monitor, name string) (*Monitor, error) {
	for i := range monitors {
		if monitors[i].Name == name {
			return &monitors[i], nil
		}
	}
	return nil, fmt.Errorf("monitor %q not found", name)
}

// ArrangeForFleet distributes sessionCount sessions across the given monitors,
// returning a MonitorAssignment for each session. Monitors are sorted by
// desirability: primary first, then by pixel area (descending), then by name
// for determinism. Sessions are assigned round-robin across sorted monitors.
// Each assignment gets a workspace name of the form "fleet-<N>" where N is
// the 1-based session index.
func ArrangeForFleet(monitors []Monitor, sessionCount int) []MonitorAssignment {
	if len(monitors) == 0 || sessionCount <= 0 {
		return nil
	}

	// Sort monitors: primary first, then descending area, then name.
	sorted := make([]Monitor, len(monitors))
	copy(sorted, monitors)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Primary != sorted[j].Primary {
			return sorted[i].Primary
		}
		areaI := sorted[i].Width * sorted[i].Height
		areaJ := sorted[j].Width * sorted[j].Height
		if areaI != areaJ {
			return areaI > areaJ
		}
		return sorted[i].Name < sorted[j].Name
	})

	// Round-robin assignment: pick the monitor with the fewest assignments.
	slots := make([]int, len(sorted))
	assignments := make([]MonitorAssignment, sessionCount)

	for s := range sessionCount {
		bestIdx := 0
		for i := 1; i < len(sorted); i++ {
			if slots[i] < slots[bestIdx] {
				bestIdx = i
			}
		}
		slots[bestIdx]++

		assignments[s] = MonitorAssignment{
			Monitor:      sorted[bestIdx],
			SessionIndex: s,
			Workspace:    fmt.Sprintf("fleet-%d", s+1),
		}
	}

	return assignments
}
