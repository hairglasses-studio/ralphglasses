package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleAwesomeReportNoData(t *testing.T) {
	t.Parallel()

	// Use a temp dir with no analysis data on disk.
	root := t.TempDir()
	srv := &Server{ScanPath: root}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"save_to": root,
	}

	result, err := srv.handleAwesomeReport(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful result with no_data status, got tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v\ntext: %s", err, text)
	}

	if status, _ := body["status"].(string); status != "no_data" {
		t.Errorf("expected status=no_data, got %q", status)
	}
	if msg, _ := body["message"].(string); msg == "" {
		t.Error("expected non-empty message")
	}
}

func TestHandleAwesomeDiffNoData(t *testing.T) {
	t.Parallel()

	// Serve a minimal awesome-list README so Fetch succeeds without network.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Awesome List\n\n## Tools\n\n- [example](https://github.com/org/repo) - A tool\n"))
	}))
	defer ts.Close()

	root := t.TempDir()
	srv := &Server{
		ScanPath:   root,
		HTTPClient: ts.Client(),
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"save_to": root,
		// Use a repo string that will hit our test server.
		// Since Fetch builds a raw.githubusercontent.com URL, and our httptest
		// client's transport rewrites all requests to the test server,
		// we need to override the transport instead.
		"repo": "test/repo",
	}

	// Override the HTTP client transport to redirect all requests to the test server.
	srv.HTTPClient = &http.Client{
		Transport: &testTransport{handler: ts},
	}

	result, err := srv.handleAwesomeDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected successful result with no_data status, got tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v\ntext: %s", err, text)
	}

	if status, _ := body["status"].(string); status != "no_data" {
		t.Errorf("expected status=no_data, got %q", status)
	}
	if msg, _ := body["message"].(string); msg == "" {
		t.Error("expected non-empty message")
	}
}

// testTransport redirects all HTTP requests to the given test server.
type testTransport struct {
	handler *httptest.Server
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point at the test server.
	req.URL.Scheme = "http"
	req.URL.Host = t.handler.Listener.Addr().String()
	return http.DefaultTransport.RoundTrip(req)
}

func TestHandleAwesomeReportMissingSaveTo(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleAwesomeReport(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for missing save_to")
	}
}

func TestHandleAwesomeDiffMissingSaveTo(t *testing.T) {
	t.Parallel()

	srv := &Server{}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleAwesomeDiff(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected tool error for missing save_to")
	}
}
