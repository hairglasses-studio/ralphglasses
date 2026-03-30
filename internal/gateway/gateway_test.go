package gateway

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---------- routing tests ----------

func TestGateway_LocalRoute(t *testing.T) {
	gw := NewGateway()
	gw.Handle("/healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected ok, got %q", rec.Body.String())
	}
}

func TestGateway_JSONRPCRouting(t *testing.T) {
	// Start a backend that echoes the received body.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"echoed":%s}`, body)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	gw := NewGateway()
	gw.RegisterBackend("session", backendURL)

	payload := `{"jsonrpc":"2.0","method":"ralphglasses_session_list","id":1}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "session_list") {
		t.Fatalf("expected echoed body containing method, got %q", rec.Body.String())
	}
}

func TestGateway_JSONRPCDotNotation(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)

	gw := NewGateway()
	gw.RegisterBackend("fleet", backendURL)

	payload := `{"jsonrpc":"2.0","method":"fleet.status","id":2}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGateway_UnknownNamespaceFallsThrough(t *testing.T) {
	gw := NewGateway()
	gw.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "fallthrough")
	}))

	payload := `{"jsonrpc":"2.0","method":"unknown_namespace_tool","id":3}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)

	if rec.Body.String() != "fallthrough" {
		t.Fatalf("expected fallthrough, got %q", rec.Body.String())
	}
}

// ---------- auth tests ----------

func TestAuth_APIKey(t *testing.T) {
	auth := NewAuth(nil)
	auth.AddKey("test-key-123")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	handler := auth.Wrap(inner)

	// Valid key.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "test-key-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid key: expected 200, got %d", rec.Code)
	}

	// Invalid key.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid key: expected 401, got %d", rec.Code)
	}

	// No credentials.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no creds: expected 401, got %d", rec.Code)
	}
}

func TestAuth_RemoveKey(t *testing.T) {
	auth := NewAuth(nil)
	auth.AddKey("temp-key")
	auth.RemoveKey("temp-key")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	handler := auth.Wrap(inner)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "temp-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("removed key should be rejected, got %d", rec.Code)
	}
}

func TestAuth_JWT(t *testing.T) {
	secret := []byte("test-secret-key-for-hmac")
	auth := NewAuth(secret)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	handler := auth.Wrap(inner)

	// Build a valid JWT.
	token := makeTestJWT(t, secret, jwtClaims{
		Sub: "user1",
		Exp: time.Now().Add(time.Hour).Unix(),
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid JWT: expected 200, got %d", rec.Code)
	}

	// Expired JWT.
	expired := makeTestJWT(t, secret, jwtClaims{
		Sub: "user1",
		Exp: time.Now().Add(-time.Hour).Unix(),
	})
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+expired)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expired JWT: expected 401, got %d", rec.Code)
	}

	// Tampered JWT.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token+"x")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("tampered JWT: expected 401, got %d", rec.Code)
	}
}

// ---------- rate limiter tests ----------

func TestRateLimiter_Basic(t *testing.T) {
	rl := NewRateLimiter(10, 2) // 10/s, burst 2

	// First two should pass (burst).
	if !rl.Allow("k1") {
		t.Fatal("first request should be allowed")
	}
	if !rl.Allow("k1") {
		t.Fatal("second request should be allowed (burst)")
	}

	// Third should be denied (burst exhausted).
	if rl.Allow("k1") {
		t.Fatal("third request should be denied")
	}

	// Different key should still be allowed.
	if !rl.Allow("k2") {
		t.Fatal("different key should be allowed")
	}
}

func TestRateLimiter_Refill(t *testing.T) {
	rl := NewRateLimiter(1000, 1) // 1000/s = 1 per ms

	if !rl.Allow("k1") {
		t.Fatal("first request should be allowed")
	}
	if rl.Allow("k1") {
		t.Fatal("immediate second request should be denied")
	}

	// Wait enough for a token to refill.
	time.Sleep(5 * time.Millisecond)

	if !rl.Allow("k1") {
		t.Fatal("request after refill should be allowed")
	}
}

func TestGateway_RateLimitMiddleware(t *testing.T) {
	rl := NewRateLimiter(10, 1) // burst 1
	gw := NewGateway(WithRateLimiter(rl))
	gw.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}))

	// First request passes.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "client-a")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request is rate limited.
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "client-a")
	rec = httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
}

// ---------- audit tests ----------

func TestAuditLogger_Logs(t *testing.T) {
	var buf bytes.Buffer
	al := NewAuditLogger(&buf)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler := al.Wrap(inner)

	req := httptest.NewRequest(http.MethodPost, "/rpc", strings.NewReader(`{"test":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "sk-1234567890abcdef")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, "method=POST") {
		t.Errorf("log missing method: %s", logLine)
	}
	if !strings.Contains(logLine, "path=/rpc") {
		t.Errorf("log missing path: %s", logLine)
	}
	if !strings.Contains(logLine, "status=201") {
		t.Errorf("log missing status: %s", logLine)
	}
	// Key should be masked.
	if strings.Contains(logLine, "1234567890abcdef") {
		t.Errorf("log contains unmasked key: %s", logLine)
	}
	if !strings.Contains(logLine, "sk-1...cdef") {
		t.Errorf("log missing masked key: %s", logLine)
	}
}

// ---------- integration: auth + rate limit + routing ----------

func TestGateway_FullMiddlewareChain(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"result":"proxied"}`)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	var auditBuf bytes.Buffer

	auth := NewAuth(nil)
	auth.AddKey("valid-key")

	gw := NewGateway(
		WithAuth(auth),
		WithRateLimiter(NewRateLimiter(100, 10)),
		WithAudit(NewAuditLogger(&auditBuf)),
	)
	gw.RegisterBackend("session", backendURL)

	// Unauthenticated -> 401.
	payload := `{"jsonrpc":"2.0","method":"ralphglasses_session_list","id":1}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: expected 401, got %d", rec.Code)
	}

	// Authenticated -> proxied.
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "valid-key")
	rec = httptest.NewRecorder()
	gw.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authed: expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "proxied") {
		t.Fatalf("expected proxied response, got %q", rec.Body.String())
	}

	// Audit log should have entries.
	if auditBuf.Len() == 0 {
		t.Fatal("audit log should not be empty")
	}
}

func TestMaskKey(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "***"},
		{"short", "***"},
		{"12345678", "***"},
		{"123456789", "1234...6789"},
		{"sk-1234567890abcdef", "sk-1...cdef"},
	}
	for _, tt := range tests {
		got := maskKey(tt.in)
		if got != tt.want {
			t.Errorf("maskKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short: got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("truncate long: got %q", got)
	}
}

// ---------- helpers ----------

func makeTestJWT(t *testing.T, secret []byte, claims jwtClaims) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(header + "." + payloadEnc))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return header + "." + payloadEnc + "." + sig
}
