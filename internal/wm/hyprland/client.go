// Package hyprland provides an IPC client for the Hyprland Wayland compositor.
//
// Hyprland exposes two Unix sockets under /tmp/hypr/$HYPRLAND_INSTANCE_SIGNATURE/:
//   - .socket.sock  — request/response commands (JSON)
//   - .socket2.sock — streaming event notifications
//
// This package implements the command socket protocol. Each request is a single
// write of "/<command> <args>" and the response is JSON (or plain text for dispatch).
package hyprland

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// Client communicates with a running Hyprland instance over its IPC socket.
type Client struct {
	// socketDir is the directory containing the Hyprland sockets.
	socketDir string
}

// Monitor represents a Hyprland monitor (output).
type Monitor struct {
	ID               int     `json:"id"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	Make             string  `json:"make"`
	Model            string  `json:"model"`
	Serial           string  `json:"serial"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	RefreshRate      float64 `json:"refreshRate"`
	X                int     `json:"x"`
	Y                int     `json:"y"`
	ActiveWorkspace  WsRef   `json:"activeWorkspace"`
	SpecialWorkspace WsRef   `json:"specialWorkspace"`
	Scale            float64 `json:"scale"`
	Transform        int     `json:"transform"`
	Focused          bool    `json:"focused"`
	DpmsStatus       bool    `json:"dpmsStatus"`
	Vrr              bool    `json:"vrr"`
}

// WsRef is a lightweight workspace reference embedded in other structs.
type WsRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Workspace represents a Hyprland workspace.
type Workspace struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Monitor         string `json:"monitor"`
	MonitorID       int    `json:"monitorID"`
	Windows         int    `json:"windows"`
	HasFullscreen   bool   `json:"hasfullscreen"`
	LastWindow      string `json:"lastwindow"`
	LastWindowTitle string `json:"lastwindowtitle"`
}

// Window represents a Hyprland window (client).
type Window struct {
	Address    string  `json:"address"`
	Mapped     bool    `json:"mapped"`
	Hidden     bool    `json:"hidden"`
	At         [2]int  `json:"at"`
	Size       [2]int  `json:"size"`
	Workspace  WsRef   `json:"workspace"`
	Floating   bool    `json:"floating"`
	Monitor    int     `json:"monitor"`
	Class      string  `json:"class"`
	Title      string  `json:"title"`
	InitialClass string `json:"initialClass"`
	InitialTitle string `json:"initialTitle"`
	PID        int     `json:"pid"`
	Xwayland   bool    `json:"xwayland"`
	Pinned     bool    `json:"pinned"`
	Fullscreen int     `json:"fullscreen"`
	FakeFullscreen bool `json:"fakeFullscreen"`
	Grouped    []string `json:"grouped"`
	SwallowedBy string `json:"swallowing"`
	FocusHistoryID int `json:"focusHistoryID"`
}

// NewClient creates a Client connected to the running Hyprland instance.
// It reads HYPRLAND_INSTANCE_SIGNATURE from the environment to locate the
// IPC socket directory. Returns an error if the env var is unset or the
// socket directory does not exist.
func NewClient() (*Client, error) {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		return nil, errors.New("hyprland: HYPRLAND_INSTANCE_SIGNATURE not set; is Hyprland running?")
	}
	dir := socketDir(sig)
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("hyprland: socket directory not found: %w", err)
	}
	return &Client{socketDir: dir}, nil
}

// newClientFromDir creates a Client using an explicit socket directory.
// This is intended for testing.
func newClientFromDir(dir string) *Client {
	return &Client{socketDir: dir}
}

// Close is a no-op; each request opens a fresh connection. It exists to
// satisfy common closer interfaces.
func (c *Client) Close() error {
	return nil
}

// GetMonitors returns the list of connected monitors.
func (c *Client) GetMonitors() ([]Monitor, error) {
	data, err := c.request("j/monitors")
	if err != nil {
		return nil, err
	}
	var monitors []Monitor
	if err := json.Unmarshal(data, &monitors); err != nil {
		return nil, fmt.Errorf("hyprland: parse monitors: %w", err)
	}
	return monitors, nil
}

// GetWorkspaces returns the list of workspaces.
func (c *Client) GetWorkspaces() ([]Workspace, error) {
	data, err := c.request("j/workspaces")
	if err != nil {
		return nil, err
	}
	var workspaces []Workspace
	if err := json.Unmarshal(data, &workspaces); err != nil {
		return nil, fmt.Errorf("hyprland: parse workspaces: %w", err)
	}
	return workspaces, nil
}

// GetWindows returns the list of all windows (clients).
func (c *Client) GetWindows() ([]Window, error) {
	data, err := c.request("j/clients")
	if err != nil {
		return nil, err
	}
	var windows []Window
	if err := json.Unmarshal(data, &windows); err != nil {
		return nil, fmt.Errorf("hyprland: parse clients: %w", err)
	}
	return windows, nil
}

// GetActiveWindow returns the currently focused window.
func (c *Client) GetActiveWindow() (*Window, error) {
	data, err := c.request("j/activewindow")
	if err != nil {
		return nil, err
	}
	var w Window
	if err := json.Unmarshal(data, &w); err != nil {
		return nil, fmt.Errorf("hyprland: parse activewindow: %w", err)
	}
	return &w, nil
}

// Dispatch sends a dispatcher command to Hyprland.
// Example: Dispatch("workspace", "3") sends "dispatch workspace 3".
func (c *Client) Dispatch(command string, args ...string) error {
	parts := []string{"dispatch", command}
	parts = append(parts, args...)
	cmd := strings.Join(parts, " ")
	resp, err := c.request(cmd)
	if err != nil {
		return err
	}
	// Hyprland returns "ok" on success; anything else is an error message.
	trimmed := strings.TrimSpace(string(resp))
	if trimmed != "ok" && trimmed != "" {
		return fmt.Errorf("hyprland: dispatch %s: %s", command, trimmed)
	}
	return nil
}

// MoveToWorkspace moves the window identified by address to the given workspace number.
func (c *Client) MoveToWorkspace(windowAddr string, workspace int) error {
	return c.Dispatch("movetoworkspacesilent", fmt.Sprintf("%d,address:%s", workspace, windowAddr))
}

// SocketPath returns the path to the command socket.
func (c *Client) SocketPath() string {
	return filepath.Join(c.socketDir, ".socket.sock")
}

// EventSocketPath returns the path to the event socket.
func (c *Client) EventSocketPath() string {
	return filepath.Join(c.socketDir, ".socket2.sock")
}

// socketDir returns the socket directory for a given instance signature.
func socketDir(signature string) string {
	return filepath.Join("/tmp", "hypr", signature)
}

// FormatCommand builds a raw IPC command string from a command and arguments.
// Exported for testing.
func FormatCommand(command string, args ...string) string {
	parts := []string{"dispatch", command}
	parts = append(parts, args...)
	return strings.Join(parts, " ")
}

// request sends a single IPC request and returns the raw response bytes.
func (c *Client) request(cmd string) ([]byte, error) {
	conn, err := net.Dial("unix", c.SocketPath())
	if err != nil {
		return nil, fmt.Errorf("hyprland: connect: %w", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte(cmd)); err != nil {
		return nil, fmt.Errorf("hyprland: write: %w", err)
	}

	// Read full response. Hyprland closes the connection after sending.
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, readErr := conn.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	return buf, nil
}
