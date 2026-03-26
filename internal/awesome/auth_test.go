package awesome

import (
	"net/http"
	"testing"
)

func TestAddAuthHeader_NoToken(t *testing.T) {
	// Cannot use t.Parallel with t.Setenv
	t.Setenv("GITHUB_TOKEN", "")

	req, _ := http.NewRequest("GET", "https://api.github.com/repos/test/repo", nil)
	addAuthHeader(req)

	if auth := req.Header.Get("Authorization"); auth != "" {
		t.Errorf("expected no Authorization header, got %q", auth)
	}
	if accept := req.Header.Get("Accept"); accept != "application/vnd.github.v3+json" {
		t.Errorf("Accept = %q, want application/vnd.github.v3+json", accept)
	}
}

func TestAddAuthHeader_WithToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "ghp_test123")

	req, _ := http.NewRequest("GET", "https://api.github.com/repos/test/repo", nil)
	addAuthHeader(req)

	if auth := req.Header.Get("Authorization"); auth != "Bearer ghp_test123" {
		t.Errorf("Authorization = %q, want Bearer ghp_test123", auth)
	}
	if accept := req.Header.Get("Accept"); accept != "application/vnd.github.v3+json" {
		t.Errorf("Accept = %q", accept)
	}
}
