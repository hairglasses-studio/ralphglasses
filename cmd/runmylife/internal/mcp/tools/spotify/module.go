// Package spotify provides MCP tools for Spotify integration.
package spotify

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

func (m *Module) Name() string        { return "spotify" }
func (m *Module) Description() string { return "Spotify music player and listening stats" }

var spotifyHints = map[string]string{
	"player/now_playing": "Current track info",
	"player/recent":      "Recently played tracks",
	"player/play":        "Play a track/playlist by URI",
	"player/pause":       "Pause playback",
	"player/skip":        "Skip to next track",
	"playlists/list":     "List user playlists",
	"stats/top_tracks":   "Top tracks by time range",
	"stats/top_artists":  "Top artists by time range",
	"search/find":        "Search tracks/artists/albums",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("spotify").
		Domain("player", common.ActionRegistry{
			"now_playing": handleNowPlaying,
			"recent":      handleRecent,
			"play":        handlePlay,
			"pause":       handlePause,
			"skip":        handleSkip,
		}).
		Domain("playlists", common.ActionRegistry{
			"list": handlePlaylistsList,
		}).
		Domain("stats", common.ActionRegistry{
			"top_tracks":  handleTopTracks,
			"top_artists": handleTopArtists,
		}).
		Domain("search", common.ActionRegistry{
			"find": handleSearch,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_spotify",
				mcp.WithDescription("Spotify gateway for music and listening stats.\n\n"+dispatcher.DescribeActionsWithHints(spotifyHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: player, playlists, stats, search")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("uri", mcp.Description("Spotify URI (for play)")),
				mcp.WithString("query", mcp.Description("Search query")),
				mcp.WithString("time_range", mcp.Description("short_term, medium_term, or long_term (for stats)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "spotify",
			Subcategory:         "gateway",
			Tags:                []string{"spotify", "music", "entertainment"},
			Complexity:          tools.ComplexitySimple,
			IsWrite:             true,
			CircuitBreakerGroup: "spotify_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func spotifyClient() (*clients.SpotifyClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	token := cfg.Credentials["spotify"]
	if token == "" {
		return nil, fmt.Errorf("spotify token not configured — add 'spotify' to credentials in config.json")
	}
	return clients.NewSpotifyClient(token), nil
}

func handleNowPlaying(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	np, err := client.NowPlaying(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Now Playing")
	if np.Track == nil || !np.IsPlaying {
		md.Text("Nothing is currently playing.")
	} else {
		md.KeyValue("Track", np.Track.Name)
		md.KeyValue("Artist", np.Track.Artist)
		md.KeyValue("Album", np.Track.Album)
		md.KeyValue("Device", np.DeviceName)
		progress := time.Duration(np.ProgressMs) * time.Millisecond
		total := time.Duration(np.Track.DurationMs) * time.Millisecond
		md.KeyValue("Progress", fmt.Sprintf("%s / %s", progress.Round(time.Second), total.Round(time.Second)))
	}
	return tools.TextResult(md.String()), nil
}

func handleRecent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	limit := common.GetLimitParam(req, 20)
	tracks, err := client.RecentlyPlayed(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Recently Played")
	headers := []string{"Track", "Artist", "Album", "Played At"}
	var rows [][]string
	for _, t := range tracks {
		rows = append(rows, []string{t.Name, t.Artist, t.Album, t.PlayedAt})
	}
	if len(rows) == 0 {
		md.EmptyList("recently played tracks")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handlePlay(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	uri := common.GetStringParam(req, "uri", "")
	if err := client.Play(ctx, uri); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	if uri != "" {
		return tools.TextResult(fmt.Sprintf("Playing: %s", uri)), nil
	}
	return tools.TextResult("Playback resumed."), nil
}

func handlePause(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	if err := client.Pause(ctx); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult("Playback paused."), nil
}

func handleSkip(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	if err := client.SkipNext(ctx); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult("Skipped to next track."), nil
}

func handlePlaylistsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	limit := common.GetLimitParam(req, 20)
	playlists, err := client.ListPlaylists(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Playlists")
	headers := []string{"Name", "Owner", "Tracks", "Public"}
	var rows [][]string
	for _, p := range playlists {
		pub := "no"
		if p.Public {
			pub = "yes"
		}
		rows = append(rows, []string{p.Name, p.Owner, fmt.Sprintf("%d", p.TrackCount), pub})
	}
	if len(rows) == 0 {
		md.EmptyList("playlists")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTopTracks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	timeRange := common.GetStringParam(req, "time_range", "medium_term")
	limit := common.GetLimitParam(req, 20)
	tracks, err := client.TopTracks(ctx, timeRange, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Top Tracks (%s)", timeRange))
	headers := []string{"#", "Track", "Artist", "Album"}
	var rows [][]string
	for i, t := range tracks {
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), t.Name, t.Artist, t.Album})
	}
	if len(rows) == 0 {
		md.EmptyList("top tracks")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTopArtists(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	timeRange := common.GetStringParam(req, "time_range", "medium_term")
	limit := common.GetLimitParam(req, 20)
	artists, err := client.TopArtists(ctx, timeRange, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Top Artists (%s)", timeRange))
	headers := []string{"#", "Artist", "Genres"}
	var rows [][]string
	for i, a := range artists {
		genres := ""
		if len(a.Genres) > 0 {
			genres = fmt.Sprintf("%v", a.Genres)
		}
		rows = append(rows, []string{fmt.Sprintf("%d", i+1), a.Name, genres})
	}
	if len(rows) == 0 {
		md.EmptyList("top artists")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := spotifyClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add spotify token to config"), nil
	}
	query := common.GetStringParam(req, "query", "")
	if query == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "query is required"), nil
	}
	limit := common.GetLimitParam(req, 10)
	tracks, err := client.Search(ctx, query, "track", limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Search: " + query)
	headers := []string{"Track", "Artist", "Album", "URI"}
	var rows [][]string
	for _, t := range tracks {
		rows = append(rows, []string{t.Name, t.Artist, t.Album, t.URI})
	}
	if len(rows) == 0 {
		md.EmptyList("results")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}
