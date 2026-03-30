package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func slackTestNotification() Notification {
	return Notification{
		Title:     "Session Started",
		Body:      "Session: sess-001 | Repo: ralphglasses | Provider: claude",
		Severity:  "info",
		EventType: events.SessionStarted,
		SessionID: "sess-001",
		RepoName:  "ralphglasses",
		Provider:  "claude",
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
		Data:      map[string]any{"cost": 1.25},
	}
}

func TestSlackNotifier_Send(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []byte
	var headers http.Header

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = body
		headers = r.Header.Clone()
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer ts.Close()

	notifier := NewSlackNotifier(ts.URL)
	err := notifier.Send(context.Background(), slackTestNotification())
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify content type.
	if got := headers.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}

	// Verify payload structure.
	var msg SlackMessage
	if err := json.Unmarshal(captured, &msg); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	// Should have fallback text.
	if msg.Text == "" {
		t.Error("expected non-empty fallback text")
	}

	// Should have exactly 1 attachment with color sidebar.
	if len(msg.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
	}

	att := msg.Attachments[0]
	if att.Color != "#2EB67D" {
		t.Errorf("attachment Color = %q; want #2EB67D (green for info)", att.Color)
	}

	// Should have at least header + field section blocks.
	if len(att.Blocks) < 2 {
		t.Errorf("expected at least 2 blocks in attachment, got %d", len(att.Blocks))
	}

	// First block should be header.
	if att.Blocks[0].Type != "header" {
		t.Errorf("first block type = %q; want header", att.Blocks[0].Type)
	}

	// Second block should be section with fields.
	if att.Blocks[1].Type != "section" {
		t.Errorf("second block type = %q; want section", att.Blocks[1].Type)
	}
	if len(att.Blocks[1].Fields) < 2 {
		t.Errorf("expected at least 2 fields (event + timestamp), got %d", len(att.Blocks[1].Fields))
	}
}

func TestSlackNotifier_FormatMessage_Severity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		severity  string
		wantColor string
		wantTag   string
	}{
		{"info", "info", "#2EB67D", "INFO"},
		{"warning", "warning", "#ECB22E", "WARNING"},
		{"error", "error", "#E01E5A", "ERROR"},
	}

	notifier := NewSlackNotifier("https://hooks.slack.com/test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := slackTestNotification()
			n.Severity = tt.severity
			msg := notifier.FormatMessage(n)

			if len(msg.Attachments) != 1 {
				t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
			}
			if msg.Attachments[0].Color != tt.wantColor {
				t.Errorf("Color = %q; want %q", msg.Attachments[0].Color, tt.wantColor)
			}
			if !strings.Contains(msg.Text, tt.wantTag) {
				t.Errorf("Text = %q; want to contain %q", msg.Text, tt.wantTag)
			}
		})
	}
}

func TestSlackNotifier_FormatMessage_ActionButtons(t *testing.T) {
	t.Parallel()

	notifier := NewSlackNotifier("https://hooks.slack.com/test")

	t.Run("error_with_session_has_buttons", func(t *testing.T) {
		t.Parallel()
		n := slackTestNotification()
		n.Severity = "error"
		n.SessionID = "sess-err"
		msg := notifier.FormatMessage(n)

		// Find actions block.
		var actionsBlock *SlackBlock
		for i := range msg.Attachments[0].Blocks {
			if msg.Attachments[0].Blocks[i].Type == "actions" {
				actionsBlock = &msg.Attachments[0].Blocks[i]
				break
			}
		}
		if actionsBlock == nil {
			t.Fatal("expected actions block for error severity")
		}

		// Should have View Session + Acknowledge buttons.
		if len(actionsBlock.Elements) != 2 {
			t.Fatalf("expected 2 action elements, got %d", len(actionsBlock.Elements))
		}

		if actionsBlock.Elements[0].Text.Text != "View Session" {
			t.Errorf("first button text = %q; want View Session", actionsBlock.Elements[0].Text.Text)
		}
		if actionsBlock.Elements[0].Style != "primary" {
			t.Errorf("first button style = %q; want primary", actionsBlock.Elements[0].Style)
		}
		if actionsBlock.Elements[1].Text.Text != "Acknowledge" {
			t.Errorf("second button text = %q; want Acknowledge", actionsBlock.Elements[1].Text.Text)
		}
		if actionsBlock.Elements[1].Style != "danger" {
			t.Errorf("second button style = %q; want danger", actionsBlock.Elements[1].Style)
		}
	})

	t.Run("warning_with_session_has_view_button", func(t *testing.T) {
		t.Parallel()
		n := slackTestNotification()
		n.Severity = "warning"
		n.SessionID = "sess-warn"
		msg := notifier.FormatMessage(n)

		var actionsBlock *SlackBlock
		for i := range msg.Attachments[0].Blocks {
			if msg.Attachments[0].Blocks[i].Type == "actions" {
				actionsBlock = &msg.Attachments[0].Blocks[i]
				break
			}
		}
		if actionsBlock == nil {
			t.Fatal("expected actions block for warning severity")
		}

		// Warning gets View Session but NOT Acknowledge.
		if len(actionsBlock.Elements) != 1 {
			t.Fatalf("expected 1 action element for warning, got %d", len(actionsBlock.Elements))
		}
		if actionsBlock.Elements[0].Text.Text != "View Session" {
			t.Errorf("button text = %q; want View Session", actionsBlock.Elements[0].Text.Text)
		}
	})

	t.Run("info_has_no_actions", func(t *testing.T) {
		t.Parallel()
		n := slackTestNotification()
		n.Severity = "info"
		msg := notifier.FormatMessage(n)

		for _, b := range msg.Attachments[0].Blocks {
			if b.Type == "actions" {
				t.Error("info severity should not have actions block")
			}
		}
	})
}

