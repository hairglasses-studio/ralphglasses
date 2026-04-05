// Package notifications provides multi-channel notification dispatch with
// urgency-based routing and ADHD-friendly rate limiting.
package notifications

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
)

// Urgency levels determine which channels are used.
type Urgency int

const (
	UrgencyLow      Urgency = iota // suppress (log only)
	UrgencyNormal                  // slack webhook
	UrgencyHigh                    // discord DM
	UrgencyCritical                // discord DM + SMS
)

func (u Urgency) String() string {
	switch u {
	case UrgencyLow:
		return "low"
	case UrgencyNormal:
		return "normal"
	case UrgencyHigh:
		return "high"
	case UrgencyCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Channel represents a notification delivery channel.
type Channel string

const (
	ChannelDiscordDM Channel = "discord_dm"
	ChannelSlack     Channel = "slack"
	ChannelSMS       Channel = "sms"
	ChannelHA        Channel = "homeassistant"
	ChannelLog       Channel = "log"
)

// Notification is a message to be dispatched.
type Notification struct {
	Title   string
	Message string
	Urgency Urgency
	Source  string // which worker task generated this (e.g., "reply_radar", "overwhelm_detector")
}

// Dispatcher routes notifications to the appropriate channels based on urgency.
type Dispatcher struct {
	mu          sync.Mutex
	cfg         *config.Config
	db          *sql.DB
	rateCounts  map[Channel]int       // notifications sent this window
	rateResetAt map[Channel]time.Time // when the rate window resets
	maxPerHour  int                   // max notifications per channel per hour
}

// NewDispatcher creates a notification dispatcher.
func NewDispatcher(cfg *config.Config, db *sql.DB) *Dispatcher {
	return &Dispatcher{
		cfg:         cfg,
		db:          db,
		rateCounts:  make(map[Channel]int),
		rateResetAt: make(map[Channel]time.Time),
		maxPerHour:  5, // ADHD overwhelm prevention: max 5 per channel per hour
	}
}

// Send dispatches a notification to the appropriate channels based on urgency.
// Returns the channels it was delivered to.
func (d *Dispatcher) Send(ctx context.Context, n Notification) []Channel {
	channels := d.routeByUrgency(n.Urgency)
	var delivered []Channel

	for _, ch := range channels {
		if d.isRateLimited(ch) {
			log.Printf("[notify] Rate limited on %s, skipping: %s", ch, n.Title)
			continue
		}

		var err error
		switch ch {
		case ChannelDiscordDM:
			err = d.sendDiscordDM(ctx, n)
		case ChannelSlack:
			err = d.sendSlackWebhook(ctx, n)
		case ChannelHA:
			err = d.sendHANotification(ctx, n)
		case ChannelLog:
			log.Printf("[notify] [%s] %s: %s", n.Urgency, n.Title, n.Message)
		}

		if err != nil {
			log.Printf("[notify] Failed to send via %s: %v", ch, err)
			continue
		}

		d.recordSend(ch)
		delivered = append(delivered, ch)
	}

	d.logToDB(ctx, n, delivered)
	return delivered
}

// routeByUrgency maps urgency levels to notification channels.
func (d *Dispatcher) routeByUrgency(urgency Urgency) []Channel {
	switch urgency {
	case UrgencyCritical:
		return []Channel{ChannelDiscordDM, ChannelHA}
	case UrgencyHigh:
		return []Channel{ChannelDiscordDM}
	case UrgencyNormal:
		return []Channel{ChannelSlack}
	case UrgencyLow:
		return []Channel{ChannelLog}
	default:
		return []Channel{ChannelLog}
	}
}

// isRateLimited returns true if the channel has hit its hourly limit.
func (d *Dispatcher) isRateLimited(ch Channel) bool {
	if ch == ChannelLog {
		return false // never rate limit log-only
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	resetAt, ok := d.rateResetAt[ch]
	if !ok || now.After(resetAt) {
		d.rateCounts[ch] = 0
		d.rateResetAt[ch] = now.Add(time.Hour)
	}

	return d.rateCounts[ch] >= d.maxPerHour
}

// recordSend increments the rate counter for a channel.
func (d *Dispatcher) recordSend(ch Channel) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.rateCounts[ch]++
}

// sendDiscordDM sends a notification via Discord DM to the configured user.
func (d *Dispatcher) sendDiscordDM(ctx context.Context, n Notification) error {
	token := d.cfg.Credentials["discord"]
	if token == "" {
		return fmt.Errorf("no discord token configured")
	}
	userID := d.cfg.Credentials["discord_user_id"]
	if userID == "" {
		return fmt.Errorf("no discord_user_id configured for DM notifications")
	}

	client := clients.NewDiscordClient(token)

	// Open or get existing DM channel
	dm, err := client.CreateDM(ctx, userID)
	if err != nil {
		return fmt.Errorf("create DM channel: %w", err)
	}

	// Format message
	msg := fmt.Sprintf("**[%s] %s**\n%s", strings.ToUpper(n.Urgency.String()), n.Title, n.Message)
	if n.Source != "" {
		msg += fmt.Sprintf("\n-# source: %s", n.Source)
	}

	return client.SendMessage(ctx, dm.ID, msg)
}

// sendSlackWebhook sends a notification via Slack incoming webhook.
func (d *Dispatcher) sendSlackWebhook(ctx context.Context, n Notification) error {
	webhookURL := d.cfg.Credentials["slack_webhook"]
	if webhookURL == "" {
		// Fall back to log if no webhook configured
		log.Printf("[notify] [slack-fallback] %s: %s", n.Title, n.Message)
		return nil
	}

	payload := fmt.Sprintf(`{"text":"*[%s] %s*\n%s"}`,
		strings.ToUpper(n.Urgency.String()), n.Title, n.Message)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}

// sendHANotification sends a notification via Home Assistant's notify service.
func (d *Dispatcher) sendHANotification(ctx context.Context, n Notification) error {
	haURL := d.cfg.Credentials["homeassistant_url"]
	haToken := d.cfg.Credentials["homeassistant"]
	if haURL == "" || haToken == "" {
		return fmt.Errorf("no Home Assistant credentials configured")
	}

	client := clients.NewHomeAssistantClient(haURL, haToken)
	return client.SetEntityState(ctx, "notify", "notify", map[string]interface{}{
		"message": fmt.Sprintf("[%s] %s: %s", strings.ToUpper(n.Urgency.String()), n.Title, n.Message),
		"title":   "runmylife",
	})
}

// logToDB records the notification dispatch to the database.
func (d *Dispatcher) logToDB(ctx context.Context, n Notification, channels []Channel) {
	if d.db == nil {
		return
	}

	chNames := make([]string, len(channels))
	for i, ch := range channels {
		chNames[i] = string(ch)
	}

	_, _ = d.db.ExecContext(ctx,
		`INSERT INTO notification_log (title, message, urgency, source, channels, sent_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.Title, n.Message, n.Urgency.String(), n.Source,
		strings.Join(chNames, ","), time.Now().Format(time.RFC3339),
	)
}
