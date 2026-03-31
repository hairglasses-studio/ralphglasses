package wm

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/wm/sway"
)

// Monitor represents a detected physical display.
type Monitor struct {
	Name       string `json:"name"`        // output name, e.g. "DP-1", "HDMI-2"
	Width      int    `json:"width"`       // horizontal resolution in pixels
	Height     int    `json:"height"`      // vertical resolution in pixels
	OffsetX    int    `json:"offset_x"`    // x position in the virtual screen
	OffsetY    int    `json:"offset_y"`    // y position in the virtual screen
	Primary    bool   `json:"primary"`     // xrandr --primary flag
	Connected  bool   `json:"connected"`   // whether the output is connected
	RefreshHz  float64 `json:"refresh_hz,omitempty"` // current refresh rate
}

// Area returns the total pixel area (width * height).
func (m Monitor) Area() int {
	return m.Width * m.Height
}

// MonitorPlan maps sessions to workspaces on specific monitors.
type MonitorPlan struct {
	Assignments []WorkspaceAssignment `json:"assignments"`
}

// WorkspaceAssignment binds a session to a workspace on a monitor.
type WorkspaceAssignment struct {
	SessionID   string `json:"session_id"`
	SessionName string `json:"session_name,omitempty"`
	Workspace   string `json:"workspace"`   // workspace name/number
	MonitorName string `json:"monitor_name"` // output name of the target monitor
	Priority    int    `json:"priority"`     // original priority (lower = more important)
}

// SessionInfo holds the minimal session metadata needed for workspace assignment.
// This avoids importing the session package and creating a circular dependency.
type SessionInfo struct {
	ID       string
	Name     string
	Priority int // lower number = higher priority (0 is highest)
}

// ParseXrandrOutput parses the text output of `xrandr --query` and returns
// the list of connected monitors with their resolutions and positions.
func ParseXrandrOutput(output string) ([]Monitor, error) {
	var monitors []Monitor

	// Match lines like:
	//   DP-1 connected primary 3840x2160+0+0 (normal left ...) 600mm x 340mm
	//   HDMI-2 connected 1920x1080+3840+0 (normal left ...) 530mm x 300mm
	//   DP-3 connected 2560x1440+5760+0
	//   eDP-1 disconnected (normal left inverted right x axis y axis)
	outputRe := regexp.MustCompile(
		`^(\S+)\s+(connected|disconnected)\s*(primary)?\s*(?:(\d+)x(\d+)\+(\d+)\+(\d+))?\s*`,
	)

	// Match the current mode line (marked with *) for refresh rate:
	//   3840x2160     60.00*+  30.00
	modeRe := regexp.MustCompile(`^\s+(\d+)x(\d+)\s+.*?(\d+\.\d+)\*`)

	lines := strings.Split(output, "\n")
	var current *Monitor

	for _, line := range lines {
		if m := outputRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			connected := m[2] == "connected"
			primary := m[3] == "primary"

			if !connected {
				current = nil
				continue
			}

			mon := Monitor{
				Name:      name,
				Connected: true,
				Primary:   primary,
			}

			// Parse resolution and offset if present (may be absent for
			// connected outputs with no active mode).
			if m[4] != "" {
				mon.Width, _ = strconv.Atoi(m[4])
				mon.Height, _ = strconv.Atoi(m[5])
				mon.OffsetX, _ = strconv.Atoi(m[6])
				mon.OffsetY, _ = strconv.Atoi(m[7])
			}

			monitors = append(monitors, mon)
			current = &monitors[len(monitors)-1]
			continue
		}

		// Try to pick up refresh rate from the active mode line.
		if current != nil {
			if m := modeRe.FindStringSubmatch(line); m != nil {
				hz, err := strconv.ParseFloat(m[3], 64)
				if err == nil {
					current.RefreshHz = hz
				}
			}
		}
	}

	if len(monitors) == 0 {
		return nil, fmt.Errorf("no connected monitors found in xrandr output")
	}
	return monitors, nil
}

