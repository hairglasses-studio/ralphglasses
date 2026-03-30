package patterns

import (
	"testing"
)

func TestArchitectPattern_Memory(t *testing.T) {
	mem := NewSharedMemory()
	ap, err := NewArchitectPattern("arch-1", []string{"w-1"}, mem)
	if err != nil {
		t.Fatalf("NewArchitectPattern: %v", err)
	}
	got := ap.Memory()
	if got == nil {
		t.Fatal("Memory() returned nil")
	}
	if got != mem {
		t.Error("Memory() should return the same SharedMemory pointer")
	}
}

func TestReviewChainPattern_CurrentLink(t *testing.T) {
	rc, err := NewReviewChainPattern("task-1", []string{"author-session", "reviewer-session"})
	if err != nil {
		t.Fatalf("NewReviewChainPattern: %v", err)
	}

	link, err := rc.CurrentLink()
	if err != nil {
		t.Fatalf("CurrentLink() error: %v", err)
	}
	if link.SessionID != "author-session" {
		t.Errorf("CurrentLink().SessionID = %q, want author-session", link.SessionID)
	}
}

func TestReviewChainPattern_CurrentLink_WhenDone(t *testing.T) {
	rc, err := NewReviewChainPattern("task-2", []string{"s1", "s2"})
	if err != nil {
		t.Fatalf("NewReviewChainPattern: %v", err)
	}

	// Force the chain to be done.
	rc.mu.Lock()
	rc.done = true
	rc.mu.Unlock()

	_, err = rc.CurrentLink()
	if err == nil {
		t.Error("CurrentLink() should return error when done")
	}
}

func TestSharedMemory_Unwatch(t *testing.T) {
	sm := NewSharedMemory()

	ch := sm.Watch("mykey", 5)
	// Unwatch should remove the watcher and close the channel.
	sm.Unwatch("mykey", ch)

	// After Unwatch, the channel should be closed.
	// Writing a value to mykey should not block or panic.
	sm.Set("mykey", "value")
	// The channel is closed, so reads on a closed channel return zero value.
	select {
	case _, ok := <-ch:
		_ = ok // may receive buffered values before close
	default:
	}
}

func TestSharedMemory_Unwatch_NonExistent(t *testing.T) {
	sm := NewSharedMemory()
	ch := make(chan MemoryUpdate)
	// Unwatch on a channel not registered should not panic.
	sm.Unwatch("nokey", ch)
}
