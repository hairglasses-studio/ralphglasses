package session

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newTestLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func TestJSONAuditLogger_LogSuccess(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(newTestLogger(&buf))

	logger.LogSuccess("sess-1", `{"type":"result"}`, 17, 5*time.Millisecond)

	output := buf.String()
	if !strings.Contains(output, "json parse succeeded") {
		t.Errorf("expected 'json parse succeeded' in output, got: %s", output)
	}
	if !strings.Contains(output, "sess-1") {
		t.Errorf("expected session ID in output, got: %s", output)
	}
	if !strings.Contains(output, "DEBUG") {
		t.Errorf("expected DEBUG level in output, got: %s", output)
	}
}

func TestJSONAuditLogger_LogRecoverableFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(newTestLogger(&buf))

	logger.LogRecoverableFailure("sess-2", "not json", 8, 2*time.Millisecond, errors.New("invalid character"))

	output := buf.String()
	if !strings.Contains(output, "recoverable") {
		t.Errorf("expected 'recoverable' in output, got: %s", output)
	}
	if !strings.Contains(output, "WARN") {
		t.Errorf("expected WARN level in output, got: %s", output)
	}
	if !strings.Contains(output, "invalid character") {
		t.Errorf("expected error message in output, got: %s", output)
	}
}

func TestJSONAuditLogger_LogUnrecoverableFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(newTestLogger(&buf))

	logger.LogUnrecoverableFailure("sess-3", "{bad", 4, 1*time.Millisecond, errors.New("unexpected EOF"))

	output := buf.String()
	if !strings.Contains(output, "unrecoverable") {
		t.Errorf("expected 'unrecoverable' in output, got: %s", output)
	}
	if !strings.Contains(output, "ERROR") {
		t.Errorf("expected ERROR level in output, got: %s", output)
	}
}

func TestJSONAuditLogger_InputPreviewTruncation(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(newTestLogger(&buf))

	// Input longer than auditInputPreviewLen (200 chars).
	longInput := strings.Repeat("x", 500)
	logger.LogSuccess("sess-4", longInput, 500, time.Millisecond)

	output := buf.String()
	// The preview should be truncated, so the full 500-char string should not appear.
	if strings.Contains(output, longInput) {
		t.Error("expected input to be truncated in log output")
	}
	if !strings.Contains(output, "...") {
		t.Error("expected '...' truncation indicator in log output")
	}
}

func TestJSONAuditLogger_NilLogger(t *testing.T) {
	// Should not panic with nil logger.
	logger := NewJSONAuditLogger(nil)
	logger.LogSuccess("sess-5", "{}", 2, time.Millisecond)
}

func TestJSONAuditLogger_ParseAttemptDirect(t *testing.T) {
	var buf bytes.Buffer
	logger := NewJSONAuditLogger(newTestLogger(&buf))

	logger.LogParseAttempt(ParseAttempt{
		SessionID:   "sess-6",
		InputSize:   42,
		Input:       `{"partial": true`,
		Duration:    3 * time.Millisecond,
		Success:     false,
		Recoverable: true,
		Error:       "unexpected end of JSON",
	})

	output := buf.String()
	if !strings.Contains(output, "sess-6") {
		t.Errorf("expected session ID, got: %s", output)
	}
	if !strings.Contains(output, "WARN") {
		t.Errorf("expected WARN level for recoverable failure, got: %s", output)
	}
}
