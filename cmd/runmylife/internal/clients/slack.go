package clients

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const slackAPIBaseURL = "https://slack.com/api"

// SlackClient provides both Slack Web API access and local DB persistence.
type SlackClient struct {
	httpClient *http.Client
	token      string
	db         *sql.DB
}

// SlackChannel represents a Slack channel.
type SlackChannel struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Topic       string    `json:"topic"`
	Purpose     string    `json:"purpose"`
	MemberCount int       `json:"member_count"`
	IsArchived  bool      `json:"is_archived"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// SlackMessage represents a Slack message.
type SlackMessage struct {
	ID            string    `json:"id"`
	ChannelID     string    `json:"channel_id"`
	UserID        string    `json:"user_id"`
	UserName      string    `json:"user_name"`
	Text          string    `json:"text"`
	Timestamp     string    `json:"timestamp"`
	ThreadTS      string    `json:"thread_ts"`
	ReactionCount int       `json:"reaction_count"`
	Reactions     string    `json:"reactions"`
	FetchedAt     time.Time `json:"fetched_at"`
}

// SlackUser represents a Slack user profile.
type SlackUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	RealName string `json:"real_name"`
	Email    string `json:"email"`
}

// NewSlackClient creates a Slack API client with optional DB for persistence.
func NewSlackClient(token string, db *sql.DB) *SlackClient {
	return &SlackClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		token:      token,
		db:         db,
	}
}

// --- HTTP API Methods ---

// ListChannels fetches channels from Slack API.
func (s *SlackClient) ListChannels(ctx context.Context, limit int) ([]SlackChannel, error) {
	if limit <= 0 {
		limit = 100
	}
	url := fmt.Sprintf("%s/conversations.list?types=public_channel,private_channel&limit=%d&exclude_archived=true", slackAPIBaseURL, limit)
	var resp struct {
		OK       bool `json:"ok"`
		Channels []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Topic      struct{ Value string } `json:"topic"`
			Purpose    struct{ Value string } `json:"purpose"`
			NumMembers int  `json:"num_members"`
			IsArchived bool `json:"is_archived"`
		} `json:"channels"`
		Error string `json:"error"`
	}
	if err := s.doGet(ctx, url, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack API: %s", resp.Error)
	}

	channels := make([]SlackChannel, len(resp.Channels))
	for i, ch := range resp.Channels {
		channels[i] = SlackChannel{
			ID:          ch.ID,
			Name:        ch.Name,
			Topic:       ch.Topic.Value,
			Purpose:     ch.Purpose.Value,
			MemberCount: ch.NumMembers,
			IsArchived:  ch.IsArchived,
			FetchedAt:   time.Now(),
		}
	}
	return channels, nil
}

// GetChannelHistory fetches recent messages from a channel.
func (s *SlackClient) GetChannelHistory(ctx context.Context, channelID string, limit int) ([]SlackMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	url := fmt.Sprintf("%s/conversations.history?channel=%s&limit=%d", slackAPIBaseURL, channelID, limit)
	var resp struct {
		OK       bool `json:"ok"`
		Messages []struct {
			Type      string `json:"type"`
			User      string `json:"user"`
			Text      string `json:"text"`
			TS        string `json:"ts"`
			ThreadTS  string `json:"thread_ts"`
			Reactions []struct {
				Name  string `json:"name"`
				Count int    `json:"count"`
			} `json:"reactions"`
		} `json:"messages"`
		Error string `json:"error"`
	}
	if err := s.doGet(ctx, url, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack API: %s", resp.Error)
	}

	msgs := make([]SlackMessage, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		totalReactions := 0
		for _, r := range m.Reactions {
			totalReactions += r.Count
		}
		reactionsJSON, _ := json.Marshal(m.Reactions)
		msgs = append(msgs, SlackMessage{
			ID:            uuid.New().String(),
			ChannelID:     channelID,
			UserID:        m.User,
			Text:          m.Text,
			Timestamp:     m.TS,
			ThreadTS:      m.ThreadTS,
			ReactionCount: totalReactions,
			Reactions:     string(reactionsJSON),
			FetchedAt:     time.Now(),
		})
	}
	return msgs, nil
}

// PostMessage sends a message to a Slack channel.
func (s *SlackClient) PostMessage(ctx context.Context, channelID, text string) error {
	payload := fmt.Sprintf(`{"channel":"%s","text":%s}`, channelID, jsonString(text))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, slackAPIBaseURL+"/chat.postMessage", strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack API: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("slack API: %s", result.Error)
	}
	return nil
}

// GetUserInfo fetches a user's profile.
func (s *SlackClient) GetUserInfo(ctx context.Context, userID string) (*SlackUser, error) {
	url := fmt.Sprintf("%s/users.info?user=%s", slackAPIBaseURL, userID)
	var resp struct {
		OK   bool `json:"ok"`
		User struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Profile struct {
				RealName string `json:"real_name"`
				Email    string `json:"email"`
			} `json:"profile"`
		} `json:"user"`
		Error string `json:"error"`
	}
	if err := s.doGet(ctx, url, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack API: %s", resp.Error)
	}
	return &SlackUser{
		ID:       resp.User.ID,
		Name:     resp.User.Name,
		RealName: resp.User.Profile.RealName,
		Email:    resp.User.Profile.Email,
	}, nil
}

// SearchMessages searches Slack message history.
func (s *SlackClient) SearchMessages(ctx context.Context, query string, limit int) ([]SlackMessage, error) {
	if limit <= 0 {
		limit = 20
	}
	url := fmt.Sprintf("%s/search.messages?query=%s&count=%d", slackAPIBaseURL, query, limit)
	var resp struct {
		OK       bool `json:"ok"`
		Messages struct {
			Matches []struct {
				Channel struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"channel"`
				User      string `json:"user"`
				Username  string `json:"username"`
				Text      string `json:"text"`
				TS        string `json:"ts"`
				Permalink string `json:"permalink"`
			} `json:"matches"`
		} `json:"messages"`
		Error string `json:"error"`
	}
	if err := s.doGet(ctx, url, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("slack API: %s", resp.Error)
	}

	msgs := make([]SlackMessage, 0, len(resp.Messages.Matches))
	for _, m := range resp.Messages.Matches {
		msgs = append(msgs, SlackMessage{
			ID:        uuid.New().String(),
			ChannelID: m.Channel.ID,
			UserID:    m.User,
			UserName:  m.Username,
			Text:      m.Text,
			Timestamp: m.TS,
			FetchedAt: time.Now(),
		})
	}
	return msgs, nil
}

