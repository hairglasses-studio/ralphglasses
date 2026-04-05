// Package comms provides a unified message abstraction across all communication channels.
package comms

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

// Channel represents a communication channel type.
type Channel string

const (
	ChannelSMS     Channel = "sms"
	ChannelDiscord Channel = "discord"
	ChannelGmail   Channel = "gmail"
	ChannelSlack   Channel = "slack"
	ChannelBluesky Channel = "bluesky"
)

// Direction represents message direction.
type Direction string

const (
	DirectionIncoming Direction = "incoming"
	DirectionOutgoing Direction = "outgoing"
)

// ContactTier represents how important a contact is for reply urgency.
type ContactTier string

const (
	TierVIP    ContactTier = "vip"
	TierClose  ContactTier = "close"
	TierNormal ContactTier = "normal"
	TierLow    ContactTier = "low"
)

// UnifiedMessage represents a message from any channel in a common format.
type UnifiedMessage struct {
	ID               string    `json:"id"`
	Channel          Channel   `json:"channel"`
	ChannelMessageID string    `json:"channel_message_id"`
	ContactID        string    `json:"contact_id"`
	ContactName      string    `json:"contact_name"`
	Preview          string    `json:"preview"`
	Direction        Direction `json:"direction"`
	ReceivedAt       time.Time `json:"received_at"`
	NeedsReply       bool      `json:"needs_reply"`
	ConversationID   string    `json:"conversation_id"`
}

// UrgencyFactors holds the breakdown of an urgency score.
type UrgencyFactors struct {
	ContactTierScore float64 `json:"contact_tier"`
	TimeDecayScore   float64 `json:"time_decay"`
	ContentScore     float64 `json:"content_signals"`
	MomentumScore    float64 `json:"momentum"`
	ReciprocityScore float64 `json:"reciprocity"`
	Total            float64 `json:"total"`
	Reason           string  `json:"reason"`
}

// ScanResult holds a unified message with its computed urgency.
type ScanResult struct {
	Message UnifiedMessage `json:"message"`
	Urgency UrgencyFactors `json:"urgency"`
}

// ScoreUrgency computes the urgency score (0.0-1.0) for an unreplied message.
func ScoreUrgency(msg UnifiedMessage, tier ContactTier, replyWindowHours float64, unrepliedCount int, reciprocityRatio float64) UrgencyFactors {
	f := UrgencyFactors{}
	var reasons []string

	// Contact tier (0-0.3)
	switch tier {
	case TierVIP:
		f.ContactTierScore = 0.3
		reasons = append(reasons, "VIP contact")
	case TierClose:
		f.ContactTierScore = 0.2
		reasons = append(reasons, "close contact")
	case TierNormal:
		f.ContactTierScore = 0.1
	case TierLow:
		f.ContactTierScore = 0.05
	}

	// Time decay (0-0.3): exponential, hits 0.3 at 2x reply window
	hoursSince := time.Since(msg.ReceivedAt).Hours()
	if replyWindowHours <= 0 {
		replyWindowHours = 24 // default 24h reply window
	}
	ratio := hoursSince / replyWindowHours
	f.TimeDecayScore = 0.3 * (1 - math.Exp(-0.693*ratio)) // 0.693 = ln(2), so at ratio=1 we get 0.15
	if ratio > 2 {
		reasons = append(reasons, fmt.Sprintf("%.0fh overdue", hoursSince-replyWindowHours))
	} else if ratio > 1 {
		reasons = append(reasons, fmt.Sprintf("past %.0fh reply window", replyWindowHours))
	}

	// Content signals (0-0.2)
	preview := strings.ToLower(msg.Preview)
	if strings.Contains(preview, "?") {
		f.ContentScore += 0.08
		reasons = append(reasons, "contains question")
	}
	timeSensitive := []string{"urgent", "asap", "today", "tonight", "tomorrow", "deadline", "soon", "quickly", "hurry", "emergency"}
	for _, word := range timeSensitive {
		if strings.Contains(preview, word) {
			f.ContentScore += 0.07
			reasons = append(reasons, "time-sensitive language")
			break
		}
	}
	emotional := []string{"miss you", "worried", "sorry", "love you", "are you ok", "please", "help"}
	for _, phrase := range emotional {
		if strings.Contains(preview, phrase) {
			f.ContentScore += 0.05
			reasons = append(reasons, "emotional content")
			break
		}
	}
	if f.ContentScore > 0.2 {
		f.ContentScore = 0.2
	}

	// Conversation momentum (0-0.1): multiple unreplied messages spike urgency
	if unrepliedCount > 1 {
		f.MomentumScore = math.Min(0.1, float64(unrepliedCount-1)*0.03)
		reasons = append(reasons, fmt.Sprintf("%d unreplied messages", unrepliedCount))
	}

	// Reciprocity gap (0-0.1): one-sided conversation patterns
	// reciprocityRatio = their_messages / your_messages (higher = more one-sided)
	if reciprocityRatio > 2.0 {
		f.ReciprocityScore = math.Min(0.1, (reciprocityRatio-2.0)*0.03)
		reasons = append(reasons, "one-sided conversation")
	}

	f.Total = math.Min(1.0, f.ContactTierScore+f.TimeDecayScore+f.ContentScore+f.MomentumScore+f.ReciprocityScore)
	f.Reason = strings.Join(reasons, "; ")

	return f
}

