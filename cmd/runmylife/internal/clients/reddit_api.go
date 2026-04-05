package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const redditAPIBaseURL = "https://oauth.reddit.com"

// RedditClient is a REST client for Reddit's API.
type RedditClient struct {
	httpClient *http.Client
	token      string
}

// RedditPost represents a saved post or feed item.
type RedditPost struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Subreddit string `json:"subreddit"`
	Author    string `json:"author"`
	URL       string `json:"url"`
	Permalink string `json:"permalink"`
	Type      string `json:"type"` // link or comment
	Score     int    `json:"score"`
	Body      string `json:"body,omitempty"`
	CreatedAt string `json:"created_at"`
}

// RedditSubreddit represents a subreddit subscription.
type RedditSubreddit struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Subscribers int    `json:"subscribers"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// NewRedditClient creates a new Reddit API client.
func NewRedditClient(token string) *RedditClient {
	return &RedditClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// SavedPosts returns the user's saved posts and comments.
func (r *RedditClient) SavedPosts(ctx context.Context, limit int) ([]RedditPost, error) {
	if limit <= 0 {
		limit = 25
	}
	var listing redditListing
	if err := r.doGet(ctx, fmt.Sprintf("/user/me/saved?limit=%d", limit), &listing); err != nil {
		return nil, err
	}
	return listing.toPosts(), nil
}

// HomeFeed returns the user's home feed.
func (r *RedditClient) HomeFeed(ctx context.Context, limit int) ([]RedditPost, error) {
	if limit <= 0 {
		limit = 25
	}
	var listing redditListing
	if err := r.doGet(ctx, fmt.Sprintf("/best?limit=%d", limit), &listing); err != nil {
		return nil, err
	}
	return listing.toPosts(), nil
}

// SubredditPosts returns posts from a subreddit.
func (r *RedditClient) SubredditPosts(ctx context.Context, subreddit string, sort string, limit int) ([]RedditPost, error) {
	if sort == "" {
		sort = "hot"
	}
	if limit <= 0 {
		limit = 25
	}
	var listing redditListing
	if err := r.doGet(ctx, fmt.Sprintf("/r/%s/%s?limit=%d", subreddit, sort, limit), &listing); err != nil {
		return nil, err
	}
	return listing.toPosts(), nil
}

// Subscriptions returns the user's subscribed subreddits.
func (r *RedditClient) Subscriptions(ctx context.Context, limit int) ([]RedditSubreddit, error) {
	if limit <= 0 {
		limit = 100
	}
	var raw struct {
		Data struct {
			Children []struct {
				Data struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
					Subscribers int    `json:"subscribers"`
					PublicDesc  string `json:"public_description"`
					URL         string `json:"url"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := r.doGet(ctx, fmt.Sprintf("/subreddits/mine/subscriber?limit=%d", limit), &raw); err != nil {
		return nil, err
	}
	var subs []RedditSubreddit
	for _, child := range raw.Data.Children {
		d := child.Data
		subs = append(subs, RedditSubreddit{ID: d.ID, Name: d.DisplayName, Subscribers: d.Subscribers, Description: d.PublicDesc, URL: "https://reddit.com" + d.URL})
	}
	return subs, nil
}

// Search searches Reddit.
func (r *RedditClient) Search(ctx context.Context, query string, limit int) ([]RedditPost, error) {
	if limit <= 0 {
		limit = 25
	}
	var listing redditListing
	path := fmt.Sprintf("/search?q=%s&limit=%d&sort=relevance", url.QueryEscape(query), limit)
	if err := r.doGet(ctx, path, &listing); err != nil {
		return nil, err
	}
	return listing.toPosts(), nil
}

type redditListing struct {
	Data struct {
		Children []struct {
			Kind string `json:"kind"`
			Data struct {
				ID            string  `json:"id"`
				Title         string  `json:"title"`
				Subreddit     string  `json:"subreddit"`
				Author        string  `json:"author"`
				URL           string  `json:"url"`
				Permalink     string  `json:"permalink"`
				Score         int     `json:"score"`
				Body          string  `json:"body"`
				CreatedUTC    float64 `json:"created_utc"`
			} `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func (l *redditListing) toPosts() []RedditPost {
	var posts []RedditPost
	for _, child := range l.Data.Children {
		d := child.Data
		postType := "link"
		if child.Kind == "t1" {
			postType = "comment"
		}
		createdAt := time.Unix(int64(d.CreatedUTC), 0).Format(time.RFC3339)
		posts = append(posts, RedditPost{
			ID: d.ID, Title: d.Title, Subreddit: d.Subreddit, Author: d.Author,
			URL: d.URL, Permalink: "https://reddit.com" + d.Permalink,
			Type: postType, Score: d.Score, Body: d.Body, CreatedAt: createdAt,
		})
	}
	return posts
}

func (r *RedditClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, redditAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	req.Header.Set("User-Agent", "runmylife/1.0")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("reddit API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "reddit"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