func TestSlackNotifier_FormatMessage_MinimalNotification(t *testing.T) {
	t.Parallel()

	notifier := NewSlackNotifier("https://hooks.slack.com/test")
	n := Notification{
		Title:     "Test",
		Severity:  "info",
		EventType: events.ConfigChanged,
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}
	msg := notifier.FormatMessage(n)

	if len(msg.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(msg.Attachments))
	}

	// Should still have header + field section.
	if len(msg.Attachments[0].Blocks) < 2 {
		t.Errorf("expected at least 2 blocks, got %d", len(msg.Attachments[0].Blocks))
	}

	// Field section should have event + timestamp but no session/repo/provider.
	fieldSection := msg.Attachments[0].Blocks[1]
	for _, f := range fieldSection.Fields {
		if strings.Contains(f.Text, "*Session:*") ||
			strings.Contains(f.Text, "*Repo:*") ||
			strings.Contains(f.Text, "*Provider:*") {
			t.Errorf("unexpected field in minimal notification: %s", f.Text)
		}
	}
}

func TestSlackNotifier_FormatMessage_DataSection(t *testing.T) {
	t.Parallel()

	notifier := NewSlackNotifier("https://hooks.slack.com/test")
	n := slackTestNotification()
	n.Data = map[string]any{"cost": 5.0, "step": 3}
	msg := notifier.FormatMessage(n)

	// Find data section block.
	found := false
	for _, b := range msg.Attachments[0].Blocks {
		if b.Type == "section" && b.Text != nil && strings.Contains(b.Text.Text, "*cost:*") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a data section block with cost field")
	}
}

func TestSlackNotifier_WithOptions(t *testing.T) {
	t.Parallel()

	// Default values.
	s := NewSlackNotifier("https://hooks.slack.com/test")
	if s.username != "ralphglasses" {
		t.Errorf("default username = %q; want ralphglasses", s.username)
	}
	if s.client == nil {
		t.Fatal("default client is nil")
	}

	// Custom username.
	s2 := NewSlackNotifier("https://hooks.slack.com/test",
		WithSlackUsername("custom-bot"),
	)
	if s2.username != "custom-bot" {
		t.Errorf("custom username = %q; want custom-bot", s2.username)
	}

	// Custom client.
	customClient := &http.Client{Timeout: 30 * time.Second}
	s3 := NewSlackNotifier("https://hooks.slack.com/test",
		WithSlackClient(customClient),
	)
	if s3.client != customClient {
		t.Error("custom client was not applied")
	}

	// Multiple options.
	s4 := NewSlackNotifier("https://hooks.slack.com/test",
		WithSlackUsername("multi"),
		WithSlackClient(customClient),
	)
	if s4.username != "multi" || s4.client != customClient {
		t.Error("multiple options were not all applied")
	}
}

func TestSlackNotifier_InvalidURL(t *testing.T) {
	t.Parallel()

	notifier := NewSlackNotifier("://invalid-url")
	err := notifier.Send(context.Background(), slackTestNotification())
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestSlackNotifier_ServerError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	notifier := NewSlackNotifier(ts.URL)
	err := notifier.Send(context.Background(), slackTestNotification())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
	if !strings.Contains(err.Error(), "slack: http 500") {
		t.Errorf("error = %q; want to contain 'slack: http 500'", err.Error())
	}
}

func TestSlackNotifier_ContextCancelled(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // slow server
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	notifier := NewSlackNotifier(ts.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := notifier.Send(ctx, slackTestNotification())
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestSlackNotifier_PayloadJSON(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var captured []byte

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		captured = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok")) //nolint:errcheck
	}))
	defer ts.Close()

	notifier := NewSlackNotifier(ts.URL)
	n := slackTestNotification()
	n.Severity = "error"
	err := notifier.Send(context.Background(), n)
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Verify it's valid JSON that round-trips.
	var msg SlackMessage
	if err := json.Unmarshal(captured, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Re-marshal and verify no data loss.
	roundTripped, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}

	var msg2 SlackMessage
	if err := json.Unmarshal(roundTripped, &msg2); err != nil {
		t.Fatalf("unmarshal round-tripped: %v", err)
	}

	if msg2.Text != msg.Text {
		t.Errorf("round-trip text mismatch: %q vs %q", msg2.Text, msg.Text)
	}
	if len(msg2.Attachments) != len(msg.Attachments) {
		t.Errorf("round-trip attachment count mismatch: %d vs %d",
			len(msg2.Attachments), len(msg.Attachments))
	}
}

func TestFormatSlack_BackwardCompat(t *testing.T) {
	t.Parallel()

	ev := events.Event{
		Type:      events.SessionStarted,
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
		SessionID: "sess-001",
		RepoName:  "ralphglasses",
		Provider:  "claude",
	}

	payload := FormatSlack(ev)

	if len(payload.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(payload.Attachments))
	}

	att := payload.Attachments[0]
	if att.Color != "#2EB67D" {
		t.Errorf("Color = %q; want #2EB67D (green)", att.Color)
	}
	if len(att.Blocks) < 2 {
		t.Errorf("expected at least 2 blocks, got %d", len(att.Blocks))
	}
}
