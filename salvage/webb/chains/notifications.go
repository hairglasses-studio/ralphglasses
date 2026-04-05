package chains

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// NotificationChannel represents a notification destination
type NotificationChannel string

const (
	NotifySlack   NotificationChannel = "slack"
	NotifyWebhook NotificationChannel = "webhook"
	NotifyLog     NotificationChannel = "log"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	NotifyChainStarted     NotificationType = "chain.started"
	NotifyChainCompleted   NotificationType = "chain.completed"
	NotifyChainFailed      NotificationType = "chain.failed"
	NotifyStepCompleted    NotificationType = "step.completed"
	NotifyStepFailed       NotificationType = "step.failed"
	NotifyGateAwaitingApproval NotificationType = "gate.awaiting_approval"
	NotifyGateApproved     NotificationType = "gate.approved"
	NotifyGateRejected     NotificationType = "gate.rejected"
	NotifyGateTimeout      NotificationType = "gate.timeout"
)

// Notification represents a notification to be sent
type Notification struct {
	Type        NotificationType       `json:"type"`
	ChainName   string                 `json:"chain_name"`
	ExecutionID string                 `json:"execution_id"`
	StepID      string                 `json:"step_id,omitempty"`
	Message     string                 `json:"message"`
	Severity    string                 `json:"severity"` // info, warning, error
	Data        map[string]interface{} `json:"data,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
}

// NotificationHandler processes notifications
type NotificationHandler interface {
	Send(ctx context.Context, notification Notification) error
	Channel() NotificationChannel
}

// NotificationManager manages notification handlers
type NotificationManager struct {
	handlers map[NotificationChannel]NotificationHandler
	enabled  bool
}

// NewNotificationManager creates a new notification manager
func NewNotificationManager() *NotificationManager {
	return &NotificationManager{
		handlers: make(map[NotificationChannel]NotificationHandler),
		enabled:  true,
	}
}

// RegisterHandler registers a notification handler
func (m *NotificationManager) RegisterHandler(handler NotificationHandler) {
	m.handlers[handler.Channel()] = handler
}

// SetEnabled enables or disables notifications
func (m *NotificationManager) SetEnabled(enabled bool) {
	m.enabled = enabled
}

// Notify sends a notification to all registered handlers
func (m *NotificationManager) Notify(ctx context.Context, notification Notification) {
	if !m.enabled {
		return
	}

	if notification.Timestamp.IsZero() {
		notification.Timestamp = time.Now()
	}

	for channel, handler := range m.handlers {
		go func(ch NotificationChannel, h NotificationHandler) {
			if err := h.Send(ctx, notification); err != nil {
				log.Printf("[chains] Failed to send notification to %s: %v", ch, err)
			}
		}(channel, handler)
	}
}

// NotifyChainStart sends a chain started notification
func (m *NotificationManager) NotifyChainStart(ctx context.Context, exec *ChainExecution) {
	m.Notify(ctx, Notification{
		Type:        NotifyChainStarted,
		ChainName:   exec.ChainName,
		ExecutionID: exec.ID,
		Message:     fmt.Sprintf("Chain '%s' started", exec.ChainName),
		Severity:    "info",
		Data: map[string]interface{}{
			"triggered_by": exec.TriggeredBy,
			"input":        exec.Input,
		},
	})
}

// NotifyChainComplete sends a chain completed notification
func (m *NotificationManager) NotifyChainComplete(ctx context.Context, exec *ChainExecution) {
	m.Notify(ctx, Notification{
		Type:        NotifyChainCompleted,
		ChainName:   exec.ChainName,
		ExecutionID: exec.ID,
		Message:     fmt.Sprintf("Chain '%s' completed successfully", exec.ChainName),
		Severity:    "info",
		Data: map[string]interface{}{
			"duration": time.Since(exec.StartedAt).String(),
		},
	})
}

// NotifyChainFailed sends a chain failed notification
func (m *NotificationManager) NotifyChainFailed(ctx context.Context, exec *ChainExecution, err error) {
	m.Notify(ctx, Notification{
		Type:        NotifyChainFailed,
		ChainName:   exec.ChainName,
		ExecutionID: exec.ID,
		Message:     fmt.Sprintf("Chain '%s' failed: %v", exec.ChainName, err),
		Severity:    "error",
		Data: map[string]interface{}{
			"error":        err.Error(),
			"current_step": exec.CurrentStep,
		},
	})
}

// NotifyGateAwaiting sends a gate awaiting approval notification
func (m *NotificationManager) NotifyGateAwaiting(ctx context.Context, exec *ChainExecution, step *ChainStep) {
	m.Notify(ctx, Notification{
		Type:        NotifyGateAwaitingApproval,
		ChainName:   exec.ChainName,
		ExecutionID: exec.ID,
		StepID:      step.ID,
		Message:     fmt.Sprintf("Chain '%s' awaiting approval at step '%s'", exec.ChainName, step.ID),
		Severity:    "warning",
		Data: map[string]interface{}{
			"gate_type": step.GateType,
			"message":   step.Message,
			"timeout":   step.GateTimeout,
		},
	})
}

// SlackNotificationHandler sends notifications to Slack
type SlackNotificationHandler struct {
	slackClient SlackPoster
	channel     string
}

// SlackPoster defines the interface for posting to Slack
type SlackPoster interface {
	PostMessage(ctx context.Context, channel, message string) error
}

// NewSlackNotificationHandler creates a new Slack notification handler
func NewSlackNotificationHandler(client SlackPoster, channel string) *SlackNotificationHandler {
	return &SlackNotificationHandler{
		slackClient: client,
		channel:     channel,
	}
}

// Channel returns the notification channel type
func (h *SlackNotificationHandler) Channel() NotificationChannel {
	return NotifySlack
}

// Send sends a notification to Slack
func (h *SlackNotificationHandler) Send(ctx context.Context, notification Notification) error {
	message := h.formatMessage(notification)
	return h.slackClient.PostMessage(ctx, h.channel, message)
}

func (h *SlackNotificationHandler) formatMessage(n Notification) string {
	var emoji string
	switch n.Severity {
	case "error":
		emoji = ":x:"
	case "warning":
		emoji = ":warning:"
	default:
		emoji = ":information_source:"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s *%s*\n", emoji, n.Message))
	sb.WriteString(fmt.Sprintf("• Chain: `%s`\n", n.ChainName))
	sb.WriteString(fmt.Sprintf("• Execution: `%s`\n", n.ExecutionID))

	if n.StepID != "" {
		sb.WriteString(fmt.Sprintf("• Step: `%s`\n", n.StepID))
	}

	// Add specific data based on notification type
	switch n.Type {
	case NotifyGateAwaitingApproval:
		if msg, ok := n.Data["message"].(string); ok && msg != "" {
			sb.WriteString(fmt.Sprintf("• Message: %s\n", msg))
		}
		sb.WriteString(fmt.Sprintf("\nTo approve: `webb_chain_approve execution_id=%s step_id=%s approved=true`", n.ExecutionID, n.StepID))
	case NotifyChainFailed:
		if errMsg, ok := n.Data["error"].(string); ok {
			sb.WriteString(fmt.Sprintf("• Error: %s\n", errMsg))
		}
	case NotifyChainCompleted:
		if dur, ok := n.Data["duration"].(string); ok {
			sb.WriteString(fmt.Sprintf("• Duration: %s\n", dur))
		}
	}

	return sb.String()
}

// LogNotificationHandler logs notifications
type LogNotificationHandler struct{}

// NewLogNotificationHandler creates a new log notification handler
func NewLogNotificationHandler() *LogNotificationHandler {
	return &LogNotificationHandler{}
}

// Channel returns the notification channel type
func (h *LogNotificationHandler) Channel() NotificationChannel {
	return NotifyLog
}

// Send logs the notification
func (h *LogNotificationHandler) Send(ctx context.Context, notification Notification) error {
	log.Printf("[chains] %s: %s (chain=%s, exec=%s)",
		notification.Type, notification.Message, notification.ChainName, notification.ExecutionID)
	return nil
}

// WebhookNotificationHandler sends notifications to a webhook
type WebhookNotificationHandler struct {
	webhookURL string
}

// NewWebhookNotificationHandler creates a new webhook notification handler
func NewWebhookNotificationHandler(webhookURL string) *WebhookNotificationHandler {
	return &WebhookNotificationHandler{webhookURL: webhookURL}
}

// Channel returns the notification channel type
func (h *WebhookNotificationHandler) Channel() NotificationChannel {
	return NotifyWebhook
}

// Send sends a notification to the webhook
func (h *WebhookNotificationHandler) Send(ctx context.Context, notification Notification) error {
	// Implementation would use http.Post to send JSON payload
	// For now, just log
	log.Printf("[chains] Webhook notification: %s -> %s", notification.Type, h.webhookURL)
	return nil
}