// GhostProbability estimates the chance of ghosting based on past reply behavior.
// avgReplyMinutes is the user's average reply time, hoursSinceReceived is how long since the message.
func GhostProbability(avgReplyMinutes float64, hoursSinceReceived float64) float64 {
	if avgReplyMinutes <= 0 {
		return 0.5 // no data
	}
	avgReplyHours := avgReplyMinutes / 60.0
	// Sigmoid: 50% at 2x avg reply time, approaches 100% at 5x
	ratio := hoursSinceReceived / avgReplyHours
	prob := 1.0 / (1.0 + math.Exp(-1.5*(ratio-2.0)))
	return math.Min(prob, 0.99)
}

// RelationshipWeather returns a weather emoji/label based on interaction health.
func RelationshipWeather(daysSinceLastReply float64, reciprocityRatio float64) string {
	if daysSinceLastReply > 30 {
		return "drought"
	}
	if daysSinceLastReply > 14 || reciprocityRatio > 4.0 {
		return "stormy"
	}
	if daysSinceLastReply > 7 || reciprocityRatio > 2.5 {
		return "cloudy"
	}
	return "sunny"
}

// WeatherEmoji returns an emoji for relationship weather.
func WeatherEmoji(weather string) string {
	switch weather {
	case "sunny":
		return "☀️"
	case "cloudy":
		return "☁️"
	case "stormy":
		return "⛈️"
	case "drought":
		return "🏜️"
	default:
		return "❓"
	}
}

// ScanSMS scans the sms_messages table for unreplied incoming messages.
func ScanSMS(ctx context.Context, db *sql.DB) ([]UnifiedMessage, error) {
	// Find incoming messages where we haven't replied after them
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.conversation_id, m.sender, m.body, m.sent_at, c.display_name
		FROM sms_messages m
		JOIN sms_conversations c ON c.id = m.conversation_id
		WHERE m.direction = 'incoming'
		  AND m.sent_at IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM sms_messages m2
		    WHERE m2.conversation_id = m.conversation_id
		      AND m2.direction = 'outgoing'
		      AND m2.sent_at > m.sent_at
		  )
		ORDER BY m.sent_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("scan SMS: %w", err)
	}
	defer rows.Close()

	var msgs []UnifiedMessage
	for rows.Next() {
		var id, convID, sender, body, sentAt string
		var displayName sql.NullString
		if err := rows.Scan(&id, &convID, &sender, &body, &sentAt, &displayName); err != nil {
			continue
		}
		name := sender
		if displayName.Valid && displayName.String != "" {
			name = displayName.String
		}
		parsed, _ := time.Parse("2006-01-02T15:04:05-07:00", sentAt)
		if parsed.IsZero() {
			parsed, _ = time.Parse("2006-01-02 15:04:05", sentAt)
		}
		msgs = append(msgs, UnifiedMessage{
			ID:               fmt.Sprintf("sms-%s", id),
			Channel:          ChannelSMS,
			ChannelMessageID: id,
			ContactID:        sender,
			ContactName:      name,
			Preview:          truncate(body, 120),
			Direction:        DirectionIncoming,
			ReceivedAt:       parsed,
			NeedsReply:       true,
			ConversationID:   convID,
		})
	}
	return msgs, nil
}

