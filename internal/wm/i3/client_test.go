package i3

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// mockServer creates a Unix socket that speaks the i3 IPC protocol.
// It reads one message, calls handler with the type and payload,
// and writes back the response using the i3 IPC framing.
type mockServer struct {
	listener net.Listener
	sockPath string
}

func newMockServer(t *testing.T, handler func(msgType uint32, payload []byte) []byte) *mockServer {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "i3-test.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &mockServer{listener: ln, sockPath: sock}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go s.handleConn(conn, handler)
		}
	}()

	return s
}

func (s *mockServer) handleConn(conn net.Conn, handler func(uint32, []byte) []byte) {
	defer conn.Close()
	for {
		// Read header: 6 magic + 4 length + 4 type = 14.
		hdr := make([]byte, headerLen)
		if _, err := readFull(conn, hdr); err != nil {
			return
		}

		payloadLen := binary.LittleEndian.Uint32(hdr[6:10])
		msgType := binary.LittleEndian.Uint32(hdr[10:14])

		var payload []byte
		if payloadLen > 0 {
			payload = make([]byte, payloadLen)
			if _, err := readFull(conn, payload); err != nil {
				return
			}
		}

		resp := handler(msgType, payload)

		// Write response with i3 IPC framing.
		respHdr := make([]byte, headerLen)
		copy(respHdr, magic)
		binary.LittleEndian.PutUint32(respHdr[6:10], uint32(len(resp)))
		binary.LittleEndian.PutUint32(respHdr[10:14], msgType)
		_, _ = conn.Write(respHdr)
		if len(resp) > 0 {
			_, _ = conn.Write(resp)
		}
	}
}

func (s *mockServer) close() {
	s.listener.Close()
	os.Remove(s.sockPath)
}

func TestGetWorkspaces(t *testing.T) {
	workspaces := []Workspace{
		{Num: 1, Name: "1:code", Visible: true, Focused: true, Output: "DP-1"},
		{Num: 2, Name: "2:web", Visible: true, Focused: false, Output: "DP-2"},
		{Num: 3, Name: "3:chat", Visible: false, Focused: false, Output: "DP-1", Urgent: true},
	}

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeGetWorkspaces {
			t.Errorf("expected message type %d, got %d", MsgTypeGetWorkspaces, msgType)
		}
		data, _ := json.Marshal(workspaces)
		return data
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	got, err := c.GetWorkspaces()
	if err != nil {
		t.Fatalf("GetWorkspaces: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 workspaces, got %d", len(got))
	}
	if got[0].Name != "1:code" {
		t.Errorf("workspace[0].Name = %q, want %q", got[0].Name, "1:code")
	}
	if !got[0].Focused {
		t.Error("workspace[0] should be focused")
	}
	if got[1].Output != "DP-2" {
		t.Errorf("workspace[1].Output = %q, want %q", got[1].Output, "DP-2")
	}
	if !got[2].Urgent {
		t.Error("workspace[2] should be urgent")
	}
}

func TestRunCommand(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeRunCommand {
			t.Errorf("expected message type %d, got %d", MsgTypeRunCommand, msgType)
		}
		// Echo back success.
		return []byte(`[{"success":true}]`)
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	if err := c.FocusWorkspace("2:web"); err != nil {
		t.Fatalf("FocusWorkspace: %v", err)
	}
}

func TestRunCommandError(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		return []byte(`[{"success":false,"error":"No such workspace"}]`)
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	err = c.FocusWorkspace("nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := err.Error(); got != "i3 command: No such workspace" {
		t.Errorf("error = %q, want %q", got, "i3 command: No such workspace")
	}
}

func TestGetTree(t *testing.T) {
	tree := Node{
		ID:   1,
		Name: "root",
		Type: "root",
		Nodes: []Node{
			{
				ID:     2,
				Name:   "DP-1",
				Type:   "output",
				Output: "DP-1",
				Nodes: []Node{
					{
						ID:   3,
						Name: "content",
						Type: "con",
						Nodes: []Node{
							{
								ID:      100,
								Name:    "vim",
								Type:    "con",
								Focused: true,
							},
							{
								ID:   101,
								Name: "terminal",
								Type: "con",
							},
						},
					},
				},
			},
		},
	}

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeGetTree {
			t.Errorf("expected message type %d, got %d", MsgTypeGetTree, msgType)
		}
		data, _ := json.Marshal(tree)
		return data
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	root, err := c.GetTree()
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}

	windows := root.Windows()
	if len(windows) != 2 {
		t.Fatalf("expected 2 windows, got %d", len(windows))
	}
	if windows[0].Name != "vim" {
		t.Errorf("windows[0].Name = %q, want %q", windows[0].Name, "vim")
	}
	if !windows[0].Focused {
		t.Error("windows[0] should be focused")
	}
	if windows[0].Output != "DP-1" {
		t.Errorf("windows[0].Output = %q, want %q", windows[0].Output, "DP-1")
	}
	if windows[1].Name != "terminal" {
		t.Errorf("windows[1].Name = %q, want %q", windows[1].Name, "terminal")
	}
}

func TestConnectionError(t *testing.T) {
	_, err := NewClient("/tmp/nonexistent-i3-socket-test.sock")
	if err == nil {
		t.Fatal("expected error connecting to nonexistent socket")
	}
}

func TestMultipleCommands(t *testing.T) {
	callCount := 0
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		callCount++
		return []byte(`[{"success":true}]`)
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	// Send multiple commands on the same connection.
	if err := c.FocusWorkspace("1"); err != nil {
		t.Fatalf("FocusWorkspace(1): %v", err)
	}
	if err := c.MoveToWorkspace("2"); err != nil {
		t.Fatalf("MoveToWorkspace(2): %v", err)
	}
	if err := c.FocusWindow(42); err != nil {
		t.Fatalf("FocusWindow(42): %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 commands, server handled %d", callCount)
	}
}
