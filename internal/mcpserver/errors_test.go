package mcpserver

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestCodedError_IsError(t *testing.T) {
	t.Parallel()
	r := codedError(ErrInternal, "boom")
	if !r.IsError {
		t.Fatal("codedError result must have IsError=true")
	}
}

func TestCodedError_JSONStructure(t *testing.T) {
	t.Parallel()
	r := codedError(ErrRepoNotFound, "repo xyz not found")

	text := getResultText(r)
	if text == "" {
		t.Fatal("expected non-empty text content")
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("error content is not valid JSON: %v", err)
	}

	if code, ok := payload["error_code"]; !ok {
		t.Error("JSON payload missing 'error_code' key")
	} else if code != string(ErrRepoNotFound) {
		t.Errorf("error_code = %q, want %q", code, ErrRepoNotFound)
	}

	if errMsg, ok := payload["error"]; !ok {
		t.Error("JSON payload missing 'error' key")
	} else {
		if got := errMsg; got == "" {
			t.Error("error message should not be empty")
		}
	}
}

func TestCodedError_ContainsCode(t *testing.T) {
	t.Parallel()
	r := codedError(ErrBudgetExceeded, "over budget")
	text := getResultText(r)

	var payload map[string]string
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}

	// The "error" field should contain the code in brackets.
	if errMsg := payload["error"]; errMsg == "" {
		t.Fatal("error field empty")
	} else if !contains(errMsg, "[BUDGET_EXCEEDED]") {
		t.Errorf("error field %q does not contain [BUDGET_EXCEEDED]", errMsg)
	}
}

func TestCodedError_SingleTextContent(t *testing.T) {
	t.Parallel()
	r := codedError(ErrInvalidParams, "bad input")

	if len(r.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(r.Content))
	}
	tc, ok := r.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want mcp.TextContent", r.Content[0])
	}
	if tc.Type != "text" {
		t.Errorf("content Type = %q, want %q", tc.Type, "text")
	}
}

func TestErrorCode_Constants_NonEmpty(t *testing.T) {
	t.Parallel()
	codes := []ErrorCode{
		ErrRepoNotFound, ErrRepoNameInvalid, ErrSessionNotFound,
		ErrSessionNotRunning, ErrLoopNotFound, ErrBudgetExceeded,
		ErrProviderUnavailable, ErrInvalidParams, ErrInternal,
		ErrScanFailed, ErrLoopStart, ErrLaunchFailed, ErrToolExec,
		ErrTeamNotFound, ErrNotRunning, ErrFilesystem, ErrConfigInvalid,
		ErrWorkflow, ErrGateFailed, ErrServiceNotFound,
	}

	seen := make(map[ErrorCode]bool)
	for _, code := range codes {
		if code == "" {
			t.Error("found empty ErrorCode constant")
		}
		if seen[code] {
			t.Errorf("duplicate ErrorCode constant: %q", code)
		}
		seen[code] = true
	}
}

func TestCodedError_DifferentCodes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code ErrorCode
		msg  string
	}{
		{ErrRepoNotFound, "repo missing"},
		{ErrSessionNotFound, "session gone"},
		{ErrProviderUnavailable, "provider down"},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			r := codedError(tt.code, tt.msg)
			if !r.IsError {
				t.Fatal("expected IsError=true")
			}
			text := getResultText(r)
			var payload map[string]string
			if err := json.Unmarshal([]byte(text), &payload); err != nil {
				t.Fatalf("bad JSON: %v", err)
			}
			if payload["error_code"] != string(tt.code) {
				t.Errorf("error_code = %q, want %q", payload["error_code"], tt.code)
			}
		})
	}
}
