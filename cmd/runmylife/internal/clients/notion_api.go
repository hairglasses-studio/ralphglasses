package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	notionAPIBaseURL = "https://api.notion.com/v1"
	notionVersion    = "2022-06-28"
)

// NotionClient is a thin REST client for the Notion API.
type NotionClient struct {
	httpClient *http.Client
	token      string
}

// NotionDatabase represents a Notion database.
type NotionDatabase struct {
	ID    string `json:"id"`
	Title []struct {
		PlainText string `json:"plain_text"`
	} `json:"title"`
	Description []struct {
		PlainText string `json:"plain_text"`
	} `json:"description"`
	URL string `json:"url"`
}

// NotionPage represents a Notion page.
type NotionPage struct {
	ID             string                 `json:"id"`
	URL            string                 `json:"url"`
	CreatedTime    string                 `json:"created_time"`
	LastEditedTime string                 `json:"last_edited_time"`
	Properties     map[string]interface{} `json:"properties"`
	Parent         map[string]interface{} `json:"parent"`
}

// NewNotionClient creates a new Notion REST API client.
func NewNotionClient(token string) *NotionClient {
	return &NotionClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// ListDatabases searches for all databases accessible to the integration.
func (n *NotionClient) ListDatabases(ctx context.Context) ([]NotionDatabase, error) {
	body := map[string]interface{}{
		"filter": map[string]string{"value": "database", "property": "object"},
	}
	var result struct {
		Results []NotionDatabase `json:"results"`
	}
	if err := n.doPost(ctx, "/search", body, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

// QueryDatabase queries a database with an optional filter.
func (n *NotionClient) QueryDatabase(ctx context.Context, databaseID string, filterJSON string) ([]NotionPage, error) {
	var body map[string]interface{}
	if filterJSON != "" {
		if err := json.Unmarshal([]byte(filterJSON), &body); err != nil {
			body = map[string]interface{}{}
		}
	} else {
		body = map[string]interface{}{}
	}

	var result struct {
		Results []NotionPage `json:"results"`
	}
	if err := n.doPost(ctx, fmt.Sprintf("/databases/%s/query", databaseID), body, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

// GetPage retrieves a single page by ID.
func (n *NotionClient) GetPage(ctx context.Context, pageID string) (*NotionPage, error) {
	var page NotionPage
	if err := n.doGet(ctx, fmt.Sprintf("/pages/%s", pageID), &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// CreatePage creates a new page in the specified database.
func (n *NotionClient) CreatePage(ctx context.Context, parentDBID string, properties map[string]interface{}) (*NotionPage, error) {
	body := map[string]interface{}{
		"parent":     map[string]string{"database_id": parentDBID},
		"properties": properties,
	}
	var page NotionPage
	if err := n.doPost(ctx, "/pages", body, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// UpdatePage updates properties on an existing page.
func (n *NotionClient) UpdatePage(ctx context.Context, pageID string, properties map[string]interface{}) (*NotionPage, error) {
	body := map[string]interface{}{
		"properties": properties,
	}
	var page NotionPage
	if err := n.doPatch(ctx, fmt.Sprintf("/pages/%s", pageID), body, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// Search searches for pages matching the query.
func (n *NotionClient) Search(ctx context.Context, query string) ([]NotionPage, error) {
	body := map[string]interface{}{
		"query":  query,
		"filter": map[string]string{"value": "page", "property": "object"},
	}
	var result struct {
		Results []NotionPage `json:"results"`
	}
	if err := n.doPost(ctx, "/search", body, &result); err != nil {
		return nil, err
	}
	return result.Results, nil
}

func (n *NotionClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, notionAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	n.setHeaders(req)
	return n.doRequest(req, out)
}

func (n *NotionClient) doPost(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, notionAPIBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	n.setHeaders(req)
	return n.doRequest(req, out)
}

func (n *NotionClient) doPatch(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, notionAPIBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	n.setHeaders(req)
	return n.doRequest(req, out)
}

func (n *NotionClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+n.token)
	req.Header.Set("Notion-Version", notionVersion)
}

func (n *NotionClient) doRequest(req *http.Request, out interface{}) error {
	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notion API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "notion"}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
