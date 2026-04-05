// Package discord provides MCP tools for Discord server and DM management.
package discord

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for Discord integration.
type Module struct{}

func (m *Module) Name() string        { return "discord" }
func (m *Module) Description() string { return "Discord server, channel, and DM management" }

var discordHints = map[string]string{
	"servers/list":     "List all Discord servers the bot is in",
	"channels/list":    "List channels in a Discord server",
	"channels/messages": "View recent messages in a channel",
	"dms/list":         "List DM channels",
	"dms/search":       "Search messages in a DM channel",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("discord").
		Domain("servers", common.ActionRegistry{
			"list": handleServersList,
		}).
		Domain("channels", common.ActionRegistry{
			"list":     handleChannelsList,
			"messages": handleChannelsMessages,
		}).
		Domain("dms", common.ActionRegistry{
			"list":   handleDMsList,
			"search": handleDMsSearch,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_discord",
				mcp.WithDescription(
					"Discord gateway. Manages servers, channels, and DMs.\n\n"+
						dispatcher.DescribeActionsWithHints(discordHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: servers, channels, dms")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("server_id", mcp.Description("Discord server (guild) ID")),
				mcp.WithString("channel_id", mcp.Description("Discord channel ID")),
				mcp.WithString("query", mcp.Description("Search query for DM search")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "discord",
			Subcategory:         "gateway",
			Tags:                []string{"discord", "messaging", "community"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			CircuitBreakerGroup: "discord_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func getDiscordClient() (*clients.DiscordClient, *mcp.CallToolResult) {
	cfg, err := config.Load()
	if err != nil {
		return nil, common.CodedErrorResult(common.ErrConfig, err)
	}
	token := cfg.Credentials["discord"]
	if token == "" {
		return nil, common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("discord bot token not configured"),
			"Add your Discord bot token to ~/.config/runmylife/config.json under credentials.discord",
			"Create a bot at https://discord.com/developers/applications",
		)
	}
	return clients.NewDiscordClient(token), nil
}

// channelTypeName maps Discord channel type integers to human-readable names.
func channelTypeName(t int) string {
	switch t {
	case 0:
		return "text"
	case 2:
		return "voice"
	case 4:
		return "category"
	case 5:
		return "announcement"
	case 13:
		return "stage"
	case 15:
		return "forum"
	default:
		return fmt.Sprintf("type-%d", t)
	}
}

func handleServersList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, errResult := getDiscordClient()
	if errResult != nil {
		return errResult, nil
	}

	guilds, err := client.GetGuilds(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	// Cache in database
	database, dbErr := common.OpenDB()
	if dbErr == nil {
		defer database.Close()
		for _, g := range guilds {
			database.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO discord_servers (id, name, member_count, cached_at) VALUES (?, ?, ?, datetime('now'))`,
				g.ID, g.Name, g.MemberCount,
			)
		}
	}

	md := common.NewMarkdownBuilder().Title("Discord Servers")
	if len(guilds) == 0 {
		md.EmptyList("servers")
	} else {
		headers := []string{"Name", "Members", "ID"}
		var rows [][]string
		for _, g := range guilds {
			rows = append(rows, []string{g.Name, fmt.Sprintf("%d", g.MemberCount), g.ID})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleChannelsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	serverID, ok := common.RequireStringParam(req, "server_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "server_id is required for channels/list"), nil
	}

	client, errResult := getDiscordClient()
	if errResult != nil {
		return errResult, nil
	}

	channels, err := client.GetChannels(ctx, serverID)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	// Cache in database
	database, dbErr := common.OpenDB()
	if dbErr == nil {
		defer database.Close()
		for _, ch := range channels {
			database.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO discord_channels (id, server_id, name, type, topic, cached_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
				ch.ID, serverID, ch.Name, channelTypeName(ch.Type), ch.Topic,
			)
		}
	}

	md := common.NewMarkdownBuilder().Title("Channels")
	if len(channels) == 0 {
		md.EmptyList("channels")
	} else {
		headers := []string{"Name", "Type", "Topic", "ID"}
		var rows [][]string
		for _, ch := range channels {
			topic := common.TruncateWords(ch.Topic, 40)
			rows = append(rows, []string{ch.Name, channelTypeName(ch.Type), topic, ch.ID})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleChannelsMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channelID, ok := common.RequireStringParam(req, "channel_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "channel_id is required for channels/messages"), nil
	}

	limit := common.GetLimitParam(req, 20)

	client, errResult := getDiscordClient()
	if errResult != nil {
		return errResult, nil
	}

	messages, err := client.GetMessages(ctx, channelID, limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	// Cache in database
	database, dbErr := common.OpenDB()
	if dbErr == nil {
		defer database.Close()
		for _, msg := range messages {
			database.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO discord_messages (id, channel_id, author, content, sent_at, cached_at) VALUES (?, ?, ?, ?, ?, datetime('now'))`,
				msg.ID, msg.ChannelID, msg.Author.Username, msg.Content, msg.Timestamp,
			)
		}
	}

	md := common.NewMarkdownBuilder().Title("Messages")
	if len(messages) == 0 {
		md.EmptyList("messages")
	} else {
		for _, msg := range messages {
			content := common.TruncateWords(msg.Content, 200)
			md.Text(fmt.Sprintf("**%s** (%s)\n%s", msg.Author.Username, msg.Timestamp, content))
		}
		md.Text(fmt.Sprintf("*Showing %d messages*", len(messages)))
	}
	return tools.TextResult(md.String()), nil
}

func handleDMsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, errResult := getDiscordClient()
	if errResult != nil {
		return errResult, nil
	}

	channels, err := client.GetDMChannels(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	md := common.NewMarkdownBuilder().Title("DM Channels")
	if len(channels) == 0 {
		md.EmptyList("DM channels")
	} else {
		headers := []string{"Recipient", "Channel ID", "Last Message ID"}
		var rows [][]string
		for _, ch := range channels {
			recipient := "Unknown"
			if len(ch.Recipients) > 0 {
				recipient = ch.Recipients[0].Username
			}
			rows = append(rows, []string{recipient, ch.ID, ch.LastMessageID})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleDMsSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channelID, ok := common.RequireStringParam(req, "channel_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "channel_id is required for dms/search"), nil
	}
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required for dms/search"), nil
	}

	limit := common.GetLimitParam(req, 20)

	client, errResult := getDiscordClient()
	if errResult != nil {
		return errResult, nil
	}

	messages, err := client.SearchDMs(ctx, channelID, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ClassifyClientError(err), err), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("DM Search: %q", query))
	if len(messages) == 0 {
		md.EmptyList("matching messages")
	} else {
		for _, msg := range messages {
			content := common.TruncateWords(msg.Content, 200)
			md.Text(fmt.Sprintf("**%s** (%s)\n%s", msg.Author.Username, msg.Timestamp, content))
		}
		md.Text(fmt.Sprintf("*Found %d matching messages*", len(messages)))
	}
	return tools.TextResult(md.String()), nil
}
