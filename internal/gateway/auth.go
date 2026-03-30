package gateway

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Auth provides API key and JWT authentication middleware.
type Auth struct {
	mu      sync.RWMutex
	apiKeys map[string]bool // set of valid API keys
	jwtKey  []byte          // HMAC-SHA256 signing key for JWT
}

// NewAuth creates an authenticator. If jwtKey is nil, JWT validation is
// disabled and only API key auth is used.
func NewAuth(jwtKey []byte) *Auth {
	return &Auth{
		apiKeys: make(map[string]bool),
		jwtKey:  jwtKey,
	}
}

// AddKey registers a valid API key.
func (a *Auth) AddKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.apiKeys[key] = true
}

// RemoveKey deregisters an API key.
func (a *Auth) RemoveKey(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.apiKeys, key)
}

// Wrap returns middleware that checks for a valid API key (X-API-Key header)
// or a valid JWT (Authorization: Bearer <token>). Requests without valid
// credentials receive 401 Unauthorized.
func (a *Auth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try API key first.
		if key := r.Header.Get("X-API-Key"); key != "" {
			a.mu.RLock()
			ok := a.apiKeys[key]
			a.mu.RUnlock()
			if ok {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Try JWT bearer token.
		if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			token := strings.TrimPrefix(auth, "Bearer ")
			if _, err := a.validateJWT(token); err == nil {
				next.ServeHTTP(w, r)
				return
			}
		}

		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
	})
}

// jwtClaims is the minimal claims set we verify.
type jwtClaims struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
}

var (
	errInvalidToken  = errors.New("invalid token format")
	errBadSignature  = errors.New("bad signature")
	errTokenExpired  = errors.New("token expired")
	errNoSigningKey  = errors.New("no jwt signing key configured")
)

// validateJWT performs HMAC-SHA256 validation and expiry check on a compact
// JWT (header.payload.signature). This is intentionally minimal — production
// deployments should use a dedicated JWT library.
func (a *Auth) validateJWT(token string) (*jwtClaims, error) {
	if len(a.jwtKey) == 0 {
		return nil, errNoSigningKey
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errInvalidToken
	}

	// Verify signature.
	mac := hmac.New(sha256.New, a.jwtKey)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(expectedSig)) {
		return nil, errBadSignature
	}

	// Decode payload.
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errInvalidToken
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errInvalidToken
	}

	// Check expiry.
	if claims.Exp > 0 && time.Now().Unix() > claims.Exp {
		return nil, errTokenExpired
	}

	return &claims, nil
}
