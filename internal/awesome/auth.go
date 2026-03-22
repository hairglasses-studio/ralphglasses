package awesome

import (
	"net/http"
	"os"
)

// addAuthHeader adds a GitHub token if available.
func addAuthHeader(req *http.Request) {
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
}
