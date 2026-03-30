package gateway

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"
)

// AuditLogger logs request and response metadata for every proxied call.
type AuditLogger struct {
	Logger *log.Logger
}

// NewAuditLogger creates an audit logger that writes to the given writer.
func NewAuditLogger(w io.Writer) *AuditLogger {
	return &AuditLogger{Logger: log.New(w, "[audit] ", log.LstdFlags)}
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

		key := maskKey(r.Header.Get("X-API-Key"))
		al.Logger.Printf("method=%s path=%s key=%s status=%d duration=%s body_prefix=%q",
			r.Method, r.URL.Path, key, rec.status, time.Since(start).Round(time.Microsecond), truncate(bodySnippet, 128))
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
