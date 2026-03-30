//go:build !linux

package healthz

import (
	"context"
	"time"
)

// SDNotify is a no-op on non-Linux platforms. The sd_notify protocol is
// Linux-specific (systemd).
func SDNotify(_ string) error { return nil }

// Ready is a no-op on non-Linux platforms.
func Ready() error { return nil }

// Stopping is a no-op on non-Linux platforms.
func Stopping() error { return nil }

// SetStatus is a no-op on non-Linux platforms.
func SetStatus(_ string) error { return nil }

// WatchdogEnabled always returns false on non-Linux platforms.
func WatchdogEnabled() (bool, time.Duration) { return false, 0 }

// StartWatchdog is a no-op on non-Linux platforms.
func StartWatchdog(_ context.Context) error { return nil }
