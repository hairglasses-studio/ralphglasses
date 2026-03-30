package sway

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Helper: skip when Sway is not running
// ---------------------------------------------------------------------------

func requireSway(t *testing.T) {
	t.Helper()
	if os.Getenv("SWAYSOCK") == "" {
		t.Skip("sway not running")
	}
}

// ---------------------------------------------------------------------------
// Integration tests — run only when Sway is available
// ---------------------------------------------------------------------------

func TestIntegration_Connect(t *testing.T) {
	requireSway(t)

	c, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()
}

func TestIntegration_GetTree(t *testing.T) {
	requireSway(t)

	c, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	root, err := c.GetTree()
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	if root == nil {
		t.Fatal("GetTree returned nil root")
	}
	// The root node in Sway is always named "root".
	if root.Name != "root" {
		t.Errorf("root.Name = %q, want %q", root.Name, "root")
	}
	if root.Type != "root" {
		t.Errorf("root.Type = %q, want %q", root.Type, "root")
	}
	// Root should have at least one child (an output node).
	if len(root.Nodes) == 0 {
		t.Error("root.Nodes is empty; expected at least one output")
	}
}

func TestIntegration_GetWorkspaces(t *testing.T) {
	requireSway(t)

	c, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	ws, err := c.GetWorkspaces()
	if err != nil {
		t.Fatalf("GetWorkspaces: %v", err)
	}
	if len(ws) == 0 {
		t.Fatal("GetWorkspaces returned 0 workspaces; expected at least 1")
	}

	// At least one workspace should be focused.
	hasFocused := false
	for _, w := range ws {
		if w.Focused {
			hasFocused = true
			break
		}
	}
	if !hasFocused {
		t.Error("no workspace is focused")
	}
}

func TestIntegration_GetOutputs(t *testing.T) {
	requireSway(t)

	c, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	outputs, err := c.GetOutputs()
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}
	if len(outputs) == 0 {
		t.Fatal("GetOutputs returned 0 outputs; expected at least 1")
	}

	// At least one output should be active.
	hasActive := false
	for _, o := range outputs {
		if o.Active {
			hasActive = true
			break
		}
	}
	if !hasActive {
		t.Error("no output is active")
	}
}

func TestIntegration_RunCommand_Nop(t *testing.T) {
	requireSway(t)

	c, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Close()

	// "nop" is a safe no-op command in Sway/i3.
	if err := c.RunCommand("nop"); err != nil {
		t.Fatalf("RunCommand(nop): %v", err)
	}
}

func TestIntegration_Lifecycle(t *testing.T) {
	requireSway(t)

	// Phase 1: connect.
	c, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Phase 2: query tree.
	root, err := c.GetTree()
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	if root == nil {
		t.Fatal("GetTree returned nil")
	}

	// Phase 3: query workspaces.
	ws, err := c.GetWorkspaces()
	if err != nil {
		t.Fatalf("GetWorkspaces: %v", err)
	}
	if len(ws) == 0 {
		t.Fatal("no workspaces")
	}

	// Phase 4: query outputs.
	outputs, err := c.GetOutputs()
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}
	if len(outputs) == 0 {
		t.Fatal("no outputs")
	}

	// Phase 5: safe command.
	if err := c.RunCommand("nop"); err != nil {
		t.Fatalf("RunCommand(nop): %v", err)
	}

	// Phase 6: close.
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock-based tests — always run, no Sway required
// ---------------------------------------------------------------------------

// mockServer creates a Unix socket that speaks the i3/Sway IPC protocol.
type mockServer struct {
	listener net.Listener
	sockPath string
}

func newMockServer(t *testing.T, handler func(msgType uint32, payload []byte) []byte) *mockServer {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "sway-test.sock")

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
		hdr := make([]byte, headerLen)
		if _, err := readFull(conn, hdr); err != nil {
			return
		}

		// Validate magic bytes.
		for i := 0; i < len(magic); i++ {
			if hdr[i] != magic[i] {
				return
			}
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
}

