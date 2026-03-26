package fleet

import (
	"net"
	"net/http"
	"time"
)

// DefaultTransport returns an *http.Transport configured with connection
// pooling appropriate for fleet client communication. It keeps idle
// connections alive to avoid the overhead of repeated TCP+TLS handshakes
// when talking to the coordinator or workers.
func DefaultTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,

		// Reasonable dial/TLS timeouts so unhealthy nodes fail fast.
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
}
