package mcpserver

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ErrorCode is a machine-parseable error code for MCP tool responses.
type ErrorCode string

const (
	ErrRepoNotFound        ErrorCode = "REPO_NOT_FOUND"
	ErrRepoNameInvalid     ErrorCode = "REPO_NAME_INVALID"
	ErrSessionNotFound     ErrorCode = "SESSION_NOT_FOUND"
	ErrSessionNotRunning   ErrorCode = "SESSION_NOT_RUNNING"
	ErrLoopNotFound        ErrorCode = "LOOP_NOT_FOUND"
	ErrBudgetExceeded      ErrorCode = "BUDGET_EXCEEDED"
	ErrProviderUnavailable ErrorCode = "PROVIDER_UNAVAILABLE"
	ErrInvalidParams       ErrorCode = "INVALID_PARAMS"
	ErrInternal            ErrorCode = "INTERNAL_ERROR"
	ErrScanFailed          ErrorCode = "SCAN_FAILED"
	ErrLoopStart           ErrorCode = "LOOP_START_FAILED"
	ErrLaunchFailed        ErrorCode = "LAUNCH_FAILED"
	ErrToolExec            ErrorCode = "TOOL_EXEC_FAILED"
	ErrTeamNotFound        ErrorCode = "TEAM_NOT_FOUND"
	ErrNotRunning          ErrorCode = "NOT_RUNNING"
	ErrFilesystem          ErrorCode = "FILESYSTEM_ERROR"
	ErrConfigInvalid       ErrorCode = "CONFIG_INVALID"
	ErrWorkflow            ErrorCode = "WORKFLOW_ERROR"
	ErrGateFailed          ErrorCode = "GATE_FAILED"
)

// codedError returns an MCP error result with both a machine-parseable code
// and a human-readable message. The text content is JSON containing error_code
// and error fields for structured parsing, prefixed with [CODE] for quick grep.
func codedError(code ErrorCode, msg string) *mcp.CallToolResult {
	data, _ := json.Marshal(map[string]string{
		"error":      fmt.Sprintf("[%s] %s", code, msg),
		"error_code": string(code),
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: string(data),
		}},
	}
}
