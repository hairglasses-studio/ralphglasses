package session

import (
	"context"
	"testing"
	"time"
)

type mockNotifier struct {
	called int
	lastReq EscalationRequest
}

func (m *mockNotifier) Notify(req EscalationRequest) error {
	m.called++
	m.lastReq = req
	return nil
}

func TestEscalation_Timeout(t *testing.T) {
	t.Parallel()
	h := NewEscalationHandler(nil)
	resp, err := h.Escalate(context.Background(), EscalationRequest{
		SessionID:     "sess-1",
		Question:      "continue?",
		Timeout:       50 * time.Millisecond,
		DefaultAction: "stop",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.TimedOut {
		t.Error("expected timeout")
	}
	if resp.Action != "stop" {
		t.Errorf("expected default action 'stop', got %q", resp.Action)
	}
}

func TestEscalation_Respond(t *testing.T) {
	t.Parallel()
	notifier := &mockNotifier{}
	h := NewEscalationHandler(notifier)

	done := make(chan *EscalationResponse, 1)
	go func() {
		resp, _ := h.Escalate(context.Background(), EscalationRequest{
			SessionID: "sess-2",
			Question:  "approve?",
			Timeout:   5 * time.Second,
		})
		done <- resp
	}()

	// Give escalation time to register.
	time.Sleep(20 * time.Millisecond)

	if h.PendingCount() != 1 {
		t.Errorf("expected 1 pending, got %d", h.PendingCount())
	}

	ok := h.Respond("sess-2", "approved", "looks good")
	if !ok {
		t.Fatal("respond returned false")
	}

	resp := <-done
	if resp.Action != "approved" {
		t.Errorf("expected 'approved', got %q", resp.Action)
	}
	if resp.TimedOut {
		t.Error("should not have timed out")
	}
	if notifier.called != 1 {
		t.Errorf("expected notifier called once, got %d", notifier.called)
	}
}

func TestEscalation_ContextCancel(t *testing.T) {
	t.Parallel()
	h := NewEscalationHandler(nil)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	resp, err := h.Escalate(ctx, EscalationRequest{
		SessionID:     "sess-3",
		Question:      "continue?",
		Timeout:       5 * time.Second,
		DefaultAction: "abort",
	})
	if err == nil {
		t.Error("expected context error")
	}
	if resp.Action != "abort" {
		t.Errorf("expected default 'abort', got %q", resp.Action)
	}
}

func TestEscalation_EmptyQuestion(t *testing.T) {
	t.Parallel()
	h := NewEscalationHandler(nil)
	_, err := h.Escalate(context.Background(), EscalationRequest{})
	if err == nil {
		t.Error("expected error for empty question")
	}
}

func TestEscalation_RespondNoSession(t *testing.T) {
	t.Parallel()
	h := NewEscalationHandler(nil)
	ok := h.Respond("nonexistent", "ok", "")
	if ok {
		t.Error("expected false for nonexistent session")
	}
}

func TestShouldEscalate(t *testing.T) {
	t.Parallel()
	destructive := &Intent{Type: IntentStop, Destructive: true}
	safe := &Intent{Type: IntentQuery, Destructive: false}

	cases := []struct {
		name       string
		level      int
		intent     *Intent
		confidence float64
		expected   bool
	}{
		{"L0-destructive", 0, destructive, 0.9, true},
		{"L0-safe", 0, safe, 0.9, false},
		{"L1-destructive", 1, destructive, 0.9, true},
		{"L2-low-confidence", 2, safe, 0.5, true},
		{"L2-high-confidence", 2, safe, 0.9, false},
		{"L2-destructive", 2, destructive, 0.9, true},
		{"L3-destructive", 3, destructive, 0.9, false},
		{"L3-safe", 3, safe, 0.5, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := ShouldEscalate(tc.level, tc.intent, tc.confidence)
			if result != tc.expected {
				t.Errorf("ShouldEscalate(%d, %s, %.1f) = %v, want %v",
					tc.level, tc.intent.Type, tc.confidence, result, tc.expected)
			}
		})
	}
}
