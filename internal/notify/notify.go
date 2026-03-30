package notify

import (
	"os/exec"
	"runtime"
)

// EventType defines the category of notification events.
type EventType string

const (
	EventSessionComplete    EventType = "session_complete"
	EventBudgetWarning      EventType = "budget_warning"
	EventCircuitBreakerTrip EventType = "circuit_breaker_trip"
	EventCrash              EventType = "crash"
	EventRestart            EventType = "restart"
)

// AllEventTypes returns all defined notification event types.
func AllEventTypes() []EventType {
	return []EventType{
		EventSessionComplete,
		EventBudgetWarning,
		EventCircuitBreakerTrip,
		EventCrash,
		EventRestart,
	}
}

// Send dispatches a desktop notification.
// macOS: osascript, Linux: notify-send, other: no-op.
func Send(title, body string) error {
	return sendForOS(runtime.GOOS, title, body)
}

func sendForOS(goos, title, body string) error {
	switch goos {
	case "darwin":
		script := `display notification "` + escapeOSA(body) + `" with title "` + escapeOSA(title) + `"`
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return nil // no-op if not available
		}
		return exec.Command("notify-send", title, body).Run()
	default:
		return nil
	}
}

func escapeOSA(s string) string {
	// Escape backslashes and double quotes for AppleScript strings
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			out = append(out, '\\', '\\')
		case '"':
			out = append(out, '\\', '"')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
