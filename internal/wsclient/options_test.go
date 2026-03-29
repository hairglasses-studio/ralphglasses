package wsclient

import (
	"net/http"
	"testing"
	"time"
)

func TestWithHTTPClient_SetsClient(t *testing.T) {
	t.Parallel()

	custom := &http.Client{Timeout: 99 * time.Second}
	c := NewClient("sk-test", WithHTTPClient(custom))

	if c.httpClient != custom {
		t.Error("WithHTTPClient should set the httpClient field")
	}
	if c.httpClient.Timeout != 99*time.Second {
		t.Errorf("timeout = %v, want 99s", c.httpClient.Timeout)
	}
}

func TestWithHTTPClient_OverridesDefault(t *testing.T) {
	t.Parallel()

	// Default client has 30s timeout.
	c := NewClient("sk-test")
	if c.httpClient.Timeout != 30*time.Second {
		t.Fatalf("default timeout = %v, want 30s", c.httpClient.Timeout)
	}

	// Override with custom client.
	custom := &http.Client{Timeout: 5 * time.Second}
	c2 := NewClient("sk-test", WithHTTPClient(custom))
	if c2.httpClient.Timeout != 5*time.Second {
		t.Errorf("overridden timeout = %v, want 5s", c2.httpClient.Timeout)
	}
}
