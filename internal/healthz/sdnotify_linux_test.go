//go:build linux

package healthz

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// startTestSocket creates a Unix datagram socket in a temp dir that
// collects all messages sent to it. Returns the socket path and a
// channel that receives each datagram as a string.
func startTestSocket(t *testing.T) (string, <-chan string) {
	t.Helper()

	dir := t.TempDir()
	sock := filepath.Join(dir, "notify.sock")

	conn, err := net.ListenPacket("unixgram", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ch := make(chan string, 64)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			ch <- string(buf[:n])
		}
	}()

	return sock, ch
}

func TestSDNotify(t *testing.T) {
	sock, ch := startTestSocket(t)
	t.Setenv("NOTIFY_SOCKET", sock)

	if err := SDNotify("READY=1"); err != nil {
		t.Fatalf("SDNotify: %v", err)
	}

	select {
	case msg := <-ch:
		if msg != "READY=1" {
			t.Errorf("got %q, want %q", msg, "READY=1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestReady(t *testing.T) {
	sock, ch := startTestSocket(t)
	t.Setenv("NOTIFY_SOCKET", sock)

	if err := Ready(); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	select {
	case msg := <-ch:
		if msg != "READY=1" {
			t.Errorf("got %q, want %q", msg, "READY=1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestStopping(t *testing.T) {
	sock, ch := startTestSocket(t)
	t.Setenv("NOTIFY_SOCKET", sock)

	if err := Stopping(); err != nil {
		t.Fatalf("Stopping: %v", err)
	}

	select {
	case msg := <-ch:
		if msg != "STOPPING=1" {
			t.Errorf("got %q, want %q", msg, "STOPPING=1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSetStatus(t *testing.T) {
	sock, ch := startTestSocket(t)
	t.Setenv("NOTIFY_SOCKET", sock)

	if err := SetStatus("running 5 sessions"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	select {
	case msg := <-ch:
		want := "STATUS=running 5 sessions"
		if msg != want {
			t.Errorf("got %q, want %q", msg, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

func TestSDNotifyNoSocket(t *testing.T) {
	t.Setenv("NOTIFY_SOCKET", "")

	// Should be a no-op, no error.
	if err := SDNotify("READY=1"); err != nil {
		t.Fatalf("expected nil error without NOTIFY_SOCKET, got: %v", err)
	}
}

func TestWatchdogEnabled(t *testing.T) {
	tests := []struct {
		name    string
		usec    string
		pid     string
		wantOn  bool
		wantDur time.Duration
	}{
		{
			name:   "not set",
			usec:   "",
			wantOn: false,
		},
		{
			name:    "2 seconds",
			usec:    "2000000",
			wantOn:  true,
			wantDur: 2 * time.Second,
		},
		{
			name:   "zero",
			usec:   "0",
			wantOn: false,
		},
		{
			name:   "invalid",
			usec:   "notanumber",
			wantOn: false,
		},
		{
			name:    "500ms",
			usec:    "500000",
			wantOn:  true,
			wantDur: 500 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("WATCHDOG_USEC", tt.usec)
			if tt.pid != "" {
				t.Setenv("WATCHDOG_PID", tt.pid)
			} else {
				t.Setenv("WATCHDOG_PID", "")
			}

			on, dur := WatchdogEnabled()
			if on != tt.wantOn {
				t.Errorf("enabled = %v, want %v", on, tt.wantOn)
			}
			if dur != tt.wantDur {
				t.Errorf("duration = %v, want %v", dur, tt.wantDur)
			}
		})
	}
}

func TestWatchdogEnabledPIDMismatch(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "2000000")
	t.Setenv("WATCHDOG_PID", "99999999") // unlikely to be our PID

	on, _ := WatchdogEnabled()
	if on {
		t.Error("expected disabled when PID does not match")
	}
}

func TestWatchdogEnabledPIDMatch(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "2000000")

	// With empty WATCHDOG_PID, PID check is skipped — should be enabled.
	t.Setenv("WATCHDOG_PID", "")
	on, dur := WatchdogEnabled()
	if !on || dur != 2*time.Second {
		t.Errorf("expected enabled with 2s, got on=%v dur=%v", on, dur)
	}

	// Now set to our actual PID — should still be enabled.
	t.Setenv("WATCHDOG_PID", fmt.Sprintf("%d", os.Getpid()))
	on, dur = WatchdogEnabled()
	if !on || dur != 2*time.Second {
		t.Errorf("expected enabled with matching PID, got on=%v dur=%v", on, dur)
	}
}

func TestStartWatchdog(t *testing.T) {
	sock, ch := startTestSocket(t)
	t.Setenv("NOTIFY_SOCKET", sock)
	t.Setenv("WATCHDOG_USEC", "100000") // 100ms → tick every 50ms
	t.Setenv("WATCHDOG_PID", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := StartWatchdog(ctx); err != nil {
		t.Fatalf("StartWatchdog: %v", err)
	}

	// Collect at least 2 watchdog pings.
	var pings int
	deadline := time.After(2 * time.Second)
	for pings < 2 {
		select {
		case msg := <-ch:
			if msg == "WATCHDOG=1" {
				pings++
			}
		case <-deadline:
			t.Fatalf("only received %d pings, wanted at least 2", pings)
		}
	}

	// Cancel and verify goroutine stops (no more pings after drain).
	cancel()
	time.Sleep(150 * time.Millisecond)

	// Drain any in-flight messages.
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func TestStartWatchdogNotEnabled(t *testing.T) {
	t.Setenv("WATCHDOG_USEC", "")
	t.Setenv("NOTIFY_SOCKET", "")

	// Should return nil without starting anything.
	if err := StartWatchdog(context.Background()); err != nil {
		t.Fatalf("expected nil when watchdog not enabled, got: %v", err)
	}
}
