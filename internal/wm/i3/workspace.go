package i3

import (
	"encoding/json"
	"fmt"
)

// Workspace represents an i3 workspace.
type Workspace struct {
	Num     int    `json:"num"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
	Focused bool   `json:"focused"`
	Output  string `json:"output"`
	Urgent  bool   `json:"urgent"`
}

// GetWorkspaces returns the list of i3 workspaces.
func (c *Client) GetWorkspaces() ([]Workspace, error) {
	data, err := c.sendMessage(MsgTypeGetWorkspaces, nil)
	if err != nil {
		return nil, err
	}
	var ws []Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("i3 parse workspaces: %w", err)
	}
	return ws, nil
}

// FocusWorkspace switches focus to the named workspace.
func (c *Client) FocusWorkspace(name string) error {
	return c.runCommand(fmt.Sprintf("workspace %s", name))
}

// MoveToWorkspace moves the currently focused container to the named workspace.
func (c *Client) MoveToWorkspace(name string) error {
	return c.runCommand(fmt.Sprintf("move container to workspace %s", name))
}
