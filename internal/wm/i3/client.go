// Package i3 provides an IPC client for the i3 window manager.
//
// It communicates over a Unix socket using the i3 IPC protocol:
// "i3-ipc" magic bytes (6) + payload length (uint32 LE) + message type (uint32 LE) + payload.
package i3

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
)

// i3 IPC message types.
const (
	MsgTypeRunCommand    uint32 = 0
	MsgTypeGetWorkspaces uint32 = 1
	MsgTypeGetOutputs    uint32 = 3
	MsgTypeGetTree       uint32 = 4
	MsgTypeGetVersion    uint32 = 7
)

// magic is the i3 IPC protocol magic string.
var magic = []byte("i3-ipc")

// headerLen is the total header size: 6 (magic) + 4 (length) + 4 (type).
const headerLen = 14

// Client is an i3 IPC client that communicates over a Unix socket.
type Client struct {
	conn net.Conn
}

// NewClient connects to the i3 IPC socket at the given path.
func NewClient(socketPath string) (*Client, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("i3 ipc connect: %w", err)
	}
	return &Client{conn: conn}, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// sendMessage sends an i3 IPC message and reads the response payload.
func (c *Client) sendMessage(msgType uint32, payload []byte) ([]byte, error) {
	// Build request: magic + length(u32 LE) + type(u32 LE) + payload.
	header := make([]byte, headerLen)
	copy(header, magic)
	binary.LittleEndian.PutUint32(header[6:10], uint32(len(payload)))
	binary.LittleEndian.PutUint32(header[10:14], msgType)

	if _, err := c.conn.Write(header); err != nil {
		return nil, fmt.Errorf("i3 ipc write header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := c.conn.Write(payload); err != nil {
			return nil, fmt.Errorf("i3 ipc write payload: %w", err)
		}
	}

	// Read response header.
	respHeader := make([]byte, headerLen)
	if _, err := readFull(c.conn, respHeader); err != nil {
		return nil, fmt.Errorf("i3 ipc read header: %w", err)
	}

	// Validate magic.
	for i := range magic {
		if respHeader[i] != magic[i] {
			return nil, fmt.Errorf("i3 ipc: invalid magic in response")
		}
	}

	respLen := binary.LittleEndian.Uint32(respHeader[6:10])
	if respLen == 0 {
		return nil, nil
	}

	respPayload := make([]byte, respLen)
	if _, err := readFull(c.conn, respPayload); err != nil {
		return nil, fmt.Errorf("i3 ipc read payload: %w", err)
	}
	return respPayload, nil
}

// runCommand sends an i3 command string and returns success results.
func (c *Client) runCommand(cmd string) error {
	data, err := c.sendMessage(MsgTypeRunCommand, []byte(cmd))
	if err != nil {
		return err
	}
	var results []struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(data, &results); err != nil {
		return fmt.Errorf("i3 ipc parse command result: %w", err)
	}
	for _, r := range results {
		if !r.Success {
			msg := r.Error
			if msg == "" {
				msg = "command failed"
			}
			return fmt.Errorf("i3 command: %s", msg)
		}
	}
	return nil
}

// readFull reads exactly len(buf) bytes from the reader.
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
