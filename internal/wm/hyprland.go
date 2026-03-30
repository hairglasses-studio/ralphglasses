package wm

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// HyprlandMonitor represents a Hyprland monitor.
type HyprlandMonitor struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Scale       float64 `json:"scale"`
	Focused     bool   `json:"focused"`
}

// HyprlandMonitors returns the list of monitors via hyprctl.
func HyprlandMonitors() ([]HyprlandMonitor, error) {
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return nil, fmt.Errorf("hyprctl monitors: %w", err)
	}
	var monitors []HyprlandMonitor
	if err := json.Unmarshal(out, &monitors); err != nil {
		return nil, fmt.Errorf("parse monitors: %w", err)
	}
	return monitors, nil
}

// HyprlandDispatchWorkspace switches to a named workspace.
func HyprlandDispatchWorkspace(name string) error {
	return exec.Command("hyprctl", "dispatch", "workspace", "name:"+name).Run()
}

// HyprlandCreateWorkspace creates a named workspace on a specific monitor.
func HyprlandCreateWorkspace(name, monitor string) error {
	// Move to the workspace first
	if err := HyprlandDispatchWorkspace(name); err != nil {
		return err
	}
	// Move it to the target monitor
	if monitor != "" {
		return exec.Command("hyprctl", "dispatch", "movecurrentworkspacetomonitor", monitor).Run()
	}
	return nil
}

// HyprlandAvailable returns true if hyprctl is available.
func HyprlandAvailable() bool {
	_, err := exec.LookPath("hyprctl")
	return err == nil
}

// HyprlandVersion returns the Hyprland version string.
func HyprlandVersion() string {
	out, err := exec.Command("hyprctl", "version", "-j").Output()
	if err != nil {
		return ""
	}
	var v struct {
		Tag string `json:"tag"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return strings.TrimSpace(string(out))
	}
	return v.Tag
}
