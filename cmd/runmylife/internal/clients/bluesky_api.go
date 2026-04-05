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

const blueskyDefaultPDS = "https://bsky.social"

// BlueskyClient is a REST client for the AT Protocol (Bluesky).
type BlueskyClient struct {
	httpClient *http.Client
	pdsURL     string
	did        string
	accessJWT  string
	refreshJWT string
}

// BlueskyPost represents a Bluesky post.
type BlueskyPost struct {
	URI        string `json:"uri"`
	CID        string `json:"cid"`
	Author     string `json:"author"`
	AuthorDID  string `json:"author_did"`
	Text       string `json:"text"`
	LikeCount  int    `json:"like_count"`
	RepostCount int   `json:"repost_count"`
	ReplyCount int    `json:"reply_count"`
	CreatedAt  string `json:"created_at"`
}

// BlueskyProfile represents a user profile.
type BlueskyProfile struct {
	DID           string `json:"did"`
	Handle        string `json:"handle"`
	DisplayName   string `json:"display_name"`
	Description   string `json:"description"`
	FollowersCount int   `json:"followers_count"`
	FollowsCount   int   `json:"follows_count"`
	PostsCount     int   `json:"posts_count"`
}

// BlueskyFollow represents a followed account.
type BlueskyFollow struct {
	DID         string `json:"did"`
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
}

// NewBlueskyClient creates a Bluesky client and authenticates with app password.
func NewBlueskyClient(ctx context.Context, handle, appPassword string) (*BlueskyClient, error) {
	c := &BlueskyClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		pdsURL:     blueskyDefaultPDS,
	}
	if err := c.createSession(ctx, handle, appPassword); err != nil {
		return nil, fmt.Errorf("bluesky auth: %w", err)
	}
	return c, nil
}

