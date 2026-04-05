package clients

import "fmt"

// HTTPError represents an error response from an external HTTP API.
type HTTPError struct {
	StatusCode int
	Body       string
	API        string
}

func (e *HTTPError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s API error %d: %s", e.API, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("%s API error %d", e.API, e.StatusCode)
}

func (e *HTTPError) IsRateLimit() bool      { return e.StatusCode == 429 }
func (e *HTTPError) IsAuth() bool           { return e.StatusCode == 401 || e.StatusCode == 403 }
func (e *HTTPError) IsServerError() bool    { return e.StatusCode >= 500 }
func (e *HTTPError) IsRetryable() bool      { return e.IsRateLimit() || e.IsServerError() }
func (e *HTTPError) IsSessionExpired() bool { return e.StatusCode == 302 || e.StatusCode == 401 || e.StatusCode == 403 }
