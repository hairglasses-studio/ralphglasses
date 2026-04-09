package mcpserver

import (
	"context"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Anthropic key",
			input:    "Here is the key: sk-ant-api03-abcdef1234567890-XYZ",
			expected: "Here is the key: [REDACTED]",
		},
		{
			name:     "OpenAI key",
			input:    "My token is sk-12345678901234567890123456789012",
			expected: "My token is [REDACTED]",
		},
		{
			name:     "Google key",
			input:    "Google key AIzaSyDabcdefghijklmnopqrstuvwx",
			expected: "Google key [REDACTED]",
		},
		{
			name:     "Slack key",
			input:    "Slack token xoxb-123456789012-1234567890123-abcdef1234",
			expected: "Slack token [REDACTED]",
		},
		{
			name:     "No secrets",
			input:    "Just a normal string",
			expected: "Just a normal string",
		},
		{
			name:     "Multiple secrets",
			input:    "Keys: sk-ant-123 and sk-12345678901234567890123456789012",
			expected: "Keys: [REDACTED] and [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactSecrets(tt.input); got != tt.expected {
				t.Errorf("RedactSecrets() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSecretSanitizationMiddleware(t *testing.T) {
	mw := SecretSanitizationMiddleware()

	mockHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Found key: sk-ant-test1234",
				},
			},
		}, fmt.Errorf("error with token sk-12345678901234567890123456789012")
	}

	handler := mw(mockHandler)

	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	expectedErr := "error with token [REDACTED]"
	if err.Error() != expectedErr {
		t.Errorf("Expected err %q, got %q", expectedErr, err.Error())
	}

	if result == nil || len(result.Content) != 1 {
		t.Fatalf("Expected 1 content item, got %v", result)
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("Expected TextContent")
	}

	expectedText := "Found key: [REDACTED]"
	if tc.Text != expectedText {
		t.Errorf("Expected text %q, got %q", expectedText, tc.Text)
	}
}

func TestSecretSanitizationMiddleware_ErrorInResult(t *testing.T) {
	mw := SecretSanitizationMiddleware()

	mockHandler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: "Internal error: AIzaSyDabcdefghijklmnopqrstuvwx",
				},
			},
		}, nil
	}

	handler := mw(mockHandler)

	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)

	if err != nil {
		t.Fatalf("Expected nil error, got %v", err)
	}

	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("Expected TextContent")
	}

	expectedText := "Internal error: [REDACTED]"
	if tc.Text != expectedText {
		t.Errorf("Expected text %q, got %q", expectedText, tc.Text)
	}
}
