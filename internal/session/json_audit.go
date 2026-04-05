package session

import (
	"log/slog"
	"time"
)

const (
	// auditInputPreviewLen is the maximum number of characters of the raw
	// input to include in audit log entries for debugging.
	auditInputPreviewLen = 200
)

// JSONAuditLogger logs every JSON parse attempt with structured metadata.
// It uses slog for output, emitting DEBUG for successes, WARN for recoverable
// failures, and ERROR for unrecoverable failures.
type JSONAuditLogger struct {
	logger *slog.Logger
}

// NewJSONAuditLogger creates a JSONAuditLogger backed by the given slog.Logger.
// If logger is nil, slog.Default() is used.
func NewJSONAuditLogger(logger *slog.Logger) *JSONAuditLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &JSONAuditLogger{logger: logger}
}

// ParseAttempt represents a single JSON parse attempt for audit logging.
type ParseAttempt struct {
	// SessionID is the session that produced the output.
	SessionID string
	// InputSize is the size in bytes of the raw input.
	InputSize int
	// Input is the raw input string (will be truncated to auditInputPreviewLen).
	Input string
	// Duration is how long the parse took.
	Duration time.Duration
	// Success indicates whether the parse succeeded.
	Success bool
	// Recoverable indicates whether a failure is recoverable (e.g., fallback to text).
	Recoverable bool
	// Error is the error message if the parse failed.
	Error string
}

// LogParseAttempt records a JSON parse attempt.
//   - Success: logged at DEBUG level.
//   - Recoverable failure: logged at WARN level.
//   - Unrecoverable failure: logged at ERROR level.
func (a *JSONAuditLogger) LogParseAttempt(attempt ParseAttempt) {
	preview := attempt.Input
	if len(preview) > auditInputPreviewLen {
		preview = preview[:auditInputPreviewLen] + "..."
	}

	attrs := []slog.Attr{
		slog.String("session_id", attempt.SessionID),
		slog.Int("input_size", attempt.InputSize),
		slog.String("input_preview", preview),
		slog.Duration("duration", attempt.Duration),
		slog.Bool("success", attempt.Success),
	}

	if attempt.Success {
		a.logger.LogAttrs(nil, slog.LevelDebug, "json parse succeeded", attrs...)
		return
	}

	attrs = append(attrs, slog.String("error", attempt.Error))
	attrs = append(attrs, slog.Bool("recoverable", attempt.Recoverable))

	if attempt.Recoverable {
		a.logger.LogAttrs(nil, slog.LevelWarn, "json parse failed (recoverable)", attrs...)
		return
	}

	a.logger.LogAttrs(nil, slog.LevelError, "json parse failed (unrecoverable)", attrs...)
}

// LogSuccess is a convenience method for logging a successful parse.
func (a *JSONAuditLogger) LogSuccess(sessionID, input string, inputSize int, duration time.Duration) {
	a.LogParseAttempt(ParseAttempt{
		SessionID: sessionID,
		InputSize: inputSize,
		Input:     input,
		Duration:  duration,
		Success:   true,
	})
}

// LogRecoverableFailure is a convenience method for logging a recoverable parse failure.
func (a *JSONAuditLogger) LogRecoverableFailure(sessionID, input string, inputSize int, duration time.Duration, err error) {
	a.LogParseAttempt(ParseAttempt{
		SessionID:   sessionID,
		InputSize:   inputSize,
		Input:       input,
		Duration:    duration,
		Success:     false,
		Recoverable: true,
		Error:       err.Error(),
	})
}

// LogUnrecoverableFailure is a convenience method for logging an unrecoverable parse failure.
func (a *JSONAuditLogger) LogUnrecoverableFailure(sessionID, input string, inputSize int, duration time.Duration, err error) {
	a.LogParseAttempt(ParseAttempt{
		SessionID:   sessionID,
		InputSize:   inputSize,
		Input:       input,
		Duration:    duration,
		Success:     false,
		Recoverable: false,
		Error:       err.Error(),
	})
}
