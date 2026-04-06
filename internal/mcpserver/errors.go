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
	ErrServiceNotFound     ErrorCode = "SERVICE_NOT_FOUND"
	ErrNoActiveSessions    ErrorCode = "NO_ACTIVE_SESSIONS"
	ErrFleetNotRunning     ErrorCode = "FLEET_NOT_RUNNING"
	ErrNoLogFile           ErrorCode = "NO_LOG_FILE"
	ErrTimeout             ErrorCode = "TIMEOUT"
	ErrRateLimited         ErrorCode = "RATE_LIMITED"
	ErrPermissionDenied    ErrorCode = "PERMISSION_DENIED"
)

// ErrorType classifies errors for structured retry guidance. This addresses
// root cause #3 (20%) of the JSON retry rate: generic error messages that
// are not actionable for retry logic.
type ErrorType string

const (
	ErrorTypeValidation ErrorType = "validation"
	ErrorTypeNetwork    ErrorType = "network"
	ErrorTypeTimeout    ErrorType = "timeout"
	ErrorTypeAuth       ErrorType = "auth"
	ErrorTypeNotFound   ErrorType = "not_found"
	ErrorTypeRateLimit  ErrorType = "rate_limit"
	ErrorTypeBudget     ErrorType = "budget"
	ErrorTypeInternal   ErrorType = "internal"
)

// codedError returns an MCP error result with both a machine-parseable code
// and a human-readable message. The text content is JSON containing error_code
// and error fields for structured parsing, prefixed with [CODE] for quick grep.
func codedError(code ErrorCode, msg string) *mcp.CallToolResult {
	errType, suggestion := classifyError(code)
	data, _ := json.Marshal(map[string]string{
		"error":      fmt.Sprintf("[%s] %s", code, msg),
		"error_code": string(code),
		"error_type": string(errType),
		"what_failed": msg,
		"suggested_fix": suggestion,
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: string(data),
		}},
	}
}

// classifyError maps an ErrorCode to an ErrorType and returns a human-readable
// suggested fix. This replaces generic "error occurred" messages with actionable
// guidance that helps agents self-correct without retrying blindly.
func classifyError(code ErrorCode) (ErrorType, string) {
	switch code {
	case ErrInvalidParams, ErrConfigInvalid:
		return ErrorTypeValidation, "Check parameter names and types against the tool schema. Required fields may be missing or have wrong types."
	case ErrRepoNotFound, ErrSessionNotFound, ErrLoopNotFound, ErrTeamNotFound,
		ErrServiceNotFound, ErrNoActiveSessions, ErrNoLogFile:
		return ErrorTypeNotFound, "The requested resource does not exist. Verify the ID/name is correct and the resource has been created."
	case ErrTimeout:
		return ErrorTypeTimeout, "The operation timed out. Try again with a smaller scope or increase the timeout."
	case ErrRateLimited:
		return ErrorTypeRateLimit, "Rate limit exceeded. Wait a few seconds before retrying."
	case ErrBudgetExceeded:
		return ErrorTypeBudget, "Cost budget exhausted. Check remaining budget with session_status before continuing."
	case ErrPermissionDenied:
		return ErrorTypeAuth, "Permission denied. Verify API keys and access rights are configured."
	case ErrProviderUnavailable:
		return ErrorTypeNetwork, "The LLM provider is unreachable. Check network connectivity or try a different provider."
	case ErrScanFailed, ErrFilesystem:
		return ErrorTypeInternal, "Filesystem operation failed. Check that paths exist and are accessible."
	case ErrSessionNotRunning, ErrNotRunning, ErrFleetNotRunning:
		return ErrorTypeValidation, "The session/fleet is not running. Start it first with the appropriate launch tool."
	case ErrLaunchFailed, ErrLoopStart:
		return ErrorTypeInternal, "Launch failed. Check logs for details — common causes: missing binary, port conflict, or config error."
	case ErrToolExec:
		return ErrorTypeInternal, "Tool execution failed. Check the error message for specifics. This may be a transient failure."
	case ErrRepoNameInvalid:
		return ErrorTypeValidation, "Invalid repo name. Use lowercase alphanumeric with hyphens only (e.g., 'my-repo')."
	case ErrGateFailed:
		return ErrorTypeValidation, "A quality gate failed. Review the gate output and fix the underlying issue before retrying."
	case ErrWorkflow:
		return ErrorTypeInternal, "Workflow error. Check the workflow definition and ensure all steps are correctly configured."
	default:
		return ErrorTypeInternal, "An unexpected error occurred. Check the error message for details."
	}
}