// AssignWorkspaces distributes sessions across monitors, assigning one
// workspace per session. Higher-priority sessions (lower Priority number)
// are placed on larger / primary monitors first.
//
// The algorithm:
//  1. Sort monitors by desirability: primary first, then by pixel area (descending).
//  2. Sort sessions by priority (ascending, i.e. 0 = most important first).
//  3. Round-robin assign sessions to monitors in the sorted order.
//  4. Each assignment gets a deterministic workspace name: "ws-<monitorIndex+1>-<slot>".
func AssignWorkspaces(monitors []Monitor, sessions []SessionInfo) MonitorPlan {
	if len(monitors) == 0 || len(sessions) == 0 {
		return MonitorPlan{}
	}

	// Sort monitors: primary first, then descending area, then by name for stability.
	sorted := make([]Monitor, len(monitors))
	copy(sorted, monitors)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Primary != sorted[j].Primary {
			return sorted[i].Primary
		}
		if sorted[i].Area() != sorted[j].Area() {
			return sorted[i].Area() > sorted[j].Area()
		}
		return sorted[i].Name < sorted[j].Name
	})

	// Sort sessions by priority (ascending = most important first).
	sortedSessions := make([]SessionInfo, len(sessions))
	copy(sortedSessions, sessions)
	sort.SliceStable(sortedSessions, func(i, j int) bool {
		if sortedSessions[i].Priority != sortedSessions[j].Priority {
			return sortedSessions[i].Priority < sortedSessions[j].Priority
		}
		return sortedSessions[i].ID < sortedSessions[j].ID
	})

	// Track how many sessions have been assigned to each monitor.
	slots := make([]int, len(sorted))

	var assignments []WorkspaceAssignment
	for _, sess := range sortedSessions {
		// Pick the monitor with the fewest assignments so far (round-robin),
		// respecting the sorted order for tie-breaking.
		bestIdx := 0
		for i := 1; i < len(sorted); i++ {
			if slots[i] < slots[bestIdx] {
				bestIdx = i
			}
		}

		slots[bestIdx]++
		ws := fmt.Sprintf("ws-%d-%d", bestIdx+1, slots[bestIdx])

		assignments = append(assignments, WorkspaceAssignment{
			SessionID:   sess.ID,
			SessionName: sess.Name,
			Workspace:   ws,
			MonitorName: sorted[bestIdx].Name,
			Priority:    sess.Priority,
		})
	}

	return MonitorPlan{Assignments: assignments}
}

// ParseSwayOutputs converts Sway IPC output data to the generic Monitor format.
func ParseSwayOutputs(outputs []sway.Output) []Monitor {
	var monitors []Monitor
	for _, o := range outputs {
		if !o.Active {
			continue
		}
		monitors = append(monitors, Monitor{
			Name:      o.Name,
			Width:     o.CurrentMode.Width,
			Height:    o.CurrentMode.Height,
			OffsetX:   o.Rect.X,
			OffsetY:   o.Rect.Y,
			Connected: true,
			RefreshHz: float64(o.CurrentMode.Refresh) / 1000.0, // Sway reports mHz
		})
	}
	return monitors
}

// DetectMonitors returns the list of connected monitors using the appropriate
// method for the detected window manager.
func DetectMonitors() ([]Monitor, error) {
	switch Detect() {
	case TypeSway:
		client, err := sway.Connect()
		if err != nil {
			return nil, fmt.Errorf("detect monitors (sway): %w", err)
		}
		defer client.Close()
		outputs, err := client.GetOutputs()
		if err != nil {
			return nil, fmt.Errorf("detect monitors (sway get outputs): %w", err)
		}
		monitors := ParseSwayOutputs(outputs)
		if len(monitors) == 0 {
			return nil, fmt.Errorf("detect monitors: no active sway outputs")
		}
		return monitors, nil
	default:
		return nil, fmt.Errorf("detect monitors: unsupported WM %q (use ParseXrandrOutput for X11)", Detect())
	}
}
