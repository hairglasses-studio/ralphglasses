package i3

import (
	"encoding/json"
	"fmt"
)

// Node represents a node in the i3 layout tree.
type Node struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Focused  bool   `json:"focused"`
	Floating string `json:"floating_con,omitempty"`
	Output   string `json:"output"`
	Nodes    []Node `json:"nodes"`
	Floating_Nodes []Node `json:"floating_nodes"`
}

// Window represents a simplified view of an i3 window (leaf node).
type Window struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Focused  bool   `json:"focused"`
	Floating bool   `json:"floating"`
	Output   string `json:"output"`
}

// GetTree returns the full i3 layout tree.
func (c *Client) GetTree() (*Node, error) {
	data, err := c.sendMessage(MsgTypeGetTree, nil)
	if err != nil {
		return nil, err
	}
	var root Node
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("i3 parse tree: %w", err)
	}
	return &root, nil
}

// FocusWindow focuses the window with the given container ID.
func (c *Client) FocusWindow(id int64) error {
	return c.runCommand(fmt.Sprintf("[con_id=%d] focus", id))
}

// MoveWindow moves the window with the given container ID to the named workspace.
func (c *Client) MoveWindow(id int64, workspace string) error {
	return c.runCommand(fmt.Sprintf("[con_id=%d] move container to workspace %s", id, workspace))
}

// Windows returns a flat list of all leaf windows in the tree.
func (n *Node) Windows() []Window {
	var windows []Window
	n.collectWindows(&windows, "")
	return windows
}

func (n *Node) collectWindows(out *[]Window, output string) {
	// Track the output name as we descend.
	if n.Type == "output" {
		output = n.Name
	}

	// A leaf container with a name is a window.
	if len(n.Nodes) == 0 && len(n.Floating_Nodes) == 0 && n.Name != "" && n.Type == "con" {
		*out = append(*out, Window{
			ID:       n.ID,
			Name:     n.Name,
			Focused:  n.Focused,
			Floating: false,
			Output:   output,
		})
		return
	}

	for i := range n.Nodes {
		n.Nodes[i].collectWindows(out, output)
	}
	for i := range n.Floating_Nodes {
		n.Floating_Nodes[i].collectWindows(out, output)
	}
}
