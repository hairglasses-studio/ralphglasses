package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func testNotification() Notification {
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

func TestDiscordNotifier_Send(t *testing.T) {
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
		w.WriteHeader(http.StatusNoContent) // Discord returns 204
	}))
	defer ts.Close()

	notifier := NewDiscordNotifier(ts.URL, WithDiscordUsername("test-bot"))
	err := notifier.Send(context.Background(), testNotification())
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
	var payload DiscordPayload
	if err := json.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload.Username != "test-bot" {
		t.Errorf("Username = %q; want test-bot", payload.Username)
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(payload.Embeds))
	}

	embed := payload.Embeds[0]
	if embed.Color != 0x00ff00 {
		t.Errorf("embed Color = 0x%06x; want 0x00ff00 (green for info)", embed.Color)
	}
	if embed.Timestamp != "2026-03-30T12:00:00Z" {
		t.Errorf("embed Timestamp = %q; want 2026-03-30T12:00:00Z", embed.Timestamp)
	}

	// Verify fields contain event type.
	foundEvent := false
	for _, f := range embed.Fields {
		if f.Name == "Event" {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Error("expected an Event field in the embed")
	}
}

func TestDiscordNotifier_FormatEmbed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity string
		wantColor int
		wantTag  string
	}{
		{"info", "info", 0x00ff00, "INFO"},
		{"warning", "warning", 0xffff00, "WARNING"},
		{"error", "error", 0xff0000, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			n := testNotification()
			n.Severity = tt.severity
			embed := FormatEmbed(n)

			if embed.Color != tt.wantColor {
				t.Errorf("Color = 0x%06x; want 0x%06x", embed.Color, tt.wantColor)
			}
			if len(embed.Title) == 0 {
				t.Fatal("Title is empty")
			}
			if got := embed.Title; !containsStr(got, tt.wantTag) {
				t.Errorf("Title = %q; want to contain %q", got, tt.wantTag)
			}
		})
	}

	// Verify fields are populated.
	n := testNotification()
	embed := FormatEmbed(n)

	// Event + Session + Repo + Provider + 1 data key = at least 5
	if len(embed.Fields) < 5 {
		t.Errorf("expected at least 5 fields, got %d", len(embed.Fields))
	}

	// Verify timestamp.
	if embed.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestDiscordNotifier_WithOptions(t *testing.T) {
	t.Parallel()

	// Default values.
	d := NewDiscordNotifier("https://discord.com/api/webhooks/test")
	if d.username != "ralphglasses" {
		t.Errorf("default username = %q; want ralphglasses", d.username)
	}
	if d.client == nil {
		t.Fatal("default client is nil")
	}

	// Custom username.
	d2 := NewDiscordNotifier("https://discord.com/api/webhooks/test",
		WithDiscordUsername("custom-bot"),
	)
	if d2.username != "custom-bot" {
		t.Errorf("custom username = %q; want custom-bot", d2.username)
	}

	// Custom client.
	customClient := &http.Client{Timeout: 30 * time.Second}
	d3 := NewDiscordNotifier("https://discord.com/api/webhooks/test",
		WithDiscordClient(customClient),
	)
	if d3.client != customClient {
		t.Error("custom client was not applied")
	}

	// Multiple options.
	d4 := NewDiscordNotifier("https://discord.com/api/webhooks/test",
		WithDiscordUsername("multi"),
		WithDiscordClient(customClient),
	)
	if d4.username != "multi" || d4.client != customClient {
		t.Error("multiple options were not all applied")
	}
}

func TestDiscordNotifier_InvalidURL(t *testing.T) {
	t.Parallel()

	notifier := NewDiscordNotifier("://invalid-url")
	err := notifier.Send(context.Background(), testNotification())
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestDiscordNotifier_ServerError(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	notifier := NewDiscordNotifier(ts.URL)
	err := notifier.Send(context.Background(), testNotification())
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestDiscordNotifier_ContextCancelled(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // slow server
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	notifier := NewDiscordNotifier(ts.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := notifier.Send(ctx, testNotification())
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestNotificationFromEvent(t *testing.T) {
	t.Parallel()

	ev := events.Event{
		Type:      events.BudgetExceeded,
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
		SessionID: "sess-001",
		RepoName:  "ralphglasses",
		Provider:  "claude",
		Data:      map[string]any{"cost": 5.0},
	}

	n := NotificationFromEvent(ev)
	if n.Severity != "error" {
		t.Errorf("Severity = %q; want error for BudgetExceeded", n.Severity)
	}
	if n.EventType != events.BudgetExceeded {
		t.Errorf("EventType = %q; want %q", n.EventType, events.BudgetExceeded)
	}
	if n.SessionID != "sess-001" {
		t.Errorf("SessionID = %q; want sess-001", n.SessionID)
	}
}

func TestFormatEmbed_MinimalNotification(t *testing.T) {
	t.Parallel()

	n := Notification{
		Title:     "Test",
		Severity:  "info",
		EventType: events.ConfigChanged,
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
	}
	embed := FormatEmbed(n)

	// Should have at least the Event field.
	if len(embed.Fields) < 1 {
		t.Errorf("expected at least 1 field, got %d", len(embed.Fields))
	}

	// No session/repo/provider fields expected.
	for _, f := range embed.Fields {
		if f.Name == "Session" || f.Name == "Repo" || f.Name == "Provider" {
			t.Errorf("unexpected field %q in minimal notification", f.Name)
		}
	}
}

// containsStr is a test helper for string containment.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
