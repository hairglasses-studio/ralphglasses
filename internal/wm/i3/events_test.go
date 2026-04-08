package i3

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// testSockPath returns a short socket path that fits within the Unix domain
// socket name limit (108 bytes on macOS). t.TempDir() paths can exceed this.
func testSockPath(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp(os.TempDir(), "i3t")
	if err != nil {
		t.Fatalf("mkdirtemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, name)
}

func TestEncodeDecodeIPCMessage(t *testing.T) {
	// Verify that writeMessage + readMessage round-trip correctly.
	sock := testSockPath(t, "rt.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the message sent by the client.
		payload, msgType, err := readMessage(conn)
		if err != nil {
			t.Errorf("server readMessage: %v", err)
			return
		}
		if msgType != 42 {
			t.Errorf("server msgType = %d, want 42", msgType)
		}
		if string(payload) != `{"hello":"world"}` {
			t.Errorf("server payload = %q, want %q", payload, `{"hello":"world"}`)
		}

		// Write a response back.
		if err := writeMessage(conn, 99, []byte(`{"ok":true}`)); err != nil {
			t.Errorf("server writeMessage: %v", err)
		}
	}()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Write a message.
	if err := writeMessage(conn, 42, []byte(`{"hello":"world"}`)); err != nil {
		t.Fatalf("client writeMessage: %v", err)
	}

	// Read the response.
	payload, msgType, err := readMessage(conn)
	if err != nil {
		t.Fatalf("client readMessage: %v", err)
	}
	if msgType != 99 {
		t.Errorf("client msgType = %d, want 99", msgType)
	}
	if string(payload) != `{"ok":true}` {
		t.Errorf("client payload = %q, want %q", payload, `{"ok":true}`)
	}

	<-done
}

func TestEncodeDecodeEmptyPayload(t *testing.T) {
	sock := testSockPath(t, "em.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		payload, msgType, err := readMessage(conn)
		if err != nil {
			t.Errorf("readMessage: %v", err)
			return
		}
		if msgType != 7 {
			t.Errorf("msgType = %d, want 7", msgType)
		}
		if payload != nil {
			t.Errorf("payload = %v, want nil", payload)
		}
	}()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := writeMessage(conn, 7, nil); err != nil {
		t.Fatalf("writeMessage: %v", err)
	}

	<-done
}

func TestInvalidMagicBytes(t *testing.T) {
	sock := testSockPath(t, "bm.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		// Write a header with bad magic bytes.
		bad := make([]byte, headerLen)
		copy(bad, []byte("BADIPC"))
		binary.LittleEndian.PutUint32(bad[6:10], 0)
		binary.LittleEndian.PutUint32(bad[10:14], 0)
		_, _ = conn.Write(bad)
	}()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_, _, err = readMessage(conn)
	if err == nil {
		t.Fatal("expected error for invalid magic bytes")
	}
}

func TestEventTypeParsing(t *testing.T) {
	tests := []struct {
		et   EventType
		name string
	}{
		{WorkspaceEvent, "workspace"},
		{OutputEvent, "output"},
		{ModeEvent, "mode"},
		{WindowEvent, "window"},
		{EventType(99), "unknown(99)"},
	}
	for _, tt := range tests {
		if got := tt.et.String(); got != tt.name {
			t.Errorf("EventType(%d).String() = %q, want %q", tt.et, got, tt.name)
		}
	}
}

// mockEventServer is an i3-like server that accepts subscriptions and pushes events.
type mockEventServer struct {
	listener net.Listener
	sockPath string
	conns    []net.Conn
	mu       sync.Mutex
}

func newMockEventServer(t *testing.T) *mockEventServer {
	t.Helper()
	sock := testSockPath(t, "ev.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	s := &mockEventServer{listener: ln, sockPath: sock}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			s.mu.Lock()
			s.conns = append(s.conns, conn)
			s.mu.Unlock()

			go s.handleConn(conn)
		}
	}()

	return s
}

func (s *mockEventServer) handleConn(conn net.Conn) {
	for {
		payload, msgType, err := readMessage(conn)
		if err != nil {
			return
		}
		_ = payload

		// Respond to subscribe messages.
		if msgType == msgTypeSubscribe {
			resp := []byte(`{"success":true}`)
			if err := writeMessage(conn, msgTypeSubscribe, resp); err != nil {
				return
			}
		}
	}
}

// pushEvent sends an event to all connected clients.
func (s *mockEventServer) pushEvent(evtType EventType, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msgType := uint32(evtType) | eventBit
	for _, conn := range s.conns {
		_ = writeMessage(conn, msgType, payload)
	}
}

func (s *mockEventServer) close() {
	s.listener.Close()
	s.mu.Lock()
	for _, c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
}

func TestListenerSubscribeAndListen(t *testing.T) {
	srv := newMockEventServer(t)
	defer srv.close()

	el, err := NewEventListenerFromPath(srv.sockPath)
	if err != nil {
		t.Fatalf("NewEventListenerFromPath: %v", err)
	}
	defer el.Close()

	if err := el.Subscribe(WorkspaceEvent, WindowEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var received []Event
	var mu sync.Mutex
	ready := make(chan struct{}, 1)

	go func() {
		close(ready)
		_ = el.Listen(ctx, func(evt Event) {
			mu.Lock()
			received = append(received, evt)
			mu.Unlock()
			if len(received) >= 2 {
				cancel()
			}
		})
	}()

	<-ready
	// Give the listener time to start reading.
	time.Sleep(50 * time.Millisecond)

	// Push a workspace event.
	wsPayload, _ := json.Marshal(map[string]string{"change": "focus", "name": "1:code"})
	srv.pushEvent(WorkspaceEvent, wsPayload)

	// Push a window event.
	winPayload, _ := json.Marshal(map[string]string{"change": "new", "container": "vim"})
	srv.pushEvent(WindowEvent, winPayload)

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(received))
	}

	if received[0].Type != WorkspaceEvent {
		t.Errorf("event[0].Type = %v, want WorkspaceEvent", received[0].Type)
	}
	if received[0].Change != "focus" {
		t.Errorf("event[0].Change = %q, want %q", received[0].Change, "focus")
	}

	if received[1].Type != WindowEvent {
		t.Errorf("event[1].Type = %v, want WindowEvent", received[1].Type)
	}
	if received[1].Change != "new" {
		t.Errorf("event[1].Change = %q, want %q", received[1].Change, "new")
	}
}

