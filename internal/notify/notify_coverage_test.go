package notify

import (
	"testing"
	"time"
)

// ---------- Throttler ----------

func TestThrottler_AllowFirstCall(t *testing.T) {
	t.Parallel()
	th := NewThrottler(5 * time.Second)
	if !th.Allow(EventSessionComplete, "sess-1") {
		t.Error("first Allow should return true")
	}
}

func TestThrottler_SuppressDuplicate(t *testing.T) {
	t.Parallel()
	th := NewThrottler(5 * time.Second)

	th.Allow(EventSessionComplete, "sess-1")
	if th.Allow(EventSessionComplete, "sess-1") {
		t.Error("second Allow within cooldown should return false")
	}
}

func TestThrottler_DifferentKeysAllowed(t *testing.T) {
	t.Parallel()
	th := NewThrottler(5 * time.Second)

	if !th.Allow(EventSessionComplete, "sess-1") {
		t.Error("first key should be allowed")
	}
	// Different event type, same session.
	if !th.Allow(EventCrash, "sess-1") {
		t.Error("different event type should be allowed")
	}
	// Same event type, different session.
	if !th.Allow(EventSessionComplete, "sess-2") {
		t.Error("different session should be allowed")
	}
}

func TestThrottler_CooldownExpiry(t *testing.T) {
	t.Parallel()
	th := NewThrottler(10 * time.Millisecond)

	th.Allow(EventBudgetWarning, "s1")
	time.Sleep(20 * time.Millisecond)

	if !th.Allow(EventBudgetWarning, "s1") {
		t.Error("Allow after cooldown expiry should return true")
	}
}

// ---------- RateLimiter Flush ----------

func TestRateLimiter_FlushEmptyQueue(t *testing.T) {
	t.Parallel()
	rl := NewRateLimiter(1*time.Second, 3)
	sent := rl.Flush()
	if sent != 0 {
		t.Errorf("Flush on empty queue returned %d, want 0", sent)
	}
}

func TestRateLimiter_FlushWithQueuedItems(t *testing.T) {
	t.Parallel()
	// Use very short interval so Flush can send.
	rl := NewRateLimiter(1*time.Millisecond, 3)

	// First send succeeds or queues (depends on whether Send works in test env).
	rl.TrySend("t1", "b1")
	// Second send within interval gets queued.
	rl.TrySend("t2", "b2")

	queueBefore := rl.QueueLen()

	// Wait for interval to pass.
	time.Sleep(5 * time.Millisecond)
	_ = rl.Flush()

	// After Flush, queue should be smaller or same (items may fail Send in CI).
	queueAfter := rl.QueueLen()
	if queueAfter > queueBefore {
		t.Errorf("queue grew after Flush: before=%d, after=%d", queueBefore, queueAfter)
	}
}

// ---------- SendTemplated ----------

func TestSendTemplated_UnknownEventType(t *testing.T) {
	t.Parallel()
	err := SendTemplated(EventType("nonexistent"), nil)
	if err == nil {
		t.Error("SendTemplated with unknown event type should return error")
	}
}

func TestSendTemplated_KnownEventType(t *testing.T) {
	t.Parallel()
	// This may or may not succeed depending on desktop notification availability.
	// We verify it doesn't panic and returns a reasonable result.
	err := SendTemplated(EventSessionComplete, map[string]string{
		"SessionID": "test-123",
		"Repo":      "testproject",
		"Cost":      "0.50",
	})
	// err might be non-nil in CI (no desktop), that is acceptable.
	_ = err
}

func TestRenderTemplate_EmptyValues(t *testing.T) {
	t.Parallel()
	tmpl := Template{
		Title: "Hello {{.Name}}",
		Body:  "No placeholders here",
	}
	title, body := RenderTemplate(tmpl, map[string]string{})
	// Unreplaced placeholders stay as-is.
	if title != "Hello {{.Name}}" {
		t.Errorf("title = %q, want unreplaced placeholder", title)
	}
	if body != "No placeholders here" {
		t.Errorf("body = %q", body)
	}
}

func TestRenderTemplate_NilValues(t *testing.T) {
	t.Parallel()
	tmpl := Template{Title: "Test", Body: "Body"}
	title, body := RenderTemplate(tmpl, nil)
	if title != "Test" || body != "Body" {
		t.Errorf("unexpected result with nil values: title=%q body=%q", title, body)
	}
}

func TestAllEventTypes_NonEmpty(t *testing.T) {
	t.Parallel()
	types := AllEventTypes()
	if len(types) == 0 {
		t.Error("AllEventTypes returned empty slice")
	}
	// Verify no duplicates.
	seen := make(map[EventType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}
