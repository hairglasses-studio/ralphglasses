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

const readwiseAPIBaseURL = "https://readwise.io/api/v2"

// ReadwiseClient is a REST client for the Readwise API.
type ReadwiseClient struct {
	httpClient *http.Client
	token      string
}

// ReadwiseBook represents a book or article in Readwise.
type ReadwiseBook struct {
	ID            int    `json:"id"`
	Title         string `json:"title"`
	Author        string `json:"author"`
	Source        string `json:"source"`
	NumHighlights int    `json:"num_highlights"`
	URL           string `json:"source_url"`
}

// ReadwiseHighlight represents a highlight from Readwise.
type ReadwiseHighlight struct {
	ID            int    `json:"id"`
	BookID        int    `json:"book_id"`
	Text          string `json:"text"`
	Note          string `json:"note"`
	Location      int    `json:"location"`
	URL           string `json:"url"`
	HighlightedAt string `json:"highlighted_at"`
}

// NewReadwiseClient creates a new Readwise API client.
func NewReadwiseClient(token string) *ReadwiseClient {
	return &ReadwiseClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// ListBooks returns all books/articles.
func (r *ReadwiseClient) ListBooks(ctx context.Context, limit int) ([]ReadwiseBook, error) {
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Results []ReadwiseBook `json:"results"`
	}
	if err := r.doGet(ctx, fmt.Sprintf("/books/?page_size=%d", limit), &raw); err != nil {
		return nil, err
	}
	return raw.Results, nil
}

// GetBookHighlights returns highlights for a specific book.
func (r *ReadwiseClient) GetBookHighlights(ctx context.Context, bookID int, limit int) ([]ReadwiseHighlight, error) {
	if limit <= 0 {
		limit = 50
	}
	var raw struct {
		Results []ReadwiseHighlight `json:"results"`
	}
	if err := r.doGet(ctx, fmt.Sprintf("/highlights/?book_id=%d&page_size=%d", bookID, limit), &raw); err != nil {
		return nil, err
	}
	return raw.Results, nil
}

// ListHighlights returns recent highlights across all books.
func (r *ReadwiseClient) ListHighlights(ctx context.Context, limit int) ([]ReadwiseHighlight, error) {
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Results []ReadwiseHighlight `json:"results"`
	}
	if err := r.doGet(ctx, fmt.Sprintf("/highlights/?page_size=%d", limit), &raw); err != nil {
		return nil, err
	}
	return raw.Results, nil
}

// SearchHighlights searches highlights by text.
func (r *ReadwiseClient) SearchHighlights(ctx context.Context, query string, limit int) ([]ReadwiseHighlight, error) {
	if limit <= 0 {
		limit = 20
	}
	var raw struct {
		Results []ReadwiseHighlight `json:"results"`
	}
	path := fmt.Sprintf("/highlights/?search=%s&page_size=%d", url.QueryEscape(query), limit)
	if err := r.doGet(ctx, path, &raw); err != nil {
		return nil, err
	}
	return raw.Results, nil
}

// DailyReview returns today's daily review highlights.
func (r *ReadwiseClient) DailyReview(ctx context.Context) ([]ReadwiseHighlight, error) {
	var raw struct {
		Highlights []ReadwiseHighlight `json:"highlights"`
	}
	if err := r.doGet(ctx, "/review/", &raw); err != nil {
		return nil, err
	}
	return raw.Highlights, nil
}

func (r *ReadwiseClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, readwiseAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Token "+r.token)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("readwise API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "readwise"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
