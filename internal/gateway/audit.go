package gateway

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// AuditLogger logs request and response metadata for every proxied call.
type AuditLogger struct {
	Logger *slog.Logger
}

// NewAuditLogger creates an audit logger that writes to the given writer.
func NewAuditLogger(w io.Writer) *AuditLogger {
	h := slog.NewTextHandler(w, &slog.HandlerOptions{
		// Remove time key so tests don't depend on timestamps.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
				return slog.Attr{}
			}
			return a
		},
	})
	return &AuditLogger{Logger: slog.New(h).With("component", "audit")}
}

// Wrap returns middleware that logs each request's method, path, API key
// (masked), status code, and duration.
func (al *AuditLogger) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Capture the request body for logging (limit to 4KB).
		var bodySnippet string
		if r.Body != nil {
			limited := io.LimitReader(r.Body, 4096)
			buf, _ := io.ReadAll(limited)
			bodySnippet = string(buf)
			// Restore the body so downstream handlers can read it.
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), r.Body))
		}

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		al.Logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"key", maskKey(r.Header.Get("X-API-Key")),
			"status", rec.status,
			"duration", time.Since(start).Round(time.Microsecond).String(),
			"body_prefix", truncate(bodySnippet, 128),
		)
	})
}

// statusRecorder captures the HTTP status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// maskKey returns a masked version of an API key for safe logging.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
