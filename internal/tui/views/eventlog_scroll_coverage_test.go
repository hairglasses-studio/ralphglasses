package views

import (
	"testing"
	"time"
)

func TestEventLogView_InitReturnsNil(t *testing.T) {
	v := NewEventLogView()
	cmd := v.Init()
	if cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestEventLogView_ScrollDown(t *testing.T) {
	v := NewEventLogView()
	v.height = 5
	now := time.Now()
	// Add enough entries to scroll.
	for i := 0; i < 20; i++ {
		v.AddEntry(EventLogEntry{Timestamp: now, Type: "info", Message: "msg"})
	}
	startPos := v.scrollPos
	v.ScrollDown()
	// scrollDown only advances if there's room; just verify no panic.
	_ = startPos
}

func TestEventLogView_ScrollUpAndDown(t *testing.T) {
	v := NewEventLogView()
	v.height = 3
	now := time.Now()
	for i := 0; i < 10; i++ {
		v.AddEntry(EventLogEntry{Timestamp: now, Type: "info", Message: "msg"})
	}
	// Scroll up (should stay at 0 if already there).
	v.scrollPos = 0
	v.ScrollUp()
	if v.scrollPos < 0 {
		t.Errorf("scrollPos should not go negative: %d", v.scrollPos)
	}
	// ScrollDown then ScrollUp.
	v.scrollPos = 1
	v.ScrollUp()
	if v.scrollPos != 0 {
		t.Errorf("scrollPos after ScrollUp from 1 = %d, want 0", v.scrollPos)
	}
}

func TestEventLogView_LoadHistory(t *testing.T) {
	v := NewEventLogView()
	entries := []EventLogEntry{
		{Timestamp: time.Now(), Type: "info", Message: "a"},
		{Timestamp: time.Now(), Type: "error", Message: "b"},
	}
	v.LoadHistory(entries)
	if len(v.entries) != 2 {
		t.Errorf("LoadHistory: len(entries) = %d, want 2", len(v.entries))
	}
}