func TestMock_GetTree(t *testing.T) {
	tree := Node{
		ID:   1,
		Name: "root",
		Type: "root",
		Nodes: []Node{
			{
				ID:   2,
				Name: "DP-1",
				Type: "output",
				Nodes: []Node{
					{
						ID:   3,
						Name: "workspace-1",
						Type: "workspace",
						Nodes: []Node{
							{ID: 100, Name: "alacritty", Type: "con", Focused: true},
						},
					},
				},
			},
		},
	}

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeGetTree {
			t.Errorf("expected msg type %d, got %d", MsgTypeGetTree, msgType)
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
	if root.Name != "root" {
		t.Errorf("root.Name = %q, want %q", root.Name, "root")
	}
	if len(root.Nodes) != 1 {
		t.Fatalf("root.Nodes len = %d, want 1", len(root.Nodes))
	}
	if root.Nodes[0].Name != "DP-1" {
		t.Errorf("output.Name = %q, want %q", root.Nodes[0].Name, "DP-1")
	}
}

func TestMock_GetWorkspaces(t *testing.T) {
	workspaces := []Workspace{
		{Num: 1, Name: "1:code", Visible: true, Focused: true, Output: "DP-1"},
		{Num: 2, Name: "2:web", Visible: true, Focused: false, Output: "DP-2"},
	}

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeGetWorkspaces {
			t.Errorf("expected msg type %d, got %d", MsgTypeGetWorkspaces, msgType)
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
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "1:code" {
		t.Errorf("ws[0].Name = %q, want %q", got[0].Name, "1:code")
	}
	if !got[0].Focused {
		t.Error("ws[0] should be focused")
	}
	if got[1].Output != "DP-2" {
		t.Errorf("ws[1].Output = %q, want %q", got[1].Output, "DP-2")
	}
}

func TestMock_GetOutputs(t *testing.T) {
	outputs := []Output{
		{Name: "DP-1", Make: "Dell", Model: "U2723QE", Active: true, CurrentWorkspace: "1:code"},
		{Name: "HDMI-A-1", Make: "LG", Model: "27UK850", Active: true, CurrentWorkspace: "2:web"},
	}

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeGetOutputs {
			t.Errorf("expected msg type %d, got %d", MsgTypeGetOutputs, msgType)
		}
		data, _ := json.Marshal(outputs)
		return data
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	got, err := c.GetOutputs()
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Name != "DP-1" {
		t.Errorf("output[0].Name = %q, want %q", got[0].Name, "DP-1")
	}
	if !got[0].Active {
		t.Error("output[0] should be active")
	}
	if got[1].CurrentWorkspace != "2:web" {
		t.Errorf("output[1].CurrentWorkspace = %q, want %q", got[1].CurrentWorkspace, "2:web")
	}
}

func TestMock_RunCommand(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		if msgType != MsgTypeRunCommand {
			t.Errorf("expected msg type %d, got %d", MsgTypeRunCommand, msgType)
		}
		if string(payload) != "nop" {
			t.Errorf("payload = %q, want %q", string(payload), "nop")
		}
		return []byte(`[{"success":true}]`)
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	if err := c.RunCommand("nop"); err != nil {
		t.Fatalf("RunCommand(nop): %v", err)
	}
}

func TestMock_RunCommandError(t *testing.T) {
	srv := newMockServer(t, func(msgType uint32, _ []byte) []byte {
		return []byte(`[{"success":false,"error":"Unknown/invalid command"}]`)
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	err = c.RunCommand("badcommand")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	want := "sway command: Unknown/invalid command"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

func TestMock_MessageFormat(t *testing.T) {
	// Verify the client sends correctly formatted i3-ipc messages.
	var gotHeader []byte
	var gotPayload []byte

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		// Reconstruct what was sent by encoding it back.
		hdr := make([]byte, headerLen)
		copy(hdr, magic)
		binary.LittleEndian.PutUint32(hdr[6:10], uint32(len(payload)))
		binary.LittleEndian.PutUint32(hdr[10:14], msgType)
		gotHeader = hdr
		gotPayload = payload
		return []byte(`[{"success":true}]`)
	})
	defer srv.close()

	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	_ = c.RunCommand("nop")

	// Verify magic bytes.
	if string(gotHeader[:6]) != "i3-ipc" {
		t.Errorf("magic = %q, want %q", string(gotHeader[:6]), "i3-ipc")
	}

	// Verify message type is 0 (RUN_COMMAND).
	msgType := binary.LittleEndian.Uint32(gotHeader[10:14])
	if msgType != MsgTypeRunCommand {
		t.Errorf("msg type = %d, want %d", msgType, MsgTypeRunCommand)
	}

	// Verify payload length matches.
	payloadLen := binary.LittleEndian.Uint32(gotHeader[6:10])
	if payloadLen != uint32(len(gotPayload)) {
		t.Errorf("header payload len = %d, actual = %d", payloadLen, len(gotPayload))
	}

	// Verify payload content.
	if string(gotPayload) != "nop" {
		t.Errorf("payload = %q, want %q", string(gotPayload), "nop")
	}
}

func TestMock_Lifecycle(t *testing.T) {
	callCount := 0

	srv := newMockServer(t, func(msgType uint32, payload []byte) []byte {
		callCount++
		switch msgType {
		case MsgTypeGetTree:
			return []byte(`{"id":1,"name":"root","type":"root","nodes":[]}`)
		case MsgTypeGetWorkspaces:
			return []byte(`[{"num":1,"name":"1","visible":true,"focused":true,"output":"DP-1"}]`)
		case MsgTypeGetOutputs:
			return []byte(`[{"name":"DP-1","active":true,"current_workspace":"1"}]`)
		case MsgTypeRunCommand:
			return []byte(`[{"success":true}]`)
		default:
			return []byte(`{}`)
		}
	})
	defer srv.close()

	// Connect.
	c, err := NewClient(srv.sockPath)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Query tree.
	root, err := c.GetTree()
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	if root.Name != "root" {
		t.Errorf("root.Name = %q", root.Name)
	}

	// Query workspaces.
	ws, err := c.GetWorkspaces()
	if err != nil {
		t.Fatalf("GetWorkspaces: %v", err)
	}
	if len(ws) != 1 {
		t.Errorf("workspaces len = %d", len(ws))
	}

	// Query outputs.
	outputs, err := c.GetOutputs()
	if err != nil {
		t.Fatalf("GetOutputs: %v", err)
	}
	if len(outputs) != 1 {
		t.Errorf("outputs len = %d", len(outputs))
	}

	// Run command.
	if err := c.RunCommand("nop"); err != nil {
		t.Fatalf("RunCommand: %v", err)
	}

	// Close.
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if callCount != 4 {
		t.Errorf("expected 4 IPC calls, got %d", callCount)
	}
}

func TestMock_ConnectionError(t *testing.T) {
	_, err := NewClient("/tmp/nonexistent-sway-socket-test.sock")
	if err == nil {
		t.Fatal("expected error connecting to nonexistent socket")
	}
}

func TestMock_ConnectWithoutSWAYSOCK(t *testing.T) {
	// Temporarily unset SWAYSOCK to test the Connect() error path.
	orig := os.Getenv("SWAYSOCK")
	os.Unsetenv("SWAYSOCK")
	defer func() {
		if orig != "" {
			os.Setenv("SWAYSOCK", orig)
		}
	}()

	_, err := Connect()
	if err == nil {
		t.Fatal("expected error when SWAYSOCK is not set")
	}
}
