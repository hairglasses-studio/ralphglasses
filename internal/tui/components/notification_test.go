package components

import (
	"testing"
	"time"
)

func TestNotificationShow(t *testing.T) {
	nm := NotificationManager{}
	nm.Show("hello", 5*time.Second)
	if nm.Current == nil {
		t.Fatal("Current is nil after Show")
	}
	if nm.Current.Message != "hello" {
		t.Errorf("Message = %q", nm.Current.Message)
	}
}

func TestNotificationActive(t *testing.T) {
	nm := NotificationManager{}
	if nm.Active() {
		t.Error("should not be active when empty")
	}
	nm.Show("test", 1*time.Hour)
	if !nm.Active() {
		t.Error("should be active after Show")
	}
}

func TestNotificationExpired(t *testing.T) {
	nm := NotificationManager{}
	nm.Show("expired", 1*time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if nm.Active() {
		t.Error("should not be active after expiry")
	}
	if nm.View() != "" {
		t.Error("expired view should be empty")
	}
}

func TestNotificationView(t *testing.T) {
	nm := NotificationManager{}
	nm.Show("hello world", 5*time.Second)
	if nm.View() == "" {
		t.Error("active notification view should not be empty")
	}
}