func (b *BlueskyClient) createSession(ctx context.Context, handle, password string) error {
	body := map[string]string{"identifier": handle, "password": password}
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.pdsURL+"/xrpc/com.atproto.server.createSession", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), API: "bluesky"}
	}
	var session struct {
		DID        string `json:"did"`
		AccessJwt  string `json:"accessJwt"`
		RefreshJwt string `json:"refreshJwt"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return err
	}
	b.did = session.DID
	b.accessJWT = session.AccessJwt
	b.refreshJWT = session.RefreshJwt
	return nil
}

// Timeline returns the home timeline.
func (b *BlueskyClient) Timeline(ctx context.Context, limit int) ([]BlueskyPost, error) {
	if limit <= 0 {
		limit = 25
	}
	var raw struct {
		Feed []struct {
			Post struct {
				URI    string `json:"uri"`
				CID    string `json:"cid"`
				Author struct {
					DID         string `json:"did"`
					Handle      string `json:"handle"`
					DisplayName string `json:"displayName"`
				} `json:"author"`
				Record struct {
					Text      string `json:"text"`
					CreatedAt string `json:"createdAt"`
				} `json:"record"`
				LikeCount   int `json:"likeCount"`
				RepostCount int `json:"repostCount"`
				ReplyCount  int `json:"replyCount"`
			} `json:"post"`
		} `json:"feed"`
	}
	if err := b.doGet(ctx, fmt.Sprintf("/xrpc/app.bsky.feed.getTimeline?limit=%d", limit), &raw); err != nil {
		return nil, err
	}
	var posts []BlueskyPost
	for _, item := range raw.Feed {
		p := item.Post
		posts = append(posts, BlueskyPost{URI: p.URI, CID: p.CID, Author: p.Author.Handle, AuthorDID: p.Author.DID, Text: p.Record.Text, LikeCount: p.LikeCount, RepostCount: p.RepostCount, ReplyCount: p.ReplyCount, CreatedAt: p.Record.CreatedAt})
	}
	return posts, nil
}

// AuthorFeed returns posts by a specific user.
func (b *BlueskyClient) AuthorFeed(ctx context.Context, actor string, limit int) ([]BlueskyPost, error) {
	if limit <= 0 {
		limit = 25
	}
	var raw struct {
		Feed []struct {
			Post struct {
				URI    string `json:"uri"`
				CID    string `json:"cid"`
				Author struct {
					DID         string `json:"did"`
					Handle      string `json:"handle"`
					DisplayName string `json:"displayName"`
				} `json:"author"`
				Record struct {
					Text      string `json:"text"`
					CreatedAt string `json:"createdAt"`
				} `json:"record"`
				LikeCount   int `json:"likeCount"`
				RepostCount int `json:"repostCount"`
				ReplyCount  int `json:"replyCount"`
			} `json:"post"`
		} `json:"feed"`
	}
	if err := b.doGet(ctx, fmt.Sprintf("/xrpc/app.bsky.feed.getAuthorFeed?actor=%s&limit=%d", url.QueryEscape(actor), limit), &raw); err != nil {
		return nil, err
	}
	var posts []BlueskyPost
	for _, item := range raw.Feed {
		p := item.Post
		posts = append(posts, BlueskyPost{URI: p.URI, CID: p.CID, Author: p.Author.Handle, AuthorDID: p.Author.DID, Text: p.Record.Text, LikeCount: p.LikeCount, RepostCount: p.RepostCount, ReplyCount: p.ReplyCount, CreatedAt: p.Record.CreatedAt})
	}
	return posts, nil
}

// CreatePost creates a new post.
func (b *BlueskyClient) CreatePost(ctx context.Context, text string) (*BlueskyPost, error) {
	record := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}
	body := map[string]interface{}{
		"repo":       b.did,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}
	var raw struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := b.doPost(ctx, "/xrpc/com.atproto.repo.createRecord", body, &raw); err != nil {
		return nil, err
	}
	return &BlueskyPost{URI: raw.URI, CID: raw.CID, Author: b.did, Text: text, CreatedAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

// GetProfile returns a user's profile.
func (b *BlueskyClient) GetProfile(ctx context.Context, actor string) (*BlueskyProfile, error) {
	var raw struct {
		DID           string `json:"did"`
		Handle        string `json:"handle"`
		DisplayName   string `json:"displayName"`
		Description   string `json:"description"`
		FollowersCount int   `json:"followersCount"`
		FollowsCount   int   `json:"followsCount"`
		PostsCount     int   `json:"postsCount"`
	}
	if err := b.doGet(ctx, "/xrpc/app.bsky.actor.getProfile?actor="+url.QueryEscape(actor), &raw); err != nil {
		return nil, err
	}
	return &BlueskyProfile{DID: raw.DID, Handle: raw.Handle, DisplayName: raw.DisplayName, Description: raw.Description, FollowersCount: raw.FollowersCount, FollowsCount: raw.FollowsCount, PostsCount: raw.PostsCount}, nil
}

// SearchPosts searches for posts.
func (b *BlueskyClient) SearchPosts(ctx context.Context, query string, limit int) ([]BlueskyPost, error) {
	if limit <= 0 {
		limit = 25
	}
	var raw struct {
		Posts []struct {
			URI    string `json:"uri"`
			CID    string `json:"cid"`
			Author struct {
				DID    string `json:"did"`
				Handle string `json:"handle"`
			} `json:"author"`
			Record struct {
				Text      string `json:"text"`
				CreatedAt string `json:"createdAt"`
			} `json:"record"`
			LikeCount   int `json:"likeCount"`
			RepostCount int `json:"repostCount"`
			ReplyCount  int `json:"replyCount"`
		} `json:"posts"`
	}
	if err := b.doGet(ctx, fmt.Sprintf("/xrpc/app.bsky.feed.searchPosts?q=%s&limit=%d", url.QueryEscape(query), limit), &raw); err != nil {
		return nil, err
	}
	var posts []BlueskyPost
	for _, p := range raw.Posts {
		posts = append(posts, BlueskyPost{URI: p.URI, CID: p.CID, Author: p.Author.Handle, AuthorDID: p.Author.DID, Text: p.Record.Text, LikeCount: p.LikeCount, RepostCount: p.RepostCount, ReplyCount: p.ReplyCount, CreatedAt: p.Record.CreatedAt})
	}
	return posts, nil
}

func (b *BlueskyClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.pdsURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.accessJWT)
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bluesky API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "bluesky"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (b *BlueskyClient) doPost(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.pdsURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+b.accessJWT)
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("bluesky API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(respBody), API: "bluesky"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