// --- DB Persistence Methods ---

// SaveChannel upserts a channel to local DB.
func (s *SlackClient) SaveChannel(ctx context.Context, ch *SlackChannel) error {
	if s.db == nil {
		return nil
	}
	if ch.ID == "" {
		ch.ID = uuid.New().String()
	}
	if ch.FetchedAt.IsZero() {
		ch.FetchedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO slack_channels (id, name, topic, purpose, member_count, is_archived, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   name = excluded.name, topic = excluded.topic, purpose = excluded.purpose,
		   member_count = excluded.member_count, is_archived = excluded.is_archived,
		   fetched_at = excluded.fetched_at`,
		ch.ID, ch.Name, ch.Topic, ch.Purpose, ch.MemberCount, ch.IsArchived, ch.FetchedAt,
	)
	return err
}

// SaveMessage upserts a message to local DB.
func (s *SlackClient) SaveMessage(ctx context.Context, msg *SlackMessage) error {
	if s.db == nil {
		return nil
	}
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.FetchedAt.IsZero() {
		msg.FetchedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO slack_channel_messages (id, channel_id, user_id, user_name, text, timestamp, thread_ts, reaction_count, reactions, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   text = excluded.text, reaction_count = excluded.reaction_count,
		   reactions = excluded.reactions, fetched_at = excluded.fetched_at`,
		msg.ID, msg.ChannelID, msg.UserID, msg.UserName, msg.Text, msg.Timestamp,
		msg.ThreadTS, msg.ReactionCount, msg.Reactions, msg.FetchedAt,
	)
	return err
}

// QueryChannels returns channels from local DB.
func (s *SlackClient) QueryChannels(ctx context.Context, limit, offset int) ([]SlackChannel, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("no database configured")
	}
	var total int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM slack_channels").Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		"SELECT id, name, topic, purpose, member_count, is_archived, fetched_at FROM slack_channels ORDER BY name ASC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var channels []SlackChannel
	for rows.Next() {
		var ch SlackChannel
		if err := rows.Scan(&ch.ID, &ch.Name, &ch.Topic, &ch.Purpose, &ch.MemberCount, &ch.IsArchived, &ch.FetchedAt); err != nil {
			continue
		}
		channels = append(channels, ch)
	}
	return channels, total, nil
}

// QueryMessages returns messages from local DB for a channel.
func (s *SlackClient) QueryMessages(ctx context.Context, channelID string, limit, offset int) ([]SlackMessage, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("no database configured")
	}
	var total int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM slack_channel_messages WHERE channel_id = ?", channelID).Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_id, user_id, user_name, text, timestamp, thread_ts, reaction_count, reactions, fetched_at
		 FROM slack_channel_messages WHERE channel_id = ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		channelID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []SlackMessage
	for rows.Next() {
		var m SlackMessage
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.UserID, &m.UserName, &m.Text, &m.Timestamp, &m.ThreadTS, &m.ReactionCount, &m.Reactions, &m.FetchedAt); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, total, nil
}

// QuerySearchMessages searches messages in local DB.
func (s *SlackClient) QuerySearchMessages(ctx context.Context, query string, limit, offset int) ([]SlackMessage, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("no database configured")
	}
	q := "%" + query + "%"
	var total int
	_ = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM slack_channel_messages WHERE text LIKE ?", q).Scan(&total)

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, channel_id, user_id, user_name, text, timestamp, thread_ts, reaction_count, reactions, fetched_at
		 FROM slack_channel_messages WHERE text LIKE ? ORDER BY timestamp DESC LIMIT ? OFFSET ?`,
		q, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []SlackMessage
	for rows.Next() {
		var m SlackMessage
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.UserID, &m.UserName, &m.Text, &m.Timestamp, &m.ThreadTS, &m.ReactionCount, &m.Reactions, &m.FetchedAt); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs, total, nil
}

func (s *SlackClient) doGet(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("slack API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "slack"}
	}

	return json.NewDecoder(resp.Body).Decode(result)
}