// ScanDiscordDMs scans discord_messages for unreplied DMs.
func ScanDiscordDMs(ctx context.Context, db *sql.DB, myUserID string) ([]UnifiedMessage, error) {
	// Find DM channels (type = 'DM') with unreplied messages
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.channel_id, m.author, m.content, m.sent_at
		FROM discord_messages m
		JOIN discord_channels ch ON ch.id = m.channel_id
		WHERE ch.type = 'DM'
		  AND m.author != ?
		  AND m.sent_at IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM discord_messages m2
		    WHERE m2.channel_id = m.channel_id
		      AND m2.author = ?
		      AND m2.sent_at > m.sent_at
		  )
		ORDER BY m.sent_at DESC
	`, myUserID, myUserID)
	if err != nil {
		return nil, fmt.Errorf("scan Discord DMs: %w", err)
	}
	defer rows.Close()

	var msgs []UnifiedMessage
	for rows.Next() {
		var id, channelID, author, content, sentAt string
		if err := rows.Scan(&id, &channelID, &author, &content, &sentAt); err != nil {
			continue
		}
		parsed, _ := time.Parse("2006-01-02T15:04:05-07:00", sentAt)
		if parsed.IsZero() {
			parsed, _ = time.Parse("2006-01-02 15:04:05", sentAt)
		}
		msgs = append(msgs, UnifiedMessage{
			ID:               fmt.Sprintf("discord-%s", id),
			Channel:          ChannelDiscord,
			ChannelMessageID: id,
			ContactID:        author,
			ContactName:      author,
			Preview:          truncate(content, 120),
			Direction:        DirectionIncoming,
			ReceivedAt:       parsed,
			NeedsReply:       true,
			ConversationID:   channelID,
		})
	}
	return msgs, nil
}

// ScanGmail scans gmail_messages for unreplied emails needing response.
func ScanGmail(ctx context.Context, db *sql.DB) ([]UnifiedMessage, error) {
	// Emails that are untriaged and appear to need a reply (have a question or are from a person, not a notification)
	rows, err := db.QueryContext(ctx, `
		SELECT id, thread_id, from_addr, subject, snippet, timestamp
		FROM gmail_messages
		WHERE triaged = 0
		  AND from_addr NOT LIKE '%noreply%'
		  AND from_addr NOT LIKE '%no-reply%'
		  AND from_addr NOT LIKE '%notifications%'
		  AND from_addr NOT LIKE '%mailer-daemon%'
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("scan Gmail: %w", err)
	}
	defer rows.Close()

	var msgs []UnifiedMessage
	for rows.Next() {
		var id, threadID, from, subject, snippet string
		var timestamp sql.NullString
		if err := rows.Scan(&id, &threadID, &from, &subject, &snippet, &timestamp); err != nil {
			continue
		}
		var parsed time.Time
		if timestamp.Valid {
			parsed, _ = time.Parse("2006-01-02T15:04:05-07:00", timestamp.String)
			if parsed.IsZero() {
				parsed, _ = time.Parse("2006-01-02 15:04:05", timestamp.String)
			}
		}
		msgs = append(msgs, UnifiedMessage{
			ID:               fmt.Sprintf("gmail-%s", id),
			Channel:          ChannelGmail,
			ChannelMessageID: id,
			ContactID:        from,
			ContactName:      extractName(from),
			Preview:          truncate(subject+": "+snippet, 120),
			Direction:        DirectionIncoming,
			ReceivedAt:       parsed,
			NeedsReply:       true,
			ConversationID:   threadID,
		})
	}
	return msgs, nil
}

