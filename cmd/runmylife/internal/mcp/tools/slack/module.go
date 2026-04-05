// Package slack provides MCP tools for Slack workspace management.
package slack

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

// Module implements the ToolModule interface for Slack integration.
type Module struct{}

func (m *Module) Name() string        { return "slack" }
func (m *Module) Description() string { return "Slack workspace channels, messages, and search" }

var slackHints = map[string]string{
	"channels/list":    "List Slack channels in workspace",
	"channels/history": "View recent messages in a channel",
	"messages/search":  "Search Slack messages",
	"messages/post":    "Post a message to a channel",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("slack").
		Domain("channels", common.ActionRegistry{
			"list":    handleChannelsList,
			"history": handleChannelsHistory,
		}).
		Domain("messages", common.ActionRegistry{
			"search": handleMessagesSearch,
			"post":   handleMessagesPost,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_slack",
				mcp.WithDescription(
					"Slack gateway. Manages workspace channels, messages, and search.\n\n"+
						dispatcher.DescribeActionsWithHints(slackHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: channels, messages")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("channel_id", mcp.Description("Slack channel ID")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithString("text", mcp.Description("Message text to post")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "slack",
			Subcategory:         "gateway",
			Tags:                []string{"slack", "messaging", "workspace"},
			UseCases:            []string{"Browse Slack channels", "Search message history", "Post messages"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			ProducesRefs:        []string{"slack_message"},
			CircuitBreakerGroup: "slack_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func getSlackClient() (*clients.SlackClient, *mcp.CallToolResult) {
	cfg, err := config.Load()
	if err != nil {
		return nil, common.CodedErrorResult(common.ErrConfig, err)
	}
	token := cfg.Credentials["slack"]
	if token == "" {
		return nil, common.CodedErrorResultf(common.ErrConfig, "no slack token in config.json credentials")
	}
	db, _ := common.SqlDB()
	return clients.NewSlackClient(token, db), nil
}

func handleChannelsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, errResult := getSlackClient()
	if errResult != nil {
		return errResult, nil
	}
	limit := common.GetIntParam(req, "limit", 50)

	channels, err := client.ListChannels(ctx, limit)
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check Slack token permissions (channels:read)"), nil
	}

	// Save to local DB
	for i := range channels {
		_ = client.SaveChannel(ctx, &channels[i])
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Slack Channels (%d)", len(channels)))
	headers := []string{"Name", "Members", "Topic"}
	var rows [][]string
	for _, ch := range channels {
		topic := ch.Topic
		if len(topic) > 50 {
			topic = topic[:47] + "..."
		}
		rows = append(rows, []string{
			fmt.Sprintf("#%s (%s)", ch.Name, ch.ID),
			fmt.Sprintf("%d", ch.MemberCount),
			topic,
		})
	}
	md.Table(headers, rows)
	return tools.TextResult(md.String()), nil
}

func handleChannelsHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channelID, ok := common.RequireStringParam(req, "channel_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "channel_id required"), nil
	}
	client, errResult := getSlackClient()
	if errResult != nil {
		return errResult, nil
	}
	limit := common.GetIntParam(req, "limit", 20)

	messages, err := client.GetChannelHistory(ctx, channelID, limit)
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check channel_id and permissions"), nil
	}

	// Save to local DB
	for i := range messages {
		_ = client.SaveMessage(ctx, &messages[i])
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Channel History (%d messages)", len(messages)))
	for _, m := range messages {
		user := m.UserID
		if m.UserName != "" {
			user = m.UserName
		}
		text := m.Text
		if len(text) > 200 {
			text = text[:197] + "..."
		}
		md.Text(fmt.Sprintf("**%s** (ts: %s)\n%s", user, m.Timestamp, text))
		if m.ReactionCount > 0 {
			md.Text(fmt.Sprintf("  Reactions: %d", m.ReactionCount))
		}
	}
	return tools.TextResult(md.String()), nil
}

func handleMessagesSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := common.RequireStringParam(req, "query")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query required"), nil
	}
	client, errResult := getSlackClient()
	if errResult != nil {
		return errResult, nil
	}
	limit := common.GetIntParam(req, "limit", 20)

	messages, err := client.SearchMessages(ctx, query, limit)
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check Slack token permissions (search:read)"), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Search Results for %q (%d)", query, len(messages)))
	for _, m := range messages {
		user := m.UserID
		if m.UserName != "" {
			user = m.UserName
		}
		text := m.Text
		if len(text) > 200 {
			text = text[:197] + "..."
		}
		md.Text(fmt.Sprintf("**%s** in %s\n%s", user, m.ChannelID, text))
	}
	if len(messages) == 0 {
		md.EmptyList("messages")
	}
	return tools.TextResult(md.String()), nil
}

func handleMessagesPost(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channelID, ok := common.RequireStringParam(req, "channel_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "channel_id required"), nil
	}
	text, ok := common.RequireStringParam(req, "text")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "text required"), nil
	}
	client, errResult := getSlackClient()
	if errResult != nil {
		return errResult, nil
	}

	if err := client.PostMessage(ctx, channelID, text); err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check channel_id and chat:write permission"), nil
	}

	return tools.TextResult(fmt.Sprintf("Message posted to channel %s.", channelID)), nil
}
