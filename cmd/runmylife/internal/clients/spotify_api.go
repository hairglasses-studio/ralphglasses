package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const spotifyAPIBaseURL = "https://api.spotify.com/v1"

// SpotifyClient is a REST client for the Spotify Web API.
type SpotifyClient struct {
	httpClient *http.Client
	token      string
}

// SpotifyTrack represents a Spotify track.
type SpotifyTrack struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Artist     string `json:"artist"`
	Album      string `json:"album"`
	DurationMs int    `json:"duration_ms"`
	PlayedAt   string `json:"played_at,omitempty"`
	URI        string `json:"uri"`
}

// SpotifyPlaylist represents a Spotify playlist.
type SpotifyPlaylist struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Owner      string `json:"owner"`
	TrackCount int    `json:"track_count"`
	Public     bool   `json:"public"`
	URL        string `json:"url"`
}

// SpotifyNowPlaying holds current playback info.
type SpotifyNowPlaying struct {
	IsPlaying  bool          `json:"is_playing"`
	Track      *SpotifyTrack `json:"track,omitempty"`
	DeviceName string        `json:"device_name"`
	ProgressMs int           `json:"progress_ms"`
}

// SpotifyArtist holds artist info from top artists endpoint.
type SpotifyArtist struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Genres []string `json:"genres"`
	URL    string   `json:"url"`
}

// NewSpotifyClient creates a new Spotify API client.
func NewSpotifyClient(token string) *SpotifyClient {
	return &SpotifyClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// NowPlaying returns the currently playing track.
func (s *SpotifyClient) NowPlaying(ctx context.Context) (*SpotifyNowPlaying, error) {
	var raw struct {
		IsPlaying  bool   `json:"is_playing"`
		ProgressMs int    `json:"progress_ms"`
		Device     struct{ Name string } `json:"device"`
		Item       *struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			URI        string `json:"uri"`
			DurationMs int    `json:"duration_ms"`
			Album      struct{ Name string } `json:"album"`
			Artists    []struct{ Name string } `json:"artists"`
		} `json:"item"`
	}
	if err := s.doGet(ctx, "/me/player/currently-playing", &raw); err != nil {
		return nil, err
	}
	np := &SpotifyNowPlaying{IsPlaying: raw.IsPlaying, DeviceName: raw.Device.Name, ProgressMs: raw.ProgressMs}
	if raw.Item != nil {
		artist := ""
		if len(raw.Item.Artists) > 0 {
			artist = raw.Item.Artists[0].Name
		}
		np.Track = &SpotifyTrack{ID: raw.Item.ID, Name: raw.Item.Name, Artist: artist, Album: raw.Item.Album.Name, DurationMs: raw.Item.DurationMs, URI: raw.Item.URI}
	}
	return np, nil
}

// RecentlyPlayed returns recently played tracks.
func (s *SpotifyClient) RecentlyPlayed(ctx context.Context, limit int) ([]SpotifyTrack, error) {
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Items []struct {
			Track struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				URI        string `json:"uri"`
				DurationMs int    `json:"duration_ms"`
				Album      struct{ Name string } `json:"album"`
				Artists    []struct{ Name string } `json:"artists"`
			} `json:"track"`
			PlayedAt string `json:"played_at"`
		} `json:"items"`
	}
	if err := s.doGet(ctx, fmt.Sprintf("/me/player/recently-played?limit=%d", limit), &raw); err != nil {
		return nil, err
	}
	var tracks []SpotifyTrack
	for _, item := range raw.Items {
		artist := ""
		if len(item.Track.Artists) > 0 {
			artist = item.Track.Artists[0].Name
		}
		tracks = append(tracks, SpotifyTrack{ID: item.Track.ID, Name: item.Track.Name, Artist: artist, Album: item.Track.Album.Name, DurationMs: item.Track.DurationMs, PlayedAt: item.PlayedAt, URI: item.Track.URI})
	}
	return tracks, nil
}

// ListPlaylists returns the user's playlists.
func (s *SpotifyClient) ListPlaylists(ctx context.Context, limit int) ([]SpotifyPlaylist, error) {
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Items []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Owner  struct{ DisplayName string `json:"display_name"` } `json:"owner"`
			Public bool   `json:"public"`
			Tracks struct{ Total int } `json:"tracks"`
			ExternalURLs struct{ Spotify string } `json:"external_urls"`
		} `json:"items"`
	}
	if err := s.doGet(ctx, fmt.Sprintf("/me/playlists?limit=%d", limit), &raw); err != nil {
		return nil, err
	}
	var playlists []SpotifyPlaylist
	for _, item := range raw.Items {
		playlists = append(playlists, SpotifyPlaylist{ID: item.ID, Name: item.Name, Owner: item.Owner.DisplayName, TrackCount: item.Tracks.Total, Public: item.Public, URL: item.ExternalURLs.Spotify})
	}
	return playlists, nil
}

