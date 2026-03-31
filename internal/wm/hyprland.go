package wm

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/wm/hyprland"
)

// HyprlandMonitors returns the list of monitors via the Hyprland IPC client.
func HyprlandMonitors() ([]hyprland.Monitor, error) {
	client, err := hyprland.NewClient()
	if err != nil {
		return nil, fmt.Errorf("hyprland monitors: %w", err)
	}
	defer client.Close()
	return client.GetMonitors()
}

// HyprlandDispatchWorkspace switches to a named workspace via the IPC client.
func HyprlandDispatchWorkspace(name string) error {
	client, err := hyprland.NewClient()
	if err != nil {
		return fmt.Errorf("hyprland dispatch workspace: %w", err)
	}
	defer client.Close()
	return client.Dispatch("workspace", "name:"+name)
}

// HyprlandCreateWorkspace creates a named workspace on a specific monitor
// via the IPC client.
func HyprlandCreateWorkspace(name, monitor string) error {
	client, err := hyprland.NewClient()
	if err != nil {
		return fmt.Errorf("hyprland create workspace: %w", err)
	}
	defer client.Close()

	// Move to the workspace first.
	if err := client.Dispatch("workspace", "name:"+name); err != nil {
		return err
	}
	// Move it to the target monitor.
	if monitor != "" {
		return client.Dispatch("movecurrentworkspacetomonitor", monitor)
	}
	return nil
}

// HyprlandAvailable returns true if hyprctl is available.
func HyprlandAvailable() bool {
	_, err := exec.LookPath("hyprctl")
	return err == nil
}

// HyprlandVersion returns the Hyprland version string. It tries the IPC
// client first and falls back to exec.Command("hyprctl").
func HyprlandVersion() string {
	client, err := hyprland.NewClient()
	if err == nil {
		defer client.Close()
		data, reqErr := client.GetVersion()
		if reqErr == nil && data != "" {
			return data
		}
	}
	// Fallback to hyprctl exec.
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
