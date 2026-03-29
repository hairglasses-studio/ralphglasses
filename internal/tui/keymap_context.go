package tui

import (
	"unsafe"

	"github.com/charmbracelet/bubbles/key"
)

// globalBindings are always enabled regardless of view.
var globalBindings = []func(*KeyMap) *key.Binding{
	func(k *KeyMap) *key.Binding { return &k.Quit },
	func(k *KeyMap) *key.Binding { return &k.CmdMode },
	func(k *KeyMap) *key.Binding { return &k.FilterMode },
	func(k *KeyMap) *key.Binding { return &k.Help },
	func(k *KeyMap) *key.Binding { return &k.Escape },
	func(k *KeyMap) *key.Binding { return &k.Refresh },
	func(k *KeyMap) *key.Binding { return &k.Tab1 },
	func(k *KeyMap) *key.Binding { return &k.Tab2 },
	func(k *KeyMap) *key.Binding { return &k.Tab3 },
	func(k *KeyMap) *key.Binding { return &k.Tab4 },
	func(k *KeyMap) *key.Binding { return &k.Down },
	func(k *KeyMap) *key.Binding { return &k.Up },
	func(k *KeyMap) *key.Binding { return &k.Enter },
	func(k *KeyMap) *key.Binding { return &k.Sort },
}

// viewBindings maps each ViewMode to the bindings it enables (in addition to globals).
var viewBindings = map[ViewMode][]func(*KeyMap) *key.Binding{
	ViewOverview: {
		func(k *KeyMap) *key.Binding { return &k.StartLoop },
		func(k *KeyMap) *key.Binding { return &k.StopAction },
		func(k *KeyMap) *key.Binding { return &k.PauseLoop },
		func(k *KeyMap) *key.Binding { return &k.Space },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewRepoDetail: {
		func(k *KeyMap) *key.Binding { return &k.StartLoop },
		func(k *KeyMap) *key.Binding { return &k.StopAction },
		func(k *KeyMap) *key.Binding { return &k.PauseLoop },
		func(k *KeyMap) *key.Binding { return &k.EditConfig },
		func(k *KeyMap) *key.Binding { return &k.WriteConfig },
		func(k *KeyMap) *key.Binding { return &k.DiffView },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LaunchSession },
		func(k *KeyMap) *key.Binding { return &k.TimelineView },
		func(k *KeyMap) *key.Binding { return &k.LoopHealth },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.ObservationView },
	},
	ViewSessions: {
		func(k *KeyMap) *key.Binding { return &k.StopAction },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.Space },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.TimelineView },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewSessionDetail: {
		func(k *KeyMap) *key.Binding { return &k.StopAction },
		func(k *KeyMap) *key.Binding { return &k.DiffView },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.OutputView },
		func(k *KeyMap) *key.Binding { return &k.TimelineView },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewTeams: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewTeamDetail: {
		func(k *KeyMap) *key.Binding { return &k.DiffView },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.TimelineView },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewFleet: {
		func(k *KeyMap) *key.Binding { return &k.StopAction },
		func(k *KeyMap) *key.Binding { return &k.DiffView },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.TimelineView },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewLogs: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewConfigEditor: {
		func(k *KeyMap) *key.Binding { return &k.EditConfig },
		func(k *KeyMap) *key.Binding { return &k.WriteConfig },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewTimeline: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewDiff: {
		func(k *KeyMap) *key.Binding { return &k.DiffView },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewHelp: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailPause },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlStep },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlToggle },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlPause },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
	},
	ViewLoopHealth: {
		// No specific case in original — all view-specific bindings remain enabled.
		func(k *KeyMap) *key.Binding { return &k.StartLoop },
		func(k *KeyMap) *key.Binding { return &k.StopAction },
		func(k *KeyMap) *key.Binding { return &k.PauseLoop },
		func(k *KeyMap) *key.Binding { return &k.EditConfig },
		func(k *KeyMap) *key.Binding { return &k.WriteConfig },
		func(k *KeyMap) *key.Binding { return &k.DiffView },
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.Space },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LaunchSession },
		func(k *KeyMap) *key.Binding { return &k.OutputView },
		func(k *KeyMap) *key.Binding { return &k.TimelineView },
		func(k *KeyMap) *key.Binding { return &k.LoopHealth },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopListStart },
		func(k *KeyMap) *key.Binding { return &k.LoopListStop },
		func(k *KeyMap) *key.Binding { return &k.LoopListPause },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailStep },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailToggle },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailPause },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlStep },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlToggle },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlPause },
		func(k *KeyMap) *key.Binding { return &k.ObservationView },
		func(k *KeyMap) *key.Binding { return &k.EventLogView },
	},
	ViewLoopList: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopListStart },
		func(k *KeyMap) *key.Binding { return &k.LoopListStop },
		func(k *KeyMap) *key.Binding { return &k.LoopListPause },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewLoopDetail: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailStep },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailToggle },
		func(k *KeyMap) *key.Binding { return &k.LoopDetailPause },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewLoopControl: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlStep },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlToggle },
		func(k *KeyMap) *key.Binding { return &k.LoopCtrlPause },
	},
	ViewObservation: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
	ViewEventLog: {
		func(k *KeyMap) *key.Binding { return &k.GotoEnd },
		func(k *KeyMap) *key.Binding { return &k.GotoStart },
		func(k *KeyMap) *key.Binding { return &k.FollowToggle },
		func(k *KeyMap) *key.Binding { return &k.PageUp },
		func(k *KeyMap) *key.Binding { return &k.PageDown },
		func(k *KeyMap) *key.Binding { return &k.ActionsMenu },
		func(k *KeyMap) *key.Binding { return &k.LoopPanel },
		func(k *KeyMap) *key.Binding { return &k.LoopControlPanel },
	},
}

// globalOverrides maps specific views to global bindings that should be disabled.
// This handles cases where a normally-global binding must be turned off for a view.
var globalOverrides = map[ViewMode][]func(*KeyMap) *key.Binding{
	ViewLoopDetail: {
		func(k *KeyMap) *key.Binding { return &k.Refresh },
	},
}

// allViewBindings holds every view-specific binding accessor, deduplicated.
// Populated by init().
var allViewBindings []func(*KeyMap) *key.Binding

func init() {
	seen := make(map[uintptr]bool)
	var ref KeyMap
	base := uintptr(unsafe.Pointer(&ref))
	for _, accessors := range viewBindings {
		for _, acc := range accessors {
			offset := uintptr(unsafe.Pointer(acc(&ref))) - base
			if !seen[offset] {
				seen[offset] = true
				allViewBindings = append(allViewBindings, acc)
			}
		}
	}
}

// disableAllViewBindings disables all non-global bindings.
func (k *KeyMap) disableAllViewBindings() {
	for _, acc := range allViewBindings {
		acc(k).SetEnabled(false)
	}
}

// SetViewContext enables only the bindings relevant to the given view.
func (k *KeyMap) SetViewContext(view ViewMode) {
	k.disableAllViewBindings()

	// Globals are always enabled.
	for _, acc := range globalBindings {
		acc(k).SetEnabled(true)
	}

	// View-specific bindings.
	if accessors, ok := viewBindings[view]; ok {
		for _, acc := range accessors {
			acc(k).SetEnabled(true)
		}
	}

	// Apply global overrides (disable specific globals for certain views).
	if overrides, ok := globalOverrides[view]; ok {
		for _, acc := range overrides {
			acc(k).SetEnabled(false)
		}
	}
}
