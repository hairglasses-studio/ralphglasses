package notify

import (
	"os/exec"
	"runtime"
	"sync"
	"time"
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

// Throttler deduplicates notifications by event type + session ID.
// A notification for the same key is suppressed if sent within the cooldown window.
type Throttler struct {
	cooldown time.Duration
	mu       sync.Mutex
	last     map[string]time.Time
}

// NewThrottler creates a throttler with the given cooldown (e.g. 60s).
func NewThrottler(cooldown time.Duration) *Throttler {
	return &Throttler{cooldown: cooldown, last: make(map[string]time.Time)}
}

// Allow returns true if a notification for this event+session should be sent.
func (t *Throttler) Allow(eventType EventType, sessionID string) bool {
	key := string(eventType) + ":" + sessionID
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.last[key]; ok && time.Since(last) < t.cooldown {
		return false
	}
	t.last[key] = time.Now()
	return true
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