func TestListenerClose(t *testing.T) {
	srv := newMockEventServer(t)
	defer srv.close()

	el, err := NewEventListenerFromPath(srv.sockPath)
	if err != nil {
		t.Fatalf("NewEventListenerFromPath: %v", err)
	}

	// Subscribe first.
	if err := el.Subscribe(WorkspaceEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Close should be idempotent.
	if err := el.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := el.Close(); err != nil {
		t.Fatalf("Close (second): %v", err)
	}

	// Subscribe after close should fail.
	if err := el.Subscribe(WindowEvent); err == nil {
		t.Fatal("expected error subscribing on closed listener")
	}
}

func TestListenerReconnect(t *testing.T) {
	sock := testSockPath(t, "rc.sock")

	// Start initial server.
	ln1, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	var subscribeCount atomic.Int32

	handleConns := func(ln net.Listener) {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				for {
					_, msgType, err := readMessage(conn)
					if err != nil {
						return
					}
					if msgType == msgTypeSubscribe {
						subscribeCount.Add(1)
						_ = writeMessage(conn, msgTypeSubscribe, []byte(`{"success":true}`))
					}
				}
			}()
		}
	}

	go handleConns(ln1)

	el, err := NewEventListenerFromPath(sock)
	if err != nil {
		t.Fatalf("NewEventListenerFromPath: %v", err)
	}
	defer el.Close()

	// Use a fast reconnect delay for testing.
	el.reconnectDelay = 10 * time.Millisecond
	el.maxReconnect = 50 * time.Millisecond

	if err := el.Subscribe(WorkspaceEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Close the first server to trigger reconnect.
	ln1.Close()

	// Start a new server on the same socket path.
	// Small delay to simulate server restart.
	time.Sleep(20 * time.Millisecond)
	ln2, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen (second): %v", err)
	}
	defer ln2.Close()

	go handleConns(ln2)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// The listener will get a read error (first server gone), attempt reconnect,
	// connect to the second server, and re-subscribe.
	listenerDone := make(chan error, 1)
	go func() {
		listenerDone <- el.Listen(ctx, func(evt Event) {})
	}()

	// Wait for reconnect to happen (subscribe count should reach 2).
	deadline := time.After(2 * time.Second)
	for subscribeCount.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for re-subscribe; subscribe count = %d", subscribeCount.Load())
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	cancel()
	<-listenerDone

	if got := subscribeCount.Load(); got < 2 {
		t.Errorf("expected at least 2 subscribe calls (initial + reconnect), got %d", got)
	}
}

func TestListenerContextCancel(t *testing.T) {
	srv := newMockEventServer(t)
	defer srv.close()

	el, err := NewEventListenerFromPath(srv.sockPath)
	if err != nil {
		t.Fatalf("NewEventListenerFromPath: %v", err)
	}
	defer el.Close()

	if err := el.Subscribe(WorkspaceEvent); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- el.Listen(ctx, func(evt Event) {})
	}()

	// Cancel context; Listen should return promptly.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("Listen returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Listen did not return after context cancel")
	}
}
