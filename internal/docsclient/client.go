// Package docsclient provides a lightweight interface to docs-mcp tools.
// It spawns the docs-mcp binary as a subprocess and communicates via the
// MCP stdio JSON-RPC protocol, sending tool call requests over stdin and
// reading responses from stdout.
package docsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Client provides a lightweight interface to docs-mcp tools.
// It shells out to the docs-mcp binary via MCP JSON-RPC over stdio
// rather than maintaining a persistent connection.
type Client struct {
	binaryPath string
	docsRoot   string
	timeout    time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithTimeout sets the command execution timeout. Default is 30s.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// New creates a docs-mcp client. binaryPath defaults to "docs-mcp"
// if empty. docsRoot defaults to ~/hairglasses-studio/docs.
func New(binaryPath, docsRoot string, opts ...Option) *Client {
	if binaryPath == "" {
		binaryPath = "docs-mcp"
	}
	if docsRoot == "" {
		home, _ := os.UserHomeDir()
		docsRoot = filepath.Join(home, "hairglasses-studio", "docs")
	}
	c := &Client{
		binaryPath: binaryPath,
		docsRoot:   docsRoot,
		timeout:    30 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// BinaryPath returns the configured binary path.
func (c *Client) BinaryPath() string { return c.binaryPath }

// DocsRoot returns the configured docs root directory.
func (c *Client) DocsRoot() string { return c.docsRoot }

// KnowledgeResult holds the response from a knowledge gate query.
type KnowledgeResult struct {
	Topic          string   `json:"topic"`
	Confidence     float64  `json:"confidence"`
	Recommendation string   `json:"recommendation"`
	ExistingDocs   int      `json:"existing_docs"`
	Sources        []string `json:"sources,omitempty"`
}

// KnowledgeCheck queries docs-mcp for existing research on a topic.
// It calls the docs_knowledge_gate tool via MCP JSON-RPC.
func (c *Client) KnowledgeCheck(ctx context.Context, topic string) (*KnowledgeResult, error) {
	if topic == "" {
		return nil, fmt.Errorf("topic must not be empty")
	}

	args := map[string]any{
		"query": topic,
	}

	raw, err := c.callTool(ctx, "docs_knowledge_gate", args)
	if err != nil {
		return nil, fmt.Errorf("knowledge gate call: %w", err)
	}

	var result KnowledgeResult
	result.Topic = topic

	// The response is a JSON object with various fields from the gateway.
	var gw map[string]any
	if err := json.Unmarshal(raw, &gw); err != nil {
		return nil, fmt.Errorf("parse knowledge gate response: %w", err)
	}

	if v, ok := gw["confidence"].(float64); ok {
		result.Confidence = v
	}
	if v, ok := gw["recommendation"].(string); ok {
		result.Recommendation = v
	}
	if v, ok := gw["existing_docs"].(float64); ok {
		result.ExistingDocs = int(v)
	}
	// Collect source file paths from docs_results if present.
	if docs, ok := gw["docs_results"].([]any); ok {
		for _, d := range docs {
			if m, ok := d.(map[string]any); ok {
				if p, ok := m["path"].(string); ok {
					result.Sources = append(result.Sources, p)
				}
			}
		}
	}

	return &result, nil
}

// DedupCheck queries docs-mcp to check if a topic has already been researched.
// Returns a KnowledgeResult with confidence indicating overlap likelihood.
func (c *Client) DedupCheck(ctx context.Context, topic string) (*KnowledgeResult, error) {
	if topic == "" {
		return nil, fmt.Errorf("topic must not be empty")
	}

	args := map[string]any{
		"topic": topic,
	}

	raw, err := c.callTool(ctx, "docs_dedup_check", args)
	if err != nil {
		return nil, fmt.Errorf("dedup check call: %w", err)
	}

	var result KnowledgeResult
	result.Topic = topic

	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse dedup response: %w", err)
	}

	if v, ok := resp["confidence"].(float64); ok {
		result.Confidence = v
	}
	if v, ok := resp["recommendation"].(string); ok {
		result.Recommendation = v
	}
	if v, ok := resp["existing_docs"].(float64); ok {
		result.ExistingDocs = int(v)
	}

	return &result, nil
}

// PersistRequest describes research findings to persist to the docs repo.
type PersistRequest struct {
	Title   string   `json:"title"`
	Domain  string   `json:"domain"`
	Content string   `json:"content"`
	Sources []string `json:"sources,omitempty"`
}

// Validate checks that required fields are set and the domain is valid.
func (r PersistRequest) Validate() error {
	if r.Title == "" {
		return fmt.Errorf("title is required")
	}
	if r.Domain == "" {
		return fmt.Errorf("domain is required")
	}
	if r.Content == "" {
		return fmt.Errorf("content is required")
	}
	validDomains := map[string]bool{
		"mcp": true, "agents": true, "orchestration": true,
		"cost-optimization": true, "go-ecosystem": true,
		"terminal": true, "competitive": true,
	}
	if !validDomains[r.Domain] {
		return fmt.Errorf("invalid domain %q; valid: %s", r.Domain, strings.Join(validDomainList(), ", "))
	}
	return nil
}

func validDomainList() []string {
	return []string{"mcp", "agents", "orchestration", "cost-optimization", "go-ecosystem", "terminal", "competitive"}
}

// PersistFindings writes research findings to the docs repo via the
// persist_research MCP tool.
func (c *Client) PersistFindings(ctx context.Context, req PersistRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("invalid persist request: %w", err)
	}

	args := map[string]any{
		"title":   req.Title,
		"domain":  req.Domain,
		"content": req.Content,
	}
	if len(req.Sources) > 0 {
		args["sources"] = req.Sources
	}

	_, err := c.callTool(ctx, "persist_research", args)
	if err != nil {
		return fmt.Errorf("persist_research call: %w", err)
	}
	return nil
}

// callTool sends a JSON-RPC tools/call request to the docs-mcp binary
// via stdio and returns the result content.
func (c *Client) callTool(ctx context.Context, toolName string, arguments map[string]any) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Build the MCP JSON-RPC request.
	// MCP uses JSON-RPC 2.0 with method "tools/call".
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	// We also need to send an initialize request first, then the tool call.
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "ralphglasses-docsclient",
				"version": "0.1.0",
			},
		},
	}

	initBytes, err := json.Marshal(initReq)
	if err != nil {
		return nil, fmt.Errorf("marshal init request: %w", err)
	}

	reqBytes, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal tool request: %w", err)
	}

	// Combine: init request + newline + tool call request + newline
	var stdin bytes.Buffer
	stdin.Write(initBytes)
	stdin.WriteByte('\n')
	stdin.Write(reqBytes)
	stdin.WriteByte('\n')

	cmd := exec.CommandContext(ctx, c.binaryPath)
	cmd.Stdin = &stdin
	cmd.Env = append(os.Environ(),
		"DOCS_ROOT="+c.docsRoot,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// The MCP server may exit after processing (no persistent connection),
		// which can produce an exit error even on success. Check stdout first.
		if stdout.Len() == 0 {
			return nil, fmt.Errorf("docs-mcp exec: %w (stderr: %s)", err, stderr.String())
		}
	}

	// Parse the response lines. The last JSON-RPC response with our tool call
	// ID (1) contains the result.
	return parseToolResponse(stdout.Bytes())
}

