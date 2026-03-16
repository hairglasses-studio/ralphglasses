package components

import (
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// Notification is a temporary toast message.
type Notification struct {
	Message   string
	ExpiresAt time.Time
}

// NotificationManager holds active notifications.
type NotificationManager struct {
	Current *Notification
}

// Show displays a notification for a duration.
func (nm *NotificationManager) Show(msg string, dur time.Duration) {
	nm.Current = &Notification{
		Message:   msg,
		ExpiresAt: time.Now().Add(dur),
	}
}

// View returns the rendered notification, or empty if expired.
func (nm *NotificationManager) View() string {
	if nm.Current == nil || time.Now().After(nm.Current.ExpiresAt) {
		nm.Current = nil
		return ""
	}
	return styles.NotificationStyle.Render(nm.Current.Message)
}

// Active reports whether a notification is showing.
func (nm *NotificationManager) Active() bool {
	if nm.Current == nil {
		return false
	}
	if time.Now().After(nm.Current.ExpiresAt) {
		nm.Current = nil
		return false
	}
	return true
}