// TopTracks returns the user's top tracks.
func (s *SpotifyClient) TopTracks(ctx context.Context, timeRange string, limit int) ([]SpotifyTrack, error) {
	if timeRange == "" {
		timeRange = "medium_term"
	}
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Items []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			URI        string `json:"uri"`
			DurationMs int    `json:"duration_ms"`
			Album      struct{ Name string } `json:"album"`
			Artists    []struct{ Name string } `json:"artists"`
		} `json:"items"`
	}
	if err := s.doGet(ctx, fmt.Sprintf("/me/top/tracks?time_range=%s&limit=%d", timeRange, limit), &raw); err != nil {
		return nil, err
	}
	var tracks []SpotifyTrack
	for _, item := range raw.Items {
		artist := ""
		if len(item.Artists) > 0 {
			artist = item.Artists[0].Name
		}
		tracks = append(tracks, SpotifyTrack{ID: item.ID, Name: item.Name, Artist: artist, Album: item.Album.Name, DurationMs: item.DurationMs, URI: item.URI})
	}
	return tracks, nil
}

// TopArtists returns the user's top artists.
func (s *SpotifyClient) TopArtists(ctx context.Context, timeRange string, limit int) ([]SpotifyArtist, error) {
	if timeRange == "" {
		timeRange = "medium_term"
	}
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Items []struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Genres []string `json:"genres"`
			ExternalURLs struct{ Spotify string } `json:"external_urls"`
		} `json:"items"`
	}
	if err := s.doGet(ctx, fmt.Sprintf("/me/top/artists?time_range=%s&limit=%d", timeRange, limit), &raw); err != nil {
		return nil, err
	}
	var artists []SpotifyArtist
	for _, item := range raw.Items {
		artists = append(artists, SpotifyArtist{ID: item.ID, Name: item.Name, Genres: item.Genres, URL: item.ExternalURLs.Spotify})
	}
	return artists, nil
}

// Search searches for tracks, artists, or albums.
func (s *SpotifyClient) Search(ctx context.Context, query, searchType string, limit int) ([]SpotifyTrack, error) {
	if searchType == "" {
		searchType = "track"
	}
	if limit <= 0 {
		limit = 10
	}
	var raw struct {
		Tracks struct {
			Items []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				URI        string `json:"uri"`
				DurationMs int    `json:"duration_ms"`
				Album      struct{ Name string } `json:"album"`
				Artists    []struct{ Name string } `json:"artists"`
			} `json:"items"`
		} `json:"tracks"`
	}
	path := fmt.Sprintf("/search?q=%s&type=%s&limit=%d", url.QueryEscape(query), searchType, limit)
	if err := s.doGet(ctx, path, &raw); err != nil {
		return nil, err
	}
	var tracks []SpotifyTrack
	for _, item := range raw.Tracks.Items {
		artist := ""
		if len(item.Artists) > 0 {
			artist = item.Artists[0].Name
		}
		tracks = append(tracks, SpotifyTrack{ID: item.ID, Name: item.Name, Artist: artist, Album: item.Album.Name, DurationMs: item.DurationMs, URI: item.URI})
	}
	return tracks, nil
}

// Play starts or resumes playback. Pass URI to play specific content.
func (s *SpotifyClient) Play(ctx context.Context, uri string) error {
	var body interface{}
	if uri != "" {
		body = map[string]interface{}{"uris": []string{uri}}
	}
	return s.doPut(ctx, "/me/player/play", body)
}

// Pause pauses playback.
func (s *SpotifyClient) Pause(ctx context.Context) error {
	return s.doPut(ctx, "/me/player/pause", nil)
}

// SkipNext skips to the next track.
func (s *SpotifyClient) SkipNext(ctx context.Context) error {
	return s.doPostNoBody(ctx, "/me/player/next")
}

func (s *SpotifyClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, spotifyAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	s.setHeaders(req)
	return s.doRequest(req, out)
}

func (s *SpotifyClient) doPut(ctx context.Context, path string, body interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, spotifyAPIBaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	s.setHeaders(req)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(b), API: "spotify"}
	}
	return nil
}

func (s *SpotifyClient) doPostNoBody(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	s.setHeaders(req)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(b), API: "spotify"}
	}
	return nil
}

func (s *SpotifyClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.token)
}

func (s *SpotifyClient) doRequest(req *http.Request, out interface{}) error {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("spotify API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 204 {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(b), API: "spotify"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
