package notify

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// SlackPayload represents a Slack Block Kit message.
type SlackPayload struct {
	Blocks      []SlackBlock      `json:"blocks"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackBlock is a single block in a Slack message.
type SlackBlock struct {
	Type   string     `json:"type"`
	Text   *SlackText `json:"text,omitempty"`
	Fields []SlackText `json:"fields,omitempty"`
}

// SlackText is a text element in a Slack block.
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SlackAttachment provides the color sidebar.
type SlackAttachment struct {
	Color  string       `json:"color"`
	Blocks []SlackBlock `json:"blocks,omitempty"`
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
