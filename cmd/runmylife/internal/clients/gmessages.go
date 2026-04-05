package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/config"
)

// SMSMessage represents a text message (SMS or RCS).
type SMSMessage struct {
	ID             string
	ConversationID string
	Sender         string
	Body           string
	SentAt         time.Time
	Direction      string // "incoming" or "outgoing"
	IsRCS          bool
	FetchedAt      time.Time
}

// GMessagesClient handles Google Messages database operations.
// Live connection features (pairing, sending) are not yet implemented.
type GMessagesClient struct {
	db *sql.DB
}

// NewGMessagesClient creates a new Google Messages client with database access.
func NewGMessagesClient(db *sql.DB) *GMessagesClient {
	return &GMessagesClient{db: db}
}

// SessionPath returns the path to the Google Messages session file.
func SessionPath() string {
	return filepath.Join(config.DefaultDir(), "gmessages_session.json")
}

// HasSession checks if a Google Messages session file exists.
func HasSession() bool {
	_, err := os.Stat(SessionPath())
	return err == nil
}

// LoadAndConnect attempts to load a saved session and connect.
// Not yet implemented — requires libgm dependency.
func (c *GMessagesClient) LoadAndConnect(_ context.Context) error {
	return fmt.Errorf("Google Messages live connection not yet configured. Run pairing flow to enable messaging")
}

// StartGaiaPairing initiates a Google account-based pairing flow.
// Not yet implemented — requires libgm dependency.
func (c *GMessagesClient) StartGaiaPairing(_ context.Context) error {
	return fmt.Errorf("Google Messages pairing not yet configured. libgm dependency required for pairing")
}

// SaveSession persists the current session to disk.
// Not yet implemented — requires active connection.
func (c *GMessagesClient) SaveSession() error {
	return fmt.Errorf("no active session to save")
}

// IsConnected returns whether the client has an active connection.
func (c *GMessagesClient) IsConnected() bool {
	return false
}

// IsLoggedIn returns whether the client is authenticated.
func (c *GMessagesClient) IsLoggedIn() bool {
	return HasSession()
}

// Disconnect closes the active connection.
func (c *GMessagesClient) Disconnect() {
	log.Printf("[runmylife] GMessages: no active connection to disconnect")
}

// SaveMessage persists an SMS message to the database.
func (c *GMessagesClient) SaveMessage(ctx context.Context, msg SMSMessage) error {
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
	}
	if msg.FetchedAt.IsZero() {
		msg.FetchedAt = time.Now()
	}
	_, err := c.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO sms_messages (id, conversation_id, sender, body, sent_at, direction, is_rcs, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.ConversationID, msg.Sender, msg.Body,
		msg.SentAt.Format(time.RFC3339), msg.Direction, msg.IsRCS,
		msg.FetchedAt.Format(time.RFC3339),
	)
	return err
}

// ListMessages retrieves messages for a conversation ordered by sent time.
func (c *GMessagesClient) ListMessages(ctx context.Context, conversationID string, limit int) ([]SMSMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT id, conversation_id, sender, body, sent_at, direction, is_rcs, fetched_at
		 FROM sms_messages WHERE conversation_id = ?
		 ORDER BY sent_at DESC LIMIT ?`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// SearchMessages searches messages by body text.
func (c *GMessagesClient) SearchMessages(ctx context.Context, query string, limit int) ([]SMSMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT id, conversation_id, sender, body, sent_at, direction, is_rcs, fetched_at
		 FROM sms_messages WHERE body LIKE ?
		 ORDER BY sent_at DESC LIMIT ?`,
		"%"+query+"%", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]SMSMessage, error) {
	var messages []SMSMessage
	for rows.Next() {
		var msg SMSMessage
		var sentAt, fetchedAt string
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Sender, &msg.Body,
			&sentAt, &msg.Direction, &msg.IsRCS, &fetchedAt); err != nil {
			log.Printf("[runmylife] GMessages: scan error: %v", err)
			continue
		}
		msg.SentAt = parseSQLiteTime(sentAt)
		msg.FetchedAt = parseSQLiteTime(fetchedAt)
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func parseSQLiteTime(s string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// SessionInfo holds information about the current session state.
type SessionInfo struct {
	HasSession  bool   `json:"has_session"`
	SessionPath string `json:"session_path"`
	Connected   bool   `json:"connected"`
	LoggedIn    bool   `json:"logged_in"`
}

// GetSessionInfo returns the current session status.
func (c *GMessagesClient) GetSessionInfo() SessionInfo {
	return SessionInfo{
		HasSession:  HasSession(),
		SessionPath: SessionPath(),
		Connected:   c.IsConnected(),
		LoggedIn:    c.IsLoggedIn(),
	}
}

// ListConversations retrieves a summary of conversations from the database.
func (c *GMessagesClient) ListConversations(ctx context.Context, limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := c.db.QueryContext(ctx,
		`SELECT conversation_id, sender,
		        COUNT(*) as msg_count,
		        MAX(sent_at) as last_message_at,
		        (SELECT body FROM sms_messages m2 WHERE m2.conversation_id = m.conversation_id ORDER BY sent_at DESC LIMIT 1) as last_body
		 FROM sms_messages m
		 GROUP BY conversation_id
		 ORDER BY last_message_at DESC
		 LIMIT ?`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query conversations: %w", err)
	}
	defer rows.Close()

	var convos []map[string]interface{}
	for rows.Next() {
		var conversationID, sender, lastMessageAt, lastBody string
		var msgCount int
		if err := rows.Scan(&conversationID, &sender, &msgCount, &lastMessageAt, &lastBody); err != nil {
			log.Printf("[runmylife] GMessages: scan conversation error: %v", err)
			continue
		}
		convos = append(convos, map[string]interface{}{
			"conversation_id": conversationID,
			"participant":     sender,
			"message_count":   msgCount,
			"last_message_at": lastMessageAt,
			"last_body":       lastBody,
		})
	}
	return convos, rows.Err()
}

// ConversationToJSON serializes session info for storage.
func (c *GMessagesClient) ConversationToJSON() ([]byte, error) {
	info := c.GetSessionInfo()
	return json.MarshalIndent(info, "", "  ")
}
