package notify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// dbusDefaults fills in zero-value desktop notification fields.
func dbusDefaults(n Notification) Notification {
	if n.Urgency == "" {
		n.Urgency = "normal"
	}
	return n
}

// DBusNotifier sends desktop notifications over D-Bus using command-line
// tools (notify-send or gdbus). No cgo dependency is required.
type DBusNotifier struct {
	busAddress string
	appName    string
}

// NewDBusNotifier creates a DBusNotifier with the given application name.
// It captures the current DBUS_SESSION_BUS_ADDRESS from the environment.
func NewDBusNotifier(appName string) *DBusNotifier {
	return &DBusNotifier{
		busAddress: os.Getenv("DBUS_SESSION_BUS_ADDRESS"),
		appName:    appName,
	}
}

// IsAvailable returns true if a D-Bus session bus is accessible and at
// least one supported notification tool (notify-send or gdbus) is on PATH.
func (d *DBusNotifier) IsAvailable() bool {
	if d.busAddress == "" {
		return false
	}
	if _, err := exec.LookPath("notify-send"); err == nil {
		return true
	}
	if _, err := exec.LookPath("gdbus"); err == nil {
		return true
	}
	return false
}

// Send dispatches a desktop notification via D-Bus. It prefers notify-send
// when available and falls back to gdbus. The context controls cancellation
// and timeout of the underlying command execution.
func (d *DBusNotifier) Send(ctx context.Context, notification Notification) error {
	notification = dbusDefaults(notification)

	if d.busAddress == "" {
		return fmt.Errorf("dbus: DBUS_SESSION_BUS_ADDRESS not set")
	}

	name, args := d.formatCommand(notification)
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "DBUS_SESSION_BUS_ADDRESS="+d.busAddress)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("dbus: %s failed: %w (output: %s)", name, err, string(output))
	}
	return nil
}

// formatCommand builds the executable name and argument list for the
// notification. It prefers notify-send when available, falling back to gdbus.
func (d *DBusNotifier) formatCommand(n Notification) (string, []string) {
	if _, err := exec.LookPath("notify-send"); err == nil {
		return d.formatNotifySend(n)
	}
	return d.formatGdbus(n)
}

// formatNotifySend builds args for the notify-send command.
func (d *DBusNotifier) formatNotifySend(n Notification) (string, []string) {
	args := []string{
		"--app-name=" + d.appName,
		"--urgency=" + n.Urgency,
	}
	if n.Icon != "" {
		args = append(args, "--icon="+n.Icon)
	}
	if n.Timeout > 0 {
		args = append(args, "--expire-time="+strconv.FormatInt(n.Timeout.Milliseconds(), 10))
	}
	args = append(args, n.Title, n.Body)
	return "notify-send", args
}

// formatGdbus builds args for the gdbus command using the
// org.freedesktop.Notifications interface.
func (d *DBusNotifier) formatGdbus(n Notification) (string, []string) {
	timeoutMs := int64(-1) // server default
	if n.Timeout > 0 {
		timeoutMs = n.Timeout.Milliseconds()
	}

	icon := n.Icon
	if icon == "" {
		icon = ""
	}

	// gdbus call --session --dest org.freedesktop.Notifications \
	//   --object-path /org/freedesktop/Notifications \
	//   --method org.freedesktop.Notifications.Notify \
	//   app_name replaces_id icon summary body actions hints timeout
	args := []string{
		"call",
		"--session",
		"--dest", "org.freedesktop.Notifications",
		"--object-path", "/org/freedesktop/Notifications",
		"--method", "org.freedesktop.Notifications.Notify",
		d.appName,             // app_name
		"0",                   // replaces_id (uint32)
		icon,                  // icon
		n.Title,               // summary
		n.Body,                // body
		"[]",                  // actions (empty array)
		"{}",                  // hints (empty dict)
		fmt.Sprintf("%d", timeoutMs), // timeout in ms
	}
	return "gdbus", args
}
