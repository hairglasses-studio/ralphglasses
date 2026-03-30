package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// Notification is a provider-agnostic notification payload used by notifier
// implementations. It carries the essential fields from an events.Event plus
// optional desktop-notification hints (Urgency, Icon, Timeout).
type Notification struct {
	Title     string
	Body      string
	Severity  string // "info", "warning", "error"
	Urgency   string // "low", "normal", "critical"; desktop notifications
	Icon      string // icon name or path; desktop notifications
	Timeout   time.Duration // 0 means server default; desktop notifications
	EventType events.EventType
	SessionID string
	RepoName  string
	Provider  string
	Timestamp time.Time
	Data      map[string]any
}

// NotificationFromEvent converts an events.Event into a Notification.
func NotificationFromEvent(ev events.Event) Notification {
	return Notification{
		Title:     eventTitle(ev.Type),
		Body:      formatNotificationBody(ev),
		Severity:  severityLabel(ev.Type),
		EventType: ev.Type,
		SessionID: ev.SessionID,
		RepoName:  ev.RepoName,
		Provider:  ev.Provider,
		Timestamp: ev.Timestamp,
		Data:      ev.Data,
	}
}

// formatNotificationBody builds a human-readable body string from event data.
func formatNotificationBody(ev events.Event) string {
	var parts []string
	if ev.SessionID != "" {
		parts = append(parts, fmt.Sprintf("Session: %s", ev.SessionID))
	}
	if ev.RepoName != "" {
		parts = append(parts, fmt.Sprintf("Repo: %s", ev.RepoName))
	}
	if ev.Provider != "" {
		parts = append(parts, fmt.Sprintf("Provider: %s", ev.Provider))
	}
	for k, v := range ev.Data {
		parts = append(parts, fmt.Sprintf("%s: %v", k, v))
	}
	return strings.Join(parts, " | ")
}

// DiscordNotifier sends notifications to a Discord channel via webhook.
type DiscordNotifier struct {
	webhookURL string
	username   string
	client     *http.Client
}

// DiscordOption configures a DiscordNotifier.
type DiscordOption func(*DiscordNotifier)

// WithDiscordUsername sets the bot username displayed in Discord messages.
func WithDiscordUsername(name string) DiscordOption {
	return func(d *DiscordNotifier) {
		d.username = name
	}
}

// WithDiscordClient sets a custom HTTP client for the notifier.
func WithDiscordClient(client *http.Client) DiscordOption {
	return func(d *DiscordNotifier) {
		d.client = client
	}
}

// NewDiscordNotifier creates a DiscordNotifier for the given webhook URL.
func NewDiscordNotifier(webhookURL string, opts ...DiscordOption) *DiscordNotifier {
	d := &DiscordNotifier{
		webhookURL: webhookURL,
		username:   "ralphglasses",
		client:     &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// DiscordPayload is the JSON body POSTed to a Discord webhook endpoint.
type DiscordPayload struct {
	Content  string         `json:"content,omitempty"`
	Username string         `json:"username,omitempty"`
	Embeds   []DiscordEmbed `json:"embeds,omitempty"`
}

// DiscordEmbed is a rich embed object in a Discord message.
type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []DiscordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

// DiscordField is a name/value pair displayed in an embed.
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// discordSeverityColor maps severity labels to Discord embed color integers.
func discordSeverityColor(severity string) int {
	switch severity {
	case "error":
		return 0xff0000 // red
	case "warning":
		return 0xffff00 // yellow
	default:
		return 0x00ff00 // green
	}
}

// FormatEmbed converts a Notification into a rich Discord embed.
func FormatEmbed(n Notification) DiscordEmbed {
	color := discordSeverityColor(n.Severity)
	title := fmt.Sprintf("[%s] %s", strings.ToUpper(n.Severity), n.Title)

	var fields []DiscordField

	fields = append(fields, DiscordField{
		Name:   "Event",
		Value:  fmt.Sprintf("`%s`", n.EventType),
		Inline: true,
	})

	if n.SessionID != "" {
		fields = append(fields, DiscordField{
			Name:   "Session",
			Value:  fmt.Sprintf("`%s`", n.SessionID),
			Inline: true,
		})
	}
	if n.RepoName != "" {
		fields = append(fields, DiscordField{
			Name:   "Repo",
			Value:  n.RepoName,
			Inline: true,
		})
	}
	if n.Provider != "" {
		fields = append(fields, DiscordField{
			Name:   "Provider",
			Value:  n.Provider,
			Inline: true,
		})
	}

	for k, v := range n.Data {
		fields = append(fields, DiscordField{
			Name:   k,
			Value:  fmt.Sprintf("%v", v),
			Inline: true,
		})
	}

	var ts string
	if !n.Timestamp.IsZero() {
		ts = n.Timestamp.UTC().Format(time.RFC3339)
	}

	return DiscordEmbed{
		Title:       title,
		Description: n.Body,
		Color:       color,
		Fields:      fields,
		Timestamp:   ts,
	}
}

// Send dispatches a Notification to the configured Discord webhook.
func (d *DiscordNotifier) Send(ctx context.Context, n Notification) error {
	embed := FormatEmbed(n)
	payload := DiscordPayload{
		Username: d.username,
		Embeds:   []DiscordEmbed{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord: http post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("discord: http %d from webhook", resp.StatusCode)
}
