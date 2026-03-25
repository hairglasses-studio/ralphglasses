package wsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	// DefaultEndpoint is the WebSocket endpoint for the OpenAI Responses API.
	DefaultEndpoint = "wss://api.openai.com/v1/responses"

	// DefaultHTTPEndpoint is the HTTP fallback endpoint.
	DefaultHTTPEndpoint = "https://api.openai.com/v1/responses"

	// DefaultMaxConnAge is the maximum connection age before reconnecting.
	// OpenAI WebSocket connections have a server-side 60-minute limit.
	DefaultMaxConnAge = 60 * time.Minute
)

// Client provides WebSocket transport to OpenAI's Responses API.
// WebSocket connections are 40% faster for 20+ tool call chains.
type Client struct {
	apiKey      string
	endpoint    string // default: wss://api.openai.com/v1/responses
	httpURL     string // HTTP fallback URL
	conn        *websocket.Conn
	mu          sync.Mutex
	connected   bool
	connectedAt time.Time
	maxConnAge  time.Duration // 60 minutes default
	httpClient  *http.Client

	// UseWebSocket controls whether the client prefers WebSocket transport.
	// When false or when WebSocket fails, the client falls back to HTTP.
	UseWebSocket bool
}

// Request represents a WebSocket request to the OpenAI Responses API.
type Request struct {
	Type         string `json:"type"`                   // "response.create"
	Instructions string `json:"instructions,omitempty"`
	Input        string `json:"input"`
	Model        string `json:"model"`
}

// Response represents a WebSocket response from the OpenAI Responses API.
type Response struct {
	ID     string       `json:"id"`
	Type   string       `json:"type"`
	Output []OutputItem `json:"output"`
	Status string       `json:"status"`
}

// OutputItem is a single output item from the Responses API.
type OutputItem struct {
	Type    string          `json:"type"`
	Content []ContentBlock  `json:"content"`
}

// ContentBlock is a content block within a response output.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Option configures a Client.
type Option func(*Client)

// WithEndpoint sets the WebSocket endpoint URL.
func WithEndpoint(endpoint string) Option {
	return func(c *Client) {
		c.endpoint = endpoint
	}
}

// WithHTTPURL sets the HTTP fallback URL.
func WithHTTPURL(url string) Option {
	return func(c *Client) {
		c.httpURL = url
	}
}

// WithMaxConnAge sets the maximum connection age before auto-reconnect.
func WithMaxConnAge(d time.Duration) Option {
	return func(c *Client) {
		c.maxConnAge = d
	}
}

// WithHTTPClient sets the HTTP client used for fallback requests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// WithWebSocket enables or disables WebSocket transport preference.
func WithWebSocket(enabled bool) Option {
	return func(c *Client) {
		c.UseWebSocket = enabled
	}
}

// NewClient creates a new WebSocket client for the OpenAI Responses API.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		apiKey:       apiKey,
		endpoint:     DefaultEndpoint,
		httpURL:      DefaultHTTPEndpoint,
		maxConnAge:   DefaultMaxConnAge,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		UseWebSocket: true,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes a WebSocket connection with authentication headers.
func (c *Client) Connect(ctx context.Context) error {
	if c.apiKey == "" {
		return errors.New("wsclient: API key is required")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	conn, _, err := websocket.Dial(ctx, c.endpoint, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + c.apiKey},
		},
	})
	if err != nil {
		return fmt.Errorf("wsclient: dial %s: %w", c.endpoint, err)
	}

	c.conn = conn
	c.connected = true
	c.connectedAt = time.Now()
	return nil
}

// Send sends a request and receives a response. If the WebSocket connection
// is expired or unavailable, it auto-reconnects. On WebSocket failure, it
// falls back to HTTP.
func (c *Client) Send(ctx context.Context, req *Request) (*Response, error) {
	if c.apiKey == "" {
		return nil, errors.New("wsclient: API key is required")
	}

	if !c.UseWebSocket {
		return c.sendHTTP(ctx, req)
	}

	// Auto-reconnect on expired connections.
	if c.isExpired() {
		_ = c.closeConn()
	}

	if !c.isConnected() {
		if err := c.Connect(ctx); err != nil {
			// Fallback to HTTP on WebSocket connection failure.
			return c.sendHTTP(ctx, req)
		}
	}

	resp, err := c.sendWS(ctx, req)
	if err != nil {
		// Fallback to HTTP on WebSocket send/receive failure.
		_ = c.closeConn()
		return c.sendHTTP(ctx, req)
	}
	return resp, nil
}

// Close closes the WebSocket connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return nil
	}

	err := c.conn.Close(websocket.StatusNormalClosure, "client closing")
	c.connected = false
	c.conn = nil
	return err
}

// isExpired checks if the connection has exceeded the max connection age.
func (c *Client) isExpired() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return false
	}
	return time.Since(c.connectedAt) >= c.maxConnAge
}

// isConnected returns whether the client has an active WebSocket connection.
func (c *Client) isConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// closeConn closes the connection without the outer lock.
func (c *Client) closeConn() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return nil
	}
	err := c.conn.Close(websocket.StatusNormalClosure, "reconnecting")
	c.connected = false
	c.conn = nil
	return err
}

// sendWS sends a request and reads a response over the WebSocket connection.
func (c *Client) sendWS(ctx context.Context, req *Request) (*Response, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return nil, errors.New("wsclient: not connected")
	}

	if err := wsjson.Write(ctx, conn, req); err != nil {
		return nil, fmt.Errorf("wsclient: write: %w", err)
	}

	var resp Response
	if err := wsjson.Read(ctx, conn, &resp); err != nil {
		return nil, fmt.Errorf("wsclient: read: %w", err)
	}

	return &resp, nil
}

// httpRequest is the HTTP fallback request body (matches OpenAI Responses API).
type httpRequest struct {
	Model        string `json:"model"`
	Instructions string `json:"instructions,omitempty"`
	Input        string `json:"input"`
}

// sendHTTP falls back to the standard HTTP Responses API.
func (c *Client) sendHTTP(ctx context.Context, req *Request) (*Response, error) {
	body := httpRequest{
		Model:        req.Model,
		Instructions: req.Instructions,
		Input:        req.Input,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("wsclient: marshal http request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.httpURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("wsclient: create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("wsclient: http request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("wsclient: read http response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wsclient: http error (status %d): %s", httpResp.StatusCode, string(respBody))
	}

	var resp Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("wsclient: unmarshal http response: %w", err)
	}

	return &resp, nil
}