// ScanSlackDMs scans slack_channel_messages for unreplied DMs (direct messages from others).
func ScanSlackDMs(ctx context.Context, db *sql.DB, myUserID string) ([]UnifiedMessage, error) {
	// Find Slack DM messages where we haven't replied after them
	// Slack DM channel names often start with the other user's name
	rows, err := db.QueryContext(ctx, `
		SELECT m.id, m.channel_id, m.user_id, m.user_name, m.text, m.timestamp
		FROM slack_channel_messages m
		WHERE m.user_id != ?
		  AND m.user_id != ''
		  AND m.timestamp != ''
		  AND NOT EXISTS (
		    SELECT 1 FROM slack_channel_messages m2
		    WHERE m2.channel_id = m.channel_id
		      AND m2.user_id = ?
		      AND m2.timestamp > m.timestamp
		  )
		ORDER BY m.timestamp DESC
	`, myUserID, myUserID)
	if err != nil {
		return nil, fmt.Errorf("scan Slack DMs: %w", err)
	}
	defer rows.Close()

	var msgs []UnifiedMessage
	for rows.Next() {
		var id, channelID, userID, userName, text, ts string
		if err := rows.Scan(&id, &channelID, &userID, &userName, &text, &ts); err != nil {
			continue
		}
		name := userName
		if name == "" {
			name = userID
		}
		// Slack timestamps are Unix epoch with decimal (e.g., "1700000001.000")
		parsed := parseSlackTS(ts)
		msgs = append(msgs, UnifiedMessage{
			ID:               fmt.Sprintf("slack-%s", id),
			Channel:          ChannelSlack,
			ChannelMessageID: id,
			ContactID:        userID,
			ContactName:      name,
			Preview:          truncate(text, 120),
			Direction:        DirectionIncoming,
			ReceivedAt:       parsed,
			NeedsReply:       true,
			ConversationID:   channelID,
		})
	}
	return msgs, nil
}

// ScanAll scans all channels and returns combined unreplied messages.
func ScanAll(ctx context.Context, db *sql.DB, discordUserID string, blueskyDID string) ([]UnifiedMessage, error) {
	var all []UnifiedMessage

	sms, err := ScanSMS(ctx, db)
	if err == nil {
		all = append(all, sms...)
	}

	discord, err := ScanDiscordDMs(ctx, db, discordUserID)
	if err == nil {
		all = append(all, discord...)
	}

	gmail, err := ScanGmail(ctx, db)
	if err == nil {
		all = append(all, gmail...)
	}

	slack, err := ScanSlackDMs(ctx, db, "")
	if err == nil {
		all = append(all, slack...)
	}

	bluesky, err := ScanBluesky(ctx, db, blueskyDID)
	if err == nil {
		all = append(all, bluesky...)
	}

	return all, nil
}

func parseSlackTS(ts string) time.Time {
	// Slack timestamps are "epoch.sequence" e.g. "1700000001.000123"
	var sec int64
	if _, err := fmt.Sscanf(ts, "%d", &sec); err == nil && sec > 0 {
		return time.Unix(sec, 0)
	}
	return time.Time{}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func extractName(email string) string {
	// "John Doe <john@example.com>" → "John Doe"
	if idx := strings.Index(email, "<"); idx > 0 {
		name := strings.TrimSpace(email[:idx])
		name = strings.Trim(name, "\"")
		if name != "" {
			return name
		}
	}
	// "john@example.com" → "john"
	if idx := strings.Index(email, "@"); idx > 0 {
		return email[:idx]
	}
	return email
}
