package hyprland

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// ---------- fixture data ----------

const monitorsJSON = `[
  {
    "id": 0,
    "name": "DP-1",
    "description": "Dell U2723QE",
    "make": "Dell Inc.",
    "model": "U2723QE",
    "serial": "ABC123",
    "width": 3840,
    "height": 2160,
    "refreshRate": 60.0,
    "x": 0,
    "y": 0,
    "activeWorkspace": {"id": 1, "name": "1"},
    "specialWorkspace": {"id": 0, "name": ""},
    "scale": 1.5,
    "transform": 0,
    "focused": true,
    "dpmsStatus": true,
    "vrr": false
  },
  {
    "id": 1,
    "name": "HDMI-A-1",
    "description": "LG 27UK850",
    "make": "LG Electronics",
    "model": "27UK850",
    "serial": "XYZ789",
    "width": 3840,
    "height": 2160,
    "refreshRate": 59.94,
    "x": 3840,
    "y": 0,
    "activeWorkspace": {"id": 2, "name": "2"},
    "specialWorkspace": {"id": 0, "name": ""},
    "scale": 1.0,
    "transform": 0,
    "focused": false,
    "dpmsStatus": true,
    "vrr": false
  }
]`

const workspacesJSON = `[
  {
    "id": 1,
    "name": "1",
    "monitor": "DP-1",
    "monitorID": 0,
    "windows": 2,
    "hasfullscreen": false,
    "lastwindow": "0x563a1b2c3d4e",
    "lastwindowtitle": "nvim"
  },
  {
    "id": 2,
    "name": "2",
    "monitor": "HDMI-A-1",
    "monitorID": 1,
    "windows": 1,
    "hasfullscreen": false,
    "lastwindow": "0x563a1b2c3d5f",
    "lastwindowtitle": "firefox"
  }
]`

const windowsJSON = `[
  {
    "address": "0x563a1b2c3d4e",
    "mapped": true,
    "hidden": false,
    "at": [0, 0],
    "size": [1920, 1080],
    "workspace": {"id": 1, "name": "1"},
    "floating": false,
    "monitor": 0,
    "class": "kitty",
    "title": "nvim",
    "initialClass": "kitty",
    "initialTitle": "kitty",
    "pid": 12345,
    "xwayland": false,
    "pinned": false,
    "fullscreen": 0,
    "fakeFullscreen": false,
    "grouped": [],
    "swallowing": "",
    "focusHistoryID": 0
  }
]`

const activeWindowJSON = `{
  "address": "0x563a1b2c3d4e",
  "mapped": true,
  "hidden": false,
  "at": [100, 200],
  "size": [800, 600],
  "workspace": {"id": 1, "name": "1"},
  "floating": true,
  "monitor": 0,
  "class": "kitty",
  "title": "zsh",
  "initialClass": "kitty",
  "initialTitle": "kitty",
  "pid": 54321,
  "xwayland": false,
  "pinned": false,
  "fullscreen": 0,
  "fakeFullscreen": false,
  "grouped": [],
  "swallowing": "",
  "focusHistoryID": 0
}`

// ---------- JSON parsing tests ----------

