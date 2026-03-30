// Package layout provides predefined monitor layout presets and a manager
// for mapping sessions to monitors and workspaces in multi-monitor setups.
package layout

// MonitorAssignment describes the workspace configuration for a single monitor.
type MonitorAssignment struct {
	MonitorIndex int      `json:"monitor_index"` // zero-based index into the monitor list
	Workspaces   []string `json:"workspaces"`    // workspace names assigned to this monitor
	Primary      bool     `json:"primary"`       // whether this is the primary/focus monitor
}

// LayoutPreset is a named, reusable monitor layout configuration.
type LayoutPreset struct {
	Name               string                       `json:"name"`
	Description        string                       `json:"description"`
	MonitorAssignments map[int]MonitorAssignment     `json:"monitor_assignments"` // keyed by monitor index
}

// DefaultPresets returns all built-in layout presets.
func DefaultPresets() []LayoutPreset {
	return []LayoutPreset{
		SingleMonitorPreset(),
		DualMonitorPreset(),
		SevenMonitorPreset(),
	}
}

// SingleMonitorPreset returns a layout for a single-monitor setup (e.g. laptop).
// All workspaces are assigned to monitor 0.
func SingleMonitorPreset() LayoutPreset {
	return LayoutPreset{
		Name:        "single",
		Description: "Single monitor layout — all sessions on one display",
		MonitorAssignments: map[int]MonitorAssignment{
			0: {
				MonitorIndex: 0,
				Workspaces:   []string{"ws-1", "ws-2", "ws-3", "ws-4"},
				Primary:      true,
			},
		},
	}
}

// DualMonitorPreset returns a layout for a typical dual-monitor developer setup.
// Monitor 0 is primary (orchestrator), monitor 1 is for workers.
func DualMonitorPreset() LayoutPreset {
	return LayoutPreset{
		Name:        "dual",
		Description: "Dual monitor layout — primary for orchestration, secondary for workers",
		MonitorAssignments: map[int]MonitorAssignment{
			0: {
				MonitorIndex: 0,
				Workspaces:   []string{"ws-1", "ws-2"},
				Primary:      true,
			},
			1: {
				MonitorIndex: 1,
				Workspaces:   []string{"ws-3", "ws-4"},
				Primary:      false,
			},
		},
	}
}

// SevenMonitorPreset returns a layout for the 7-monitor thin client setup
// (dual-GPU RTX 4090). Monitor 0 is the primary orchestrator display;
// monitors 1-2 are high-priority worker displays; monitors 3-6 handle
// background tasks and monitoring.
func SevenMonitorPreset() LayoutPreset {
	return LayoutPreset{
		Name:        "seven",
		Description: "Seven monitor thin client — orchestrator + workers + background displays",
		MonitorAssignments: map[int]MonitorAssignment{
			0: {
				MonitorIndex: 0,
				Workspaces:   []string{"ws-1"},
				Primary:      true,
			},
			1: {
				MonitorIndex: 1,
				Workspaces:   []string{"ws-2"},
				Primary:      false,
			},
			2: {
				MonitorIndex: 2,
				Workspaces:   []string{"ws-3"},
				Primary:      false,
			},
			3: {
				MonitorIndex: 3,
				Workspaces:   []string{"ws-4"},
				Primary:      false,
			},
			4: {
				MonitorIndex: 4,
				Workspaces:   []string{"ws-5"},
				Primary:      false,
			},
			5: {
				MonitorIndex: 5,
				Workspaces:   []string{"ws-6"},
				Primary:      false,
			},
			6: {
				MonitorIndex: 6,
				Workspaces:   []string{"ws-7"},
				Primary:      false,
			},
		},
	}
}
