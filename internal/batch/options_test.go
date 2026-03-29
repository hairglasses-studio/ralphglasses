package batch

import (
	"net/http"
	"testing"
	"time"
)

func TestWithHTTPClient(t *testing.T) {
	t.Parallel()

	custom := &http.Client{Timeout: 99 * time.Second}
	opt := WithHTTPClient(custom)

	cfg := &clientConfig{}
	opt(cfg)

	if cfg.HTTPClient != custom {
		t.Error("WithHTTPClient did not set the HTTPClient on clientConfig")
	}

	// Verify httpClient() returns the custom client.
	if got := cfg.httpClient(); got != custom {
		t.Error("httpClient() should return the custom client after WithHTTPClient")
	}
}

func TestWithHTTPClient_Default(t *testing.T) {
	t.Parallel()

	cfg := &clientConfig{}
	hc := cfg.httpClient()
	if hc == nil {
		t.Fatal("httpClient() should return a default client when HTTPClient is nil")
	}
	if hc.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", hc.Timeout)
	}
}