// parseToolResponse extracts the tool result from MCP JSON-RPC response lines.
func parseToolResponse(data []byte) (json.RawMessage, error) {
	lines := bytes.Split(data, []byte("\n"))

	for i := len(lines) - 1; i >= 0; i-- {
		line := bytes.TrimSpace(lines[i])
		if len(line) == 0 {
			continue
		}

		var resp struct {
			ID     any             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		// Match on id=1 (our tool call).
		idFloat, ok := resp.ID.(float64)
		if !ok || idFloat != 1 {
			continue
		}

		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		if resp.Result == nil {
			return nil, fmt.Errorf("empty result in response")
		}

		// MCP tool results have a "content" array; extract the first text content.
		var toolResult struct {
			Content []struct {
				Type string          `json:"type"`
				Text json.RawMessage `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		}
		if err := json.Unmarshal(resp.Result, &toolResult); err != nil {
			// If it doesn't match the content structure, return the raw result.
			return resp.Result, nil
		}

		if toolResult.IsError {
			if len(toolResult.Content) > 0 {
				return nil, fmt.Errorf("tool error: %s", string(toolResult.Content[0].Text))
			}
			return nil, fmt.Errorf("tool returned error with no content")
		}

		if len(toolResult.Content) == 0 {
			return nil, fmt.Errorf("tool returned no content")
		}

		// The text field may be a JSON string (quoted) or raw JSON object.
		text := toolResult.Content[0].Text
		if len(text) > 0 && text[0] == '"' {
			var s string
			if err := json.Unmarshal(text, &s); err == nil {
				return json.RawMessage(s), nil
			}
		}
		return text, nil
	}

	return nil, fmt.Errorf("no valid tool response found in output (%d bytes)", len(data))
}
