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

// SlackMessage is the top-level JSON body POSTed to a Slack webhook endpoint.
// It supports Block Kit blocks, attachments (for color sidebar), and a
// fallback text field for notification previews.
type SlackMessage struct {
	Text        string            `json:"text,omitempty"`
	Blocks      []SlackBlock      `json:"blocks,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackBlock is a single block in a Slack message.
type SlackBlock struct {
	Type     string      `json:"type"`
	Text     *SlackText  `json:"text,omitempty"`
	Fields   []SlackText `json:"fields,omitempty"`
	Elements []SlackElement `json:"elements,omitempty"`
}

// SlackText is a text element in a Slack block.
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SlackElement is an interactive element (e.g. button) in an actions block.
type SlackElement struct {
	Type  string     `json:"type"`
	Text  *SlackText `json:"text,omitempty"`
	URL   string     `json:"url,omitempty"`
	Style string     `json:"style,omitempty"` // "primary" or "danger"
}

// SlackAttachment provides the color sidebar.
type SlackAttachment struct {
	Color  string       `json:"color"`
	Blocks []SlackBlock `json:"blocks,omitempty"`
}

// SlackPayload represents a Slack Block Kit message.
// Deprecated: Use SlackMessage instead. Kept for backward compatibility
// with FormatSlack.
type SlackPayload = SlackMessage

// SlackNotifier sends notifications to a Slack channel via incoming webhook.
type SlackNotifier struct {
	webhookURL string
	username   string
	client     *http.Client
}

// SlackOption configures a SlackNotifier.
type SlackOption func(*SlackNotifier)

// WithSlackUsername sets the bot username displayed in Slack messages.
func WithSlackUsername(name string) SlackOption {
	return func(s *SlackNotifier) {
		s.username = name
	}
}

// WithSlackClient sets a custom HTTP client for the notifier.
func WithSlackClient(client *http.Client) SlackOption {
	return func(s *SlackNotifier) {
		s.client = client
	}
}

// NewSlackNotifier creates a SlackNotifier for the given webhook URL.
func NewSlackNotifier(webhookURL string, opts ...SlackOption) *SlackNotifier {
	s := &SlackNotifier{
		webhookURL: webhookURL,
		username:   "ralphglasses",
		client:     &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// slackSeverityColor maps a Notification severity string to a Slack sidebar
// hex color. It uses the Notification.Severity field rather than looking up
// by EventType, so callers can override severity if needed.
func slackSeverityColor(severity string) string {
	switch severity {
	case "error":
		return "#E01E5A" // red
	case "warning":
		return "#ECB22E" // yellow
	default:
		return "#2EB67D" // green
	}
}

// FormatMessage converts a Notification into a SlackMessage with Block Kit
// formatting including header, field section, optional data section, color
// sidebar (via attachment), and action buttons for error/warning urgency.
func (s *SlackNotifier) FormatMessage(n Notification) SlackMessage {
	color := slackSeverityColor(n.Severity)
	title := fmt.Sprintf("[%s] %s", strings.ToUpper(n.Severity), n.Title)

	// Header block
	header := SlackBlock{
		Type: "header",
		Text: &SlackText{
			Type: "plain_text",
			Text: title,
		},
	}

	// Detail fields
	var fields []SlackText
	fields = append(fields, SlackText{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Event:*\n`%s`", n.EventType),
	})

	var ts string
	if !n.Timestamp.IsZero() {
		ts = n.Timestamp.UTC().Format(time.RFC3339)
	} else {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	fields = append(fields, SlackText{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Timestamp:*\n%s", ts),
	})

	if n.SessionID != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Session:*\n`%s`", n.SessionID),
		})
	}
	if n.RepoName != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Repo:*\n%s", n.RepoName),
		})
	}
	if n.Provider != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Provider:*\n%s", n.Provider),
		})
	}

	fieldSection := SlackBlock{
		Type:   "section",
		Fields: fields,
	}

	// Body text section (if non-empty)
	var bodyBlock *SlackBlock
	if n.Body != "" {
		bodyBlock = &SlackBlock{
			Type: "section",
			Text: &SlackText{
				Type: "mrkdwn",
				Text: n.Body,
			},
		}
	}

	// Optional data section
	var dataBlock *SlackBlock
	if len(n.Data) > 0 {
		var lines []string
		for k, v := range n.Data {
			lines = append(lines, fmt.Sprintf("*%s:* %v", k, v))
		}
		dataBlock = &SlackBlock{
			Type: "section",
			Text: &SlackText{
				Type: "mrkdwn",
				Text: strings.Join(lines, "\n"),
			},
		}
	}

	// Action buttons for error/warning severity
	var actionsBlock *SlackBlock
	if n.Severity == "error" || n.Severity == "warning" {
		var elements []SlackElement
		if n.SessionID != "" {
			elements = append(elements, SlackElement{
				Type: "button",
				Text: &SlackText{
					Type: "plain_text",
					Text: "View Session",
				},
				URL:   fmt.Sprintf("ralphglasses://session/%s", n.SessionID),
				Style: "primary",
			})
		}
		if n.Severity == "error" {
			elements = append(elements, SlackElement{
				Type: "button",
				Text: &SlackText{
					Type: "plain_text",
					Text: "Acknowledge",
				},
				URL:   fmt.Sprintf("ralphglasses://ack/%s", string(n.EventType)),
				Style: "danger",
			})
		}
		if len(elements) > 0 {
			actionsBlock = &SlackBlock{
				Type:     "actions",
				Elements: elements,
			}
		}
	}

	// Assemble blocks inside the attachment for the color sidebar.
	attachBlocks := []SlackBlock{header, fieldSection}
	if bodyBlock != nil {
		attachBlocks = append(attachBlocks, *bodyBlock)
	}
	if dataBlock != nil {
		attachBlocks = append(attachBlocks, *dataBlock)
	}
	if actionsBlock != nil {
		attachBlocks = append(attachBlocks, *actionsBlock)
	}

	return SlackMessage{
		Text: title, // fallback text for notifications
		Attachments: []SlackAttachment{
			{
				Color:  color,
				Blocks: attachBlocks,
			},
		},
	}
}

// Send dispatches a Notification to the configured Slack webhook.
func (s *SlackNotifier) Send(ctx context.Context, n Notification) error {
	msg := s.FormatMessage(n)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("slack: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("slack: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack: http post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("slack: http %d from webhook", resp.StatusCode)
}

// severityColor returns a Slack color hex based on event type.
func severityColor(evType events.EventType) string {
	switch evType {
	case events.BudgetExceeded, events.SessionError, events.LoopRegression,
		events.ContextConflict:
		return "#E01E5A" // red
	case events.BudgetAlert, events.LoopStopped, events.SessionStopped,
		events.WorkerPaused, events.WorkerDeregistered:
		return "#ECB22E" // yellow
	default:
		return "#2EB67D" // green
	}
}

// severityLabel returns a human-readable severity string.
func severityLabel(evType events.EventType) string {
	switch severityColor(evType) {
	case "#E01E5A":
		return "error"
	case "#ECB22E":
		return "warning"
	default:
		return "info"
	}
}

// eventTitle produces a short human-readable title for the event type.
func eventTitle(evType events.EventType) string {
	parts := strings.SplitN(string(evType), ".", 2)
	if len(parts) == 2 {
		return strings.Title(parts[0]) + " " + strings.Title(parts[1]) //nolint:staticcheck
	}
	return string(evType)
}

// FormatSlack converts a bus event into a Slack Block Kit payload.
// This function predates SlackNotifier and works directly with events.Event.
// For the Notification-based API, use SlackNotifier.FormatMessage instead.
func FormatSlack(ev events.Event) SlackPayload {
	color := severityColor(ev.Type)
	title := eventTitle(ev.Type)
	severity := severityLabel(ev.Type)
	ts := ev.Timestamp.UTC().Format(time.RFC3339)

	// Header section
	header := SlackBlock{
		Type: "header",
		Text: &SlackText{
			Type: "plain_text",
			Text: fmt.Sprintf("[%s] %s", strings.ToUpper(severity), title),
		},
	}

	// Detail fields
	var fields []SlackText
	fields = append(fields, SlackText{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Event:*\n`%s`", ev.Type),
	})
	fields = append(fields, SlackText{
		Type: "mrkdwn",
		Text: fmt.Sprintf("*Timestamp:*\n%s", ts),
	})

	if ev.SessionID != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Session:*\n`%s`", ev.SessionID),
		})
	}
	if ev.RepoName != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Repo:*\n%s", ev.RepoName),
		})
	}
	if ev.Provider != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Provider:*\n%s", ev.Provider),
		})
	}
	if ev.NodeID != "" {
		fields = append(fields, SlackText{
			Type: "mrkdwn",
			Text: fmt.Sprintf("*Node:*\n%s", ev.NodeID),
		})
	}

	fieldSection := SlackBlock{
		Type:   "section",
		Fields: fields,
	}

	// Optional data section
	var dataBlock *SlackBlock
	if len(ev.Data) > 0 {
		var lines []string
		for k, v := range ev.Data {
			lines = append(lines, fmt.Sprintf("*%s:* %v", k, v))
		}
		dataBlock = &SlackBlock{
			Type: "section",
			Text: &SlackText{
				Type: "mrkdwn",
				Text: strings.Join(lines, "\n"),
			},
		}
	}

	// Assemble blocks inside the attachment for the color sidebar.
	attachBlocks := []SlackBlock{header, fieldSection}
	if dataBlock != nil {
		attachBlocks = append(attachBlocks, *dataBlock)
	}

	return SlackPayload{
		Attachments: []SlackAttachment{
			{
				Color:  color,
				Blocks: attachBlocks,
			},
		},
	}
}
