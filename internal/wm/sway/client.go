// Package sway provides an IPC client for the Sway compositor.
//
// Sway uses the i3 IPC protocol: "i3-ipc" magic bytes (6) + payload length
// (uint32 LE) + message type (uint32 LE) + payload. This package communicates
// over the Unix socket identified by the SWAYSOCK environment variable.
package sway

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// IPC message types (same as i3).
const (
	MsgTypeRunCommand    uint32 = 0
	MsgTypeGetWorkspaces uint32 = 1
	MsgTypeGetOutputs    uint32 = 3
	MsgTypeGetTree       uint32 = 4
	MsgTypeGetVersion    uint32 = 7
)

// magic is the i3/sway IPC protocol magic string.
var magic = []byte("i3-ipc")

// headerLen is the total header size: 6 (magic) + 4 (length) + 4 (type).
const headerLen = 14

// Client is a Sway IPC client that communicates over a Unix socket.
type Client struct {
	conn net.Conn
}

// Node represents a node in the Sway layout tree.
type Node struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	Focused        bool   `json:"focused"`
	Output         string `json:"output"`
	Nodes          []Node `json:"nodes"`
	FloatingNodes  []Node `json:"floating_nodes"`
}

// Workspace represents a Sway workspace.
type Workspace struct {
	Num     int    `json:"num"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
	Focused bool   `json:"focused"`
	Output  string `json:"output"`
	Urgent  bool   `json:"urgent"`
}

// Output represents a Sway output (monitor).
type Output struct {
	Name             string `json:"name"`
	Make             string `json:"make"`
	Model            string `json:"model"`
	Active           bool   `json:"active"`
	CurrentWorkspace string `json:"current_workspace"`
}

// SocketPath returns the SWAYSOCK environment variable value.
func SocketPath() string {
	return os.Getenv("SWAYSOCK")
}

// NewClient connects to the Sway IPC socket at the given path.
func NewClient(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("sway ipc connect: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Connect connects to the Sway IPC socket using the SWAYSOCK env var.
func Connect() (*Client, error) {
	sock := SocketPath()
	if sock == "" {
		return nil, fmt.Errorf("sway ipc: SWAYSOCK not set")
	}
	return NewClient(sock)
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// sendMessage sends a Sway IPC message and reads the response payload.
func (c *Client) sendMessage(msgType uint32, payload []byte) ([]byte, error) {
	header := make([]byte, headerLen)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[6:10], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[10:14], msgType)

	if _, err := c.conn.Write(header); err != nil {
		return nil, fmt.Errorf("sway ipc write header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return nil, fmt.Errorf("sway ipc write payload: %w", err)
		}
	}

	respHeader := make([]byte, headerLen)
	if _, err := readFull(c.conn, respHeader); err != nil {
		return nil, fmt.Errorf("sway ipc read header: %w", err)
	}

	for i := 0; i < len(magic); i++ {
		if respHeader[i] != magic[i] {
			return nil, fmt.Errorf("sway ipc: invalid magic in response")
		}
	}

	respLen := binary.LittleEndian.Uint32(respHeader[6:10])
	if respLen == 0 {
		return nil, nil
	}

	respPayload := make([]byte, respLen)
	if _, err := readFull(c.conn, respPayload); err != nil {
		return nil, fmt.Errorf("sway ipc read payload: %w", err)
	}
	return respPayload, nil
}

// RunCommand sends a Sway command string and returns an error if it fails.
func (c *Client) RunCommand(cmd string) error {
	data, err := c.sendMessage(MsgTypeRunCommand, []byte(cmd))
	if err != nil {
		return err
	}
	var results []struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return fmt.Errorf("sway ipc parse command result: %w", err)
	}
	for _, r := range results {
		if !r.Success {
			msg := r.Error
			if msg == "" {
				msg = "command failed"
			}
			return fmt.Errorf("sway command: %s", msg)
		}
	}
	return nil
}

// GetTree returns the full Sway layout tree.
func (c *Client) GetTree() (*Node, error) {
	data, err := c.sendMessage(MsgTypeGetTree, nil)
	if err != nil {
		return nil, err
	}
	var root Node
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("sway parse tree: %w", err)
	}
	return &root, nil
}

// GetWorkspaces returns the list of Sway workspaces.
func (c *Client) GetWorkspaces() ([]Workspace, error) {
	data, err := c.sendMessage(MsgTypeGetWorkspaces, nil)
	if err != nil {
		return nil, err
	}
	var ws []Workspace
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, fmt.Errorf("sway parse workspaces: %w", err)
	}
	return ws, nil
}

// GetOutputs returns the list of Sway outputs (monitors).
func (c *Client) GetOutputs() ([]Output, error) {
	data, err := c.sendMessage(MsgTypeGetOutputs, nil)
	if err != nil {
		return nil, err
	}
	var outputs []Output
	if err := json.Unmarshal(data, &outputs); err != nil {
		return nil, fmt.Errorf("sway parse outputs: %w", err)
	}
	return outputs, nil
}

// readFull reads exactly len(buf) bytes from the connection.
func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
