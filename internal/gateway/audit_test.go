package gateway

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAuditLogger(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)
	if al == nil {
		t.Fatal("NewAuditLogger returned nil")
	}
	if al.Logger == nil {
		t.Fatal("Logger field is nil")
	}
}

func TestAuditLogger_Wrap_LogsMethodPathStatusDuration(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := al.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	for _, want := range []string{"method=GET", "path=/test-path", "status=200", "duration="} {
		if !strings.Contains(logLine, want) {
			t.Errorf("log missing %q: %s", want, logLine)
		}
	}
}

func TestAuditLogger_Wrap_MasksAPIKey(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := al.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "sk-abcdefghijklmnop")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if strings.Contains(logLine, "abcdefghijklmnop") {
		t.Errorf("log contains unmasked key: %s", logLine)
	}
	if !strings.Contains(logLine, "sk-a...mnop") {
		t.Errorf("log missing masked key: %s", logLine)
	}
}

func TestAuditLogger_Wrap_NoAPIKey(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := al.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "key=***") {
		t.Errorf("expected masked empty key, got: %s", logLine)
	}
}

func TestAuditLogger_Wrap_CapturesBodySnippet(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := al.Wrap(inner)

	body := `{"action":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "body_prefix=") {
		t.Errorf("log missing body_prefix: %s", logLine)
	}
	if !strings.Contains(logLine, "test") {
		t.Errorf("log missing body content: %s", logLine)
	}
}

func TestAuditLogger_Wrap_BodyRestored(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	var innerBody string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := readAll(r.Body)
		innerBody = string(b)
		w.WriteHeader(http.StatusOK)
	})
	handler := al.Wrap(inner)

	body := `{"key":"value"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if innerBody != body {
		t.Errorf("inner handler got body %q, want %q", innerBody, body)
	}
}

func TestAuditLogger_Wrap_NilBody(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := al.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/no-body", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if buf.Len() == 0 {
		t.Fatal("expected log output for nil body request")
	}
}

func TestAuditLogger_Wrap_StatusCodes(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"ok", http.StatusOK},
		{"created", http.StatusCreated},
		{"bad_request", http.StatusBadRequest},
		{"not_found", http.StatusNotFound},
		{"internal_error", http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			al := NewAuditLogger(&buf)

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			})
			handler := al.Wrap(inner)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			want := fmt.Sprintf("status=%d", tt.status)
			if !strings.Contains(buf.String(), want) {
				t.Errorf("expected %s in log, got: %s", want, buf.String())
			}
		})
	}
}

func TestStatusRecorder_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	sr.WriteHeader(http.StatusNotFound)
	if sr.status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", sr.status, http.StatusNotFound)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("underlying recorder code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestStatusRecorder_DefaultStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	// If WriteHeader is never called, status should remain 200.
	if sr.status != http.StatusOK {
		t.Errorf("default status = %d, want %d", sr.status, http.StatusOK)
	}
}

func TestMaskKey_TableDriven(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "***"},
		{"a", "***"},
		{"abcdefgh", "***"},
		{"abcdefghi", "abcd...fghi"},
		{"sk-prod-1234567890abcdef", "sk-p...cdef"},
		{"123456789", "1234...6789"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := maskKey(tt.input)
			if got != tt.want {
				t.Errorf("maskKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncate_TableDriven(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"empty", "", 10, ""},
		{"shorter_than_limit", "hello", 10, "hello"},
		{"exact_limit", "hello", 5, "hello"},
		{"exceeds_limit", "hello world", 5, "hello..."},
		{"zero_limit", "hello", 0, "..."},
		{"one_char_limit", "hello", 1, "h..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

// readAll is a small test helper wrapping io.ReadAll to avoid import in test scope.
func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	return buf.Bytes(), err
}
