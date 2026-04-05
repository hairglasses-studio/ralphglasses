package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const discordAPIBaseURL = "https://discord.com/api/v10"

// DiscordClient is a thin REST client for the Discord API.
type DiscordClient struct {
	httpClient *http.Client
	token      string
}

// DiscordGuild represents a Discord server.
type DiscordGuild struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	MemberCount int    `json:"approximate_member_count"`
}

// DiscordChannel represents a Discord channel.
type DiscordChannel struct {
	ID      string `json:"id"`
	GuildID string `json:"guild_id"`
	Name    string `json:"name"`
	Type    int    `json:"type"`
	Topic   string `json:"topic"`
}

// DiscordMessage represents a Discord message.
type DiscordMessage struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Author    struct {
		Username string `json:"username"`
	} `json:"author"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

// DiscordDMChannel represents a Discord DM channel.
type DiscordDMChannel struct {
	ID         string `json:"id"`
	Type       int    `json:"type"`
	Recipients []struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"recipients"`
	LastMessageID string `json:"last_message_id"`
}

// NewDiscordClient creates a new Discord REST API client.
func NewDiscordClient(botToken string) *DiscordClient {
	return &DiscordClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      botToken,
	}
}

// GetGuilds returns all guilds the bot is a member of.
func (d *DiscordClient) GetGuilds(ctx context.Context) ([]DiscordGuild, error) {
	var guilds []DiscordGuild
	if err := d.doGet(ctx, "/users/@me/guilds?with_counts=true", &guilds); err != nil {
		return nil, err
	}
	return guilds, nil
}

// GetChannels returns all channels for a guild.
func (d *DiscordClient) GetChannels(ctx context.Context, guildID string) ([]DiscordChannel, error) {
	var channels []DiscordChannel
	if err := d.doGet(ctx, fmt.Sprintf("/guilds/%s/channels", guildID), &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// GetMessages returns recent messages from a channel.
func (d *DiscordClient) GetMessages(ctx context.Context, channelID string, limit int) ([]DiscordMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var messages []DiscordMessage
	if err := d.doGet(ctx, fmt.Sprintf("/channels/%s/messages?limit=%d", channelID, limit), &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

// GetDMChannels returns all DM channels for the bot user.
func (d *DiscordClient) GetDMChannels(ctx context.Context) ([]DiscordDMChannel, error) {
	var channels []DiscordDMChannel
	if err := d.doGet(ctx, "/users/@me/channels", &channels); err != nil {
		return nil, err
	}
	return channels, nil
}

// SearchDMs fetches recent messages from a DM channel and filters locally by query.
func (d *DiscordClient) SearchDMs(ctx context.Context, channelID, query string, limit int) ([]DiscordMessage, error) {
	messages, err := d.GetMessages(ctx, channelID, 100)
	if err != nil {
		return nil, err
	}
	var matched []DiscordMessage
	for _, m := range messages {
		if containsInsensitive(m.Content, query) || containsInsensitive(m.Author.Username, query) {
			matched = append(matched, m)
			if len(matched) >= limit {
				break
			}
		}
	}
	return matched, nil
}

// CreateDM opens a DM channel with the given user ID.
func (d *DiscordClient) CreateDM(ctx context.Context, recipientID string) (*DiscordDMChannel, error) {
	body := fmt.Sprintf(`{"recipient_id":"%s"}`, recipientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordAPIBaseURL+"/users/@me/channels", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+d.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), API: "discord"}
	}
	var ch DiscordDMChannel
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

// SendMessage sends a text message to a channel (works for both guild channels and DM channels).
func (d *DiscordClient) SendMessage(ctx context.Context, channelID, content string) error {
	body := fmt.Sprintf(`{"content":%s}`, jsonString(content))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordAPIBaseURL+fmt.Sprintf("/channels/%s/messages", channelID), strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+d.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), API: "discord"}
	}
	return nil
}

// jsonString returns a JSON-encoded string value.
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func containsInsensitive(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func (d *DiscordClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discordAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bot "+d.token)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "discord"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
