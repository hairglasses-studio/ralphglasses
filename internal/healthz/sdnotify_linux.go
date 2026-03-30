//go:build linux

package healthz

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// SDNotify sends a notification to the systemd service manager via the
// NOTIFY_SOCKET Unix datagram socket. It is a no-op if NOTIFY_SOCKET is
// not set. Common state strings: "READY=1", "STOPPING=1", "WATCHDOG=1",
// "STATUS=...".
func SDNotify(state string) error {
	socketAddr := os.Getenv("NOTIFY_SOCKET")
	if socketAddr == "" {
		return nil
	}

	// Support abstract and path-based sockets.
	if socketAddr[0] == '@' {
		socketAddr = "\x00" + socketAddr[1:]
	}

	conn, err := net.Dial("unixgram", socketAddr)
	if err != nil {
		return fmt.Errorf("sd_notify dial: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(state))
	if err != nil {
		return fmt.Errorf("sd_notify write: %w", err)
	}
	return nil
}

// Ready notifies systemd that the service startup is complete.
func Ready() error {
	return SDNotify("READY=1")
}

// Stopping notifies systemd that the service is beginning its shutdown.
func Stopping() error {
	return SDNotify("STOPPING=1")
}

// SetStatus sets a free-form status string visible via "systemctl status".
func SetStatus(status string) error {
	return SDNotify("STATUS=" + status)
}

// WatchdogEnabled checks whether the systemd watchdog is enabled for this
// service. It returns true and the watchdog interval if WATCHDOG_USEC is
// set to a positive value and (if set) WATCHDOG_PID matches the current
// process. The returned duration is the full interval; callers should
// notify at half the interval.
func WatchdogEnabled() (bool, time.Duration) {
	usecStr := os.Getenv("WATCHDOG_USEC")
	if usecStr == "" {
		return false, 0
	}

	usec, err := strconv.ParseUint(strings.TrimSpace(usecStr), 10, 64)
	if err != nil || usec == 0 {
		return false, 0
	}

	// If WATCHDOG_PID is set, it must match our PID.
	if pidStr := os.Getenv("WATCHDOG_PID"); pidStr != "" {
		pid, err := strconv.Atoi(strings.TrimSpace(pidStr))
		if err != nil || pid != os.Getpid() {
			return false, 0
		}
	}

	return true, time.Duration(usec) * time.Microsecond
}

// StartWatchdog starts a goroutine that sends WATCHDOG=1 at half the
// configured watchdog interval. It returns immediately. If the watchdog
// is not enabled, it returns nil without starting anything. The goroutine
// exits when ctx is cancelled.
func StartWatchdog(ctx context.Context) error {
	enabled, interval := WatchdogEnabled()
	if !enabled {
		return nil
	}

	tick := interval / 2
	if tick <= 0 {
		return fmt.Errorf("sd_notify: watchdog interval too small: %v", interval)
	}

	go func() {
		ticker := time.NewTicker(tick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = SDNotify("WATCHDOG=1")
			}
		}
	}()

	return nil
}
