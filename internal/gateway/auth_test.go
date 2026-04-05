package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewAuth(t *testing.T) {
	t.Run("with_jwt_key", func(t *testing.T) {
		a := NewAuth([]byte("secret"))
		if a == nil {
			t.Fatal("NewAuth returned nil")
		}
		if a.apiKeys == nil {
			t.Fatal("apiKeys map not initialized")
		}
		if len(a.jwtKey) == 0 {
			t.Fatal("jwtKey not set")
		}
	})

	t.Run("without_jwt_key", func(t *testing.T) {
		a := NewAuth(nil)
		if a == nil {
			t.Fatal("NewAuth returned nil")
		}
		if len(a.jwtKey) != 0 {
			t.Fatal("jwtKey should be nil")
		}
	})
}

func TestAuth_AddKey_RemoveKey(t *testing.T) {
	a := NewAuth(nil)

	a.AddKey("key-1")
	a.AddKey("key-2")

	a.mu.RLock()
	if !a.apiKeys["key-1"] || !a.apiKeys["key-2"] {
		t.Fatal("expected both keys to be present")
	}
	a.mu.RUnlock()

	a.RemoveKey("key-1")
	a.mu.RLock()
	if a.apiKeys["key-1"] {
		t.Fatal("key-1 should have been removed")
	}
	if !a.apiKeys["key-2"] {
		t.Fatal("key-2 should still be present")
	}
	a.mu.RUnlock()

	// Removing non-existent key should not panic.
	a.RemoveKey("nonexistent")
}

func TestAuth_Wrap_APIKeyAuth(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		wantStatus int
	}{
		{"valid_key", "good-key", http.StatusOK},
		{"invalid_key", "bad-key", http.StatusUnauthorized},
		{"empty_key", "", http.StatusUnauthorized},
	}

	a := NewAuth(nil)
	a.AddKey("good-key")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	handler := a.Wrap(inner)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.key != "" {
				req.Header.Set("X-API-Key", tt.key)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuth_Wrap_JWTAuth(t *testing.T) {
	secret := []byte("test-hmac-secret-256")
	a := NewAuth(secret)

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	handler := a.Wrap(inner)

	t.Run("valid_jwt", func(t *testing.T) {
		token := buildJWT(t, secret, jwtClaims{Sub: "user1", Exp: time.Now().Add(time.Hour).Unix()})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("valid JWT: got %d, want 200", rec.Code)
		}
	})

	t.Run("expired_jwt", func(t *testing.T) {
		token := buildJWT(t, secret, jwtClaims{Sub: "user1", Exp: time.Now().Add(-time.Hour).Unix()})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expired JWT: got %d, want 401", rec.Code)
		}
	})

	t.Run("bad_signature", func(t *testing.T) {
		token := buildJWT(t, []byte("wrong-secret"), jwtClaims{Sub: "user1", Exp: time.Now().Add(time.Hour).Unix()})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("bad sig JWT: got %d, want 401", rec.Code)
		}
	})

	t.Run("malformed_token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer not.a.valid.jwt.token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("malformed JWT: got %d, want 401", rec.Code)
		}
	})

	t.Run("missing_bearer_prefix", func(t *testing.T) {
		token := buildJWT(t, secret, jwtClaims{Sub: "user1", Exp: time.Now().Add(time.Hour).Unix()})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", token) // no "Bearer " prefix
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("no bearer prefix: got %d, want 401", rec.Code)
		}
	})
}

func TestAuth_Wrap_APIKeyTakesPrecedence(t *testing.T) {
	secret := []byte("test-secret")
	a := NewAuth(secret)
	a.AddKey("my-api-key")

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})
	handler := a.Wrap(inner)

	// Provide both a valid API key and an expired JWT; should succeed via API key.
	expiredToken := buildJWT(t, secret, jwtClaims{Sub: "u", Exp: time.Now().Add(-time.Hour).Unix()})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "my-api-key")
	req.Header.Set("Authorization", "Bearer "+expiredToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("API key should take precedence, got %d", rec.Code)
	}
}

func TestAuth_ValidateJWT_NoSigningKey(t *testing.T) {
	a := NewAuth(nil) // no JWT key
	_, err := a.validateJWT("any.token.here")
	if err != errNoSigningKey {
		t.Errorf("expected errNoSigningKey, got %v", err)
	}
}

func TestAuth_ValidateJWT_InvalidFormat(t *testing.T) {
	a := NewAuth([]byte("secret"))
	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"one_part", "onlyone"},
		{"two_parts", "one.two"},
		{"four_parts", "one.two.three.four"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := a.validateJWT(tt.token)
			if err != errInvalidToken {
				t.Errorf("expected errInvalidToken, got %v", err)
			}
		})
	}
}

func TestAuth_ValidateJWT_ValidToken(t *testing.T) {
	secret := []byte("my-secret")
	a := NewAuth(secret)

	token := buildJWT(t, secret, jwtClaims{Sub: "admin", Exp: time.Now().Add(time.Hour).Unix()})
	claims, err := a.validateJWT(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims.Sub != "admin" {
		t.Errorf("sub = %q, want %q", claims.Sub, "admin")
	}
}

func TestAuth_ValidateJWT_NoExpiry(t *testing.T) {
	secret := []byte("my-secret")
	a := NewAuth(secret)

	// Token with Exp=0 should be treated as no expiry and accepted.
	token := buildJWT(t, secret, jwtClaims{Sub: "user", Exp: 0})
	claims, err := a.validateJWT(token)
	if err != nil {
		t.Fatalf("token with no expiry should be valid: %v", err)
	}
	if claims.Sub != "user" {
		t.Errorf("sub = %q, want %q", claims.Sub, "user")
	}
}

func TestAuth_ConcurrentAccess(t *testing.T) {
	a := NewAuth(nil)

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", n)
			a.AddKey(key)
			// Exercise Wrap concurrently.
			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			handler := a.Wrap(inner)
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-API-Key", key)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			a.RemoveKey(key)
		}(i)
	}
	wg.Wait()
}

// buildJWT creates an HMAC-SHA256 JWT for testing.
func buildJWT(t *testing.T, secret []byte, claims jwtClaims) string {
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
