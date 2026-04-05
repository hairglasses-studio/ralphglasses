package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/mark3labs/mcp-go/mcp"
)

// Structured error codes for LLM-parseable error classification.
const (
	ErrClientInit  = "CLIENT_INIT_FAILED"
	ErrInvalidParam = "INVALID_PARAM"
	ErrTimeout      = "TIMEOUT"
	ErrNotFound     = "NOT_FOUND"
	ErrAPIError     = "API_ERROR"
	ErrDBError      = "DB_ERROR"
	ErrRateLimit    = "RATE_LIMITED"
	ErrAuth         = "AUTH_FAILED"
	ErrConfig       = "CONFIG_ERROR"
)

// CodedErrorResult returns an MCP error result with a structured error code prefix.
func CodedErrorResult(code string, err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf("[%s] %s", code, err.Error()))
}

// CodedErrorResultf returns an MCP error result with a formatted message and error code.
func CodedErrorResultf(code string, format string, args ...any) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf("[%s] %s", code, fmt.Sprintf(format, args...)))
}

// ActionableErrorResult returns an error result with code, message, and recovery suggestions.
func ActionableErrorResult(code string, err error, suggestions ...string) *mcp.CallToolResult {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[%s] %s", code, err.Error()))
	if len(suggestions) > 0 {
		sb.WriteString("\n\nSuggestions:")
		for _, s := range suggestions {
			sb.WriteString(fmt.Sprintf("\n- %s", s))
		}
	}
	return mcp.NewToolResultError(sb.String())
}

// ClassifyClientError maps a client error to a structured error code.
func ClassifyClientError(err error) string {
	var httpErr *clients.HTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.IsRateLimit():
			return ErrRateLimit
		case httpErr.IsAuth():
			return ErrAuth
		default:
			return ErrAPIError
		}
	}
	return ErrAPIError
}
