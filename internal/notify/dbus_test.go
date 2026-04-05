package notify

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestDBusNotifier_NewDBusNotifier(t *testing.T) {
	n := NewDBusNotifier("testapp")
	if n == nil {
		t.Fatal("NewDBusNotifier returned nil")
	}
	if n.appName != "testapp" {
		t.Errorf("appName = %q, want %q", n.appName, "testapp")
	}
	// busAddress is read from env; we just verify it was captured.
	_ = n.busAddress
}

func TestDBusNotifier_IsAvailable(t *testing.T) {
	// With an empty bus address, IsAvailable must return false regardless
	// of whether notify-send or gdbus exist on PATH.
	n := &DBusNotifier{busAddress: "", appName: "test"}
	if n.IsAvailable() {
		t.Error("IsAvailable() = true with empty busAddress, want false")
	}

	// With a non-empty bus address, the result depends on whether
	// notify-send or gdbus are on PATH. We just verify it does not panic.
	n2 := &DBusNotifier{busAddress: "unix:path=/run/user/1000/bus", appName: "test"}
	_ = n2.IsAvailable() // may be true or false depending on host
}

func TestDBusNotifier_Send_NoDisplay(t *testing.T) {
	// When DBUS_SESSION_BUS_ADDRESS is empty, Send should return a
	// descriptive error rather than panicking or hanging.
	n := &DBusNotifier{busAddress: "", appName: "test"}
	err := n.Send(context.Background(), Notification{
		Title: "Test",
		Body:  "body",
	})
	if err == nil {
		t.Fatal("Send() with empty busAddress should return error")
	}
	if got := err.Error(); got == "" {
		t.Error("error message should not be empty")
	}
}

func TestDBusNotifier_FormatCommand(t *testing.T) {
	n := &DBusNotifier{busAddress: "unix:path=/tmp/test", appName: "ralph"}

	notif := Notification{
		Title:   "Build Complete",
		Body:    "Session finished successfully",
		Urgency: "low",
		Icon:    "dialog-information",
		Timeout: 5 * time.Second,
	}

	name, args := n.formatCommand(notif)

	// The command must be either notify-send or gdbus.
	switch name {
	case "notify-send":
		assertContains(t, args, "--app-name=ralph")
		assertContains(t, args, "--urgency=low")
		assertContains(t, args, "--icon=dialog-information")
		assertContains(t, args, "--expire-time=5000")
		// Title and body must be the last two args.
		if len(args) < 2 {
			t.Fatal("too few args")
		}
		if args[len(args)-2] != "Build Complete" {
			t.Errorf("title arg = %q, want %q", args[len(args)-2], "Build Complete")
		}
		if args[len(args)-1] != "Session finished successfully" {
			t.Errorf("body arg = %q, want %q", args[len(args)-1], "Session finished successfully")
		}

	case "gdbus":
		assertContains(t, args, "--session")
		assertContains(t, args, "--dest")
		assertContains(t, args, "org.freedesktop.Notifications")
		assertContains(t, args, "Build Complete")
		assertContains(t, args, "Session finished successfully")
		assertContains(t, args, "ralph")
		assertContains(t, args, "5000")

	default:
		t.Fatalf("unexpected command %q, want notify-send or gdbus", name)
	}
}

func TestDBusNotifier_FormatCommand_NoIcon(t *testing.T) {
	n := &DBusNotifier{busAddress: "unix:path=/tmp/test", appName: "ralph"}
	notif := Notification{
		Title: "Test",
		Body:  "body",
	}
	name, args := n.formatCommand(notif)
	if name != "notify-send" && name != "gdbus" {
		t.Fatalf("unexpected command %q", name)
	}
	// With no icon, notify-send should not have --icon flag.
	if name == "notify-send" {
		for _, a := range args {
			if len(a) > 7 && a[:7] == "--icon=" {
				t.Error("--icon flag should not be present when Icon is empty")
			}
		}
	}
}

func TestNotification_Defaults(t *testing.T) {
	n := Notification{Title: "T", Body: "B"}
	got := dbusDefaults(n)

	if got.Urgency != "normal" {
		t.Errorf("default Urgency = %q, want %q", got.Urgency, "normal")
	}
	if got.Title != "T" {
		t.Errorf("Title = %q, want %q", got.Title, "T")
	}
	if got.Body != "B" {
		t.Errorf("Body = %q, want %q", got.Body, "B")
	}

	// Explicit urgency should be preserved.
	n2 := Notification{Title: "T", Body: "B", Urgency: "critical"}
	got2 := dbusDefaults(n2)
	if got2.Urgency != "critical" {
		t.Errorf("explicit Urgency = %q, want %q", got2.Urgency, "critical")
	}
}

func assertContains(t *testing.T, slice []string, want string) {
	t.Helper()
	if slices.Contains(slice, want) {
		return
	}
	t.Errorf("args %v does not contain %q", slice, want)
}
