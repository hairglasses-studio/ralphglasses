package hyprland

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestParseEvent(t *testing.T) {
	tests := []struct {
		line     string
		wantType EventType
		wantData string
		wantOK   bool
	}{
		{"workspace>>3", EventWorkspace, "3", true},
		{"activewindow>>kitty,nvim", EventActiveWindow, "kitty,nvim", true},
		{"openwindow>>0xabc,1,kitty,zsh", EventOpenWindow, "0xabc,1,kitty,zsh", true},
		{"closewindow>>0xabc", EventCloseWindow, "0xabc", true},
		{"monitoradded>>DP-2", EventMonitorAdded, "DP-2", true},
		{"monitorremoved>>DP-2", EventMonitorRemoved, "DP-2", true},
		{"submap>>resize", EventSubmap, "resize", true},
		{"fullscreen>>1", EventFullscreen, "1", true},
		{"configreloaded>>", EventConfigReloaded, "", true},
		// Edge cases
		{"workspace>>", EventWorkspace, "", true}, // empty data
		{"custom>>a>>b", "custom", "a>>b", true},  // multiple >> separators
		{"noevent", Event{}.Type, "", false},      // no separator
		{"", Event{}.Type, "", false},             // empty line
	}

	for _, tt := range tests {
		evt, ok := ParseEvent(tt.line)
		if ok != tt.wantOK {
			t.Errorf("ParseEvent(%q): ok=%v, want %v", tt.line, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if evt.Type != tt.wantType {
			t.Errorf("ParseEvent(%q): Type=%q, want %q", tt.line, evt.Type, tt.wantType)
		}
		if evt.Data != tt.wantData {
			t.Errorf("ParseEvent(%q): Data=%q, want %q", tt.line, evt.Data, tt.wantData)
		}
	}
}

// startMockEventSocket creates a Unix socket that writes test events, returning
// the connected client-side connection and a cleanup function.
func startMockEventSocket(t *testing.T) (net.Conn, *Client, func()) {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "hypr-evt-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	sockPath := filepath.Join(dir, ".s2.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	// Also create the command socket path for Client
	cmdSockPath := filepath.Join(dir, ".s.sock")
	cmdLn, err := net.Listen("unix", cmdSockPath)
	if err != nil {
		ln.Close()
		t.Fatalf("listen cmd: %v", err)
	}

	client := newClientFromDir(dir)

	// Accept one connection (the event listener will connect)
	var serverConn net.Conn
	var serverErr error
	accepted := make(chan struct{})
	go func() {
		serverConn, serverErr = ln.Accept()
		close(accepted)
	}()

	// Connect client side
	clientConn, err := net.Dial("unix", sockPath)
	if err != nil {
		ln.Close()
		cmdLn.Close()
		t.Fatalf("dial: %v", err)
	}

	<-accepted
	if serverErr != nil {
		clientConn.Close()
		ln.Close()
		cmdLn.Close()
		t.Fatalf("accept: %v", serverErr)
	}

	cleanup := func() {
		clientConn.Close()
		if serverConn != nil {
			serverConn.Close()
		}
		ln.Close()
		cmdLn.Close()
	}

	// We return clientConn (for the EventListener) and serverConn is available
	// via the closure for writing test events
	_ = serverConn // captured in closure for writing

	return clientConn, client, cleanup
}

func TestEventListenerReceivesEvents(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "hypr-evt-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, ".s2.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	client := newClientFromDir(dir)

	// Write events from server side
	var serverConn net.Conn
	accepted := make(chan struct{})
	go func() {
		var acceptErr error
		serverConn, acceptErr = ln.Accept()
		if acceptErr != nil {
			return
		}
		close(accepted)
	}()

	// Connect the event listener
	clientConn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	<-accepted

	el := newEventListenerForTest(clientConn, client)
	defer el.Close()

	// Collect events
	var mu sync.Mutex
	var received []Event

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start listener in background
	listenDone := make(chan error, 1)
	go func() {
		listenDone <- el.Listen(ctx, func(evt Event) {
			mu.Lock()
			received = append(received, evt)
			mu.Unlock()
			// Cancel after receiving expected events
			if len(received) >= 3 {
				cancel()
			}
		})
	}()

	// Write test events from server
	time.Sleep(50 * time.Millisecond) // let listener start
	events := "workspace>>3\nactivewindow>>kitty,nvim\nopenwindow>>0xabc,1,Alacritty,zsh\n"
	if _, err := serverConn.Write([]byte(events)); err != nil {
		t.Fatalf("write events: %v", err)
	}

	// Wait for listener to finish
	<-listenDone

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}

	if received[0].Type != EventWorkspace || received[0].Data != "3" {
		t.Errorf("event[0] = %v, want workspace>>3", received[0])
	}
	if received[1].Type != EventActiveWindow || received[1].Data != "kitty,nvim" {
		t.Errorf("event[1] = %v, want activewindow>>kitty,nvim", received[1])
	}
	if received[2].Type != EventOpenWindow || received[2].Data != "0xabc,1,Alacritty,zsh" {
		t.Errorf("event[2] = %v, want openwindow>>0xabc,1,Alacritty,zsh", received[2])
	}
}

func TestEventListenerClose(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "hypr-evt-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, ".s2.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	client := newClientFromDir(dir)

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			// Keep connection open until test completes
			time.Sleep(5 * time.Second)
			conn.Close()
		}
	}()

	clientConn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	el := newEventListenerForTest(clientConn, client)

	// Close should be idempotent
	if err := el.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := el.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestEventListenerContextCancel(t *testing.T) {
	dir, err := os.MkdirTemp(os.TempDir(), "hypr-evt-*")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	defer os.RemoveAll(dir)
	sockPath := filepath.Join(dir, ".s2.sock")

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	client := newClientFromDir(dir)

	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			time.Sleep(5 * time.Second)
			conn.Close()
		}
	}()

	clientConn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	el := newEventListenerForTest(clientConn, client)
	defer el.Close()

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- el.Listen(ctx, func(Event) {})
	}()

	// Cancel immediately
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("Listen returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Listen did not return after context cancel")
	}
}
