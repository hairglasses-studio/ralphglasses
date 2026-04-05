// Package bluesky provides MCP tools for Bluesky social network.
package bluesky

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

type Module struct{}

func (m *Module) Name() string        { return "bluesky" }
func (m *Module) Description() string { return "Bluesky social network integration" }

var bskyHints = map[string]string{
	"feed/timeline": "Home timeline",
	"feed/author":   "Posts by a user",
	"posts/create":  "Create a post (requires confirm=true)",
	"search/find":   "Search posts",
	"profile/get":   "Get user profile",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("bluesky").
		Domain("feed", common.ActionRegistry{
			"timeline": handleFeedTimeline,
			"author":   handleFeedAuthor,
		}).
		Domain("posts", common.ActionRegistry{
			"create": handlePostsCreate,
		}).
		Domain("search", common.ActionRegistry{
			"find": handleSearchFind,
		}).
		Domain("profile", common.ActionRegistry{
			"get": handleProfileGet,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_bluesky",
				mcp.WithDescription("Bluesky gateway for social networking.\n\n"+dispatcher.DescribeActionsWithHints(bskyHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: feed, posts, search, profile")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("actor", mcp.Description("User handle or DID")),
				mcp.WithString("text", mcp.Description("Post text (for create)")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithBoolean("confirm", mcp.Description("Confirm post creation")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 25)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "bluesky",
			Subcategory:         "gateway",
			Tags:                []string{"bluesky", "social", "atproto"},
			Complexity:          tools.ComplexitySimple,
			IsWrite:             true,
			CircuitBreakerGroup: "bluesky_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func bskyClient(ctx context.Context) (*clients.BlueskyClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	handle := cfg.Credentials["bluesky_handle"]
	password := cfg.Credentials["bluesky_password"]
	if handle == "" || password == "" {
		return nil, fmt.Errorf("bluesky not configured — add 'bluesky_handle' and 'bluesky_password' (app password) to credentials")
	}
	return clients.NewBlueskyClient(ctx, handle, password)
}

func handleFeedTimeline(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := bskyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add bluesky credentials to config"), nil
	}
	limit := common.GetLimitParam(req, 25)
	posts, err := client.Timeline(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts("Timeline", posts), nil
}

func handleFeedAuthor(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	actor := common.GetStringParam(req, "actor", "")
	if actor == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "actor (handle or DID) is required"), nil
	}
	client, err := bskyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add bluesky credentials to config"), nil
	}
	limit := common.GetLimitParam(req, 25)
	posts, err := client.AuthorFeed(ctx, actor, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts(fmt.Sprintf("Posts by %s", actor), posts), nil
}

func handlePostsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text := common.GetStringParam(req, "text", "")
	if text == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "text is required"), nil
	}
	confirm := common.GetBoolParam(req, "confirm", false)
	if !confirm {
		return tools.TextResult(fmt.Sprintf("# Confirm Post\n\n**Text:** %s\n\nSet `confirm=true` to publish this post. This cannot be undone.", text)), nil
	}
	client, err := bskyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add bluesky credentials to config"), nil
	}
	post, err := client.CreatePost(ctx, text)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("# Post Created\n\n- **URI:** %s\n- **CID:** %s", post.URI, post.CID)), nil
}

func handleSearchFind(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required"), nil
	}
	client, err := bskyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add bluesky credentials to config"), nil
	}
	limit := common.GetLimitParam(req, 25)
	posts, err := client.SearchPosts(ctx, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return formatPosts("Search: "+query, posts), nil
}

func handleProfileGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	actor := common.GetStringParam(req, "actor", "")
	if actor == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "actor (handle or DID) is required"), nil
	}
	client, err := bskyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add bluesky credentials to config"), nil
	}
	profile, err := client.GetProfile(ctx, actor)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title(profile.DisplayName)
	md.KeyValue("Handle", profile.Handle)
	md.KeyValue("DID", profile.DID)
	md.KeyValue("Posts", fmt.Sprintf("%d", profile.PostsCount))
	md.KeyValue("Followers", fmt.Sprintf("%d", profile.FollowersCount))
	md.KeyValue("Following", fmt.Sprintf("%d", profile.FollowsCount))
	if profile.Description != "" {
		md.Section("Bio").Text(profile.Description)
	}
	return tools.TextResult(md.String()), nil
}

func formatPosts(title string, posts []clients.BlueskyPost) *mcp.CallToolResult {
	md := common.NewMarkdownBuilder().Title(title)
	for _, p := range posts {
		text := p.Text
		if len(text) > 120 {
			text = text[:120] + "..."
		}
		md.Section(p.Author)
		md.Text(text)
		md.Text(fmt.Sprintf("*%s | Likes: %d | Reposts: %d | Replies: %d*", p.CreatedAt, p.LikeCount, p.RepostCount, p.ReplyCount))
	}
	if len(posts) == 0 {
		md.EmptyList("posts")
	}
	return tools.TextResult(md.String())
}