func TestParseMonitors(t *testing.T) {
	var monitors []Monitor
	if err := json.Unmarshal([]byte(monitorsJSON), &monitors); err != nil {
		t.Fatalf("unmarshal monitors: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
	m := monitors[0]
	if m.Name != "DP-1" {
		t.Errorf("monitor[0].Name = %q, want DP-1", m.Name)
	}
	if m.Width != 3840 || m.Height != 2160 {
		t.Errorf("monitor[0] resolution = %dx%d, want 3840x2160", m.Width, m.Height)
	}
	if m.ActiveWorkspace.ID != 1 {
		t.Errorf("monitor[0].ActiveWorkspace.ID = %d, want 1", m.ActiveWorkspace.ID)
	}
	if !m.Focused {
		t.Error("monitor[0].Focused = false, want true")
	}
	if m.Scale != 1.5 {
		t.Errorf("monitor[0].Scale = %f, want 1.5", m.Scale)
	}
}

func TestParseWorkspaces(t *testing.T) {
	var workspaces []Workspace
	if err := json.Unmarshal([]byte(workspacesJSON), &workspaces); err != nil {
		t.Fatalf("unmarshal workspaces: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}
	ws := workspaces[1]
	if ws.Monitor != "HDMI-A-1" {
		t.Errorf("workspace[1].Monitor = %q, want HDMI-A-1", ws.Monitor)
	}
	if ws.Windows != 1 {
		t.Errorf("workspace[1].Windows = %d, want 1", ws.Windows)
	}
	if ws.LastWindowTitle != "firefox" {
		t.Errorf("workspace[1].LastWindowTitle = %q, want firefox", ws.LastWindowTitle)
	}
}

func TestParseWindows(t *testing.T) {
	var windows []Window
	if err := json.Unmarshal([]byte(windowsJSON), &windows); err != nil {
		t.Fatalf("unmarshal windows: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	w := windows[0]
	if w.Address != "0x563a1b2c3d4e" {
		t.Errorf("window.Address = %q, want 0x563a1b2c3d4e", w.Address)
	}
	if w.Class != "kitty" {
		t.Errorf("window.Class = %q, want kitty", w.Class)
	}
	if w.PID != 12345 {
		t.Errorf("window.PID = %d, want 12345", w.PID)
	}
	if w.At != [2]int{0, 0} {
		t.Errorf("window.At = %v, want [0 0]", w.At)
	}
	if w.Size != [2]int{1920, 1080} {
		t.Errorf("window.Size = %v, want [1920 1080]", w.Size)
	}
	if w.Workspace.ID != 1 {
		t.Errorf("window.Workspace.ID = %d, want 1", w.Workspace.ID)
	}
}

func TestParseActiveWindow(t *testing.T) {
	var w Window
	if err := json.Unmarshal([]byte(activeWindowJSON), &w); err != nil {
		t.Fatalf("unmarshal activewindow: %v", err)
	}
	if w.Address != "0x563a1b2c3d4e" {
		t.Errorf("Address = %q, want 0x563a1b2c3d4e", w.Address)
	}
	if !w.Floating {
		t.Error("Floating = false, want true")
	}
	if w.PID != 54321 {
		t.Errorf("PID = %d, want 54321", w.PID)
	}
}

// ---------- command formatting ----------

func TestFormatCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		args []string
		want string
	}{
		{"workspace", []string{"3"}, "dispatch workspace 3"},
		{"movetoworkspacesilent", []string{"2,address:0xabc"}, "dispatch movetoworkspacesilent 2,address:0xabc"},
		{"togglefloating", nil, "dispatch togglefloating"},
		{"exec", []string{"kitty", "--title", "test"}, "dispatch exec kitty --title test"},
	}
	for _, tt := range tests {
		got := FormatCommand(tt.cmd, tt.args...)
		if got != tt.want {
			t.Errorf("FormatCommand(%q, %v) = %q, want %q", tt.cmd, tt.args, got, tt.want)
		}
	}
}

// ---------- socket path construction ----------

func TestResolveSocketDir(t *testing.T) {
	const sig = "test-resolve-sig"

	t.Run("xdg_exists", func(t *testing.T) {
		xdgDir := t.TempDir()
		sigDir := filepath.Join(xdgDir, "hypr", sig)
		os.MkdirAll(sigDir, 0o755)
		t.Setenv("XDG_RUNTIME_DIR", xdgDir)
		got := resolveSocketDir(sig)
		if got != sigDir {
			t.Errorf("got %q, want %q", got, sigDir)
		}
	})

	t.Run("xdg_preferred_over_legacy", func(t *testing.T) {
		// Both XDG and legacy exist — XDG should win.
		xdgDir := t.TempDir()
		sigDir := filepath.Join(xdgDir, "hypr", sig)
		os.MkdirAll(sigDir, 0o755)
		t.Setenv("XDG_RUNTIME_DIR", xdgDir)
		got := resolveSocketDir(sig)
		if got != sigDir {
			t.Errorf("got %q, want XDG path %q", got, sigDir)
		}
	})

	t.Run("no_xdg_falls_back_to_tmp", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "")
		got := resolveSocketDir(sig)
		want := filepath.Join("/tmp", "hypr", sig)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("neither_exists_xdg_set", func(t *testing.T) {
		xdgDir := t.TempDir() // exists, but hypr/$sig does not
		t.Setenv("XDG_RUNTIME_DIR", xdgDir)
		got := resolveSocketDir(sig)
		want := filepath.Join(xdgDir, "hypr", sig)
		if got != want {
			t.Errorf("got %q, want XDG path %q", got, want)
		}
	})

	t.Run("neither_exists_no_xdg", func(t *testing.T) {
		t.Setenv("XDG_RUNTIME_DIR", "")
		got := resolveSocketDir(sig)
		want := filepath.Join("/tmp", "hypr", sig)
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestSocketPaths(t *testing.T) {
	c := &Client{socketDir: "/tmp/hypr/test-sig"}
	if got := c.SocketPath(); got != "/tmp/hypr/test-sig/.socket.sock" {
		t.Errorf("SocketPath = %q", got)
	}
	if got := c.EventSocketPath(); got != "/tmp/hypr/test-sig/.socket2.sock" {
		t.Errorf("EventSocketPath = %q", got)
	}
}

// ---------- NewClient error handling ----------

func TestNewClientNoEnv(t *testing.T) {
	orig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	os.Unsetenv("HYPRLAND_INSTANCE_SIGNATURE")
	defer func() {
		if orig != "" {
			os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", orig)
		}
	}()

	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error when HYPRLAND_INSTANCE_SIGNATURE is unset")
	}
}

func TestNewClientBadDir(t *testing.T) {
	orig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "nonexistent-test-signature-xyz")
	defer func() {
		if orig != "" {
			os.Setenv("HYPRLAND_INSTANCE_SIGNATURE", orig)
		} else {
			os.Unsetenv("HYPRLAND_INSTANCE_SIGNATURE")
		}
	}()

	_, err := NewClient()
	if err == nil {
		t.Fatal("expected error for nonexistent socket directory")
	}
}

// ---------- mock socket IPC tests ----------

// startMockSocket creates a temporary Unix socket that responds with the
// given data to any incoming connection, then returns the Client and a
// cleanup function.
func startMockSocket(t *testing.T, response []byte) (*Client, func()) {
	t.Helper()
	dir := t.TempDir()
	sockPath := filepath.Join(dir, ".socket.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			// Read the request (we don't need it for the mock).
			buf := make([]byte, 4096)
			conn.Read(buf)
			conn.Write(response)
			conn.Close()
		}
	}()

	c := newClientFromDir(dir)
	cleanup := func() {
		ln.Close()
		<-done
	}
	return c, cleanup
}

func TestGetMonitorsIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte(monitorsJSON))
	defer cleanup()

	monitors, err := c.GetMonitors()
	if err != nil {
		t.Fatalf("GetMonitors: %v", err)
	}
	if len(monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(monitors))
	}
	if monitors[0].Name != "DP-1" {
		t.Errorf("monitors[0].Name = %q, want DP-1", monitors[0].Name)
	}
}

func TestGetWorkspacesIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte(workspacesJSON))
	defer cleanup()

	workspaces, err := c.GetWorkspaces()
	if err != nil {
		t.Fatalf("GetWorkspaces: %v", err)
	}
	if len(workspaces) != 2 {
		t.Fatalf("expected 2 workspaces, got %d", len(workspaces))
	}
}

func TestGetWindowsIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte(windowsJSON))
	defer cleanup()

	windows, err := c.GetWindows()
	if err != nil {
		t.Fatalf("GetWindows: %v", err)
	}
	if len(windows) != 1 {
		t.Fatalf("expected 1 window, got %d", len(windows))
	}
	if windows[0].Class != "kitty" {
		t.Errorf("windows[0].Class = %q, want kitty", windows[0].Class)
	}
}

func TestGetActiveWindowIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte(activeWindowJSON))
	defer cleanup()

	w, err := c.GetActiveWindow()
	if err != nil {
		t.Fatalf("GetActiveWindow: %v", err)
	}
	if !w.Floating {
		t.Error("expected floating window")
	}
}

func TestDispatchIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte("ok"))
	defer cleanup()

	if err := c.Dispatch("workspace", "3"); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
}

func TestDispatchErrorIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte("Invalid dispatcher"))
	defer cleanup()

	err := c.Dispatch("nonexistent")
	if err == nil {
		t.Fatal("expected error for invalid dispatcher")
	}
}

func TestMoveToWorkspaceIPC(t *testing.T) {
	c, cleanup := startMockSocket(t, []byte("ok"))
	defer cleanup()

	if err := c.MoveToWorkspace("0x563a1b2c3d4e", 3); err != nil {
		t.Fatalf("MoveToWorkspace: %v", err)
	}
}

func TestRequestConnectionError(t *testing.T) {
	// Client pointing at a directory with no socket.
	c := newClientFromDir(t.TempDir())
	_, err := c.GetMonitors()
	if err == nil {
		t.Fatal("expected connection error")
	}
}
